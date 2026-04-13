package query

import "github.com/networkteam/resonance/framework/sdd/model"

// LintQuery captures intent to surface graph integrity issues.
type LintQuery struct {
	Graph *model.Graph
}

// LintResult is the structured output of a LintQuery: every entry that has
// at least one warning, plus the total warning count for convenience.
type LintResult struct {
	Entries     []*model.Entry
	TotalIssues int
}
