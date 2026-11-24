package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/application/types"
)

// Changes reports what changed in the local graph between two revisions. It
// compares the two snapshots as given and does not check that From precedes
// To on one history; swapped or unrelated revisions yield a meaningless diff.
func (a *Application) Changes(ctx context.Context, identity RequestIdentity, project ProjectID, request ChangesRequest) (result ChangesResult, err error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return ChangesResult{}, err
	}
	if request.From == "" {
		return ChangesResult{}, &ApplicationError{Code: ErrorInvalidArgument, Message: "sdd: changes require a from revision"}
	}
	from, err := acquireRevision(ctx, runtime, request.Branch, request.From)
	if err != nil {
		return ChangesResult{}, err
	}
	defer from.releaseInto(&err)
	to, err := acquireRevision(ctx, runtime, request.Branch, request.To)
	if err != nil {
		return ChangesResult{}, err
	}
	defer to.releaseInto(&err)
	diff := model.DiffGraphs(from.snapshot.graph, to.snapshot.graph)
	return ChangesResult{
		Project: runtime.options.Project, From: from.snapshot.Revision(), To: to.snapshot.Revision(),
		Added: diff.Added, StatusChanged: diff.StatusChanged, SummaryChanged: diff.SummaryChanged,
	}, nil
}

// ListEntries pages a filtered, time-ordered listing of the local graph and
// the selected dependency repos.
func (a *Application) ListEntries(ctx context.Context, identity RequestIdentity, project ProjectID, request ListEntriesRequest) (result ListEntriesResult, err error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return ListEntriesResult{}, err
	}
	if request.Offset < 0 || request.Limit < 0 {
		return ListEntriesResult{}, &ApplicationError{Code: ErrorInvalidArgument, Message: "sdd: offset and limit cannot be negative"}
	}
	filter, err := listFilter(request)
	if err != nil {
		return ListEntriesResult{}, err
	}
	selected, err := acquireRevision(ctx, runtime, request.Branch, request.Revision)
	if err != nil {
		return ListEntriesResult{}, err
	}
	defer selected.releaseInto(&err)
	runtime = selected.runtime
	members := []model.ListMember{{Graph: selected.snapshot.graph}}
	repos, reach, err := a.selectedDependencies(ctx, identity, runtime, request.Repos, request.AllRepos)
	if err != nil {
		return ListEntriesResult{}, err
	}
	for _, repoID := range repos {
		dependency, err := reach.member(repoID)
		if err != nil {
			return ListEntriesResult{}, err
		}
		member, err := reach.view(dependency, "")
		if err != nil {
			return ListEntriesResult{}, err
		}
		members = append(members, model.ListMember{Graph: member.snapshot.graph, Repo: repoID})
	}
	list := model.ListEntries(members, filter, request.NewestFirst)
	page := list.Page(request.Offset, request.Limit)
	entries := make([]types.GraphEntry, len(page))
	for i, item := range page {
		entries[i] = item.Owner.StructuredEntry(item.Entry, item.Repo, false)
	}
	return ListEntriesResult{
		Project: runtime.options.Project, Revision: selected.snapshot.Revision(),
		Total: len(list.Entries), Entries: entries, Facets: list.Facets,
	}, nil
}

// acquireRevision acquires the branch's current revision, or exactly revision
// when given. Cancellation and failures already carrying an ApplicationError
// pass through; any other failure to acquire a named revision returns
// ErrorRevisionUnavailable.
func acquireRevision(ctx context.Context, runtime *ProjectRuntime, branch, revision string) (*readSnapshotSelection, error) {
	selected, err := acquireSnapshotSelection(ctx, runtime, SnapshotReadQuery{Branch: branch, ExactRevision: revision})
	if err == nil || revision == "" {
		return selected, err
	}
	var appErr *ApplicationError
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &appErr) {
		return nil, err
	}
	return nil, &ApplicationError{Code: ErrorRevisionUnavailable, Message: fmt.Sprintf("sdd: revision %s is unavailable", revision), Revision: revision, Cause: err}
}

func listFilter(request ListEntriesRequest) (model.EntryListFilter, error) {
	filter := model.EntryListFilter{IDs: request.IDs, IncludeEmbedded: request.IncludeEmbedded, Participants: request.Participants, CreatedAfter: request.CreatedAfter, CreatedBefore: request.CreatedBefore}
	for _, value := range request.Types {
		t, ok := model.ParseTypeFilter(value)
		if !ok {
			return filter, &ApplicationError{Code: ErrorInvalidArgument, Message: fmt.Sprintf("sdd: unknown type %q (want s, d, signal, or decision)", value)}
		}
		filter.Types = append(filter.Types, t)
	}
	for _, value := range request.Layers {
		l, ok := model.ParseLayerFilter(value)
		if !ok {
			return filter, &ApplicationError{Code: ErrorInvalidArgument, Message: fmt.Sprintf("sdd: unknown layer %q (want stg, cpt, tac, ops, prc, or the full name)", value)}
		}
		filter.Layers = append(filter.Layers, l)
	}
	for _, value := range request.Kinds {
		k := model.Kind(value)
		if !model.IsKnownKind(k) {
			return filter, &ApplicationError{Code: ErrorInvalidArgument, Message: fmt.Sprintf("sdd: unknown kind %q", value)}
		}
		filter.Kinds = append(filter.Kinds, k)
	}
	statuses, err := parseStatusFilter(request.Statuses)
	if err != nil {
		return filter, err
	}
	filter.Statuses = statuses
	terms, err := model.CompileSearchTerms(request.Terms)
	if err != nil {
		return filter, &ApplicationError{Code: ErrorInvalidArgument, Message: "sdd: " + err.Error(), Cause: err}
	}
	filter.Terms = terms
	return filter, nil
}
