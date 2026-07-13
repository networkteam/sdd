package sdd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/gofrs/flock"
	"github.com/networkteam/sdd/internal/model"
)

type FilesystemGraphStoreOptions struct {
	Project  ProjectID
	GraphDir string
}

// FilesystemGraphStore is the local canonical graph authority. It owns its
// revision cache and never requires callers to invalidate snapshots.
type FilesystemGraphStore struct {
	project ProjectID
	dir     string
	mu      sync.Mutex
}

type filesystemApplyRecord struct {
	Digest           string      `json:"digest"`
	ExpectedRevision string      `json:"expected_revision"`
	Result           ApplyResult `json:"result"`
}

func NewFilesystemGraphStore(options FilesystemGraphStoreOptions) (*FilesystemGraphStore, error) {
	if options.Project == "" {
		return nil, fmt.Errorf("sdd: filesystem graph project is required")
	}
	if options.GraphDir == "" {
		return nil, fmt.Errorf("sdd: filesystem graph directory is required")
	}
	if err := os.MkdirAll(options.GraphDir, 0o755); err != nil {
		return nil, fmt.Errorf("sdd: creating filesystem graph directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(options.GraphDir, ".sdd-runtime", "applied"), 0o755); err != nil {
		return nil, fmt.Errorf("sdd: creating graph runtime directory: %w", err)
	}
	return &FilesystemGraphStore{
		project: options.Project,
		dir:     options.GraphDir,
	}, nil
}

func (s *FilesystemGraphStore) Current(ctx context.Context) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlockFile(lock)
	return s.currentLocked(ctx)
}

func (s *FilesystemGraphStore) currentLocked(ctx context.Context) (*Snapshot, error) {
	revision, err := graphDirectoryRevision(s.dir)
	if err != nil {
		return nil, err
	}
	return LoadSnapshotFS(ctx, s.project, revision, os.DirFS(s.dir), ".")
}

