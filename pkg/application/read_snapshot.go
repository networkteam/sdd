package application

import (
	"context"
	"errors"
	"fmt"
)

// AttachmentPageReader reads attachment bytes from one acquired source.
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
	// Config is immutable committed configuration from this source revision.
	// Nil explicitly uses runtime configuration; configuration read failures
	// must be returned by the adapter, never converted to nil.
	Config  *ProjectConfig
	Release func() error
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
			err = errors.Join(err, snapshotRelease(source.Release))
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
	return acquireSnapshotSelection(ctx, runtime, SnapshotReadQuery{Branch: branch, IncludesRevision: includes})
}

func acquireSnapshotSelection(ctx context.Context, runtime *ProjectRuntime, q SnapshotReadQuery) (*readSnapshotSelection, error) {
	if _, ok := runtime.options.Graph.(SnapshotReader); !ok {
		if q.Branch != "" || q.ExactRevision != "" || q.IncludesRevision != "" {
			return nil, markTargetAcquisitionError(MutationTarget{Project: runtime.Project().ID, Branch: q.Branch}, fmt.Errorf("sdd: graph store does not support selected snapshot reads"))
		}
		snapshot, err := runtime.options.Graph.Current(ctx)
		if err != nil {
			return nil, err
		}
		return &readSnapshotSelection{snapshot: snapshot, store: runtime.options.Graph, runtime: runtime}, nil
	}
	source, err := acquireReadSnapshot(ctx, runtime.options.Graph, runtime.Project().ID, q)
	if err != nil {
		return nil, markTargetAcquisitionError(MutationTarget{Project: runtime.Project().ID, Branch: q.Branch}, err)
	}
	return &readSnapshotSelection{
		snapshot: source.Snapshot, store: pinnedGraphStore{GraphStore: runtime.options.Graph, source: source},
		runtime: runtime.withReadConfig(source.Config), branch: q.Branch, release: source.Release,
	}, nil
}

func (r *ProjectRuntime) withReadConfig(config *ProjectConfig) *ProjectRuntime {
	if config == nil {
		return r
	}
	clone := *r
	clone.options.Language = config.Language
	clone.options.Dependencies = append([]string(nil), config.Dependencies...)
	return &clone
}

func readMaterializedSnapshot(ctx context.Context, runtime *ProjectRuntime, branch string) (snapshot *Snapshot, effective *ProjectRuntime, err error) {
	selected, err := acquireSnapshotForReadBranch(ctx, runtime, branch)
	if err != nil {
		return nil, nil, err
	}
	defer selected.releaseInto(&err)
	return selected.snapshot, selected.runtime, nil
}

type snapshotReleaseError struct{ cause error }

func (e *snapshotReleaseError) Error() string { return "releasing snapshot: " + e.cause.Error() }
func (e *snapshotReleaseError) Unwrap() error { return e.cause }
func snapshotRelease(release func() error) error {
	if err := release(); err != nil {
		return &snapshotReleaseError{cause: err}
	}
	return nil
}
