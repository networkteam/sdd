package application

import (
	"context"
	"fmt"
	"iter"
	"sync/atomic"

	"github.com/networkteam/sdd/internal/query"
)

// SearchTarget is the fixed, authorized selection passed to PrepareSearch.
// Its read capabilities expire when Search returns; retain descriptors, not
// this value, for durable jobs. Source retention remains the consumer's duty.
type SearchTarget struct{ state *searchTargetState }

type searchTargetState struct {
	members []*searchTargetMember
	mode    SearchSyncMode
	closed  atomic.Bool
}

type searchTargetMember struct {
	runtime  *ProjectRuntime
	selected *readSnapshotSelection
	repoID   string
}

type SearchTargetProject struct {
	Project  ProjectID
	Revision string
}

func (t SearchTarget) Projects() []SearchTargetProject {
	if t.state == nil {
		return nil
	}
	projects := make([]SearchTargetProject, len(t.state.members))
	for i, member := range t.state.members {
		projects[i] = SearchTargetProject{Project: member.runtime.Project().ID, Revision: member.selected.snapshot.Revision()}
	}
	return projects
}

func (t SearchTarget) SyncMode() SearchSyncMode {
	if t.state == nil {
		return ""
	}
	return t.state.mode
}

// Entries lazily derives required versions, including attachments, and reads
// published presence. Errors terminate iteration and must be propagated by
// preparation callbacks. A Published hint may advance after it was yielded;
// SDD always reads publication again after preparation. Partial iteration does
// not narrow the target whose coverage SDD will check.
func (t SearchTarget) Entries(ctx context.Context) iter.Seq2[SearchEntryRequirement, error] {
	return func(yield func(SearchEntryRequirement, error) bool) {
		if t.state == nil || t.state.closed.Load() {
			yield(SearchEntryRequirement{}, fmt.Errorf("sdd: search target is no longer available"))
			return
		}
		for _, member := range t.state.members {
			source := &AcquiredSnapshot{Snapshot: member.selected.snapshot, Attachments: member.selected.store, Release: func() error { return nil }}
			for item, err := range member.runtime.DiscoverSearchEntries(ctx, DiscoverSearchEntriesQuery{Source: source}) {
				if !yield(item, err) || err != nil {
					return
				}
			}
		}
	}
}

// SearchCoverage reports SDD's post-preparation publication read for one fixed
// project snapshot. It contains no consumer queue or retry state.
type SearchCoverage struct {
	Project   ProjectID `json:"project"`
	Revision  string    `json:"revision"`
	Required  int       `json:"required"`
	Published int       `json:"published"`
	Complete  bool      `json:"complete"`
}

func (t SearchTarget) coverage(ctx context.Context) ([]SearchCoverage, error) {
	coverage := make([]SearchCoverage, len(t.state.members))
	for i, member := range t.state.members {
		coverage[i] = SearchCoverage{Project: member.runtime.Project().ID, Revision: member.selected.snapshot.Revision(), Complete: true}
	}
	for item, err := range t.Entries(ctx) {
		if err != nil {
			return nil, err
		}
		for i := range coverage {
			if coverage[i].Project != item.Entry.Version.Namespace.Project {
				continue
			}
			coverage[i].Required++
			if item.Published {
				coverage[i].Published++
			} else {
				coverage[i].Complete = false
			}
			break
		}
	}
	return coverage, nil
}

func (a *Application) prepareSearchTarget(ctx context.Context, target SearchTarget) error {
	if a.prepareSearch != nil {
		return a.prepareSearch(ctx, target)
	}
	for i, member := range target.state.members {
		if target.SyncMode() == SearchSyncNone || (i > 0 && target.SyncMode() == SearchSyncLocal) {
			continue
		}
		ns, err := member.runtime.indexNamespace()
		if err != nil {
			return err
		}
		if err := member.runtime.reconcileSearchSnapshot(ctx, member.selected.snapshot, member.selected.store, ns, nil, ReconcileSearchIndexCmd{}); err != nil {
			return err
		}
	}
	return nil
}

func supportsSearchCoverage(members []*searchTargetMember) bool {
	for _, member := range members {
		if _, ok := member.runtime.options.SearchIndex.(SearchIndexEntryStore); !ok {
			return false
		}
	}
	return true
}

// Text-only search has no embedding coverage to await or report.
func (a *Application) searchTarget(ctx context.Context, target SearchTarget, request query.SearchQuery) (*query.SearchResult, []SearchCoverage, error) {
	semantic := request.Phrase != ""
	var coverage []SearchCoverage
	if semantic {
		if a.prepareSearch != nil && !supportsSearchCoverage(target.state.members) {
			return nil, nil, fmt.Errorf("sdd: search preparation requires entry publication support for every selected project")
		}
		if a.prepareSearch != nil {
			for _, member := range target.state.members {
				if _, ok := member.selected.store.(pinnedGraphStore); !ok {
					return nil, nil, fmt.Errorf("sdd: search preparation requires pinned snapshot reads for every selected project")
				}
			}
		}
		if err := a.prepareSearchTarget(ctx, target); err != nil {
			return nil, nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if supportsSearchCoverage(target.state.members) {
			var err error
			coverage, err = target.coverage(ctx)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	var combined *query.SearchResult
	request.SyncMode = SearchSyncNone
	for _, member := range target.state.members {
		result, err := member.runtime.searchSnapshot(ctx, member.selected.snapshot, member.selected.store, request)
		if err != nil {
			return nil, nil, err
		}
		for i := range result.Entries {
			result.Entries[i].RepoID = member.repoID
		}
		if combined == nil {
			combined = result
		} else {
			combined.Entries = append(combined.Entries, result.Entries...)
		}
	}
	return combined, coverage, nil
}
