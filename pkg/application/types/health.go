package types

// HealthIssue is one graph-integrity problem as a displayable line: the entry
// ID (or load ref) it concerns and the human message.
type HealthIssue struct {
	Ref     string
	Message string
}

// GraphHealth is a flat summary of graph-integrity problems: the count of
// entry warnings, the count of unreadable (load-failed) entries, and every
// problem as an ordered line (load failures first, then entry warnings).
type GraphHealth struct {
	Warnings   int
	LoadErrors int
	Issues     []HealthIssue
}

// Clean reports whether the graph carries no integrity problems at all.
func (h GraphHealth) Clean() bool {
	return h.Warnings == 0 && h.LoadErrors == 0
}

// PatchPair is one exact search-replace edit, applied by the staged-attachment
// edit path (internal/textpatch carries the apply semantics).
type PatchPair struct {
	Old string
	New string
}
