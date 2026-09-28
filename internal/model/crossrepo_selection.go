package model

// DependencyClosure walks declared dependencies breadth-first from direct and
// returns every reached repo ID once, in first-reached order, never root
// itself. declared returns a reached repo's own declarations; a repo it cannot
// resolve returns none, so it stays listed but nothing behind it is reached.
func DependencyClosure(root string, direct []string, declared func(repoID string) ([]string, error)) ([]string, error) {
	seen := map[string]bool{root: true}
	var closure []string
	queue := append([]string(nil), direct...)
	for len(queue) > 0 {
		repoID := queue[0]
		queue = queue[1:]
		if seen[repoID] {
			continue
		}
		seen[repoID] = true
		closure = append(closure, repoID)
		next, err := declared(repoID)
		if err != nil {
			return nil, err
		}
		queue = append(queue, next...)
	}
	return closure, nil
}

// CitedAcross selects the entries of other repos that the local graph cites:
// the targets of its cross-repo refs, closes and supersedes, then up to hops
// further upstream steps along the same fields from each selected entry — a
// bare ID within that entry's repo, a cross-repo ID into the repo it names.
// Only repos in scope are entered, and embedded entries are never selected
// (no repo owns them). The result maps repo ID to the selected entry IDs.
func CitedAcross(local *Graph, scope []string, hops int) (map[string]map[string]bool, error) {
	inScope := make(map[string]bool, len(scope))
	for _, repoID := range scope {
		inScope[repoID] = true
	}
	type step struct {
		repoID string
		entry  *Entry
		depth  int
	}
	selected := map[string]map[string]bool{}
	var queue []step
	add := func(repoID, id string, depth int) error {
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
