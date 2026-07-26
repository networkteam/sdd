package local

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/gofrs/flock"
	app "github.com/networkteam/sdd/application"
)

// FilesystemSessionStore persists each session as append-only JSONL. Events
// stay opaque and every append is version-CAS protected.
type FilesystemSessionStore struct {
	dir                  string
	trustedStateRoot     string
	mu                   sync.Mutex
	beforeRootedMutation func()
}

// NewFilesystemSessionStoreAtStateRoot is the production constructor for a
// machine-global store. It is read-only at construction time; each operation
// resolves the category/key below the explicit state-root descriptor.
func NewFilesystemSessionStoreAtStateRoot(stateRoot, dir string) (*FilesystemSessionStore, error) {
	if stateRoot == "" || dir == "" {
		return nil, fmt.Errorf("sdd: trusted state root and session directory are required")
	}
	if !pathAtOrInside(filepath.Join(stateRoot, "sessions"), dir) {
		return nil, fmt.Errorf("sdd: session directory escapes trusted state root")
	}
	return &FilesystemSessionStore{dir: dir, trustedStateRoot: stateRoot}, nil
}

func NewFilesystemSessionStore(dir string) (*FilesystemSessionStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("sdd: session directory is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("sdd: creating session directory: %w", err)
	}
	return &FilesystemSessionStore{dir: dir}, nil
}

func (s *FilesystemSessionStore) Create(_ context.Context, metadata app.SessionMetadata) (app.StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trustedStateRoot != "" {
		return s.createRooted(metadata)
	}
	if metadata.ID == "" || metadata.Subject == "" || metadata.Project == "" {
		return app.StoredSession{}, fmt.Errorf("sdd: session ID, subject, and project are required")
	}
	filename, err := s.filename(metadata.ID)
	if err != nil {
		return app.StoredSession{}, err
	}
	lock, err := s.lock(metadata.ID)
	if err != nil {
		return app.StoredSession{}, err
	}
	defer unlockFile(lock)
	if _, err := os.Stat(filename); err == nil {
		return app.StoredSession{}, &app.ApplicationError{Code: app.ErrorSessionConflict, Message: "session already exists"}
	} else if !os.IsNotExist(err) {
		return app.StoredSession{}, err
	}
	stored := app.StoredSession{Metadata: metadata, Version: 1}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return app.StoredSession{}, err
	}
	if err := appendSessionLine(file, sessionLine{Version: 1, Metadata: &metadata}); err != nil {
		_ = file.Close()
		return app.StoredSession{}, err
	}
	if err := file.Close(); err != nil {
		return app.StoredSession{}, err
	}
	return stored, nil
}

func (s *FilesystemSessionStore) Load(_ context.Context, id app.SessionID) (app.StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trustedStateRoot != "" {
		return s.loadRooted(id)
	}
	lock, err := s.lock(id)
	if err != nil {
		return app.StoredSession{}, err
	}
	defer unlockFile(lock)
	return s.loadLocked(id)
}

func (s *FilesystemSessionStore) loadLocked(id app.SessionID) (app.StoredSession, error) {
	filename, err := s.filename(id)
	if err != nil {
		return app.StoredSession{}, err
	}
	format, err := classifySessionFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return app.StoredSession{}, fmt.Errorf("%w: %s", app.ErrSessionNotFound, id)
		}
		return app.StoredSession{}, err
	}
	if format != sessionFormatCurrent {
		return app.StoredSession{}, sessionMigrationRequired()
	}
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return app.StoredSession{}, fmt.Errorf("%w: %s", app.ErrSessionNotFound, id)
		}
		return app.StoredSession{}, err
	}
	defer func() { _ = file.Close() }()
	var stored app.StoredSession
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var line sessionLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return app.StoredSession{}, fmt.Errorf("sdd: decoding session %s line %d: %w", id, lineNumber, err)
		}
		if line.Version != uint64(lineNumber) {
			return app.StoredSession{}, fmt.Errorf("sdd: session %s has non-sequential version %d at line %d", id, line.Version, lineNumber)
		}
		if line.Metadata != nil {
			stored.Metadata = *line.Metadata
		}
		stored.Events = append(stored.Events, line.Events...)
		stored.Version = line.Version
	}
	if err := scanner.Err(); err != nil {
		return app.StoredSession{}, err
	}
	if lineNumber == 0 || stored.Metadata.ID != id {
		return app.StoredSession{}, fmt.Errorf("sdd: invalid or empty session %s", id)
	}
	return stored, nil
}

