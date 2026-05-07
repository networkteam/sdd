package finders

import (
	"fmt"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// sectionSpec is the parsed intent of one view-layout section: every
// function call in the source pipeline accumulates into one of its
// fields, then dispatch happens against the spec rather than against
// loose locals threaded through the executor. Each field is a "bucket"
// per d-tac-uww §2 — last-write-wins for non-filter modifiers,
// intersection across multiple kind() calls (still a slice-of-slices
// here), and a sentinel zero value when the call hasn't appeared.
//
// Methods on the spec validate the parsed shape (mutual exclusion,
// render-shape contracts, source-specific primitive rejection) so the
// view executor's main flow reads as a sequence of named checks rather
// than parameter-heavy helper calls. The parse loop in view.go writes
// fields directly; pure validation lives here.
type sectionSpec struct {
	// Sourcing — default "graph"; "wip" switches the executor to the
	// disk-marker path and rejects graph-side primitives.
	source string

	// Graph-side primitives.
	filter      model.GraphFilter
	kindFilters [][]model.Kind // each kind() call is a disjunction set; multiple calls intersect (d-tac-uww §2)
	sinceCutoff *time.Time     // pointer so we can distinguish "no since()" from "since(0d)"
	topicPrefix model.TopicPath
	rank        *rankSpec // last-write-wins per d-tac-uww §2
	pageN       int       // -1 = no page limit
	groupField  string    // empty = no group; non-empty = group(by(<field>))
	expandField string    // empty = no expand; non-empty = expand(<field>) (slice 7: only "involvement")

	// Focus-block-only knob.
	stalledThreshold float64
	stalledSet       bool // user-supplied threshold via stalled(value)

	// Section header (last-write-wins, including empty-string clears).
	nameSet   bool
	nameValue string

	// Render terminator — required.
	render string
}

// newSectionSpec returns a spec with the slice's defaults populated:
// graph source, no page limit, default stalled threshold.
func newSectionSpec() *sectionSpec {
	return &sectionSpec{
		source:           "graph",
		pageN:            -1,
		stalledThreshold: DefaultStalledThreshold,
	}
}

// rejectGraphPrimitivesForWip enforces the disjoint-vocabulary contract
// for source(wip) sections. Filters, rank, page, group, expand, and
// stalled all operate on graph entries — they have no defined meaning
// over WIP markers, so a section that combines them with source(wip) is
// almost certainly a paste error or misunderstanding. Returns the first
// finding so the user sees one focused issue at a time rather than a
// concatenated list.
func (s *sectionSpec) rejectGraphPrimitivesForWip() error {
	switch {
	case s.filter.OpenOnly || s.filter.Layer != "":
		return fmt.Errorf("source(wip) does not support graph filters (active, layer); markers are not graph entries")
	case len(s.kindFilters) > 0:
		return fmt.Errorf("source(wip) does not support kind() filters; markers are not graph entries")
	case s.sinceCutoff != nil:
		return fmt.Errorf("source(wip) does not support since(); slice 8 surfaces every active marker")
	case !s.topicPrefix.IsZero():
		return fmt.Errorf("source(wip) does not support topic() filters; markers do not carry topics")
	case s.rank != nil:
		return fmt.Errorf("source(wip) does not support rank(); markers are surfaced in chronological order")
	case s.pageN >= 0:
		return fmt.Errorf("source(wip) does not support n() in slice 8; the active-marker set is bounded by what's on disk")
	case s.groupField != "":
		return fmt.Errorf("source(wip) does not support group(); marker grouping is not yet defined")
	case s.expandField != "":
		return fmt.Errorf("source(wip) does not support expand(); markers have no involvement field")
	case s.stalledSet:
		return fmt.Errorf("source(wip) does not support stalled(); the threshold applies only to focus-block sections")
	}
	return nil
}

// validateMutualExclusion enforces the slice-5/7 cross-primitive
// constraints: group()/expand() each produce non-flat shapes that don't
// compose with rank()/n() the way a flat list does, and group/expand
// can't combine. stalled() applies only to focus-block sections.
// Returning the first finding keeps the error focused.
func (s *sectionSpec) validateMutualExclusion() error {
	switch {
	case s.groupField != "" && s.rank != nil:
		return fmt.Errorf("group is mutually exclusive with rank in slice 5; per-group ranking is reserved for a future slice")
	case s.groupField != "" && s.pageN >= 0:
		return fmt.Errorf("group is mutually exclusive with n in slice 5; per-group pagination is reserved for a future slice")
	case s.expandField != "" && s.rank != nil:
		return fmt.Errorf("expand is mutually exclusive with rank; focus-block targets render in involvement order, and heat for stalled classification is fixed at heat(exp-14d)")
	case s.expandField != "" && s.pageN >= 0:
		return fmt.Errorf("expand is mutually exclusive with n; focus-block target lists are bounded by the focus's involvement frontmatter, not by pagination")
	case s.expandField != "" && s.groupField != "":
		return fmt.Errorf("expand is mutually exclusive with group; focus-block has its own per-focus grouping shape")
	case s.stalledSet && s.expandField == "":
		return fmt.Errorf("stalled() applies only to focus-block sections; pair it with expand(involvement):as-focus-block")
	}
	return nil
}

// validateRenderShape checks that the section's producer-side
// primitives match the render terminator's input shape (AC 16). Fires
// before result computation so users see the structural error first.
func (s *sectionSpec) validateRenderShape() error {
	expected := renderFunctions[s.render]

	// as-wip-list is reachable only via source(wip); the source(wip)
	// branch returns earlier in the executor. Reaching here means the
	// section is graph-sourced — no markers to render.
	if s.render == "as-wip-list" {
		return fmt.Errorf(
			"render-shape mismatch: as-wip-list requires source(wip), but this section uses source(graph)")
	}
	switch {
	case s.groupField != "" && expected != model.ShapeGrouped:
		return fmt.Errorf(
			"render-shape mismatch: %s does not consume a grouped result, but group(by(...)) produces one (use as-grouped, or remove group)",
			s.render)
	case s.expandField != "" && expected != model.ShapeFocusBlock:
		return fmt.Errorf(
			"render-shape mismatch: %s does not consume a focus-block result, but expand(involvement) produces one (use as-focus-block, or remove expand)",
			s.render)
	case s.groupField == "" && expected == model.ShapeGrouped:
		return fmt.Errorf(
			"render-shape mismatch: as-grouped expects a grouped result, but the section produces a flat-list (add group(by(<field>)) before as-grouped)")
	case s.expandField == "" && expected == model.ShapeFocusBlock:
		return fmt.Errorf(
			"render-shape mismatch: as-focus-block expects a focus-block result, but the section produces a flat-list (add expand(involvement) before as-focus-block, with kind(focus) filter)")
	}
	if expected == model.ShapeParticipantsBlock {
		// rank()/n() over the participants block has no defined meaning —
		// the block is a fixed grouping of active actors with bound roles,
		// not a ranked list. Reject so users don't paste a `:n(20)` from a
		// top-N habit and get silent truncation of actor groups.
		switch {
		case s.rank != nil:
			return fmt.Errorf("as-participants-block does not support rank(); the block groups active actors with bound roles in chain order")
		case s.pageN >= 0:
			return fmt.Errorf("as-participants-block does not support n(); pagination is undefined for the participants block")
		}
	}
	return nil
}

// sectionName returns the value to use as the rendered `## <title>`
// header. Empty result means "no header" — last-write-wins on empty
// strings clears any prior name() so this is the deliberate signal.
func (s *sectionSpec) sectionName() string {
	if !s.nameSet {
		return ""
	}
	return s.nameValue
}
