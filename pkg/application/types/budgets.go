package types

// ViewBudget names the per-shape bounds a served view applies: every cut is
// at a whole unit (a focus group, a marker, a body, a ref sub-line) and the
// shape carries the dropped count plus a runnable pull for the remainder.
type ViewBudget struct {
	// GroupItems caps focus groups, participant groups, and WIP markers per
	// section.
	GroupItems int
	// RefsPerEntry caps expand(refs) sub-lines per entry.
	RefsPerEntry int
	// BodyBytes caps as-bodies sections: whole bodies while bytes fit.
	BodyBytes int
}

// ShowTreeBudget bounds a tree's expansion per direction: MaxNodes caps how
// many whole-entry nodes one direction may carry, MaxChildren caps one
// node's fan-out. Children past a bound land as TruncatedRefs — the same
// honest frontier the depth limit renders — never silently dropped. Zero
// values are unbounded: explicit pulls (sdd show, the MCP show tool) pass no
// budget and arrive complete (d-tac-rzi).
type ShowTreeBudget struct {
	MaxNodes    int
	MaxChildren int
}
