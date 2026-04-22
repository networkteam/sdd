package query

import "github.com/networkteam/sdd/internal/model"

// StatusQuery captures intent to summarise current graph state.
type StatusQuery struct {
	Graph          *model.Graph
	RecentDone     int // how many recent kind: done signals to include (default 10)
	RecentInsights int // how many recent kind: insight signals to include (default 10)
}

// StatusResult is the structured snapshot of graph state for the status view.
// Each decision kind surfaces in its own section — Aspirations guide,
// Contracts bound, Plans carry multi-step scope with ACs, Activities capture
// THAT-shaped commitments that specific work happens, Directives are the
// WHAT-shaped choices. On the signal side, Open carries the closure-gated
// attention set (gap + question), Insights and Recent are truncated
// activity streams.
//
// LocalParticipant surfaces the canonical participant name from
// .sdd/config.local.yaml so agents reading the status header see ground
// truth without inferring from entries (which may contain drift).
type StatusResult struct {
	Graph            *model.Graph // for top-line counts (entries, decisions, signals)
	LocalParticipant string       // canonical from config; empty means "not configured"
	Aspirations      []*model.Entry
	Contracts        []*model.Entry
	Plans            []*model.Entry
	Activities       []*model.Entry
	Directives       []*model.Entry
	Open             []*model.Entry // kind: gap and kind: question signals (the actionable set)
	Insights         []*model.Entry // recent kind: insight signals
	Recent           []*model.Entry // recent kind: done signals
}
