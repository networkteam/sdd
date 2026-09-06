package local

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	app "github.com/networkteam/sdd/pkg/application"
)

type retainedSnapshot struct {
	snapshot *app.Snapshot
	files    *zip.Reader
	leases   int
}

type snapshotAttachments struct{ files fs.FS }

func (s snapshotAttachments) ReadAttachmentPage(ctx context.Context, entry, name string, offset int64, limit int) (app.AttachmentPage, error) {
	if err := ctx.Err(); err != nil {
		return app.AttachmentPage{}, err
	}
	return app.PageAttachment(s.files, ".", entry, name, offset, limit)
}

// AcquireSnapshot retains immutable graph and attachment bytes while a lease
// exists. Exact revisions survive concurrent Apply calls, not process restarts;
// durable consumers supply their own revision-backed SnapshotReader. Memory
// scales with all graph and attachment bytes in live revisions. A nonempty
// requested branch must match FilesystemGraphStoreOptions.Branch.
func (s *FilesystemGraphStore) AcquireSnapshot(ctx context.Context, q app.SnapshotReadQuery) (*app.AcquiredSnapshot, error) {
	if (q.Branch != "" && q.Branch != s.branch) || (q.ExactRevision != "" && q.IncludesRevision != "") {
		return nil, fmt.Errorf("sdd: invalid filesystem snapshot selection")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if q.ExactRevision != "" {
		if retained := s.snapshots[q.ExactRevision]; retained != nil {
			return s.leaseSnapshot(retained), nil
		}
	}
	lock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock(lock)
	if err := s.recoverPendingTransactionsLocked(); err != nil {
		return nil, err
	}
	revision, err := graphDirectoryRevision(s.dir)
	if err != nil {
		return nil, err
	}
	if q.ExactRevision != "" && q.ExactRevision != revision {
		return nil, fmt.Errorf("sdd: exact source revision is no longer retained")
	}
	if q.IncludesRevision != "" {
		ok, err := s.includesRevision(revision, q.IncludesRevision)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("sdd: current revision cannot be shown to include the requested write")
		}
	}
	if retained := s.snapshots[revision]; retained != nil {
		return s.leaseSnapshot(retained), nil
	}
	files, err := freezeGraphFS(ctx, s.dir)
	if err != nil {
		return nil, err
	}
	after, err := graphDirectoryRevision(s.dir)
	if err != nil {
		return nil, err
	}
	if after != revision {
		return nil, fmt.Errorf("sdd: graph changed while acquiring snapshot")
	}
	snapshot, err := app.LoadSnapshotFS(ctx, s.project, revision, files, ".")
	if err != nil {
		return nil, err
	}
	retained := &retainedSnapshot{snapshot: snapshot, files: files}
	if s.snapshots == nil {
		s.snapshots = map[string]*retainedSnapshot{}
	}
	s.snapshots[revision] = retained
	return s.leaseSnapshot(retained), nil
}

func (s *FilesystemGraphStore) leaseSnapshot(retained *retainedSnapshot) *app.AcquiredSnapshot {
	retained.leases++
	var once sync.Once
	return &app.AcquiredSnapshot{Snapshot: retained.snapshot, Attachments: snapshotAttachments{files: retained.files}, Release: func() error {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			retained.leases--
			if retained.leases == 0 {
				delete(s.snapshots, retained.snapshot.Revision())
			}
		})
		return nil
	}}
}

func (s *FilesystemGraphStore) includesRevision(current, required string) (bool, error) {
	if current == required {
		return true, nil
	}
	names, err := os.ReadDir(filepath.Join(s.dir, ".sdd-runtime", "applied"))
	if err != nil {
		return false, err
	}
	parents := map[string][]string{}
	for _, name := range names {
		if name.IsDir() || filepath.Ext(name.Name()) != ".json" {
			continue
		}
		id := name.Name()[:len(name.Name())-5]
		record, found, err := s.loadApplyRecord(id)
		if err != nil {
			return false, err
		}
		if found && record.Result.State == app.MutationApplied {
			parents[record.Result.Revision] = append(parents[record.Result.Revision], record.ExpectedRevision)
		}
	}
	todo := []string{current}
	seen := map[string]bool{}
	for len(todo) > 0 {
		node := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if node == required {
			return true, nil
		}
		if seen[node] {
			continue
		}
		seen[node] = true
		todo = append(todo, parents[node]...)
	}
	return false, nil
}

func freezeGraphFS(ctx context.Context, dir string) (*zip.Reader, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	err := fs.WalkDir(os.DirFS(dir), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".sdd-runtime" {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("sdd: snapshot contains a symbolic link: %s", path)
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			return err
		}
		file, err := archive.CreateHeader(&zip.FileHeader{Name: path, Method: zip.Store})
		if err != nil {
			return err
		}
		_, err = file.Write(data)
		return err
	})
	if err := errors.Join(err, archive.Close()); err != nil {
		return nil, err
	}
	return zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
}