func (s *FilesystemGraphStore) Apply(ctx context.Context, expectedRevision string, batch MutationBatch, blobs StagedBlobReader) (ApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lock()
	if err != nil {
		return ApplyResult{State: MutationNotApplied}, err
	}
	defer unlockFile(lock)
	if prior, ok, err := s.loadApplyRecord(batch.ID); err != nil {
		return ApplyResult{State: MutationUnknown}, err
	} else if ok {
		if prior.Digest != batch.Digest {
			return ApplyResult{State: MutationNotApplied}, &ApplicationError{Code: ErrorRecoveryRequired, Message: "mutation ID reused with a different digest"}
		}
		if prior.Result.State == MutationUnknown {
			return s.reconcileRecord(prior)
		}
		return prior.Result, nil
	}
	currentRevision, err := graphDirectoryRevision(s.dir)
	if err != nil {
		return ApplyResult{State: MutationNotApplied}, err
	}
	if currentRevision != expectedRevision {
		return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, &ApplicationError{Code: ErrorGraphConflict, Message: "graph revision changed", Revision: currentRevision}
	}
	wantDigest, err := MutationBatchDigest(batch)
	if err != nil {
		return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, err
	}
	if batch.ID == "" || batch.Digest == "" || batch.Digest != wantDigest {
		return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, &ApplicationError{Code: ErrorRecoveryRequired, Message: "mutation batch digest mismatch"}
	}
	record := filesystemApplyRecord{Digest: batch.Digest, ExpectedRevision: expectedRevision, Result: ApplyResult{State: MutationUnknown, Revision: currentRevision}}
	if err := s.persistApplyRecord(batch.ID, record); err != nil {
		return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, err
	}

	type preparedFile struct {
		path string
		data []byte
	}
	var writes []preparedFile
	var deletes []string
	for _, change := range batch.Changes {
		target, err := safeGraphPath(s.dir, change.LogicalPath)
		if err != nil {
			return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, err
		}
		if change.Delete {
			deletes = append(deletes, target)
			continue
		}
		if len(change.CanonicalBytes) == 0 {
			return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, fmt.Errorf("sdd: empty canonical bytes for %s", change.LogicalPath)
		}
		writes = append(writes, preparedFile{path: target, data: append([]byte(nil), change.CanonicalBytes...)})
	}
	for _, attachment := range batch.Attachments {
		if blobs == nil {
			return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, fmt.Errorf("sdd: staged blob reader is required")
		}
		target, err := safeGraphPath(s.dir, attachment.LogicalPath)
		if err != nil {
			return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, err
		}
		reader, err := blobs.Open(ctx, attachment.BlobID)
		if err != nil {
			return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, err
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, readErr
		}
		if closeErr != nil {
			return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, closeErr
		}
		if int64(len(data)) != attachment.Size || !digestMatches(data, attachment.Digest) {
			return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, fmt.Errorf("sdd: staged blob %s does not match prepared facts", attachment.BlobID)
		}
		writes = append(writes, preparedFile{path: target, data: data})
	}

	temps := make([]preparedFile, 0, len(writes))
	for _, write := range writes {
		if err := os.MkdirAll(filepath.Dir(write.path), 0o755); err != nil {
			return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, err
		}
		temp, err := os.CreateTemp(filepath.Dir(write.path), ".sdd-apply-*")
		if err != nil {
			return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, err
		}
		name := temp.Name()
		if _, err := temp.Write(write.data); err != nil {
			_ = temp.Close()
			_ = os.Remove(name)
			return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, err
		}
		if err := temp.Close(); err != nil {
			_ = os.Remove(name)
			return ApplyResult{State: MutationNotApplied, Revision: currentRevision}, err
		}
		temps = append(temps, preparedFile{path: write.path, data: []byte(name)})
	}
	for _, temp := range temps {
		if err := os.Rename(string(temp.data), temp.path); err != nil {
			return ApplyResult{State: MutationUnknown, Revision: currentRevision}, err
		}
	}
	for _, target := range deletes {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return ApplyResult{State: MutationUnknown, Revision: currentRevision}, err
		}
	}
	newRevision, err := graphDirectoryRevision(s.dir)
	if err != nil {
		return ApplyResult{State: MutationUnknown}, err
	}
	result := ApplyResult{State: MutationApplied, Revision: newRevision}
	record.Result = result
	if err := s.persistApplyRecord(batch.ID, record); err != nil {
		return ApplyResult{State: MutationUnknown, Revision: newRevision}, err
	}
	return result, nil
}

func (s *FilesystemGraphStore) Reconcile(_ context.Context, mutationID, batchDigest string) (ApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lock()
	if err != nil {
		return ApplyResult{State: MutationUnknown}, err
	}
	defer unlockFile(lock)
	record, ok, err := s.loadApplyRecord(mutationID)
	if err != nil {
		return ApplyResult{State: MutationUnknown}, err
	}
	if !ok {
		return ApplyResult{State: MutationUnknown}, nil
	}
	if record.Digest != batchDigest {
		return ApplyResult{State: MutationUnknown}, &ApplicationError{Code: ErrorRecoveryRequired, Message: "mutation digest does not match recorded apply"}
	}
	return s.reconcileRecord(record)
}

