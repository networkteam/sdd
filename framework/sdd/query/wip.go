package query

import "github.com/networkteam/resonance/framework/sdd/model"

// WIPListQuery captures intent to list active WIP markers.
type WIPListQuery struct {
	GraphDir string
}

// WIPListResult is the structured output of a WIPListQuery.
type WIPListResult struct {
	Markers []*model.WIPMarker
}
