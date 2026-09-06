package application

import (
	"context"
	"errors"
	"fmt"
)

// AttachmentPageReader is the read portion of GraphStore used by indexing.
type AttachmentPageReader interface {
	ReadAttachmentPage(context.Context, string, string, int64, int) (AttachmentPage, error)
}

// SnapshotReadQuery separates exact job input from causal search freshness.
// ExactRevision selects precisely that revision. IncludesRevision selects a
// branch revision containing the write, which may be newer. They are exclusive.
type SnapshotReadQuery struct {
	Branch           string
	ExactRevision    string
	IncludesRevision string
}

// AcquiredSnapshot reuses the canonical snapshot and attachment paging types.
// Attachments must remain fixed at Snapshot.Revision until Release. Release
// is mandatory; the acquirer may share retained objects across many leases.
type AcquiredSnapshot struct {
	Snapshot    *Snapshot
	Attachments AttachmentPageReader
	Release     func() error
}

// SnapshotReader is an optional GraphStore capability for pinned reads. Hosts
// retain exact revisions independently of lease lifetime for durable jobs.
// IncludesRevision is a causal guarantee, never lexical revision comparison.
// Readers must honor Branch or reject it; an empty branch selects their current
// authority. Target-scoped readers must validate nonempty branch requests.
type SnapshotReader interface {
	AcquireSnapshot(context.Context, SnapshotReadQuery) (*AcquiredSnapshot, error)
}

func validateAcquiredSnapshot(source *AcquiredSnapshot, project ProjectID, exact string) error {
	if source == nil || source.Snapshot == nil || source.Attachments == nil || source.Release == nil {
		return fmt.Errorf("sdd: incomplete acquired snapshot")
	}
	if source.Snapshot.Project() != project || (exact != "" && source.Snapshot.Revision() != exact) {
		return fmt.Errorf("sdd: acquired snapshot does not match requested source")
	}
	return nil
}

func acquireReadSnapshot(ctx context.Context, graph GraphStore, project ProjectID, q SnapshotReadQuery) (*AcquiredSnapshot, error) {
	if q.ExactRevision != "" && q.IncludesRevision != "" {
		return nil, fmt.Errorf("sdd: exact and including revisions are exclusive")
	}
	reader, ok := graph.(SnapshotReader)
	if !ok {
		return nil, fmt.Errorf("sdd: graph store does not support pinned snapshot reads")
	}
	source, err := reader.AcquireSnapshot(ctx, q)
	if err == nil {
		err = validateAcquiredSnapshot(source, project, q.ExactRevision)
	}
	if err != nil {
		if source != nil && source.Release != nil {
			err = errors.Join(err, source.Release())
		}
		return nil, err
	}
	return source, nil
}

type pinnedGraphStore struct {
	GraphStore
	source *AcquiredSnapshot
}

func (s pinnedGraphStore) Current(context.Context) (*Snapshot, error) { return s.source.Snapshot, nil }
func (s pinnedGraphStore) ReadAttachmentPage(ctx context.Context, entry, name string, offset int64, limit int) (AttachmentPage, error) {
	return s.source.Attachments.ReadAttachmentPage(ctx, entry, name, offset, limit)
}

func acquireSnapshotForSearch(ctx context.Context, runtime *ProjectRuntime, branch, includes string) (*readSnapshotSelection, error) {
	selected, err := acquireSnapshotForReadBranch(ctx, runtime, branch)
	if err != nil {
		return nil, err
	}
	if _, ok := selected.store.(SnapshotReader); !ok {
		if includes == "" {
			return selected, nil
		}
		err := fmt.Errorf("sdd: source cannot establish read-your-writes freshness")
		selected.releaseInto(&err)
		return nil, err
	}
	source, err := acquireReadSnapshot(ctx, selected.store, runtime.Project().ID, SnapshotReadQuery{Branch: branch, IncludesRevision: includes})
	if err != nil {
		selected.releaseInto(&err)
		return nil, err
	}
	return &readSnapshotSelection{snapshot: source.Snapshot, store: pinnedGraphStore{GraphStore: selected.store, source: source}, branch: branch, release: func() error {
		err := source.Release()
		selected.releaseInto(&err)
		return err
	}}, nil
}
