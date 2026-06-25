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
	// intentFilters mirrors kindFilters for the directive intent attribute:
	// each intent() call is a disjunction set, multiple calls intersect. Only
	// directives carry intent, so a non-empty filter implicitly narrows to
	// directives with the listed posture (d-tac-n9k).
	intentFilters [][]model.Intent
	// participantFilters mirrors kindFilters: each participant() call is a
	// disjunction set (any listed canonical matches), and multiple calls
	// intersect. Names are matched exactly against the entry's canonical
	// participants per the canonical-only field contract (d-cpt-979).
	participantFilters [][]string
	sinceCutoff        *time.Time // pointer so we can distinguish "no since()" from "since(0d)"
	topicPrefix        model.TopicPath
	untagged           bool      // untagged: keep only entries whose effective topic set is empty
	idFilter           []string  // id(<id>,...): keep only the listed entries (raw IDs, resolved at apply time)
	rank               *rankSpec // last-write-wins per d-tac-uww §2
	pageN              int       // -1 = no page limit
	groupField         string    // empty = no group; non-empty = group(by(<field>))
	expandField        string    // empty = no expand; "involvement" (focus-block) or "refs" (per-row ref sub-lines on as-list)

	// expandRefsInactive is set by expand(refs(inactive)) — narrows the
	// per-entry ref sub-lines to refs whose target is currently inactive
	// (closed, superseded, or a non-active role), the inverse of the
	// `active` filter. Meaningful only when expandField == "refs".
	expandRefsInactive bool

	// Negation slots — populated by `not(<inner-filter>)` calls per
	// d-tac-e1s. Each slot mirrors the semantic of its positive
	// counterpart's storage:
	//   excludeKinds       — flat union (multiple not(kind()) calls union
	//                        their exclusion sets)
	//   excludeLayer       — last-write-wins (mirrors positive layer())
	//   excludeTopicPrefix — last-write-wins (mirrors positive topic())
	// Inner filters active and since are deferred (semantic edge cases);
	// nested not(not(...)) is rejected at parse time.
	excludeKinds       []model.Kind
	excludeIntents     []model.Intent // not(intent(...)) — flat union, mirrors excludeKinds
	excludeLayer       model.Layer
	excludeTopicPrefix model.TopicPath

	// Focus-block-only knob.
	stalledThreshold float64
	stalledSet       bool // user-supplied threshold via stalled(value)

	// Section header — two distinct slots resolved at executor time:
	//   - name(string)        → final title, suppresses any auto-suffix
	//   - name-prefix(string) → prefix that auto-derive may extend with
	//                            the rank suffix (when rank is set)
	// Both follow last-write-wins independently. Macros bake name-prefix
	// so user `name(...)` overrides cleanly without surprise auto-append.
	nameSet     bool
	nameValue   string
	prefixSet   bool
	prefixValue string

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
	case len(s.intentFilters) > 0:
		return fmt.Errorf("source(wip) does not support intent() filters; markers are not graph entries")
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
	case len(s.excludeKinds) > 0:
		return fmt.Errorf("source(wip) does not support not(kind(...)); markers are not graph entries")
	case len(s.excludeIntents) > 0:
		return fmt.Errorf("source(wip) does not support not(intent(...)); markers are not graph entries")
	case s.excludeLayer != "":
		return fmt.Errorf("source(wip) does not support not(layer(...)); markers are not graph entries")
	case !s.excludeTopicPrefix.IsZero():
		return fmt.Errorf("source(wip) does not support not(topic(...)); markers do not carry topics")
	}
	return nil
}

