package finders

import (
	"context"
	"fmt"
	"sort"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
)

// MultiSearch is the cross-graph search read: it runs the query against
// the local finder and against every repo the query selects, merging all
// hits into one list by comparable score with remote hits repo-tagged.
// Pure read — cache freshness and per-repo index fill are the handler's
// job (Handler.PrepareCrossRepoSearch), run before this like the local
// lazy-fill precedes a plain Search.
//
// Members resolve internally: the member graph comes from the query
// graph's cross-graph assembly, the per-repo index opens read-only from
// the repo's cache, and every member shares the local finder's embedder —
// the single vector space that makes cosine scores comparable. A selected
// repo whose graph is unavailable is skipped with a warning (its absence
// is visible, not silent); a member graph that fails to load is an error.
//
// Embedded (binary-scoped) entries surface exactly once: the local search
// covers them, and member hits on embedded entries are dropped (remote
// indexes exclude them; the guard here also covers text mode, which greps
// the member graph directly).
func MultiSearch(ctx context.Context, local *SearchFinder, q query.SearchQuery) (*query.SearchResult, error) {
	res, err := local.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	merged := res.Entries

	repoIDs, err := repos.SelectRepoIDs(q.Repos, q.AllRepos)
	if err != nil {
		return nil, err
	}
	logger := slogutils.FromContext(ctx)
	for _, repoID := range repoIDs {
		member, err := searchMember(q, repoID, local)
		if err != nil {
			return nil, err
		}
		if member == nil {
			logger.Warn("connected repo unavailable for search; skipping", "repo", repoID)
			continue
		}
		mq := q
		mq.Graph = member.graph
		mq.Repos, mq.AllRepos = nil, false
		mres, err := member.finder.Search(ctx, mq)
		if err != nil {
			return nil, fmt.Errorf("searching %s: %w", repoID, err)
		}
		for _, se := range mres.Entries {
			if se.Entry.Embedded {
				continue
			}
			se.RepoID = repoID
			merged = append(merged, se)
		}
	}

	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Score > merged[j].Score })
	if limit := q.EffectiveLimit(); len(merged) > limit {
		merged = merged[:limit]
	}
	return &query.SearchResult{Mode: res.Mode, Entries: merged}, nil
}

// searchMemberSource is one connected repo's resolved search source.
type searchMemberSource struct {
	graph  *model.Graph
	finder *SearchFinder
}

// searchMember resolves one selected repo into a search source: its member
// graph from the query graph's assembly and a finder over its cache graph
// dir plus per-repo index (vector mode only). nil means the repo is not
// available — not connected or not cached — which the caller reports.
func searchMember(q query.SearchQuery, repoID string, local *SearchFinder) (*searchMemberSource, error) {
	member, err := q.Graph.MemberGraph(repoID)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, nil
	}
	cacheDir, err := repos.CacheDir(repoID)
	if err != nil {
		return nil, err
	}
	graphDir, err := repos.GraphDir(cacheDir)
	if err != nil {
		return nil, err
	}
	var store *index.Index
	if q.Phrase != "" && local.embedder != nil {
		store, err = index.Open(repos.IndexDir(cacheDir))
		if err != nil {
			return nil, fmt.Errorf("opening index for %s: %w", repoID, err)
		}
	}
	return &searchMemberSource{
		graph: member,
		finder: NewSearchFinder(SearchFinderOptions{
			GraphDir:   graphDir,
			Embedder:   local.embedder,
			IndexStore: store,
		}),
	}, nil
}
