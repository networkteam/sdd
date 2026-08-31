package local

import (
	"context"
	"crypto/rand"
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

	app "github.com/networkteam/sdd/pkg/application"
)

const (
	blobSuffix         = ".blob"
	blobMetadataSuffix = ".json"
	blobRetentionsName = "retentions.json"
)

// FilesystemStagedBlobStore keeps a session's staged bytes under
// <subject>/<session>/, so the path itself says whose they are. That is what
// makes a sweep possible without reading anything: enumeration is a directory
// walk, and a staging area with nothing in it is still fully identified.
//
// Like the session store it reads across every configured location and writes
// where it resolved.
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
	ref app.SessionRef,
	filename string,
	content io.Reader,
) (app.StagedBlob, error) {
	dir, err := stagingDir(ref)
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
	if err := root.MkdirAll(dir, 0o700); err != nil {
		return app.StagedBlob{}, err
	}
	id, err := randomBlobID()
	if err != nil {
		return app.StagedBlob{}, err
	}
	blob := app.StagedBlob{
		ID: id, Session: ref, Digest: sha256Digest(data), Size: int64(len(data)), Filename: filename,
		CreatedAt: time.Now().UTC().Round(0),
	}
	blobName := filepath.Join(dir, id+blobSuffix)
	if err := publishBytes(root, blobName, data); err != nil {
		return app.StagedBlob{}, err
	}
	if err := publishJSON(root, filepath.Join(dir, id+blobMetadataSuffix), blob); err != nil {
		return app.StagedBlob{}, errors.Join(err, root.Remove(blobName))
	}
	return blob, nil
}

func (s *FilesystemStagedBlobStore) Stat(_ context.Context, ref app.SessionRef, id string) (app.StagedBlob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, dir, err := s.resolve(ref)
	if err != nil {
		return app.StagedBlob{}, err
	}
	defer func() { _ = root.Close() }()
	return readBlobMetadata(root, dir, ref, id)
}

func (s *FilesystemStagedBlobStore) Open(_ context.Context, ref app.SessionRef, id string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, dir, err := s.resolve(ref)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	if _, err := readBlobMetadata(root, dir, ref, id); err != nil {
		return nil, err
	}
	return root.Open(filepath.Join(dir, id+blobSuffix))
}

func (s *FilesystemStagedBlobStore) Retain(
	_ context.Context,
	ref app.SessionRef,
	retentionID string,
	ids []string,
) error {
	if retentionID == "" {
		return fmt.Errorf("sdd: retention ID is required")
	}
	// A retention over no blobs holds nothing, so it writes nothing. This is
	// what keeps a session that staged nothing from leaving a staging area
	// behind for a sweep to puzzle over later.
	if len(ids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, dir, err := s.resolve(ref)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if err := root.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := readBlobMetadata(root, dir, ref, id); err != nil {
			return err
		}
	}
	retentions, err := readRetentions(root, dir)
	if err != nil {
		return err
	}
	retentions[retentionID] = append([]string(nil), ids...)
	return publishJSON(root, filepath.Join(dir, blobRetentionsName), retentions)
}

// Release drops one retention. A session with nothing staged has nothing to
// release, so this creates no directory — an empty staging area would be
// scaffolding that never held anything.
func (s *FilesystemStagedBlobStore) Release(_ context.Context, ref app.SessionRef, retentionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, dir, err := s.resolve(ref)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	retentions, err := readRetentions(root, dir)
	if err != nil {
		return err
	}
	if _, held := retentions[retentionID]; !held {
		return nil
	}
	delete(retentions, retentionID)
	return publishJSON(root, filepath.Join(dir, blobRetentionsName), retentions)
}

