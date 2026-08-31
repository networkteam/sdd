package local

import (
	"bufio"
	"bytes"
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
	"github.com/networkteam/sdd/internal/engine"
	app "github.com/networkteam/sdd/pkg/application"
)

const sessionLogSuffix = ".jsonl"

// maxSessionLine bounds one log line. Dialogue payloads are the large part.
const maxSessionLine = 16 * 1024 * 1024

// FilesystemSessionStore persists each session as an append-only JSONL log.
// Events stay opaque to the store and every append is version-CAS protected.
//
// It reads across every configured location and appends to whichever one holds
// the session, so a log written by an earlier version stays live where it lies.
// New sessions are created in the first location.
type FilesystemSessionStore struct {
	locations []StoreLocation
	mu        sync.Mutex
}

func NewFilesystemSessionStore(locations ...StoreLocation) (*FilesystemSessionStore, error) {
	if len(locations) == 0 {
		return nil, fmt.Errorf("sdd: at least one session store location is required")
	}
	return &FilesystemSessionStore{locations: locations}, nil
}

// NewFilesystemSessionStoreAt is the single-directory form, for a composition
// with no location history to resolve across.
func NewFilesystemSessionStoreAt(dir string) (*FilesystemSessionStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("sdd: session directory is required")
	}
	return NewFilesystemSessionStore(StoreLocation{
		Name: dir, Sessions: dir, Subject: "local", Project: "local",
	})
}