func (s *FilesystemSessionStore) List(_ context.Context, filter app.SessionFilter) ([]app.StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trustedStateRoot != "" {
		return s.listRooted(filter)
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var sessions []app.StoredSession
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		id := app.SessionID(strings.TrimSuffix(entry.Name(), ".jsonl"))
		filename, err := s.filename(id)
		if err != nil {
			return nil, err
		}
		format, err := classifySessionFile(filename)
		if err != nil {
			return nil, err
		}
		if format != sessionFormatCurrent {
			continue
		}
		lock, err := s.lock(id)
		if err != nil {
			return nil, err
		}
		stored, err := s.loadLocked(id)
		unlockFile(lock)
		if err != nil {
			return nil, err
		}
		if filter.Subject != "" && stored.Metadata.Subject != filter.Subject {
			continue
		}
		if filter.Project != "" && stored.Metadata.Project != filter.Project {
			continue
		}
		sessions = append(sessions, stored)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].Metadata.ID < sessions[j].Metadata.ID })
	return sessions, nil
}

func (s *FilesystemSessionStore) Append(_ context.Context, id app.SessionID, expectedVersion uint64, appendData app.SessionAppend) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trustedStateRoot != "" {
		return s.appendRooted(id, expectedVersion, appendData)
	}
	lock, err := s.lock(id)
	if err != nil {
		return 0, err
	}
	defer unlockFile(lock)
	stored, err := s.loadLocked(id)
	if err != nil {
		return 0, err
	}
	if stored.Version != expectedVersion {
		return stored.Version, &app.ApplicationError{Code: app.ErrorSessionConflict, Message: "session version changed"}
	}
	if appendData.Metadata != nil {
		if appendData.Metadata.ID != id || appendData.Metadata.Subject != stored.Metadata.Subject || appendData.Metadata.Project != stored.Metadata.Project {
			return stored.Version, &app.ApplicationError{Code: app.ErrorSessionOwnership, Message: "session identity and project are immutable"}
		}
		stored.Metadata = *appendData.Metadata
	}
	stored.Events = append(stored.Events, appendData.Events...)
	stored.Version++
	filename, err := s.filename(id)
	if err != nil {
		return 0, err
	}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	line := sessionLine{Version: stored.Version, Metadata: appendData.Metadata, Events: appendData.Events}
	if err := appendSessionLine(file, line); err != nil {
		_ = file.Close()
		return 0, err
	}
	if err := file.Close(); err != nil {
		return 0, err
	}
	return stored.Version, nil
}

func (s *FilesystemSessionStore) openRooted(create bool) (*trustedStoreRoot, error) {
	return openTrustedStoreRoot(s.trustedStateRoot, "sessions", s.dir, create)
}

