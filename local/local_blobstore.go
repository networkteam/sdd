package local

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	app "github.com/networkteam/sdd/application"
)

type FilesystemStagedBlobStore struct {
	dir                  string
	trustedStateRoot     string
	mu                   sync.Mutex
	beforeRootedMutation func()
}

// NewFilesystemStagedBlobStoreAtStateRoot is the production constructor for a
// machine-global staged-blob store. Construction itself performs no I/O.
func NewFilesystemStagedBlobStoreAtStateRoot(stateRoot, dir string) (*FilesystemStagedBlobStore, error) {
	if stateRoot == "" || dir == "" {
		return nil, fmt.Errorf("sdd: trusted state root and staged blob directory are required")
	}
	if !pathAtOrInside(filepath.Join(stateRoot, "staged-blobs"), dir) {
		return nil, fmt.Errorf("sdd: staged blob directory escapes trusted state root")
	}
	return &FilesystemStagedBlobStore{dir: dir, trustedStateRoot: stateRoot}, nil
}

func NewFilesystemStagedBlobStore(dir string) (*FilesystemStagedBlobStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("sdd: staged blob directory is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FilesystemStagedBlobStore{dir: dir}, nil
}

func (s *FilesystemStagedBlobStore) Stage(_ context.Context, owner app.BlobOwner, filename string, content io.Reader) (app.StagedBlob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trustedStateRoot != "" {
		return s.stageRooted(owner, filename, content)
	}
	ownerDir, err := s.ownerDir(owner)
	if err != nil {
		return app.StagedBlob{}, err
	}
	if filename == "" || filepath.Base(filename) != filename || strings.ContainsAny(filename, `/\\`) {
		return app.StagedBlob{}, fmt.Errorf("sdd: staged filename must be a plain name")
	}
	data, err := io.ReadAll(content)
	if err != nil {
		return app.StagedBlob{}, err
	}
	if err := os.MkdirAll(ownerDir, 0o755); err != nil {
		return app.StagedBlob{}, err
	}
	id, err := randomBlobID()
	if err != nil {
		return app.StagedBlob{}, err
	}
	blob := app.StagedBlob{
		ID: id, Owner: owner, Digest: sha256Digest(data), Size: int64(len(data)), Filename: filename,
		CreatedAt: time.Now().UTC().Round(0),
	}
	if err := os.WriteFile(filepath.Join(ownerDir, id+".blob"), data, 0o600); err != nil {
		return app.StagedBlob{}, err
	}
	if err := writeJSONAtomic(filepath.Join(ownerDir, id+".json"), blob); err != nil {
		_ = os.Remove(filepath.Join(ownerDir, id+".blob"))
		return app.StagedBlob{}, err
	}
	return blob, nil
}

func (s *FilesystemStagedBlobStore) Stat(_ context.Context, owner app.BlobOwner, id string) (app.StagedBlob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trustedStateRoot != "" {
		return s.statRooted(owner, id)
	}
	return s.statLocked(owner, id)
}

func (s *FilesystemStagedBlobStore) statLocked(owner app.BlobOwner, id string) (app.StagedBlob, error) {
	ownerDir, err := s.ownerDir(owner)
	if err != nil {
		return app.StagedBlob{}, err
	}
	if err := validBlobID(id); err != nil {
		return app.StagedBlob{}, err
	}
	raw, err := os.ReadFile(filepath.Join(ownerDir, id+".json"))
	if err != nil {
		return app.StagedBlob{}, err
	}
	var blob app.StagedBlob
	if err := json.Unmarshal(raw, &blob); err != nil {
		return app.StagedBlob{}, err
	}
	if blob.Owner != owner || blob.ID != id {
		return app.StagedBlob{}, fmt.Errorf("sdd: staged blob ownership mismatch")
	}
	return blob, nil
}

func (s *FilesystemStagedBlobStore) Open(_ context.Context, owner app.BlobOwner, id string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trustedStateRoot != "" {
		return s.openBlobRooted(owner, id)
	}
	if _, err := s.statLocked(owner, id); err != nil {
		return nil, err
	}
	ownerDir, _ := s.ownerDir(owner)
	return os.Open(filepath.Join(ownerDir, id+".blob"))
}

func (s *FilesystemStagedBlobStore) Retain(_ context.Context, owner app.BlobOwner, retentionID string, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trustedStateRoot != "" {
		return s.retainRooted(owner, retentionID, ids)
	}
	if retentionID == "" {
		return fmt.Errorf("sdd: retention ID is required")
	}
	for _, id := range ids {
		if _, err := s.statLocked(owner, id); err != nil {
			return err
		}
	}
	retentions, filename, err := s.retentionsLocked(owner)
	if err != nil {
		return err
	}
	retentions[retentionID] = append([]string(nil), ids...)
	return writeJSONAtomic(filename, retentions)
}

func (s *FilesystemStagedBlobStore) Release(_ context.Context, owner app.BlobOwner, retentionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trustedStateRoot != "" {
		return s.releaseRooted(owner, retentionID)
	}
	retentions, filename, err := s.retentionsLocked(owner)
	if err != nil {
		return err
	}
	delete(retentions, retentionID)
	return writeJSONAtomic(filename, retentions)
}

func (s *FilesystemStagedBlobStore) openRooted(create bool) (*trustedStoreRoot, error) {
	return openTrustedStoreRoot(s.trustedStateRoot, "staged-blobs", s.dir, create)
}

func (s *FilesystemStagedBlobStore) ownerRelative(owner app.BlobOwner) (string, error) {
	ownerDir, err := s.ownerDir(owner)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(s.dir, ownerDir)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("sdd: blob owner escapes rooted store")
	}
	return relative, nil
}

