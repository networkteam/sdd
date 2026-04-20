package query

import "github.com/networkteam/sdd/internal/model"

// StatusQuery captures intent to summarise current graph state.
type StatusQuery struct {
	Graph      *model.Graph
	RecentDone int // how many recent kind: done signals to include (default 10)
}

// StatusResult is the structured snapshot of graph state for the status view.
// Aspirations surface separately from Contracts — both are durable decisions,
// but aspirations are "pull toward" attractors while contracts are "must hold"
// constraints, and the reader benefits from seeing them apart.
type StatusResult struct {
	Graph       *model.Graph // for top-line counts (entries, decisions, signals)
	Contracts   []*model.Entry
	Aspirations []*model.Entry
	Plans       []*model.Entry
	Active      []*model.Entry // active directive & activity decisions
	Open        []*model.Entry // open signals excluding kind: done
	Recent      []*model.Entry // recent kind: done signals
}
