package application

import (
	"context"
	"io"
	"strings"
	"time"
)

// SessionRef addresses one session inside a subject's namespace. Staged blobs
// are scoped to a session, so this is what names their area — there is no owner
// entity, just the two fields that identify whose scaffolding this is.
type SessionRef struct {
	Subject string
	Session SessionID
}

// Compare orders refs by session, then subject — the order StagedSessions
// enumerates in, so a session-ID cursor from the session store lines up.
func (r SessionRef) Compare(other SessionRef) int {
	if r.Session != other.Session {
		return strings.Compare(string(r.Session), string(other.Session))
	}
	return strings.Compare(r.Subject, other.Subject)
}

// AtOrBefore reports whether r lies at or before cursor in enumeration order,
// which is what a paged StagedSessions skips. A cursor naming only a session
// covers every subject of that session.
func (r SessionRef) AtOrBefore(cursor SessionRef) bool {
	if cursor.Session == "" {
		return false
	}
	if r.Session != cursor.Session {
		return r.Session < cursor.Session
	}
	return cursor.Subject == "" || r.Subject <= cursor.Subject
}

// StagedSessionPage is one StagedSessions result. Next is the cursor to
// continue from, empty once the store is exhausted.
type StagedSessionPage struct {
	Sessions []SessionRef
	Next     SessionRef
}

type StagedBlob struct {
	ID        string
	Session   SessionRef
	Digest    BlobDigest
	Size      int64
	Filename  string
	CreatedAt time.Time
}

// StagedBlobStore owns immutable session-scoped scratch bytes and the
// retentions holding them. Nothing here is durable: a staged blob lives as long
// as its session does, and durability is earned only by a captured entry.
//
// StagedSessions and DeleteStaged put reclamation inside the published contract,
// so a sweep enumerates staging areas and drops the ones whose session is gone
// through this interface rather than through local-only code. StagedSessions
// pages in SessionRef order after the cursor (SessionRef.AtOrBefore), a zero
// limit meaning every area. DeleteStaged must be idempotent, and removes a
// session's blobs together with its retentions.
type StagedBlobStore interface {
	Stage(context.Context, SessionRef, string, io.Reader) (StagedBlob, error)
	Stat(context.Context, SessionRef, string) (StagedBlob, error)
	Open(context.Context, SessionRef, string) (io.ReadCloser, error)
	Retain(context.Context, SessionRef, string, []string) error
	Release(context.Context, SessionRef, string) error
	StagedSessions(ctx context.Context, after SessionRef, limit int) (StagedSessionPage, error)
	DeleteStaged(context.Context, SessionRef) error
}
