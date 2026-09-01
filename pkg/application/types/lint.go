package types

// LintQuery captures intent to surface graph integrity issues. Pure intent —
// the graph is held by the GraphFinder that runs the query.
type LintQuery struct{}

// LintSeverity classifies a lint finding's consequence: an error is a graph
// integrity problem and flips the exit code; an advisory records a risk the
// author may accept and never flips it (d-tac-rzi — an overshooting spec
// still runs; d-cpt-xc3 — spec-authoring advisories are never entry warnings).
type LintSeverity string

const (
	LintError    LintSeverity = "error"
	LintAdvisory LintSeverity = "advisory"
)

// LintFinding is one categorized lint observation. Category names the
// provider that raised it (graph, index, procedure-runtime); Code names the
// specific check within it.
type LintFinding struct {
	Category string
	Code     string
	Severity LintSeverity
	// EntryID names the entry (or file path, for load errors) the finding is
	// about; empty for store-level findings like index drift.
	EntryID string
	Message string
}

// LintResult is the structured output of a LintQuery: categorized findings
// from every provider, in provider order. Presenters group by category;
// shells derive the exit code from Errors alone.
type LintResult struct {
	Findings []LintFinding
}

// Errors counts the findings whose severity flips the exit code.
func (r *LintResult) Errors() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == LintError {
			n++
		}
	}
	return n
}
