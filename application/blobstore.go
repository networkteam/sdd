package application

import (
	"context"
	"io"
	"time"
)

// SessionRef addresses one session inside a subject's namespace. Staged blobs
// are scoped to a session, so this is what names their area — there is no owner
// entity, just the two fields that identify whose scaffolding this is.
type SessionRef struct {
	Subject string
	Session SessionID
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
// through this interface rather than through local-only code. DeleteStaged must
// be idempotent, and removes a session's blobs together with its retentions.
type StagedBlobStore interface {
	Stage(context.Context, SessionRef, string, io.Reader) (StagedBlob, error)
	Stat(context.Context, SessionRef, string) (StagedBlob, error)
	Open(context.Context, SessionRef, string) (io.ReadCloser, error)
	Retain(context.Context, SessionRef, string, []string) error
	Release(context.Context, SessionRef, string) error
	StagedSessions(context.Context) ([]SessionRef, error)
	DeleteStaged(context.Context, SessionRef) error
}