// validateMutualExclusion enforces the cross-primitive constraints:
// group() produces a non-flat shape that doesn't compose with rank()/n();
// expand(involvement) renders a focus-block whose target order and stalled
// heat are fixed, so it can't combine with rank()/n() either. expand(refs)
// is the exception — it modifies the flat list in place and deliberately
// composes with rank()/n() (the catch-up layout ranks and pages before
// expanding). Neither expand variant combines with group(). stalled()
// applies only to the focus-block (expand(involvement)) section. Returning
// the first finding keeps the error focused.
func (s *sectionSpec) validateMutualExclusion() error {
	switch {
	case s.render == "as-counts" && s.groupField != "":
		return fmt.Errorf("as-counts is mutually exclusive with group; both aggregate the entry set, in different shapes (per-topic counts vs per-field buckets)")
	case s.render == "as-counts" && s.expandField != "":
		return fmt.Errorf("as-counts is mutually exclusive with expand; it produces per-topic count rows, not entry rows")
	case s.render == "as-counts" && s.rank != nil:
		return fmt.Errorf("as-counts is mutually exclusive with rank; topic-count rows carry their own ordering (count, then heat)")
	case s.render == "as-counts" && s.pageN >= 0:
		return fmt.Errorf("as-counts is mutually exclusive with n; n truncates entries before aggregation, producing wrong counts — narrow the entry set with filters instead")
	case s.groupField != "" && s.rank != nil:
		return fmt.Errorf("group is mutually exclusive with rank in slice 5; per-group ranking is reserved for a future slice")
	case s.groupField != "" && s.pageN >= 0:
		return fmt.Errorf("group is mutually exclusive with n in slice 5; per-group pagination is reserved for a future slice")
	case s.expandField == "involvement" && s.rank != nil:
		return fmt.Errorf("expand(involvement) is mutually exclusive with rank; focus-block targets render in involvement order, and heat for stalled classification is fixed at heat(exp-14d)")
	case s.expandField == "involvement" && s.pageN >= 0:
		return fmt.Errorf("expand(involvement) is mutually exclusive with n; focus-block target lists are bounded by the focus's involvement frontmatter, not by pagination")
	case s.expandField != "" && s.groupField != "":
		return fmt.Errorf("expand is mutually exclusive with group; both produce their own non-flat shape")
	case s.stalledSet && s.expandField != "involvement":
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
	case s.expandField == "involvement" && expected != model.ShapeFocusBlock:
		return fmt.Errorf(
			"render-shape mismatch: %s does not consume a focus-block result, but expand(involvement) produces one (use as-focus-block, or remove expand)",
			s.render)
	case s.expandField == "refs" && expected != model.ShapeFlatList:
		return fmt.Errorf(
			"render-shape mismatch: %s does not consume a flat-list, but expand(refs) modifies one (use as-list, or remove expand)",
			s.render)
	case s.groupField == "" && expected == model.ShapeGrouped:
		return fmt.Errorf(
			"render-shape mismatch: as-grouped expects a grouped result, but the section produces a flat-list (add group(by(<field>)) before as-grouped)")
	case s.expandField != "involvement" && expected == model.ShapeFocusBlock:
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

// resolveSectionHeader populates spec.nameValue / spec.nameSet from the
// auto-derive rules. Called after the parse loop so all bake/override
// signals are visible. Resolution order, top of pipeline first:
//
//  1. Explicit name(...) was called → already in spec.nameValue; respect.
//  2. name-prefix(...) baked + rank set → "<prefix> <suffix>"
//     (e.g. macro-baked "Top" + rank(in-degree) → "Top by in-degree")
//  3. name-prefix(...) baked, no rank → just the prefix
//     (e.g. focus macro → "Focus")
//  4. No prefix, rank set → "Top <suffix>" (covers raw `rank(heat):as-list`
//     without a macro — the implicit "Top" prefix matches the top(N) idiom)
//  5. Neither → no header.
//
// Centralizing the rule here keeps the executor's main flow as a
// sequence of named checks; macros and users only need to know the
// observable behaviour, not the resolver internals.
func (s *sectionSpec) resolveSectionHeader() {
	if s.nameSet {
		return
	}
	suffix := ""
	if s.rank != nil {
		suffix = s.rank.suffix()
	}
	switch {
	case s.prefixSet && suffix != "":
		s.nameValue = s.prefixValue + " " + suffix
		s.nameSet = true
	case s.prefixSet:
		s.nameValue = s.prefixValue
		s.nameSet = true
	case suffix != "":
		s.nameValue = "Top " + suffix
		s.nameSet = true
	}
}
