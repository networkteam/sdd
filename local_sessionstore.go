package sdd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/gofrs/flock"
)

// FilesystemSessionStore persists each session as append-only JSONL. Events
// stay opaque and every append is version-CAS protected.
type FilesystemSessionStore struct {
	dir string
	mu  sync.Mutex
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

func (s *FilesystemSessionStore) Create(_ context.Context, metadata SessionMetadata) (StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if metadata.ID == "" || metadata.Subject == "" || metadata.Project == "" {
		return StoredSession{}, fmt.Errorf("sdd: session ID, subject, and project are required")
	}
	filename, err := s.filename(metadata.ID)
	if err != nil {
		return StoredSession{}, err
	}
	lock, err := s.lock(metadata.ID)
	if err != nil {
		return StoredSession{}, err
	}
	defer unlockFile(lock)
	if _, err := os.Stat(filename); err == nil {
		return StoredSession{}, &ApplicationError{Code: ErrorSessionConflict, Message: "session already exists"}
	} else if !os.IsNotExist(err) {
		return StoredSession{}, err
	}
	stored := StoredSession{Metadata: metadata, Version: 1}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return StoredSession{}, err
	}
	if err := appendSessionLine(file, sessionLine{Version: 1, Metadata: &metadata}); err != nil {
		_ = file.Close()
		return StoredSession{}, err
	}
	if err := file.Close(); err != nil {
		return StoredSession{}, err
	}
	return stored, nil
}

func (s *FilesystemSessionStore) Load(_ context.Context, id SessionID) (StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lock(id)
	if err != nil {
		return StoredSession{}, err
	}
	defer unlockFile(lock)
	return s.loadLocked(id)
}

func (s *FilesystemSessionStore) loadLocked(id SessionID) (StoredSession, error) {
	filename, err := s.filename(id)
	if err != nil {
		return StoredSession{}, err
	}
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return StoredSession{}, fmt.Errorf("%w: %s", ErrSessionNotFound, id)
		}
		return StoredSession{}, err
	}
	defer func() { _ = file.Close() }()
	var stored StoredSession
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var line sessionLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return StoredSession{}, fmt.Errorf("sdd: decoding session %s line %d: %w", id, lineNumber, err)
		}
		if line.Version != uint64(lineNumber) {
			return StoredSession{}, fmt.Errorf("sdd: session %s has non-sequential version %d at line %d", id, line.Version, lineNumber)
		}
		if line.Metadata != nil {
			stored.Metadata = *line.Metadata
		}
		stored.Events = append(stored.Events, line.Events...)
		stored.Version = line.Version
	}
	if err := scanner.Err(); err != nil {
		return StoredSession{}, err
	}
	if lineNumber == 0 || stored.Metadata.ID != id {
		return StoredSession{}, fmt.Errorf("sdd: invalid or empty session %s", id)
	}
	return stored, nil
}

func (s *FilesystemSessionStore) List(_ context.Context, filter SessionFilter) ([]StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var sessions []StoredSession
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		id := SessionID(strings.TrimSuffix(entry.Name(), ".jsonl"))
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

func (s *FilesystemSessionStore) Append(_ context.Context, id SessionID, expectedVersion uint64, appendData SessionAppend) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
		return stored.Version, &ApplicationError{Code: ErrorSessionConflict, Message: "session version changed"}
	}
	if appendData.Metadata != nil {
		if appendData.Metadata.ID != id || appendData.Metadata.Subject != stored.Metadata.Subject || appendData.Metadata.Project != stored.Metadata.Project {
			return stored.Version, &ApplicationError{Code: ErrorSessionOwnership, Message: "session identity and project are immutable"}
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

func (s *FilesystemSessionStore) lock(id SessionID) (*flock.Flock, error) {
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

func (s *FilesystemSessionStore) filename(id SessionID) (string, error) {
	if id == "" || strings.ContainsAny(string(id), `/\\`) || filepath.Base(string(id)) != string(id) {
		return "", fmt.Errorf("sdd: invalid session ID %q", id)
	}
	return filepath.Join(s.dir, string(id)+".jsonl"), nil
}

type sessionLine struct {
	Version  uint64           `json:"version"`
	Metadata *SessionMetadata `json:"metadata,omitempty"`
	Events   []StoredEvent    `json:"events,omitempty"`
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
