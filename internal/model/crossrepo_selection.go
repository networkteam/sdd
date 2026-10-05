package model

import (
	"context"
	"iter"
)

// DependencyResolver answers one step of a declared-dependency walk: the
// dependencies k declares, as the keys they resolve to. Each composition
// resolves its own way and states its own rule for a dependency it cannot
// resolve.
type DependencyResolver[K comparable] interface {
	Dependencies(ctx context.Context, k K) ([]K, error)
}

// DependencyClosure walks the declared dependencies of root transitively and
// yields every key reached once, breadth-first, in first-reached order, never
// root itself. A resolver error is yielded and ends the walk; breaking out of
// the loop stops it.
func DependencyClosure[K comparable](ctx context.Context, root K, r DependencyResolver[K]) iter.Seq2[K, error] {
	return func(yield func(K, error) bool) {
		seen := map[K]bool{root: true}
		queue := []K{root}
		for len(queue) > 0 {
			k := queue[0]
			queue = queue[1:]
			deps, err := r.Dependencies(ctx, k)
			if err != nil {
				var zero K
				yield(zero, err)
				return
			}
			for _, dep := range deps {
				if seen[dep] {
					continue
				}
				seen[dep] = true
				if !yield(dep, nil) {
					return
				}
				queue = append(queue, dep)
			}
		}
	}
}

// CitedAcross selects the entries of other repos that the local graph cites:
// the targets of its cross-repo refs, closes and supersedes, then up to hops
// further upstream steps along the same fields from each selected entry — a
// bare ID within that entry's repo, a cross-repo ID into the repo it names.
// Only repos in scope are entered, and embedded entries are never selected
// (no repo owns them). The result maps repo ID to the selected entry IDs.
func CitedAcross(local *Graph, scope []RepoID, hops int) (map[RepoID]map[string]bool, error) {
	inScope := make(map[RepoID]bool, len(scope))
	for _, repoID := range scope {
		inScope[repoID] = true
	}
	type step struct {
		repoID RepoID
		entry  *Entry
		depth  int
	}
	selected := map[RepoID]map[string]bool{}
	var queue []step
	add := func(repoID RepoID, id string, depth int) error {
		if !inScope[repoID] || selected[repoID][id] {
			return nil
		}
		member, err := local.MemberGraph(repoID)
		if err != nil || member == nil {
			return err
		}
		e, ok := member.ByID[id]
		if !ok || e.Embedded {
			return nil
		}
		if selected[repoID] == nil {
			selected[repoID] = map[string]bool{}
		}
		selected[repoID][id] = true
		queue = append(queue, step{repoID: repoID, entry: e, depth: depth})
		return nil
	}

	for _, e := range local.Entries {
		for _, id := range upstreamIDs(e) {
			if repoID, entryID, ok := SplitCrossRepoID(id); ok {
				if err := add(repoID, entryID, 0); err != nil {
					return nil, err
				}
			}
		}
	}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		if s.depth >= hops {
			continue
		}
		for _, id := range upstreamIDs(s.entry) {
			repoID, entryID := s.repoID, id
			if r, eid, ok := SplitCrossRepoID(id); ok {
				repoID, entryID = r, eid
			}
			if err := add(repoID, entryID, s.depth+1); err != nil {
				return nil, err
			}
		}
	}
	return selected, nil
}

// upstreamIDs lists the IDs an entry points at: refs, closes, supersedes.
func upstreamIDs(e *Entry) []string {
	ids := RefIDs(e.Refs)
	ids = append(ids, e.Closes...)
	return append(ids, e.Supersedes...)
}
