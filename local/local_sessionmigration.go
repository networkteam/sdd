package local

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	app "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/engine"
)

// FilesystemLegacySessionMigrator converts the event-only session logs used
// before v0.16 into the current filesystem session envelope. Migration is an
// explicit maintenance operation; FilesystemSessionStore never invokes it
// while serving runtime reads.
type FilesystemLegacySessionMigrator struct {
	sessions *FilesystemSessionStore
	blobs    *FilesystemStagedBlobStore
	subject  string
	project  app.ProjectID
}

func NewFilesystemLegacySessionMigrator(sessionsDir, blobsDir, subject string, project app.ProjectID) (*FilesystemLegacySessionMigrator, error) {
	if sessionsDir == "" || blobsDir == "" || subject == "" || project == "" {
		return nil, fmt.Errorf("sdd: migration session directory, blob directory, subject, and project are required")
	}
	sessions := &FilesystemSessionStore{dir: sessionsDir}
	blobs := &FilesystemStagedBlobStore{dir: blobsDir}
	return &FilesystemLegacySessionMigrator{sessions: sessions, blobs: blobs, subject: subject, project: project}, nil
}

// ListLegacySessions returns absolute paths for every non-current JSONL record.
// Unreadable legacy records are included so an accepted migration reports the
// exact affected path instead of hiding them.
func (m *FilesystemLegacySessionMigrator) ListLegacySessions(_ context.Context) ([]string, error) {
	m.sessions.mu.Lock()
	defer m.sessions.mu.Unlock()

	entries, err := os.ReadDir(m.sessions.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(m.sessions.dir, entry.Name())
		format, err := classifySessionFile(path)
		if err != nil {
			return nil, err
		}
		if format == sessionFormatLegacy {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// MigrateLegacySession replaces one legacy log atomically. A current record is
// an idempotent no-op. Any staged blobs created before replacement are removed
// again if conversion fails, leaving the legacy log and staging directory
// untouched for inspection and retry.
func (m *FilesystemLegacySessionMigrator) MigrateLegacySession(ctx context.Context, path string) (err error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absDir, err := filepath.Abs(m.sessions.dir)
	if err != nil {
		return err
	}
	if filepath.Dir(absPath) != absDir || !strings.HasSuffix(filepath.Base(absPath), ".jsonl") {
		return fmt.Errorf("sdd: legacy session path is outside the session directory")
	}
	id := app.SessionID(strings.TrimSuffix(filepath.Base(absPath), ".jsonl"))
	filename, err := m.sessions.filename(id)
	if err != nil {
		return err
	}

	m.sessions.mu.Lock()
	defer m.sessions.mu.Unlock()
	lock, err := m.sessions.lock(id)
	if err != nil {
		return err
	}
	defer unlockFile(lock)

	format, err := classifySessionFile(filename)
	if err != nil {
		return err
	}
	if format == sessionFormatCurrent {
		return nil
	}

	legacy, err := readLegacyEvents(filename, id)
	if err != nil {
		return err
	}
	metadata := legacyMetadata(id, m.subject, m.project, legacy.events)
	owner := app.BlobOwner{Subject: m.subject, Session: id}
	createdBlobIDs := []string{}
	replaced := false
	defer func() {
		if err == nil || replaced {
			return
		}
		for _, blobID := range createdBlobIDs {
			_ = m.blobs.remove(owner, blobID)
		}
	}()

	storedEvents := make([]app.StoredEvent, 0, len(legacy.events))
	for _, payload := range legacy.payloads {
		storedEvents = append(storedEvents, app.StoredEvent{
			CodecVersion: app.SessionCodecVersion,
			Code:         app.WorkflowEventCode,
			Payload:      payload,
		})
	}

	stagingDir := filepath.Join(m.sessions.dir, string(id)+"-staging")
	staged, readErr := os.ReadDir(stagingDir)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	for _, entry := range staged {
		if entry.IsDir() {
			return fmt.Errorf("legacy staging entry %s is a directory", filepath.Join(stagingDir, entry.Name()))
		}
		file, openErr := os.Open(filepath.Join(stagingDir, entry.Name()))
		if openErr != nil {
			return openErr
		}
		blob, stageErr := m.blobs.Stage(ctx, owner, entry.Name(), file)
		closeErr := file.Close()
		if stageErr != nil {
			return stageErr
		}
		createdBlobIDs = append(createdBlobIDs, blob.ID)
		if closeErr != nil {
			return closeErr
		}
		payload, marshalErr := json.Marshal(map[string]string{"handle": entry.Name(), "blob_id": blob.ID})
		if marshalErr != nil {
			return marshalErr
		}
		storedEvents = append(storedEvents, app.StoredEvent{
			CodecVersion: app.SessionCodecVersion,
			Code:         "workflow_staged_blob",
			Payload:      payload,
		})
	}

	temp, err := os.CreateTemp(m.sessions.dir, ".sdd-legacy-session-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if err := appendSessionLine(temp, sessionLine{Version: 1, Metadata: &metadata, Events: storedEvents}); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, filename); err != nil {
		return err
	}
	replaced = true
	return syncDirectory(m.sessions.dir)
}

type legacyEventLog struct {
	events   []engine.Event
	payloads []json.RawMessage
}

func readLegacyEvents(filename string, id app.SessionID) (legacyEventLog, error) {
	file, err := os.Open(filename)
	if err != nil {
		return legacyEventLog{}, err
	}
	var result legacyEventLog
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var event engine.Event
		if err := json.Unmarshal(raw, &event); err != nil {
			_ = file.Close()
			return legacyEventLog{}, fmt.Errorf("session log line %d: %w", line, err)
		}
		result.events = append(result.events, event)
		result.payloads = append(result.payloads, append(json.RawMessage(nil), raw...))
	}
	readErr := scanner.Err()
	closeErr := file.Close()
	if readErr != nil {
		return legacyEventLog{}, readErr
	}
	if closeErr != nil {
		return legacyEventLog{}, closeErr
	}
	if len(result.events) == 0 {
		return legacyEventLog{}, fmt.Errorf("legacy session is empty")
	}
	for _, event := range result.events {
		if event.V != engine.LogVersion {
			return legacyEventLog{}, fmt.Errorf("legacy event seq %d has unsupported version %d", event.Seq, event.V)
		}
		if event.Session != string(id) {
			return legacyEventLog{}, fmt.Errorf("legacy event seq %d belongs to session %q", event.Seq, event.Session)
		}
		if event.Event == "" {
			return legacyEventLog{}, fmt.Errorf("legacy event seq %d has no event type", event.Seq)
		}
	}
	return result, nil
}

func legacyMetadata(id app.SessionID, subject string, project app.ProjectID, events []engine.Event) app.SessionMetadata {
	metadata := app.SessionMetadata{
		CodecVersion: app.SessionCodecVersion,
		ID:           id,
		Subject:      subject,
		Project:      project,
	}
	for _, event := range events {
		if event.TS.After(metadata.UpdatedAt) {
			metadata.UpdatedAt = event.TS
		}
		switch event.Event {
		case engine.EventSessionMeta:
			if participant, ok := event.Data["participant"].(string); ok && participant != "" {
				metadata.Participant = participant
			}
		case engine.EventLabeled:
			metadata.Label, _ = event.Data["label"].(string)
		}
	}
	return metadata
}
