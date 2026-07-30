package application

import (
	"context"
	"io"
	"time"
)

type BlobOwner struct {
	Subject string
	Session SessionID
}

type StagedBlob struct {
	ID        string
	Owner     BlobOwner
	Digest    BlobDigest
	Size      int64
	Filename  string
	CreatedAt time.Time
}

// StagedBlobStore owns immutable session-scoped scratch bytes and durable
// retention.
//
// Owners and DeleteOwner put reclamation inside the published contract: a
// sweep enumerates owners, drops those whose session is gone, and reaches both
// through this interface rather than through local-only code. DeleteOwner must
// be idempotent, and removes an owner's blobs together with its retentions.
type StagedBlobStore interface {
	Stage(context.Context, BlobOwner, string, io.Reader) (StagedBlob, error)
	Stat(context.Context, BlobOwner, string) (StagedBlob, error)
	Open(context.Context, BlobOwner, string) (io.ReadCloser, error)
	Retain(context.Context, BlobOwner, string, []string) error
	Release(context.Context, BlobOwner, string) error
	Owners(context.Context) ([]BlobOwner, error)
	DeleteOwner(context.Context, BlobOwner) error
}
