package finders

import (
	"fmt"
	"sync"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
)

// GraphSource is the read side's owner of "what the current graph is": a
// memoizing read-through over the single IO loader (LoadGraph), with an
// Invalidate hook that drops the memo after a write. It is the one seam every
// graph read flows through, so cross-repo assembly (model.MultiGraph) has
// exactly one insertion point — behind Current — with the side-effectful cache
// clone/pull staying handler-side, invalidating the source on completion.
//
// One source per read scope. A long-lived reader that reads many times within
// one scope and mutates the graph mid-scope (the engine session across an
// advance) holds a source and invalidates it post-write. Short one-shot readers
// (a CLI command, an MCP free read, a handler validating before a write) call
// Finder.CurrentGraph, which sources once — no retained cache, matching the
// prior per-command / per-call lifetime.
type GraphSource struct {
	load  func() (*model.Graph, error)
	mu    sync.Mutex
	graph *model.Graph
}

// NewGraphSource builds a memoizing source over dir. Held by long-lived readers
// (the engine session); one-shot readers use CurrentGraph.
//
// The loaded graph is assembled into a model.MultiGraph here — the single
// insertion point for cross-repo reads. Member graphs load lazily from the
// connected-repos caches (pure reads; clone/pull side effects run in
// handlers that invalidate this source), so a graph with no cross-repo
// touch pays nothing.
func (f *Finder) NewGraphSource(dir string) *GraphSource {
	return &GraphSource{load: func() (*model.Graph, error) {
		g, err := f.LoadGraph(dir)
		if err != nil {
			return nil, err
		}
		deps, err := f.declaredDependencies()
		if err != nil {
			return nil, err
		}
		model.NewMultiGraph(g, deps, f.memberGraphLoader())
		return g, nil
	}}
}

// memberGraphLoader is the lazy cache→graph read the MultiGraph resolves
// members through: connected repos come from the injected Registry, the
// graph loads from the repo's cache clone. A repo that is not connected or
// not yet cached resolves to nil (the legitimate unresolved state — the
// sync handler owns clone/pull); a present cache that fails to load is an
// error, not a silent gap. A Finder without a Registry resolves nothing —
// the same honest unresolved state.
func (f *Finder) memberGraphLoader() func(repoID string) (*model.Graph, error) {
	return func(repoID string) (*model.Graph, error) {
		if f.repos == nil {
			return nil, nil
		}
		cfg, err := f.repos.Load()
		if err != nil {
			return nil, err
		}
		if _, connected := cfg.Connected(repoID); !connected {
			return nil, nil
		}
		dir, err := f.repos.CacheDir(repoID)
		if err != nil {
			return nil, err
		}
		if !repos.IsCloned(dir) {
			return nil, nil
		}
		graphDir, err := repos.GraphDir(dir)
		if err != nil {
			return nil, err
		}
		g, err := f.LoadGraph(graphDir)
		if err != nil {
			return nil, fmt.Errorf("loading cached graph for %s: %w", repoID, err)
		}
		return g, nil
	}
}

// CurrentGraph sources the current graph once — the one-shot read every
// short-lived consumer uses instead of calling LoadGraph directly, so all reads
// share the one GraphSource seam. No memo is retained across calls (each call
// is its own scope), so a fresh command or request always loads fresh.
func (f *Finder) CurrentGraph(dir string) (*model.Graph, error) {
	return f.NewGraphSource(dir).Current()
}

// Current returns the current graph, loading it on first call and reusing the
// memoized value until Invalidate drops it. Concurrency-safe: the read side now
// owns this state, so a source shared across an engine advance is guarded.
func (gs *GraphSource) Current() (*model.Graph, error) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if gs.graph != nil {
		return gs.graph, nil
	}
	g, err := gs.load()
	if err != nil {
		return nil, err
	}
	gs.graph = g
	return g, nil
}

// Invalidate drops the memo so the next Current reloads. Called after a write
// mutates the graph, so post-write reads in the same scope see the change.
func (gs *GraphSource) Invalidate() {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.graph = nil
}
