package query

import "github.com/networkteam/resonance/framework/sdd/model"

// StatusQuery captures intent to summarise current graph state.
type StatusQuery struct {
	Graph         *model.Graph
	RecentActions int // how many recent actions to include (default 5)
}

// StatusResult is the structured snapshot of graph state for the status view.
type StatusResult struct {
	Graph     *model.Graph // for top-line counts (entries, decisions, signals, actions)
	Contracts []*model.Entry
	Plans     []*model.Entry
	Active    []*model.Entry // active directive decisions
	Open      []*model.Entry // open signals
	Recent    []*model.Entry // recent actions
}