func (s *FilesystemSessionStore) Create(_ context.Context, metadata app.SessionMetadata) (app.StoredSession, error) {
	if metadata.ID == "" || metadata.Subject == "" || metadata.Project == "" {
		return app.StoredSession{}, fmt.Errorf("sdd: session ID, subject, and project are required")
	}
	name, err := sessionLogName(metadata.ID)
	if err != nil {
		return app.StoredSession{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	location := s.locations[0]
	root, err := openStoreRoot(location.Sessions, true)
	if err != nil {
		return app.StoredSession{}, err
	}
	defer func() { _ = root.Close() }()

	lock, err := lockSession(location.Sessions, name)
	if err != nil {
		return app.StoredSession{}, err
	}
	defer unlock(lock)

	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return app.StoredSession{}, &app.ApplicationError{Code: app.ErrorSessionConflict, Message: "session already exists"}
	}
	if err != nil {
		return app.StoredSession{}, err
	}
	if err := writeSessionLine(file, sessionLine{Version: 1, Metadata: &metadata}); err != nil {
		return app.StoredSession{}, errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return app.StoredSession{}, err
	}
	return app.StoredSession{Metadata: metadata, Version: 1}, nil
}

func (s *FilesystemSessionStore) Load(_ context.Context, id app.SessionID) (app.StoredSession, error) {
	name, err := sessionLogName(id)
	if err != nil {
		return app.StoredSession{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	located, err := s.locate(name)
	if err != nil {
		return app.StoredSession{}, err
	}
	lock, err := lockSession(located.Sessions, name)
	if err != nil {
		return app.StoredSession{}, err
	}
	defer unlock(lock)
	return readSessionLog(located, name, id)
}

func (s *FilesystemSessionStore) List(_ context.Context, filter app.SessionFilter) ([]app.StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions := make([]app.StoredSession, 0)
	seen := make(map[app.SessionID]struct{})
	for _, location := range s.locations {
		names, err := listSessionLogs(location.Sessions)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			id := app.SessionID(strings.TrimSuffix(name, sessionLogSuffix))
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			lock, err := lockSession(location.Sessions, name)
			if err != nil {
				return nil, err
			}
			stored, err := readSessionLog(location, name, id)
			unlock(lock)
			if err != nil {
				// A log this binary cannot read may belong to a newer version.
				// Skipping it keeps every other session listed; it is never
				// treated as garbage.
				continue
			}
			seen[id] = struct{}{}
			if filter.Subject != "" && stored.Metadata.Subject != filter.Subject {
				continue
			}
			if filter.Project != "" && stored.Metadata.Project != filter.Project {
				continue
			}
			sessions = append(sessions, stored)
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].Metadata.ID < sessions[j].Metadata.ID })
	return sessions, nil
}

func (s *FilesystemSessionStore) Append(
	_ context.Context,
	id app.SessionID,
	expectedVersion uint64,
	appendData app.SessionAppend,
) (uint64, error) {
	name, err := sessionLogName(id)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	located, err := s.locate(name)
	if err != nil {
		return 0, err
	}
	lock, err := lockSession(located.Sessions, name)
	if err != nil {
		return 0, err
	}
	defer unlock(lock)

	stored, err := readSessionLog(located, name, id)
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

	root, err := openStoreRoot(located.Sessions, false)
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	next := stored.Version + 1
	line := sessionLine{Version: next, Metadata: appendData.Metadata, Events: appendData.Events}
	if err := writeSessionLine(file, line); err != nil {
		return 0, errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return 0, err
	}
	return next, nil
}

// Delete removes a session's log wherever it lies. A session that is already
// gone is success: a collection sweep recomputes its target set from scratch
// every run, so two processes may derive and remove the same one.
func (s *FilesystemSessionStore) Delete(_ context.Context, id app.SessionID) error {
	name, err := sessionLogName(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	located, err := s.locate(name)
	if errors.Is(err, app.ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	root, err := openStoreRoot(located.Sessions, false)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := root.Remove(name + ".lock"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncRootDir(root, ".")
}

// locate returns the location holding this session. Reads and appends both go
// to whatever answers, which is why nothing has to be relocated first.
func (s *FilesystemSessionStore) locate(name string) (StoreLocation, error) {
	for _, location := range s.locations {
		root, err := openStoreRoot(location.Sessions, false)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return StoreLocation{}, err
		}
		_, statErr := root.Stat(name)
		closeErr := root.Close()
		if closeErr != nil {
			return StoreLocation{}, closeErr
		}
		if statErr == nil {
			return location, nil
		}
		if !errors.Is(statErr, fs.ErrNotExist) {
			return StoreLocation{}, statErr
		}
	}
	return StoreLocation{}, fmt.Errorf("%w: %s", app.ErrSessionNotFound, strings.TrimSuffix(name, sessionLogSuffix))
}

func listSessionLogs(dir string) ([]string, error) {
	root, err := openStoreRoot(dir, false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), sessionLogSuffix) {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

func readSessionLog(location StoreLocation, name string, id app.SessionID) (app.StoredSession, error) {
	root, err := openStoreRoot(location.Sessions, false)
	if errors.Is(err, fs.ErrNotExist) {
		return app.StoredSession{}, fmt.Errorf("%w: %s", app.ErrSessionNotFound, id)
	}
	if err != nil {
		return app.StoredSession{}, err
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		return app.StoredSession{}, fmt.Errorf("%w: %s", app.ErrSessionNotFound, id)
	}
	if err != nil {
		return app.StoredSession{}, err
	}
	defer func() { _ = file.Close() }()
	return decodeSessionLog(file, id, app.SessionMetadata{
		CodecVersion: app.SessionCodecVersion,
		ID:           id,
		Subject:      location.Subject,
		Project:      location.Project,
	})
}

type sessionLine struct {
	Version  uint64               `json:"version"`
	Metadata *app.SessionMetadata `json:"metadata,omitempty"`
	Events   []app.StoredEvent    `json:"events,omitempty"`
}

// decodeSessionLog reads every log shape sdd has released, dispatching per
// line rather than classifying the file: a line carrying a top-level "version"
// is the current envelope, and anything else is a pre-0.16 engine event whose
// raw bytes become one workflow event so the engine replays it unchanged. Such
// a log has no metadata of its own, so what the events imply is folded into the
// fallback identity of the location it was found in.
//
// Decoding is lenient about unknown fields in both directions. That is what
// lets one binary read a log another version wrote — including a newer one —
// and it is why no registry of retired fields is needed.
func decodeSessionLog(reader io.Reader, id app.SessionID, fallback app.SessionMetadata) (app.StoredSession, error) {
	stored := app.StoredSession{Metadata: fallback}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSessionLine)
	lineNumber := uint64(0)
	for scanner.Scan() {
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		lineNumber++

		var envelope struct {
			Version *uint64 `json:"version"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return app.StoredSession{}, fmt.Errorf("sdd: decoding session %s line %d: %w", id, lineNumber, err)
		}
		if envelope.Version == nil {
			event, err := decodeLegacyEvent(raw, id, lineNumber)
			if err != nil {
				return app.StoredSession{}, err
			}
			foldLegacyMetadata(&stored.Metadata, event)
			stored.Events = append(stored.Events, app.StoredEvent{
				CodecVersion: app.SessionCodecVersion,
				Code:         app.WorkflowEventCode,
				Payload:      bytes.Clone(raw),
			})
			stored.Version = lineNumber
			continue
		}

		var line sessionLine
		if err := json.Unmarshal(raw, &line); err != nil {
			return app.StoredSession{}, fmt.Errorf("sdd: decoding session %s line %d: %w", id, lineNumber, err)
		}
		if line.Version != lineNumber {
			return app.StoredSession{}, fmt.Errorf(
				"sdd: session %s has non-sequential version %d at line %d", id, line.Version, lineNumber,
			)
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
	if lineNumber == 0 {
		return app.StoredSession{}, fmt.Errorf("sdd: session %s is empty", id)
	}
	if stored.Metadata.ID == "" {
		stored.Metadata.ID = id
	}
	if stored.Metadata.ID != id {
		return app.StoredSession{}, fmt.Errorf(
			"sdd: session log %s declares identity %s", id, stored.Metadata.ID,
		)
	}
	return stored, nil
}

func decodeLegacyEvent(raw []byte, id app.SessionID, lineNumber uint64) (engine.Event, error) {
	var event engine.Event
	if err := json.Unmarshal(raw, &event); err != nil {
		return engine.Event{}, fmt.Errorf("sdd: decoding session %s line %d: %w", id, lineNumber, err)
	}
	if event.Event == "" {
		return engine.Event{}, fmt.Errorf("sdd: session %s line %d is not a session event", id, lineNumber)
	}
	return event, nil
}

// foldLegacyMetadata recovers from an event what the pre-0.16 log had no
// metadata line to state.
func foldLegacyMetadata(metadata *app.SessionMetadata, event engine.Event) {
	if event.TS.After(metadata.UpdatedAt) {
		metadata.UpdatedAt = event.TS
	}
	switch event.Event {
	case engine.EventSessionMeta:
		if participant, ok := event.Data["participant"].(string); ok && participant != "" {
			metadata.Participant = participant
		}
	case engine.EventLabeled:
		if label, ok := event.Data["label"].(string); ok {
			metadata.Label = label
		}
	}
}

func writeSessionLine(file *os.File, line sessionLine) error {
	encoded, err := json.Marshal(line)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func sessionLogName(id app.SessionID) (string, error) {
	if id == "" || !fs.ValidPath(string(id)) || strings.ContainsAny(string(id), `/\`) || id == "." || id == ".." {
		return "", fmt.Errorf("sdd: invalid session ID %q", id)
	}
	return string(id) + sessionLogSuffix, nil
}

// lockSession serializes appends to one log so the version CAS is atomic
// across processes.
func lockSession(dir, name string) (*flock.Flock, error) {
	lock := flock.New(filepath.Join(dir, name+".lock"))
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	return lock, nil
}

func unlock(lock *flock.Flock) {
	if lock != nil {
		_ = lock.Unlock()
	}
}
