package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	sdd "github.com/networkteam/sdd/application"
)

// The stores below are what an external composition owning its own storage has
// to write. They are here rather than in a test file because conforming to the
// ports is the thing this example demonstrates, and sddtest holds them to the
// same behaviour the local adapters meet.

// memorySessionStore keeps sessions in a map. Append is the only mutation and
// compares the expected version under the lock, which is the whole of the
// concurrency contract.
type memorySessionStore struct {
	mu       sync.Mutex
	sessions map[sdd.SessionID]sdd.StoredSession
}

func newMemorySessionStore() *memorySessionStore {
	return &memorySessionStore{sessions: map[sdd.SessionID]sdd.StoredSession{}}
}

func (s *memorySessionStore) Create(_ context.Context, metadata sdd.SessionMetadata) (sdd.StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[metadata.ID]; exists {
		return sdd.StoredSession{}, &sdd.ApplicationError{
			Code: sdd.ErrorSessionConflict, Message: "session already exists",
		}
	}
	stored := sdd.StoredSession{Metadata: metadata, Version: 1}
	s.sessions[metadata.ID] = stored
	return stored, nil
}

func (s *memorySessionStore) Load(_ context.Context, id sdd.SessionID) (sdd.StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, exists := s.sessions[id]
	if !exists {
		return sdd.StoredSession{}, fmt.Errorf("%w: %s", sdd.ErrSessionNotFound, id)
	}
	return cloneSession(stored), nil
}

func (s *memorySessionStore) List(_ context.Context, filter sdd.SessionFilter) ([]sdd.StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var listed []sdd.StoredSession
	for _, stored := range s.sessions {
		if filter.Subject != "" && stored.Metadata.Subject != filter.Subject {
			continue
		}
		if filter.Project != "" && stored.Metadata.Project != filter.Project {
			continue
		}
		listed = append(listed, cloneSession(stored))
	}
	slices.SortFunc(listed, func(a, b sdd.StoredSession) int {
		return cmpID(a.Metadata.ID, b.Metadata.ID)
	})
	return listed, nil
}

func (s *memorySessionStore) Append(
	_ context.Context,
	id sdd.SessionID,
	expectedVersion uint64,
	add sdd.SessionAppend,
) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, exists := s.sessions[id]
	if !exists {
		return 0, fmt.Errorf("%w: %s", sdd.ErrSessionNotFound, id)
	}
	if stored.Version != expectedVersion {
		return stored.Version, &sdd.ApplicationError{
			Code: sdd.ErrorSessionConflict, Message: "session version changed",
		}
	}
	if add.Metadata != nil {
		if add.Metadata.ID != id || add.Metadata.Subject != stored.Metadata.Subject ||
			add.Metadata.Project != stored.Metadata.Project {
			return stored.Version, &sdd.ApplicationError{
				Code: sdd.ErrorSessionOwnership, Message: "session identity and project are immutable",
			}
		}
		stored.Metadata = *add.Metadata
	}
	stored.Events = append(slices.Clip(stored.Events), add.Events...)
	stored.Version++
	s.sessions[id] = stored
	return stored.Version, nil
}

// Delete is idempotent: two sweeps derive the same target set, so removing what
// is already gone is success.
func (s *memorySessionStore) Delete(_ context.Context, id sdd.SessionID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

// cloneSession detaches the stored event slice so a caller cannot append into
// the store's own backing array.
func cloneSession(stored sdd.StoredSession) sdd.StoredSession {
	stored.Events = slices.Clone(stored.Events)
	return stored
}

func cmpID(a, b sdd.SessionID) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// memoryStagedBlobStore keeps one staging area per session. Retentions are
// recorded but never block deletion — the collection pass decides what is safe
// to remove, not the store.
type memoryStagedBlobStore struct {
	mu     sync.Mutex
	areas  map[sdd.SessionRef]*stagingArea
	now    func() time.Time
	staged int
}

type stagingArea struct {
	blobs      map[string]stagedBlob
	retentions map[string][]string
}

type stagedBlob struct {
	blob    sdd.StagedBlob
	content []byte
}

func newMemoryStagedBlobStore(now func() time.Time) *memoryStagedBlobStore {
	if now == nil {
		now = time.Now
	}
	return &memoryStagedBlobStore{areas: map[sdd.SessionRef]*stagingArea{}, now: now}
}

func (s *memoryStagedBlobStore) Stage(
	_ context.Context,
	session sdd.SessionRef,
	filename string,
	reader io.Reader,
) (sdd.StagedBlob, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return sdd.StagedBlob{}, err
	}
	sum := sha256.Sum256(content)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Identity is per stage, not per content: staging the same bytes twice is
	// two blobs, and each is addressed on its own.
	s.staged++
	blob := sdd.StagedBlob{
		ID:        fmt.Sprintf("blob-%d", s.staged),
		Session:   session,
		Digest:    sdd.BlobDigest{Algorithm: "sha256", Value: hex.EncodeToString(sum[:])},
		Size:      int64(len(content)),
		Filename:  filename,
		CreatedAt: s.now().UTC(),
	}
	area, exists := s.areas[session]
	if !exists {
		area = &stagingArea{blobs: map[string]stagedBlob{}, retentions: map[string][]string{}}
		s.areas[session] = area
	}
	area.blobs[blob.ID] = stagedBlob{blob: blob, content: content}
	return blob, nil
}

func (s *memoryStagedBlobStore) Stat(_ context.Context, session sdd.SessionRef, id string) (sdd.StagedBlob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	staged, err := s.lookup(session, id)
	if err != nil {
		return sdd.StagedBlob{}, err
	}
	return staged.blob, nil
}

func (s *memoryStagedBlobStore) Open(_ context.Context, session sdd.SessionRef, id string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	staged, err := s.lookup(session, id)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(staged.content)), nil
}

func (s *memoryStagedBlobStore) Retain(_ context.Context, session sdd.SessionRef, mutationID string, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	area, exists := s.areas[session]
	if !exists {
		return fmt.Errorf("staging area for %s is gone", session.Session)
	}
	area.retentions[mutationID] = slices.Clone(ids)
	return nil
}

// Release drops a retention. A staging area that is already gone stays gone —
// releasing must not recreate it.
func (s *memoryStagedBlobStore) Release(_ context.Context, session sdd.SessionRef, mutationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if area, exists := s.areas[session]; exists {
		delete(area.retentions, mutationID)
	}
	return nil
}

func (s *memoryStagedBlobStore) StagedSessions(context.Context) ([]sdd.SessionRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	refs := make([]sdd.SessionRef, 0, len(s.areas))
	for ref := range s.areas {
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, func(a, b sdd.SessionRef) int {
		if subjects := strings.Compare(a.Subject, b.Subject); subjects != 0 {
			return subjects
		}
		return cmpID(a.Session, b.Session)
	})
	return refs, nil
}

// DeleteStaged removes a session's blobs together with its retentions, and is
// idempotent for the same reason Delete is.
func (s *memoryStagedBlobStore) DeleteStaged(_ context.Context, session sdd.SessionRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.areas, session)
	return nil
}

func (s *memoryStagedBlobStore) lookup(session sdd.SessionRef, id string) (stagedBlob, error) {
	area, exists := s.areas[session]
	if !exists {
		return stagedBlob{}, fmt.Errorf("no staging area for session %s", session.Session)
	}
	staged, exists := area.blobs[id]
	if !exists {
		return stagedBlob{}, fmt.Errorf("staged blob %s not found", id)
	}
	return staged, nil
}