func (s *FilesystemGraphStore) ReadAttachmentPage(_ context.Context, entryID, filename string, offset int64, maxBytes int) (AttachmentPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lock()
	if err != nil {
		return AttachmentPage{}, err
	}
	defer unlockFile(lock)
	if offset < 0 || maxBytes <= 0 {
		return AttachmentPage{}, fmt.Errorf("sdd: invalid attachment page range")
	}
	attachmentDir, err := model.AttachDirRelPath(entryID)
	if err != nil {
		return AttachmentPage{}, err
	}
	if filename == "" {
		entries, readErr := os.ReadDir(filepath.Join(s.dir, attachmentDir))
		if readErr != nil {
			return AttachmentPage{}, readErr
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				if filename != "" {
					return AttachmentPage{}, fmt.Errorf("sdd: attachment name is required when entry %s has more than one attachment", entryID)
				}
				filename = entry.Name()
			}
		}
		if filename == "" {
			return AttachmentPage{}, fmt.Errorf("sdd: entry %s has no attachments", entryID)
		}
	}
	if filepath.Base(filename) != filename || strings.ContainsAny(filename, `/\\`) {
		return AttachmentPage{}, fmt.Errorf("sdd: invalid attachment filename %q", filename)
	}
	logicalPath := filepath.ToSlash(filepath.Join(attachmentDir, filename))
	target, err := safeGraphPath(s.dir, logicalPath)
	if err != nil {
		return AttachmentPage{}, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return AttachmentPage{}, err
	}
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	end := offset + int64(maxBytes)
	if end > int64(len(data)) {
		end = int64(len(data))
	}
	return AttachmentPage{
		Filename: filename, Content: append([]byte(nil), data[offset:end]...), Offset: offset,
		NextOffset: end, TotalSize: int64(len(data)), More: end < int64(len(data)), Digest: sha256Digest(data),
	}, nil
}

func (s *FilesystemGraphStore) lock() (*flock.Flock, error) {
	lock := flock.New(filepath.Join(s.dir, ".sdd-runtime", "graph.lock"))
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	return lock, nil
}

func (s *FilesystemGraphStore) applyRecordPath(id string) (string, error) {
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return "", fmt.Errorf("sdd: invalid mutation ID %q", id)
	}
	return filepath.Join(s.dir, ".sdd-runtime", "applied", id+".json"), nil
}

func (s *FilesystemGraphStore) loadApplyRecord(id string) (filesystemApplyRecord, bool, error) {
	filename, err := s.applyRecordPath(id)
	if err != nil {
		return filesystemApplyRecord{}, false, err
	}
	raw, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return filesystemApplyRecord{}, false, nil
	}
	if err != nil {
		return filesystemApplyRecord{}, false, err
	}
	var record filesystemApplyRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return filesystemApplyRecord{}, false, err
	}
	return record, true, nil
}

func (s *FilesystemGraphStore) persistApplyRecord(id string, record filesystemApplyRecord) error {
	filename, err := s.applyRecordPath(id)
	if err != nil {
		return err
	}
	return writeJSONAtomic(filename, record)
}

func (s *FilesystemGraphStore) reconcileRecord(record filesystemApplyRecord) (ApplyResult, error) {
	if record.Result.State != MutationUnknown {
		return record.Result, nil
	}
	revision, err := graphDirectoryRevision(s.dir)
	if err != nil {
		return ApplyResult{State: MutationUnknown}, err
	}
	if revision == record.ExpectedRevision {
		return ApplyResult{State: MutationNotApplied, Revision: revision}, nil
	}
	return ApplyResult{State: MutationUnknown, Revision: revision}, nil
}

func graphDirectoryRevision(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() && entry.Name() == ".sdd-runtime" {
			return filepath.SkipDir
		}
		if !entry.IsDir() {
			files = append(files, filename)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, filename := range files {
		rel, err := filepath.Rel(dir, filename)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(hash, filepath.ToSlash(rel))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func safeGraphPath(root, logicalPath string) (string, error) {
	if logicalPath == "" || strings.Contains(logicalPath, "\\") || !fs.ValidPath(logicalPath) {
		return "", fmt.Errorf("sdd: invalid graph logical path %q", logicalPath)
	}
	target := filepath.Join(root, filepath.FromSlash(logicalPath))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("sdd: graph path escapes root: %q", logicalPath)
	}
	return target, nil
}

func sha256Digest(data []byte) BlobDigest {
	sum := sha256.Sum256(data)
	return BlobDigest{Algorithm: "sha256", Value: hex.EncodeToString(sum[:])}
}

func digestMatches(data []byte, digest BlobDigest) bool {
	return digest.Algorithm == "sha256" && sha256Digest(data).Value == digest.Value
}