func rootedSessionName(id app.SessionID) (string, error) {
	if id == "" || strings.ContainsAny(string(id), `/\`) || filepath.Base(string(id)) != string(id) {
		return "", fmt.Errorf("sdd: invalid session ID %q", id)
	}
	return string(id) + ".jsonl", nil
}

func lockRootedSession(root *os.Root, name string) (*pinnedFileLock, error) {
	file, err := root.OpenFile(name+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	lock, err := lockPinnedFile(file)
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return lock, nil
}

func (s *FilesystemSessionStore) createRooted(metadata app.SessionMetadata) (app.StoredSession, error) {
	if metadata.ID == "" || metadata.Subject == "" || metadata.Project == "" {
		return app.StoredSession{}, fmt.Errorf("sdd: session ID, subject, and project are required")
	}
	name, err := rootedSessionName(metadata.ID)
	if err != nil {
		return app.StoredSession{}, err
	}
	roots, err := s.openRooted(true)
	if err != nil {
		return app.StoredSession{}, err
	}
	defer func() { _ = roots.close() }()
	if s.beforeRootedMutation != nil {
		s.beforeRootedMutation()
		s.beforeRootedMutation = nil
		if err := roots.revalidate(); err != nil {
			return app.StoredSession{}, err
		}
	}
	lock, err := lockRootedSession(roots.store, name)
	if err != nil {
		return app.StoredSession{}, err
	}
	defer func() { _ = lock.Unlock() }()
	if _, err := roots.store.Lstat(name); err == nil {
		return app.StoredSession{}, &app.ApplicationError{Code: app.ErrorSessionConflict, Message: "session already exists"}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return app.StoredSession{}, err
	}
	file, err := roots.store.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return app.StoredSession{}, err
	}
	if err := appendSessionLine(file, sessionLine{Version: 1, Metadata: &metadata}); err != nil {
		_ = file.Close()
		return app.StoredSession{}, err
	}
	if err := file.Close(); err != nil {
		return app.StoredSession{}, err
	}
	if err := roots.revalidate(); err != nil {
		return app.StoredSession{}, err
	}
	return app.StoredSession{Metadata: metadata, Version: 1}, nil
}

func (s *FilesystemSessionStore) loadRooted(id app.SessionID) (app.StoredSession, error) {
	name, err := rootedSessionName(id)
	if err != nil {
		return app.StoredSession{}, err
	}
	roots, err := s.openRooted(false)
	if errors.Is(err, fs.ErrNotExist) {
		return app.StoredSession{}, fmt.Errorf("%w: %s", app.ErrSessionNotFound, id)
	}
	if err != nil {
		return app.StoredSession{}, err
	}
	defer func() { _ = roots.close() }()
	lock, err := lockRootedSession(roots.store, name)
	if err != nil {
		return app.StoredSession{}, err
	}
	defer func() { _ = lock.Unlock() }()
	return loadRootedSession(roots.store, name, id)
}

func loadRootedSession(root *os.Root, name string, id app.SessionID) (app.StoredSession, error) {
	file, err := openRootedRegular(root, name)
	if errors.Is(err, fs.ErrNotExist) {
		return app.StoredSession{}, fmt.Errorf("%w: %s", app.ErrSessionNotFound, id)
	}
	if err != nil {
		return app.StoredSession{}, err
	}
	defer func() { _ = file.Close() }()
	format, err := classifySessionHandle(file)
	if err != nil {
		return app.StoredSession{}, err
	}
	if format != sessionFormatCurrent {
		return app.StoredSession{}, sessionMigrationRequired()
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return app.StoredSession{}, err
	}
	return decodeStoredSession(file, id)
}

func decodeStoredSession(file io.Reader, id app.SessionID) (app.StoredSession, error) {
	var stored app.StoredSession
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var line sessionLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return app.StoredSession{}, fmt.Errorf("sdd: decoding session %s line %d: %w", id, lineNumber, err)
		}
		if line.Version != uint64(lineNumber) {
			return app.StoredSession{}, fmt.Errorf("sdd: session %s has non-sequential version %d at line %d", id, line.Version, lineNumber)
		}
		if line.Metadata != nil {
			stored.Metadata = *line.Metadata
		}
		stored.Events = append(stored.Events, line.Events...)
		stored.Version = line.Version
	}
	if err := scanner.Err(); err != nil {
		return app.StoredSession{}, err
	}
	if lineNumber == 0 || stored.Metadata.ID != id {
		return app.StoredSession{}, fmt.Errorf("sdd: invalid or empty session %s", id)
	}
	return stored, nil
}

func (s *FilesystemSessionStore) listRooted(filter app.SessionFilter) ([]app.StoredSession, error) {
	roots, err := s.openRooted(false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = roots.close() }()
	entries, err := fs.ReadDir(roots.store.FS(), ".")
	if err != nil {
		return nil, err
	}
	var sessions []app.StoredSession
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		id := app.SessionID(strings.TrimSuffix(entry.Name(), ".jsonl"))
		probe, err := openRootedRegular(roots.store, entry.Name())
		if err != nil {
			return nil, err
		}
		format, classifyErr := classifySessionHandle(probe)
		closeErr := probe.Close()
		if err := errors.Join(classifyErr, closeErr); err != nil {
			return nil, err
		}
		if format != sessionFormatCurrent {
			continue
		}
		lock, err := lockRootedSession(roots.store, entry.Name())
		if err != nil {
			return nil, err
		}
		stored, loadErr := loadRootedSession(roots.store, entry.Name(), id)
		unlockErr := lock.Unlock()
		if err := errors.Join(loadErr, unlockErr); err != nil {
			return nil, err
		}
		if filter.Subject != "" && stored.Metadata.Subject != filter.Subject {
			continue
		}
		if filter.Project != "" && stored.Metadata.Project != filter.Project {
			continue
		}
		sessions = append(sessions, stored)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].Metadata.ID < sessions[j].Metadata.ID })
	return sessions, roots.revalidate()
}

func (s *FilesystemSessionStore) appendRooted(
	id app.SessionID,
	expectedVersion uint64,
	appendData app.SessionAppend,
) (uint64, error) {
	name, err := rootedSessionName(id)
	if err != nil {
		return 0, err
	}
	roots, err := s.openRooted(false)
	if err != nil {
		return 0, err
	}
	defer func() { _ = roots.close() }()
	if s.beforeRootedMutation != nil {
		s.beforeRootedMutation()
		s.beforeRootedMutation = nil
		if err := roots.revalidate(); err != nil {
			return 0, err
		}
	}
	lock, err := lockRootedSession(roots.store, name)
	if err != nil {
		return 0, err
	}
	defer func() { _ = lock.Unlock() }()
	stored, err := loadRootedSession(roots.store, name, id)
	if err != nil {
		return 0, err
	}
	if stored.Version != expectedVersion {
		return stored.Version, &app.ApplicationError{Code: app.ErrorSessionConflict, Message: "session version changed"}
	}
	if appendData.Metadata != nil {
		if appendData.Metadata.ID != id || appendData.Metadata.Subject != stored.Metadata.Subject ||
			appendData.Metadata.Project != stored.Metadata.Project {
			return stored.Version, &app.ApplicationError{Code: app.ErrorSessionOwnership, Message: "session identity and project are immutable"}
		}
	}
	file, err := roots.store.OpenFile(name, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	next := stored.Version + 1
	if err := appendSessionLine(file, sessionLine{
		Version: next, Metadata: appendData.Metadata, Events: appendData.Events,
	}); err != nil {
		_ = file.Close()
		return 0, err
	}
	if err := file.Close(); err != nil {
		return 0, err
	}
	if err := roots.revalidate(); err != nil {
		return 0, err
	}
	return next, nil
}

func (s *FilesystemSessionStore) lock(id app.SessionID) (*flock.Flock, error) {
	filename, err := s.filename(id)
	if err != nil {
		return nil, err
	}
	lock := flock.New(filename + ".lock")
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	return lock, nil
}

func unlockFile(lock *flock.Flock) {
	if lock != nil {
		_ = lock.Unlock()
	}
}

func (s *FilesystemSessionStore) filename(id app.SessionID) (string, error) {
	if id == "" || strings.ContainsAny(string(id), `/\\`) || filepath.Base(string(id)) != string(id) {
		return "", fmt.Errorf("sdd: invalid session ID %q", id)
	}
	return filepath.Join(s.dir, string(id)+".jsonl"), nil
}

type sessionLine struct {
	Version  uint64               `json:"version"`
	Metadata *app.SessionMetadata `json:"metadata,omitempty"`
	Events   []app.StoredEvent    `json:"events,omitempty"`
}

// decodeSessionLine strictly decodes one current-envelope session log line,
// skipping only the metadata fields an earlier codec wrote and the model has
// since retired. Read-compatibility with every shape sdd has written is
// permanent (d-cpt-i2x), so a retired field is converted away instead of
// rejected — while every other unknown field still fails, keeping strict
// decoding useful as a drift alarm. This is the one strict session-line
// decoder; readers that need the tolerance go through it rather than repeating
// the rules.
func decodeSessionLine(raw []byte, line *sessionLine) error {
	normalized, err := dropRetiredSessionMetadata(raw)
	if err != nil {
		return err
	}
	return decodeStrictJSON(normalized, line)
}

// dropRetiredSessionMetadata removes retired metadata fields from one raw
// session line. It returns the input untouched when there is nothing to drop,
// so a log written by the current codec is decoded without a re-encode.
func dropRetiredSessionMetadata(raw []byte) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	encodedMetadata, ok := envelope["metadata"]
	if !ok {
		return raw, nil
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(encodedMetadata, &metadata); err != nil {
		return nil, err
	}
	var codec struct{ CodecVersion uint32 }
	if err := json.Unmarshal(encodedMetadata, &codec); err != nil {
		return nil, err
	}
	dropped := false
	for _, field := range app.RetiredSessionMetadataFields(codec.CodecVersion) {
		if _, present := metadata[field]; present {
			delete(metadata, field)
			dropped = true
		}
	}
	if !dropped {
		return raw, nil
	}
	rewritten, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	envelope["metadata"] = rewritten
	return json.Marshal(envelope)
}

type sessionFormat uint8

const (
	sessionFormatLegacy sessionFormat = iota
	sessionFormatCurrent
)

// classifySessionFile only identifies the current envelope. Everything else
// is legacy from the current runtime's perspective, including an unreadable
// first line: it must not make unrelated healthy sessions unavailable.
func classifySessionFile(filename string) (sessionFormat, error) {
	file, err := os.Open(filename)
	if err != nil {
		return sessionFormatLegacy, err
	}
	defer func() { _ = file.Close() }()
	return classifySessionHandle(file)
}

func classifySessionHandle(file *os.File) (sessionFormat, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return sessionFormatLegacy, err
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(strings.TrimSpace(string(raw))) == 0 {
			continue
		}
		var shape map[string]json.RawMessage
		if err := json.Unmarshal(raw, &shape); err != nil {
			return sessionFormatLegacy, nil
		}
		if _, ok := shape["version"]; ok {
			return sessionFormatCurrent, nil
		}
		return sessionFormatLegacy, nil
	}
	if err := scanner.Err(); err != nil {
		return sessionFormatLegacy, err
	}
	return sessionFormatLegacy, nil
}

func sessionMigrationRequired() error {
	return &app.ApplicationError{
		Code:    app.ErrorMigrationRequired,
		Message: "session migration required",
	}
}

func appendSessionLine(file *os.File, line sessionLine) error {
	encoded, err := json.Marshal(line)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if _, err := file.Write(encoded); err != nil {
		return err
	}
	return file.Sync()
}

func writeJSONAtomic(filename string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(filename), ".sdd-session-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := temp.Write(encoded); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, filename); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(filename))
}
