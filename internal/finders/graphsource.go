package finders

import (
	"sync"

	"github.com/networkteam/sdd/internal/model"
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
func (f *Finder) NewGraphSource(dir string) *GraphSource {
	return &GraphSource{load: func() (*model.Graph, error) { return f.LoadGraph(dir) }}
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
