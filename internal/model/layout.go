package model

// Layout is the parsed root of a `--layout=...` argument to `sdd view`.
// Each Section corresponds to one comma-separated entry in the source
// string; sections are rendered in source order.
type Layout struct {
	Sections []Section
}

// Section is one colon-chained pipeline within a Layout. Functions execute
// left-to-right: filters and transforms accumulate, the section terminates
// in a render function (e.g. as-list).
type Section struct {
	Functions []Function
}

// Function is one step in a Section's pipeline. Slice 1 supports bare
// names only (no parens, no args). The Args field arrives in slice 2 when
// paren parsing lands; keeping the shape stable now would invite premature
// generality.
type Function struct {
	Name string
}
