package model

import "sync"

// MultiGraph is the cross-graph read model: the local graph plus member
// graphs keyed by canonical repo-id, loaded lazily through an injected
// loader. It is assembled only inside the finder-owned GraphSource — that
// single seam gives CLI commands, MCP reads, engine serves, and
// capture-time resolution the same insertion point. A MultiGraph with no
// loader behaves exactly like the bare local graph, so the single-repo
// path is unchanged.
//
// The loader is a closure wired by the read side (the model stays free of
// I/O): given a repo-id it returns the cached member graph, nil when the
// repo is not connected or not cached, or an error when a present cache
// fails to load. Results — including errors — memoize for the lifetime of
// the MultiGraph; the GraphSource drops the whole assembly on Invalidate.
type MultiGraph struct {
	Local *Graph

	// deps is the local graph's declared cross-repo dependencies (repo-ids)
	// — the bounded scope for bare-ID resolution. Resolution never reaches a
	// repo outside this set, matching the declared-dependency precondition
	// that governs which repos a ref may point at.
	deps []string

	loader func(repoID string) (*Graph, error)

	mu      sync.Mutex
	members map[string]*memberState
}

type memberState struct {
	graph *Graph
	err   error
}

// NewMultiGraph assembles the cross-graph read model around a local graph.
// deps is the local graph's declared cross-repo dependencies (repo-ids), the
// bounded scope for bare-ID resolution. The local graph (and every lazily
// loaded member) gets back-wired so traversal code holding any *Graph can
// resolve across the boundary.
func NewMultiGraph(local *Graph, deps []string, loader func(repoID string) (*Graph, error)) *MultiGraph {
	m := &MultiGraph{Local: local, deps: deps, loader: loader, members: make(map[string]*memberState)}
	local.multi = m
	return m
}

// dependencyGraphs returns the loaded member graphs for the declared
// dependencies that are connected and cached, skipping any that are absent or
// fail to load — resolution treats an unreachable dependency as contributing
// no candidates, the same honest unresolved state a read renders.
func (m *MultiGraph) dependencyGraphs() []*Graph {
	if m == nil {
		return nil
	}
	var out []*Graph
	for _, repoID := range m.deps {
		g, err := m.Member(repoID)
		if err != nil || g == nil {
			continue
		}
		out = append(out, g)
	}
	return out
}

// Member returns the cached graph for a connected repo, loading it on first
// use. nil with no error means the repo is not resolvable (not connected,
// no cache) — the legitimate unresolved state; an error means a cache was
// present but failed to load and is propagated to callers that can act.
func (m *MultiGraph) Member(repoID string) (*Graph, error) {
	if m == nil || m.loader == nil {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.members[repoID]; ok {
		return st.graph, st.err
	}
	g, err := m.loader(repoID)
	if g != nil {
		g.multi = m
		g.repoPrefix = repoID + ":"
	}
	m.members[repoID] = &memberState{graph: g, err: err}
	return g, err
}

// MemberGraph returns the cached member graph for a connected repo from
// this graph's cross-graph assembly: nil without error when the repo is
// not resolvable (not connected, no cache, or no assembly at all), an
// error when a present cache failed to load.
func (g *Graph) MemberGraph(repoID string) (*Graph, error) {
	return g.multi.Member(repoID)
}

// DisplayID resolves an ID (bare or cross-repo) to its display and dedup
// key: bare for local and embedded entries, repo-prefixed for a member
// graph's own entries. ok is false when the ID does not resolve.
func (g *Graph) DisplayID(id string) (string, bool) {
	e, owner, ok := g.ResolveAcross(id)
	if !ok {
		return "", false
	}
	return owner.nodeKeyFor(e), true
}

// ResolveAcross resolves an ID from the perspective of graph `from`: a bare
// ID resolves within `from` itself, a cross-repo ID resolves in the named
// member graph. The returned graph is the owning graph — status derivation
// and supersede head-walks belong there. ok is false when the target repo
// is unavailable or the entry is absent (load errors resolve to
// unavailable here; callers needing the distinction use Member directly).
func (from *Graph) ResolveAcross(id string) (e *Entry, owner *Graph, ok bool) {
	repoID, entryID, isCross := SplitCrossRepoID(id)
	if !isCross {
		e, ok := from.ByID[id]
		return e, from, ok
	}
	member, err := from.multi.Member(repoID)
	if err != nil || member == nil {
		return nil, nil, false
	}
	e, found := member.ByID[entryID]
	return e, member, found
}
