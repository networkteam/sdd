package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
)

// GraphStore is the canonical graph authority: snapshot reads, atomic
// mutation, reconciliation, and canonical attachment bytes.
type GraphStore interface {
	Current(context.Context) (*Snapshot, error)
	Apply(context.Context, string, MutationBatch, StagedBlobReader) (ApplyResult, error)
	Reconcile(context.Context, string, string) (ApplyResult, error)
	ReadAttachmentPage(context.Context, string, string, int64, int) (AttachmentPage, error)
}

type MutationBatch struct {
	ID          string
	Digest      string
	Changes     []DocumentChange
	Attachments []AttachmentMaterialization
	Message     string
	Author      Author
}

// DocumentChange is storage-neutral. CanonicalBytes are rendered once by SDD;
// Document is present when the logical artifact has structured entry form.
type DocumentChange struct {
	LogicalPath    string
	Document       *EntryDocument
	CanonicalBytes []byte
	Delete         bool
}

type Author struct {
	Name  string
	Email string
}

type ApplyState string

const (
	MutationNotApplied ApplyState = "not_applied"
	MutationApplied    ApplyState = "applied"
	MutationUnknown    ApplyState = "unknown"
)

type ApplyResult struct {
	State    ApplyState
	Revision string
}

type AppliedMutation struct {
	Project  ProjectID
	BatchID  string
	Revision string
	Batch    MutationBatch
}

// MutationFinalizer is a named, idempotent post-apply effect. It cannot
// redefine or roll back the canonical MutationBatch.
type MutationFinalizer interface {
	Name() string
	Finalize(context.Context, AppliedMutation) error
}

type BlobDigest struct {
	Algorithm string
	Value     string
}

type AttachmentMaterialization struct {
	BlobID      string
	Digest      BlobDigest
	Size        int64
	SourceName  string
	LogicalPath string
}

type AttachmentPage struct {
	Filename   string
	Content    []byte
	Offset     int64
	NextOffset int64
	TotalSize  int64
	More       bool
	Digest     BlobDigest
}

// StagedBlobReader limits Apply to the blobs named by its prepared batch.
type StagedBlobReader interface {
	Open(context.Context, string) (io.ReadCloser, error)
}

// MutationBatchDigest returns the SDD-owned digest over a storage-neutral
// batch. The Digest field itself is excluded.
func MutationBatchDigest(batch MutationBatch) (string, error) {
	batch.Digest = ""
	encoded, err := json.Marshal(batch)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