func (s *FilesystemStagedBlobStore) stageRooted(
	owner app.BlobOwner,
	filename string,
	content io.Reader,
) (app.StagedBlob, error) {
	ownerRelative, err := s.ownerRelative(owner)
	if err != nil {
		return app.StagedBlob{}, err
	}
	if filename == "" || filepath.Base(filename) != filename || strings.ContainsAny(filename, `/\`) {
		return app.StagedBlob{}, fmt.Errorf("sdd: staged filename must be a plain name")
	}
	data, err := io.ReadAll(content)
	if err != nil {
		return app.StagedBlob{}, err
	}
	roots, err := s.openRooted(true)
	if err != nil {
		return app.StagedBlob{}, err
	}
	defer func() { _ = roots.close() }()
	if s.beforeRootedMutation != nil {
		s.beforeRootedMutation()
		s.beforeRootedMutation = nil
		if err := roots.revalidate(); err != nil {
			return app.StagedBlob{}, err
		}
	}
	if err := roots.store.MkdirAll(ownerRelative, 0o755); err != nil {
		return app.StagedBlob{}, err
	}
	id, err := randomBlobID()
	if err != nil {
		return app.StagedBlob{}, err
	}
	blob := app.StagedBlob{
		ID: id, Owner: owner, Digest: sha256Digest(data), Size: int64(len(data)), Filename: filename,
		CreatedAt: time.Now().UTC().Round(0),
	}
	blobName := filepath.Join(ownerRelative, id+".blob")
	file, err := roots.store.OpenFile(blobName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return app.StagedBlob{}, err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = roots.store.Remove(blobName)
		return app.StagedBlob{}, err
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		_ = roots.store.Remove(blobName)
		return app.StagedBlob{}, err
	}
	if err := writeJSONAtomicRoot(roots.store, filepath.Join(ownerRelative, id+".json"), blob); err != nil {
		_ = roots.store.Remove(blobName)
		return app.StagedBlob{}, err
	}
	if err := roots.revalidate(); err != nil {
		return app.StagedBlob{}, err
	}
	return blob, nil
}

func rootedBlobMetadata(
	root *os.Root,
	ownerRelative string,
	owner app.BlobOwner,
	id string,
) (app.StagedBlob, error) {
	if err := validBlobID(id); err != nil {
		return app.StagedBlob{}, err
	}
	file, err := openRootedRegular(root, filepath.Join(ownerRelative, id+".json"))
	if err != nil {
		return app.StagedBlob{}, err
	}
	raw, readErr := io.ReadAll(file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return app.StagedBlob{}, err
	}
	var blob app.StagedBlob
	if err := json.Unmarshal(raw, &blob); err != nil {
		return app.StagedBlob{}, err
	}
	if blob.Owner != owner || blob.ID != id {
		return app.StagedBlob{}, fmt.Errorf("sdd: staged blob ownership mismatch")
	}
	return blob, nil
}

func (s *FilesystemStagedBlobStore) statRooted(owner app.BlobOwner, id string) (app.StagedBlob, error) {
	ownerRelative, err := s.ownerRelative(owner)
	if err != nil {
		return app.StagedBlob{}, err
	}
	roots, err := s.openRooted(false)
	if err != nil {
		return app.StagedBlob{}, err
	}
	defer func() { _ = roots.close() }()
	return rootedBlobMetadata(roots.store, ownerRelative, owner, id)
}

func (s *FilesystemStagedBlobStore) openBlobRooted(owner app.BlobOwner, id string) (io.ReadCloser, error) {
	ownerRelative, err := s.ownerRelative(owner)
	if err != nil {
		return nil, err
	}
	roots, err := s.openRooted(false)
	if err != nil {
		return nil, err
	}
	if _, err := rootedBlobMetadata(roots.store, ownerRelative, owner, id); err != nil {
		_ = roots.close()
		return nil, err
	}
	file, err := openRootedRegular(roots.store, filepath.Join(ownerRelative, id+".blob"))
	if err != nil {
		_ = roots.close()
		return nil, err
	}
	if err := roots.revalidate(); err != nil {
		_ = file.Close()
		_ = roots.close()
		return nil, err
	}
	if err := roots.close(); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func (s *FilesystemStagedBlobStore) rootedRetentions(
	root *os.Root,
	owner app.BlobOwner,
) (map[string][]string, string, error) {
	ownerRelative, err := s.ownerRelative(owner)
	if err != nil {
		return nil, "", err
	}
	if err := root.MkdirAll(ownerRelative, 0o755); err != nil {
		return nil, "", err
	}
	name := filepath.Join(ownerRelative, "retentions.json")
	retentions := map[string][]string{}
	file, err := openRootedRegular(root, name)
	if err == nil {
		raw, readErr := io.ReadAll(file)
		if err := errors.Join(readErr, file.Close()); err != nil {
			return nil, "", err
		}
		if err := json.Unmarshal(raw, &retentions); err != nil {
			return nil, "", err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, "", err
	}
	return retentions, name, nil
}

func (s *FilesystemStagedBlobStore) retainRooted(owner app.BlobOwner, retentionID string, ids []string) error {
	if retentionID == "" {
		return fmt.Errorf("sdd: retention ID is required")
	}
	roots, err := s.openRooted(true)
	if err != nil {
		return err
	}
	defer func() { _ = roots.close() }()
	if s.beforeRootedMutation != nil {
		s.beforeRootedMutation()
		s.beforeRootedMutation = nil
		if err := roots.revalidate(); err != nil {
			return err
		}
	}
	ownerRelative, err := s.ownerRelative(owner)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := rootedBlobMetadata(roots.store, ownerRelative, owner, id); err != nil {
			return err
		}
	}
	retentions, name, err := s.rootedRetentions(roots.store, owner)
	if err != nil {
		return err
	}
	retentions[retentionID] = append([]string(nil), ids...)
	if err := writeJSONAtomicRoot(roots.store, name, retentions); err != nil {
		return err
	}
	return roots.revalidate()
}

func (s *FilesystemStagedBlobStore) releaseRooted(owner app.BlobOwner, retentionID string) error {
	roots, err := s.openRooted(true)
	if err != nil {
		return err
	}
	defer func() { _ = roots.close() }()
	if s.beforeRootedMutation != nil {
		s.beforeRootedMutation()
		s.beforeRootedMutation = nil
		if err := roots.revalidate(); err != nil {
			return err
		}
	}
	retentions, name, err := s.rootedRetentions(roots.store, owner)
	if err != nil {
		return err
	}
	delete(retentions, retentionID)
	if err := writeJSONAtomicRoot(roots.store, name, retentions); err != nil {
		return err
	}
	return roots.revalidate()
}

func (s *FilesystemStagedBlobStore) retentionsLocked(owner app.BlobOwner) (map[string][]string, string, error) {
	ownerDir, err := s.ownerDir(owner)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(ownerDir, 0o755); err != nil {
		return nil, "", err
	}
	filename := filepath.Join(ownerDir, "retentions.json")
	retentions := map[string][]string{}
	raw, err := os.ReadFile(filename)
	if err == nil {
		if err := json.Unmarshal(raw, &retentions); err != nil {
			return nil, "", err
		}
	} else if !os.IsNotExist(err) {
		return nil, "", err
	}
	return retentions, filename, nil
}

func (s *FilesystemStagedBlobStore) ownerDir(owner app.BlobOwner) (string, error) {
	if owner.Subject == "" || owner.Session == "" {
		return "", fmt.Errorf("sdd: blob owner subject and session are required")
	}
	sum := sha256.Sum256([]byte(owner.Subject + "\x00" + string(owner.Session)))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])), nil
}

func randomBlobID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func validBlobID(id string) error {
	if len(id) != 32 {
		return fmt.Errorf("sdd: invalid staged blob ID")
	}
	_, err := hex.DecodeString(id)
	return err
}
