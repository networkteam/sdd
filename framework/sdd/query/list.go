package query

import "github.com/networkteam/resonance/framework/sdd/model"

// ListQuery captures intent to filter graph entries.
type ListQuery struct {
	Graph  *model.Graph
	Filter model.GraphFilter
}

// ListResult is the structured output of a ListQuery.
type ListResult struct {
	Entries []*model.Entry
}
