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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	app "github.com/networkteam/sdd/application"
)

type FilesystemStagedBlobStore struct {
	dir string
	mu  sync.Mutex
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
	if _, err := s.statLocked(owner, id); err != nil {
		return nil, err
	}
	ownerDir, _ := s.ownerDir(owner)
	return os.Open(filepath.Join(ownerDir, id+".blob"))
}

func (s *FilesystemStagedBlobStore) Retain(_ context.Context, owner app.BlobOwner, retentionID string, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	retentions, filename, err := s.retentionsLocked(owner)
	if err != nil {
		return err
	}
	delete(retentions, retentionID)
	return writeJSONAtomic(filename, retentions)
}

func (s *FilesystemStagedBlobStore) remove(owner app.BlobOwner, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ownerDir, err := s.ownerDir(owner)
	if err != nil {
		return err
	}
	if err := validBlobID(id); err != nil {
		return err
	}
	return errors.Join(
		removeIfExists(filepath.Join(ownerDir, id+".blob")),
		removeIfExists(filepath.Join(ownerDir, id+".json")),
	)
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
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
