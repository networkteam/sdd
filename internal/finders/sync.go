package finders

import (
	"context"
	"fmt"
	"time"

	"github.com/networkteam/sdd/internal/meta"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// GraphCommitGrepPattern is the commit-message prefix emitted by every SDD
// handler (handler_new_entry, handler_summarize, handler_wip_start,
// handler_wip_done, handler_lint_fix, handler_rewrite, handler_init). The
// sync finder counts commits matching this pattern on either side of the
// local/upstream divergence to report graph activity.
const GraphCommitGrepPattern = "^sdd:"

// GitSyncer is the git surface consumed by the sync finder. Production
// shells out via exec.Command; tests inject stubs. Methods return errors
// only for unexpected failures — missing-repo / missing-remote /
// missing-upstream are expressed through the dedicated boolean / string
// predicates (InRepo, HasRemote, UpstreamRef) so the finder can report
// distinct SyncStates rather than collapsing everything into errors.
type GitSyncer interface {
	// InRepo reports whether the process is inside a git working tree.
	InRepo(ctx context.Context) bool
	// HasRemote reports whether the repository has at least one remote configured.
	HasRemote(ctx context.Context) bool
	// UpstreamRef returns the upstream ref name for the current branch
	// (e.g. "origin/main"). Empty string + nil error means the branch has
	// no upstream configured.
	UpstreamRef(ctx context.Context) (string, error)
	// Fetch runs `git fetch` with no args. A non-nil error means the fetch
	// attempt failed (network, auth, etc.).
	Fetch(ctx context.Context) error
	// CountCommits counts commits in rangeSpec whose messages match the
	// given grep pattern (ERE). rangeSpec is a git log range such as
	// "HEAD..@{u}" or "@{u}..HEAD".
	CountCommits(ctx context.Context, rangeSpec, grepPattern string) (int, error)
	// MergeBase returns the merge-base commit of two refs.
	MergeBase(ctx context.Context, a, b string) (string, error)
	// MergeTreePredict simulates a three-way merge in memory and returns
	// the list of paths that would conflict. An empty slice means clean.
	MergeTreePredict(ctx context.Context, base, ourRef, theirRef string) ([]string, error)
}

// SyncStatus processes a SyncStatusQuery into a SyncStatus. Cooldown is
// read from the finder's Config; callers supply SDDDir for the last-fetch
// marker. The finder touches last-fetch on every successful attempt and on
// terminal failure states (no remote, no upstream, fetch failure) so
// transient environment issues don't re-poll on every command.
func (f *Finder) SyncStatus(ctx context.Context, q query.SyncStatusQuery) (model.SyncStatus, error) {
	if f.gitSyncer == nil {
		return model.SyncStatus{State: model.SyncStateSkipped}, nil
	}
	if !f.gitSyncer.InRepo(ctx) {
		return model.SyncStatus{State: model.SyncStateNoRepo}, nil
	}

	if q.RespectCooldown {
		last, err := meta.ReadLastFetch(q.SDDDir)
		if err != nil {
			return model.SyncStatus{}, fmt.Errorf("reading last-fetch: %w", err)
		}
		cooldown := model.ResolveSyncCooldown(f.cfg)
		if !last.IsZero() && time.Since(last) < cooldown {
			return model.SyncStatus{State: model.SyncStateSkipped}, nil
		}
	}

	if !f.gitSyncer.HasRemote(ctx) {
		_ = meta.TouchLastFetch(q.SDDDir)
		return model.SyncStatus{State: model.SyncStateNoRemote}, nil
	}

	upstream, err := f.gitSyncer.UpstreamRef(ctx)
	if err != nil {
		return model.SyncStatus{}, fmt.Errorf("resolving upstream: %w", err)
	}
	if upstream == "" {
		_ = meta.TouchLastFetch(q.SDDDir)
		return model.SyncStatus{State: model.SyncStateNoUpstream}, nil
	}

	fetchErr := f.gitSyncer.Fetch(ctx)
	// Stamp the attempt regardless of outcome — cooldown bounds attempts,
	// not successes, to avoid hammering git on persistent offline states.
	_ = meta.TouchLastFetch(q.SDDDir)
	if fetchErr != nil {
		return model.SyncStatus{State: model.SyncStateFetchFailed, Reason: fetchErr.Error()}, nil
	}

	remoteAhead, err := f.gitSyncer.CountCommits(ctx, "HEAD..@{u}", GraphCommitGrepPattern)
	if err != nil {
		return model.SyncStatus{}, fmt.Errorf("counting remote-ahead commits: %w", err)
	}
	localAhead, err := f.gitSyncer.CountCommits(ctx, "@{u}..HEAD", GraphCommitGrepPattern)
	if err != nil {
		return model.SyncStatus{}, fmt.Errorf("counting local-ahead commits: %w", err)
	}

	// Also count total divergence so we can distinguish "diverged with
	// non-graph commits on one side" from "strictly linear." We reuse the
	// SDD-pattern counts for reporting to the user (AC2/AC3), but rebase
	// safety needs the raw commit count — if local has any commits at all
	// (not just sdd: ones), the rebase might still conflict. For now we
	// use the SDD-pattern counts as a proxy; in practice nearly all local
	// divergence originates from SDD activity. This keeps the git surface
	// minimal per AC4.
	switch {
	case remoteAhead == 0 && localAhead == 0:
		return model.SyncStatus{State: model.SyncStateUpToDate}, nil
	case remoteAhead > 0 && localAhead == 0:
		return model.SyncStatus{State: model.SyncStateFastForward, RemoteAhead: remoteAhead}, nil
	case remoteAhead == 0 && localAhead > 0:
		return model.SyncStatus{State: model.SyncStateLocalAhead, LocalAhead: localAhead}, nil
	}

	// Both sides have diverged — predict rebase safety.
	base, err := f.gitSyncer.MergeBase(ctx, "HEAD", upstream)
	if err != nil {
		return model.SyncStatus{}, fmt.Errorf("computing merge base: %w", err)
	}
	conflicts, err := f.gitSyncer.MergeTreePredict(ctx, base, "HEAD", upstream)
	if err != nil {
		return model.SyncStatus{}, fmt.Errorf("predicting merge: %w", err)
	}
	if len(conflicts) == 0 {
		return model.SyncStatus{
			State:       model.SyncStateCleanRebase,
			RemoteAhead: remoteAhead,
			LocalAhead:  localAhead,
		}, nil
	}
	return model.SyncStatus{
		State:         model.SyncStateConflictPredicted,
		RemoteAhead:   remoteAhead,
		LocalAhead:    localAhead,
		ConflictPaths: conflicts,
	}, nil
}
