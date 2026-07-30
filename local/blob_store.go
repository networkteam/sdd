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

const (
	blobSuffix          = ".blob"
	blobMetadataSuffix  = ".json"
	blobRetentionsName  = "retentions.json"
	blobOwnerRecordName = "owner.json"
)

// FilesystemStagedBlobStore keeps immutable session-scoped bytes plus the
// retentions holding them. Like the session store it reads across every
// configured location and writes where it resolved, so material staged by an
// earlier version stays reachable where it lies.
type FilesystemStagedBlobStore struct {
	locations []StoreLocation
	mu        sync.Mutex
}

func NewFilesystemStagedBlobStore(locations ...StoreLocation) (*FilesystemStagedBlobStore, error) {
	if len(locations) == 0 {
		return nil, fmt.Errorf("sdd: at least one staged blob store location is required")
	}
	return &FilesystemStagedBlobStore{locations: locations}, nil
}

// NewFilesystemStagedBlobStoreAt is the single-directory form, for a
// composition with no location history to resolve across.
func NewFilesystemStagedBlobStoreAt(dir string) (*FilesystemStagedBlobStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("sdd: staged blob directory is required")
	}
	return NewFilesystemStagedBlobStore(StoreLocation{
		Name: dir, StagedBlobs: dir, Subject: "local", Project: "local",
	})
}

func (s *FilesystemStagedBlobStore) Stage(
	_ context.Context,
	owner app.BlobOwner,
	filename string,
	content io.Reader,
) (app.StagedBlob, error) {
	ownerDir, err := ownerDirName(owner)
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
	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := openStoreRoot(s.locations[0].StagedBlobs, true)
	if err != nil {
		return app.StagedBlob{}, err
	}
	defer func() { _ = root.Close() }()
	if err := root.MkdirAll(ownerDir, 0o700); err != nil {
		return app.StagedBlob{}, err
	}
	if err := publishJSON(root, filepath.Join(ownerDir, blobOwnerRecordName), owner); err != nil {
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
	blobName := filepath.Join(ownerDir, id+blobSuffix)
	if err := publishBytes(root, blobName, data); err != nil {
		return app.StagedBlob{}, err
	}
	if err := publishJSON(root, filepath.Join(ownerDir, id+blobMetadataSuffix), blob); err != nil {
		return app.StagedBlob{}, errors.Join(err, root.Remove(blobName))
	}
	return blob, nil
}

func (s *FilesystemStagedBlobStore) Stat(_ context.Context, owner app.BlobOwner, id string) (app.StagedBlob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, ownerDir, err := s.resolveOwner(owner)
	if err != nil {
		return app.StagedBlob{}, err
	}
	defer func() { _ = root.Close() }()
	return readBlobMetadata(root, ownerDir, owner, id)
}

func (s *FilesystemStagedBlobStore) Open(_ context.Context, owner app.BlobOwner, id string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, ownerDir, err := s.resolveOwner(owner)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	if _, err := readBlobMetadata(root, ownerDir, owner, id); err != nil {
		return nil, err
	}
	return root.Open(filepath.Join(ownerDir, id+blobSuffix))
}

func (s *FilesystemStagedBlobStore) Retain(
	_ context.Context,
	owner app.BlobOwner,
	retentionID string,
	ids []string,
) error {
	if retentionID == "" {
		return fmt.Errorf("sdd: retention ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, ownerDir, err := s.resolveOwner(owner)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	for _, id := range ids {
		if _, err := readBlobMetadata(root, ownerDir, owner, id); err != nil {
			return err
		}
	}
	retentions, err := readRetentions(root, ownerDir)
	if err != nil {
		return err
	}
	retentions[retentionID] = append([]string(nil), ids...)
	return publishJSON(root, filepath.Join(ownerDir, blobRetentionsName), retentions)
}

func (s *FilesystemStagedBlobStore) Release(_ context.Context, owner app.BlobOwner, retentionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, ownerDir, err := s.resolveOwner(owner)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	retentions, err := readRetentions(root, ownerDir)
	if err != nil {
		return err
	}
	delete(retentions, retentionID)
	return publishJSON(root, filepath.Join(ownerDir, blobRetentionsName), retentions)
}

// resolveOwner opens the location holding this owner's directory. The caller
// owns closing the returned root.
func (s *FilesystemStagedBlobStore) resolveOwner(owner app.BlobOwner) (*os.Root, string, error) {
	ownerDir, err := ownerDirName(owner)
	if err != nil {
		return nil, "", err
	}
	for _, location := range s.locations {
		root, err := openStoreRoot(location.StagedBlobs, false)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		info, statErr := root.Stat(ownerDir)
		if statErr == nil && info.IsDir() {
			return root, ownerDir, nil
		}
		if closeErr := root.Close(); closeErr != nil {
			return nil, "", closeErr
		}
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return nil, "", statErr
		}
	}
	// Nothing staged yet: the first location is where staging would land, so
	// reads there produce the ordinary not-exist errors callers expect.
	root, err := openStoreRoot(s.locations[0].StagedBlobs, true)
	if err != nil {
		return nil, "", err
	}
	return root, ownerDir, nil
}

func readBlobMetadata(root *os.Root, ownerDir string, owner app.BlobOwner, id string) (app.StagedBlob, error) {
	if err := validBlobID(id); err != nil {
		return app.StagedBlob{}, err
	}
	var blob app.StagedBlob
	if err := readJSON(root, filepath.Join(ownerDir, id+blobMetadataSuffix), &blob); err != nil {
		return app.StagedBlob{}, err
	}
	if blob.Owner != owner || blob.ID != id {
		return app.StagedBlob{}, fmt.Errorf("sdd: staged blob ownership mismatch")
	}
	return blob, nil
}

func readRetentions(root *os.Root, ownerDir string) (map[string][]string, error) {
	if err := root.MkdirAll(ownerDir, 0o700); err != nil {
		return nil, err
	}
	retentions := map[string][]string{}
	err := readJSON(root, filepath.Join(ownerDir, blobRetentionsName), &retentions)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return retentions, nil
}

func readJSON(root *os.Root, name string, destination any) error {
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	raw, readErr := io.ReadAll(file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return err
	}
	return json.Unmarshal(raw, destination)
}

func ownerDirName(owner app.BlobOwner) (string, error) {
	if owner.Subject == "" || owner.Session == "" {
		return "", fmt.Errorf("sdd: blob owner subject and session are required")
	}
	sum := sha256.Sum256([]byte(owner.Subject + "\x00" + string(owner.Session)))
	return hex.EncodeToString(sum[:]), nil
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
