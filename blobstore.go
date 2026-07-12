package sdd

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
// retention. Sweeping is deliberately composition-side operations work.
type StagedBlobStore interface {
	Stage(context.Context, BlobOwner, string, io.Reader) (StagedBlob, error)
	Stat(context.Context, BlobOwner, string) (StagedBlob, error)
	Open(context.Context, BlobOwner, string) (io.ReadCloser, error)
	Retain(context.Context, BlobOwner, string, []string) error
	Release(context.Context, BlobOwner, string) error
}