// StagedSessions enumerates every staging area across all locations, read
// straight from the paths.
func (s *FilesystemStagedBlobStore) StagedSessions(_ context.Context) ([]app.SessionRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var refs []app.SessionRef
	seen := make(map[app.SessionRef]struct{})
	for _, location := range s.locations {
		root, err := openStoreRoot(location.StagedBlobs, false)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		subjects, readErr := fs.ReadDir(root.FS(), ".")
		if readErr != nil {
			return nil, errors.Join(readErr, root.Close())
		}
		for _, subject := range subjects {
			if !subject.IsDir() || strings.HasPrefix(subject.Name(), ".") {
				continue
			}
			sessions, err := fs.ReadDir(root.FS(), subject.Name())
			if err != nil {
				return nil, errors.Join(err, root.Close())
			}
			for _, session := range sessions {
				if !session.IsDir() || strings.HasPrefix(session.Name(), ".") {
					continue
				}
				ref := app.SessionRef{Subject: subject.Name(), Session: app.SessionID(session.Name())}
				if _, duplicate := seen[ref]; duplicate {
					continue
				}
				seen[ref] = struct{}{}
				refs = append(refs, ref)
			}
		}
		if err := root.Close(); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

// DeleteStaged removes a session's staged blobs and retentions together. An
// area that is already gone is success, so a sweep is safe to repeat and safe to
// run concurrently with another.
func (s *FilesystemStagedBlobStore) DeleteStaged(_ context.Context, ref app.SessionRef) error {
	dir, err := stagingDir(ref)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, location := range s.locations {
		root, err := openStoreRoot(location.StagedBlobs, false)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		removeErr := removeStagingDir(root, dir)
		if err := errors.Join(removeErr, root.Close()); err != nil {
			return err
		}
	}
	return nil
}

// resolve opens the location holding this session's staging area, falling back
// to the first location so a caller reading nothing staged gets the ordinary
// not-exist error. The caller owns closing the returned root.
func (s *FilesystemStagedBlobStore) resolve(ref app.SessionRef) (*os.Root, string, error) {
	dir, err := stagingDir(ref)
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
		info, statErr := root.Stat(dir)
		if statErr == nil && info.IsDir() {
			return root, dir, nil
		}
		if closeErr := root.Close(); closeErr != nil {
			return nil, "", closeErr
		}
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return nil, "", statErr
		}
	}
	root, err := openStoreRoot(s.locations[0].StagedBlobs, false)
	return root, dir, err
}

func readBlobMetadata(root *os.Root, dir string, ref app.SessionRef, id string) (app.StagedBlob, error) {
	if err := validBlobID(id); err != nil {
		return app.StagedBlob{}, err
	}
	var blob app.StagedBlob
	if err := readJSON(root, filepath.Join(dir, id+blobMetadataSuffix), &blob); err != nil {
		return app.StagedBlob{}, err
	}
	if blob.Session != ref || blob.ID != id {
		return app.StagedBlob{}, fmt.Errorf("sdd: staged blob does not belong to session %s", ref.Session)
	}
	return blob, nil
}

// readRetentions reads a staging area's retentions. It creates nothing: a
// missing file surfaces as fs.ErrNotExist for the caller to interpret.
func readRetentions(root *os.Root, dir string) (map[string][]string, error) {
	retentions := map[string][]string{}
	if err := readJSON(root, filepath.Join(dir, blobRetentionsName), &retentions); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return retentions, nil
		}
		return nil, err
	}
	return retentions, nil
}

func removeStagingDir(root *os.Root, dir string) error {
	entries, err := fs.ReadDir(root.FS(), dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := root.Remove(filepath.Join(dir, entry.Name())); err != nil &&
			!errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if err := root.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncRootDir(root, filepath.Dir(dir))
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

// stagingDir is the readable path a session's staged blobs live under.
func stagingDir(ref app.SessionRef) (string, error) {
	if err := plainSegment("subject", ref.Subject); err != nil {
		return "", err
	}
	if err := plainSegment("session", string(ref.Session)); err != nil {
		return "", err
	}
	return filepath.Join(ref.Subject, string(ref.Session)), nil
}

func plainSegment(what, value string) error {
	if value == "" || value == "." || value == ".." ||
		strings.ContainsAny(value, `/\`) || filepath.Base(value) != value {
		return fmt.Errorf("sdd: blob %s must be a plain path segment, got %q", what, value)
	}
	return nil
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
