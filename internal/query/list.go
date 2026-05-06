package query

import "github.com/networkteam/sdd/internal/model"

// ListQuery captures intent to filter graph entries.
type ListQuery struct {
	Graph  *model.Graph
	Filter model.GraphFilter
	// Topic optionally narrows results to entries whose effective topic set
	// (inline `topics:` ∪ topics declared by annotations whose refs include
	// the entry) has any label with the given path as a component-wise,
	// case-insensitive prefix. Zero value means no topic filter.
	Topic model.TopicPath
}

// ListResult is the structured output of a ListQuery.
type ListResult struct {
	Graph   *model.Graph // needed to render derived attributes like status
	Entries []*model.Entry
}
