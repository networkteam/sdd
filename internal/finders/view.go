package finders

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// View executes the layout pipeline in q.Layout against q.Graph and returns
// one SectionResult per section in source order. Per the design, every
// section must terminate in a render function (e.g. as-list); render is
// always the pipeline's terminus.
//
// Sourcing dispatches on the first `source(<name>)` call in the section
// (default `graph` when absent): graph sections walk q.Graph through the
// filter chain; wip sections use q.WIPMarkers, falling back to
// f.LoadWIPMarkers(q.GraphDir), as a disjoint data set rendered by
// as-wip-list. The two paths share the
// section-walking shell but apply distinct primitive vocabularies; cross-
// path mixing (e.g. kind() over wip markers) errors with a source-aware
// message.
//
// WIP markers are resolved once when any section in the layout uses
// source(wip), then the same slice is handed to every wip section. A nil
// marker slice and empty GraphDir errors at section evaluation; non-wip
// layouts ignore both inputs.
//
// Unknown function names return an error listing the valid set so users
// (and future-slice tests) get a clear signal.
func (gf *GraphFinder) View(q query.ViewQuery) (*query.ViewResult, error) {
	if gf.graph == nil {
		return nil, fmt.Errorf("graph is required")
	}

	// Pre-scan: if any section uses source(wip), resolve markers once and
	// reuse across sections. The finder holds them (or lazy-loads from the
	// held graph's directory); a layout that needs markers but has neither
	// fails fast with a single clear message.
	var wipMarkers []*model.WIPMarker
	if layoutHasWipSource(q.Layout) {
		markers, err := gf.WIPMarkers()
		if err != nil {
			return nil, fmt.Errorf("loading wip markers: %w", err)
		}
		if markers == nil && gf.wip == nil && gf.graph.GraphDir() == "" {
			return nil, fmt.Errorf("layout uses source(wip) but graph directory is not configured")
		}
		wipMarkers = markers
	}

	// Sections render independently — each surface carries its own
	// per-section metadata (state in focus-block, heat score in
	// ranked lists, kind header in grouped). Cross-section dedup is
	// captured as an open design question in s-cpt-tn0; the previous
	// AC 13 mechanism (focus → as-list strip) was removed pending
	// resolution of that question.
	now := time.Now()

	sections := make([]query.SectionResult, 0, len(q.Layout.Sections))
	for i, section := range q.Layout.Sections {
		sr, err := executeSection(gf.graph, wipMarkers, section, q.Budget, now)
		if err != nil {
			return nil, fmt.Errorf("section %d: %w", i+1, err)
		}
		sections = append(sections, sr)
	}

	return &query.ViewResult{Graph: gf.graph, Sections: sections}, nil
}

// layoutHasWipSource reports whether any section in the layout uses
// source(wip). Used by View() to amortise the disk read for wip markers
// across sections; the function is intentionally permissive about other
// validation (a malformed source(...) call is caught by executeSection
// when the section runs).
func layoutHasWipSource(layout model.Layout) bool {
	for _, section := range layout.Sections {
		for _, fn := range section.Functions {
			if fn.Name != "source" {
				continue
			}
			if name, err := parseSourceArg(fn.Args); err == nil && name == "wip" {
				return true
			}
		}
	}
	return false
}

// renderFunctions enumerates the function names that terminate a section,
// mapped to the shape they expect. Mismatch (e.g. as-grouped over a flat
// result, as-list over a grouped result) is the AC 16 render-shape error
// the executor emits before the presenter sees the data.
var renderFunctions = map[string]model.RenderShape{
	"as-bodies":             model.ShapeBodies,
	"as-list":               model.ShapeFlatList,
	"as-grouped":            model.ShapeGrouped,
	"as-counts":             model.ShapeCounts,
	"as-focus-block":        model.ShapeFocusBlock,
	"as-participants-block": model.ShapeParticipantsBlock,
	"as-wip-list":           model.ShapeWipList,
}

// knownFunctions lists every function name the executor recognizes. Used
// in the unknown-function error message so users see what's available.
var knownFunctions = []string{"source", "active", "indexed", "kind", "intent", "type", "layer", "since", "topic", "participant", "untagged", "id", "not", "n", "skip", "rank", "group", "expand", "name", "name-prefix", "stalled", "brief", "as-list", "as-grouped", "as-counts", "as-focus-block", "as-participants-block", "as-wip-list", "as-bodies"}

// ViewFunctionNames returns the function names accepted by the layout
// executor. Reference surfaces use this instead of maintaining their own
// vocabulary list.
func ViewFunctionNames() []string {
	return slices.Clone(knownFunctions)
}

// ViewRenderNames returns the render terminators accepted by the layout
// executor in deterministic order.
func ViewRenderNames() []string {
	names := make([]string, 0, len(renderFunctions))
	for name := range renderFunctions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// supportedNotInner lists the inner filter names accepted by `not(<inner>)`
// in d-tac-e1s's first cut. Pure set-shaped filters with unambiguous
// inverse semantics. active and since are deferred (closed-vs-superseded
// distinction, future-vs-past cutoff direction); nested not is rejected.
var supportedNotInner = []string{"kind", "intent", "layer", "topic"}

// knownSources is the user-facing list of valid `source(<name>)` arguments,
// shown in the unknown-source error message. Mirrors the data-source
// branches in executeSection: graph (default) and wip.
var knownSources = []string{"graph", "wip"}

// parseSourceArg validates the single argument to `source(<name>)`. Both
// bare identifier and quoted string forms are accepted so users can write
// either source(wip) or source("wip"). Unknown names return a listed-
// valid-set error matching the slice's other parser helpers.
func parseSourceArg(args []model.FunctionArg) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("source: requires exactly one argument (e.g. source(graph) or source(wip))")
	}
	a := args[0]
	if a.Kind != model.ArgKindIdent && a.Kind != model.ArgKindString {
		return "", fmt.Errorf("source: argument must be an identifier or string, got %s", a.Kind)
	}
	if slices.Contains(knownSources, a.String) {
		return a.String, nil
	}
	return "", fmt.Errorf("source: unknown source %q (known: %s)", a.String, strings.Join(knownSources, ", "))
}

// executeSection walks one section's pipeline left-to-right, accumulating
// each function's intent into one of these buckets:
//
//   - graph filter        (active, kind, layer, since, topic)
//   - rank                (rank(<algo>), last-write-wins)
//   - pagination          (n(N), last-write-wins)
//   - aggregation         (group(by(<field>)), last-write-wins)
//   - transform           (expand(<field>), last-write-wins)
//   - state classifier    (stalled(value), last-write-wins; focus-block only)
//   - section header      (name(<string>), last-write-wins)
//   - render terminator   (as-list / as-grouped / as-focus-block)
//
// Buckets apply in canonical filter → rank → page → group/expand → render
// order regardless of source ordering — last-write-wins per modifier kind,
// per d-tac-uww §2.
//
// Mutual exclusivity in this slice:
//   - group() with rank() or n() — per-group sort/pagination reserved
//   - expand(involvement) with rank() or n() or group() — focus-block has
//     no per-target ranking and can't aggregate
//
// now is the clock used for focus-block heat scoring (test determinism
// via injection).
func executeSection(g *model.Graph, wipMarkers []*model.WIPMarker, section model.Section, budget query.ViewBudget, now time.Time) (query.SectionResult, error) {
	if len(section.Functions) == 0 {
		return query.SectionResult{}, fmt.Errorf("empty section")
	}

	spec := newSectionSpec()
	for _, fn := range section.Functions {
		if err := parseSectionFunction(spec, fn); err != nil {
			return query.SectionResult{}, err
		}
	}
	if spec.render == "" {
		return query.SectionResult{}, fmt.Errorf(
			"section must end with a render function (one of: %s)", renderFunctionsList())
	}

	// Auto-derive section header (AC 14 per d-tac-jgi, refined by the
	// name-prefix primitive). The resolver composes macro-baked
	// name-prefix() with rank()'s suffix when both are set, falls back
	// to either alone, or to bare-rank's implicit "Top" prefix.
	// Explicit name(...) wins — already in spec.nameValue.
	spec.resolveSectionHeader()

	// Source-specific dispatch. source(wip) takes a fundamentally different
	// data path — markers from disk, not graph entries — and supports a
	// disjoint primitive set: only name() and as-wip-list compose. Filters,
	// rank, page, group, expand, and stalled are graph-side concepts that
	// don't translate to markers, so they error here with a clear pointer
	// rather than silently no-op.
	if spec.source == "wip" {
		if err := spec.rejectGraphPrimitivesForWip(); err != nil {
			return query.SectionResult{}, err
		}
		if spec.render != "as-wip-list" {
			return query.SectionResult{}, fmt.Errorf(
				"render-shape mismatch: source(wip) produces a wip-list result, but %s expects a different shape (use as-wip-list)",
				spec.render)
		}
		list := model.WipList{Markers: wipMarkers}
		if budget.GroupItems > 0 && len(list.Markers) > budget.GroupItems {
			list.Dropped = len(list.Markers) - budget.GroupItems
			list.Markers = list.Markers[:budget.GroupItems]
			list.Pull = section.Expr()
		}
		return query.SectionResult{
			Render: spec.render,
			Name:   spec.sectionName(),
			Data:   list,
		}, nil
	}

	if err := spec.validateMutualExclusion(); err != nil {
		return query.SectionResult{}, err
	}
	if err := spec.validateRenderShape(); err != nil {
		return query.SectionResult{}, err
	}

	// Apply intent in canonical pipeline order: filter → rank → page →
	// group → render. Within filter: GraphFilter (active, layer, type) →
	// kind disjunction → participant disjunction → since → topic. Order among
	// post-Graph.Filter() narrowings doesn't affect the result; chosen
	// here to keep cheaper structural checks before time/topic walks.
	entries := g.Filter(spec.filter)
	if spec.indexed {
		entries = model.FilterIndexed(entries)
	}
	for _, kinds := range spec.kindFilters {
		entries = filterByKinds(entries, kinds)
	}
	for _, names := range spec.participantFilters {
		entries = filterByParticipants(entries, names)
	}
	for _, intents := range spec.intentFilters {
		entries = filterByIntents(entries, intents)
	}
	if len(spec.excludeKinds) > 0 {
		entries = excludeByKinds(entries, spec.excludeKinds)
	}
	if len(spec.excludeIntents) > 0 {
		entries = excludeByIntents(entries, spec.excludeIntents)
	}
	if spec.excludeLayer != "" {
		entries = excludeByLayer(entries, spec.excludeLayer)
	}
	if spec.sinceCutoff != nil {
		entries = filterBySince(entries, *spec.sinceCutoff)
	}
	if !spec.topicPrefix.IsZero() {
		entries = TopicFilter{Prefix: spec.topicPrefix}.FilterEntries(g, entries)
	}
	if !spec.excludeTopicPrefix.IsZero() {
		entries = TopicFilter{Prefix: spec.excludeTopicPrefix}.ExcludeEntries(g, entries)
	}
	if spec.untagged {
		entries = filterUntagged(g, entries)
	}
	if len(spec.idFilter) > 0 {
		filtered, err := filterByIDs(g, entries, spec.idFilter)
		if err != nil {
			return query.SectionResult{}, err
		}
		entries = filtered
	}

	// as-counts aggregates the filtered entry set into per-topic rows. It
	// consumes the entries directly (no rank/page/group/expand — those are
	// rejected for as-counts in validateMutualExclusion), so it returns here
	// before the ranking pipeline. now is the injected clock for heat scoring.
	if spec.render == "as-counts" {
		return query.SectionResult{
			Render: spec.render,
			Name:   spec.sectionName(),
			Data:   g.TopicCounts(entries, now),
		}, nil
	}

	var scores []float64
	if spec.rank != nil {
		// Use the section's injected clock, not a fresh time.Now(): coldness
		// scores the entry's own age, so ranking and the focus-block scorer
		// below must share one instant within a single View call.
		entries, scores = applyRanking(g, entries, spec.rank, now)
	}
	if spec.skipN > 0 {
		if spec.skipN >= len(entries) {
			entries, scores = nil, nil
		} else {
			entries = entries[spec.skipN:]
			if scores != nil {
				scores = scores[spec.skipN:]
			}
		}
	}
	if spec.pageN >= 0 && len(entries) > spec.pageN {
		entries = entries[:spec.pageN]
		if scores != nil {
			scores = scores[:spec.pageN]
		}
	}

	// as-participants-block sources from the graph's actor-identity chains
	// rather than the section's filter chain. Filters narrow which actors
	// surface (intersection with active heads); the role cascade is always
	// derived from full chain history per d-cpt-d34. Empty intersection
	// renders as an empty block — the renderer suppresses the header so an
	// actorless filter stays quiet rather than producing a bare title.
	if spec.render == "as-participants-block" {
		block := participantsBlockFromEntries(g, entries)
		if budget.GroupItems > 0 && len(block.Groups) > budget.GroupItems {
			block.Dropped = len(block.Groups) - budget.GroupItems
			block.Groups = block.Groups[:budget.GroupItems]
			block.Pull = section.Expr()
		}
		return query.SectionResult{
			Render: spec.render,
			Name:   spec.sectionName(),
			Data:   block,
			Brief:  spec.brief,
		}, nil
	}

	// as-bodies serves the entries themselves, so the pipeline hands over the
	// ranked and paged set unchanged — the render composes each body into the
	// surrounding document's heading hierarchy.
	if spec.render == "as-bodies" {
		bodies := model.Bodies{Entries: entries}
		if budget.BodyBytes > 0 {
			// Whole bodies while bytes fit — a body cut mid-way destroys the
			// content's purpose, so the unit is the entry (d-tac-rzi).
			kept, total := 0, 0
			for _, e := range entries {
				n := len(e.Content)
				if total+n > budget.BodyBytes {
					break
				}
				kept++
				total += n
			}
			if kept < len(entries) {
				bodies.Dropped = len(entries) - kept
				bodies.Entries = entries[:kept]
				bodies.Pull = section.Expr()
			}
		}
		return query.SectionResult{
			Render: spec.render,
			Name:   spec.sectionName(),
			Data:   bodies,
		}, nil
	}

	if spec.groupField != "" {
		groups, err := groupBy(entries, spec.groupField)
		if err != nil {
			return query.SectionResult{}, fmt.Errorf("group: %w", err)
		}
		return query.SectionResult{
			Render: spec.render,
			Name:   spec.sectionName(),
			Data:   model.Grouped{Field: spec.groupField, Groups: groups},
			Brief:  spec.brief,
		}, nil
	}
	if spec.expandField == "involvement" {
		// `entries` here is the filtered focus list; expand it into a
		// FocusBlock by walking each focus's involvement and resolving
		// targets. Score is fixed at heat(exp-14d) per slice-7 design;
		// stalled threshold takes the user-supplied value if set.
		block := expandInvolvement(g, entries, focusBlockScorer(g, now), spec.stalledThreshold)
		if budget.GroupItems > 0 && len(block.Focuses) > budget.GroupItems {
			block.Dropped = len(block.Focuses) - budget.GroupItems
			block.Focuses = block.Focuses[:budget.GroupItems]
			block.Pull = section.Expr()
		}
		return query.SectionResult{
			Render: spec.render,
			Name:   spec.sectionName(),
			Data:   block,
			Brief:  spec.brief,
		}, nil
	}
	// Flat-list output: each section renders independently. Cross-
	// section repetition (same entry in focus-block and top-N) is
	// intentional under the current design pending resolution of
	// s-cpt-tn0; readers see per-section metadata for each occurrence.
	//
	// expand(refs) attaches per-entry ref sub-lines after ranking and
	// pagination so the expansion follows the already-narrowed entry set
	// (the as-list render path consumes RefExpansions when present).
	flat := model.FlatList{Entries: entries, Scores: scores}
	if spec.expandField == "refs" {
		flat.RefExpansions = expandRefs(g, entries, spec.expandRefsInactive)
		if budget.RefsPerEntry > 0 {
			flat.RefExpansionDropped = make([]int, len(flat.RefExpansions))
			for i, refs := range flat.RefExpansions {
				if len(refs) > budget.RefsPerEntry {
					flat.RefExpansionDropped[i] = len(refs) - budget.RefsPerEntry
					flat.RefExpansions[i] = refs[:budget.RefsPerEntry]
				}
			}
		}
	}
	return query.SectionResult{
		Render: spec.render,
		Name:   spec.sectionName(),
		Data:   flat,
		Brief:  spec.brief,
	}, nil
}

// parseSectionFunction folds one source-order function call into the
// spec, returning a parse error with a primitive-prefixed message when
// the call is malformed. Each branch keeps its bucket assignment local
// to the case so adding a new primitive lands in one place. Unknown
// names fall through to the unknown-function error.
func parseSectionFunction(spec *sectionSpec, fn model.Function) error {
	switch {
	case fn.Name == "source":
		s, err := parseSourceArg(fn.Args)
		if err != nil {
			return err
		}
		spec.source = s

	case fn.Name == "active":
		if len(fn.Args) > 0 {
			return fmt.Errorf("active takes no arguments")
		}
		spec.filter.OpenOnly = true

	case fn.Name == "indexed":
		if len(fn.Args) > 0 {
			return fmt.Errorf("indexed takes no arguments")
		}
		spec.indexed = true

	case fn.Name == "kind":
		kinds, err := parseKindArgs(fn.Args)
		if err != nil {
			return fmt.Errorf("kind: %w", err)
		}
		// Each kind() call is a disjunction; storing them as separate
		// sets lets the application stage intersect across calls per
		// d-tac-uww §2.
		spec.kindFilters = append(spec.kindFilters, kinds)

	case fn.Name == "intent":
		intents, err := parseIntentArgs(fn.Args)
		if err != nil {
			return fmt.Errorf("intent: %w", err)
		}
		// Mirrors kind(): each intent() call is a disjunction, multiple
		// calls intersect at the apply stage.
		spec.intentFilters = append(spec.intentFilters, intents)

	case fn.Name == "layer":
		if len(fn.Args) != 1 {
			return fmt.Errorf("layer: requires exactly one argument (e.g. layer(tac) or layer(tactical))")
		}
		s, err := argString(fn.Args[0])
		if err != nil {
			return fmt.Errorf("layer: %w", err)
		}
		if abbrev, ok := model.LayerFromAbbrev[s]; ok {
			spec.filter.Layer = abbrev
		} else {
			spec.filter.Layer = model.Layer(s)
		}

	case fn.Name == "type":
		if len(fn.Args) != 1 {
			return fmt.Errorf("type: requires exactly one argument (e.g. type(d) or type(signal))")
		}
		s, err := argString(fn.Args[0])
		if err != nil {
			return fmt.Errorf("type: %w", err)
		}
		// Resolve the abbrev form (d/s) to the canonical EntryType; full
		// names (decision/signal) pass through. g.Filter applies the
		// resulting spec.filter.Type, so no apply-stage wiring is needed —
		// this mirrors layer() onto the shared GraphFilter.
		if abbrev, ok := model.TypeFromAbbrev[s]; ok {
			spec.filter.Type = abbrev
		} else {
			spec.filter.Type = model.EntryType(s)
		}

	case fn.Name == "since":
		if len(fn.Args) != 1 {
			return fmt.Errorf("since: requires exactly one argument (e.g. since(\"7d\") or since(\"2026-04-01\"))")
		}
		s, err := argString(fn.Args[0])
		if err != nil {
			return fmt.Errorf("since: %w", err)
		}
		cutoff, err := model.ResolveSinceSpec(s, time.Now())
		if err != nil {
			return err
		}
		spec.sinceCutoff = &cutoff

	case fn.Name == "topic":
		if len(fn.Args) != 1 {
			return fmt.Errorf("topic: requires exactly one argument (e.g. topic(catch-up-scaling) or topic(\"infrastructure/cli\"))")
		}
		s, err := argString(fn.Args[0])
		if err != nil {
			return fmt.Errorf("topic: %w", err)
		}
		path, err := model.ParseTopicPath(s)
		if err != nil {
			return fmt.Errorf("topic: %w", err)
		}
		spec.topicPrefix = path

	case fn.Name == "participant":
		names, err := parseParticipantArgs(fn.Args)
		if err != nil {
			return fmt.Errorf("participant: %w", err)
		}
		// Each participant() call is a disjunction; storing them as
		// separate sets lets the application stage intersect across
		// calls, mirroring kind() (d-tac-uww §2).
		spec.participantFilters = append(spec.participantFilters, names)

	case fn.Name == "untagged":
		if len(fn.Args) > 0 {
			return fmt.Errorf("untagged takes no arguments")
		}
		spec.untagged = true

	case fn.Name == "id":
		ids, err := parseIDArgs(fn.Args)
		if err != nil {
			return fmt.Errorf("id: %w", err)
		}
		// Multiple id() calls accumulate into one selection set — the
		// realistic use is a single id(a,b,c) call, but repeats union
		// rather than surprising the user with an empty intersection.
		spec.idFilter = append(spec.idFilter, ids...)

	case fn.Name == "not":
		if err := parseNotArgs(spec, fn.Args); err != nil {
			return fmt.Errorf("not: %w", err)
		}

	case fn.Name == "rank":
		rank, err := parseRankArg(fn.Args)
		if err != nil {
			return err
		}
		spec.rank = rank

	case fn.Name == "n":
		page, err := parseIntegerArg("n", fn.Args)
		if err != nil {
			return err
		}
		spec.pageN = page

	case fn.Name == "skip":
		skip, err := parseIntegerArg("skip", fn.Args)
		if err != nil {
			return err
		}
		spec.skipN = skip

	case fn.Name == "group":
		field, err := parseGroupArgs(fn.Args)
		if err != nil {
			return err
		}
		spec.groupField = field

	case fn.Name == "name":
		s, err := parseNameArgs(fn.Args)
		if err != nil {
			return err
		}
		spec.nameValue = s
		spec.nameSet = true

	case fn.Name == "name-prefix":
		s, err := parseNameArgs(fn.Args)
		if err != nil {
			return fmt.Errorf("name-prefix: %s", strings.TrimPrefix(err.Error(), "name: "))
		}
		spec.prefixValue = s
		spec.prefixSet = true

	case fn.Name == "expand":
		field, inactive, err := parseExpandArgs(fn.Args)
		if err != nil {
			return err
		}
		spec.expandField = field
		spec.expandRefsInactive = inactive

	case fn.Name == "stalled":
		v, err := parseStalledArgs(fn.Args)
		if err != nil {
			return err
		}
		spec.stalledThreshold = v
		spec.stalledSet = true

	case fn.Name == "brief":
		if len(fn.Args) > 0 {
			return fmt.Errorf("brief takes no arguments")
		}
		spec.brief = true

	case isRenderFunction(fn.Name):
		// Render is treated like other non-filter modifiers: last-write-
		// wins per d-tac-uww §2. This lets macro expansion + user
		// modifier append work — e.g. `top(N)`'s `as-list` lands inside
		// the expansion, then user `:rank(...)` appends after it without
		// erroring on syntactic position. The canonical bucket order
		// (filter → rank → page → group → render) means the "render is
		// the terminus" property holds semantically regardless of where
		// the render token sits in source order.
		if len(fn.Args) > 0 {
			return fmt.Errorf("%s takes no arguments", fn.Name)
		}
		spec.render = fn.Name

	default:
		// List macros alongside primitives: a wrong guess like recent(15)
		// is often a reach for a macro (top, focus, done, …), and the
		// primitive-only list left that vocabulary undiscoverable.
		return fmt.Errorf(
			"unknown function %q (known primitives: %s; macros, valid at section start: %s)",
			fn.Name, strings.Join(knownFunctions, ", "), strings.Join(query.MacroNames(), ", "))
	}
	return nil
}

// participantsBlockFromEntries builds a ParticipantsBlock from the given
// (filtered) entry set, retaining only entries that are active actor
// heads. The role cascade is derived from full chain history per
// d-cpt-d34 — a role captured against a within-chain canonical
// correction still binds to the current head, regardless of which
// canonical it stored.
//
// Filter chain semantics: passing every active actor head through the
// filters produces the canonical block. Narrowing (e.g. by canonical
// match through some future filter) yields the subset; an empty
// intersection produces an empty block (renderer suppresses output).
func participantsBlockFromEntries(g *model.Graph, entries []*model.Entry) model.ParticipantsBlock {
	heads := g.ActiveActorHeads()
	if len(heads) == 0 {
		return model.ParticipantsBlock{}
	}
	// Build the set of entry IDs that survived the filter chain. Empty
	// slice means "no narrowing applied yet" — fall back to all heads.
	var allowed map[string]struct{}
	if entries != nil {
		allowed = make(map[string]struct{}, len(entries))
		for _, e := range entries {
			allowed[e.ID] = struct{}{}
		}
	}
	roles := g.ActiveRoles()
	groups := make([]model.ParticipantsGroup, 0, len(heads))
	for _, a := range heads {
		if allowed != nil {
			if _, ok := allowed[a.ID]; !ok {
				continue
			}
		}
		var bound []*model.Entry
		for _, r := range roles {
			chain := g.ResolveRoleChain(r)
			if chain != nil && chain.Head != nil && chain.Head.ID == a.ID {
				bound = append(bound, r)
			}
		}
		groups = append(groups, model.ParticipantsGroup{Actor: a, Roles: bound})
	}
	return model.ParticipantsBlock{Groups: groups}
}

// parseExpandArgs validates the `expand(<field>)` primitive's argument and
// returns the field name plus, for the refs case, whether the optional
// inactive filter is set. Three forms are recognized:
//
//   - expand(involvement)     — the focus-block transform (as-focus-block)
//   - expand(refs)            — per-row ref sub-lines on as-list (all refs)
//   - expand(refs(inactive))  — narrows to refs whose target is currently
//     inactive (closed, superseded, or a non-active role) — the inverse of
//     the `active` filter, for the lean catch-up mode
//
// The nested-call form follows the existing pattern (rank(heat(exp-14d)),
// group(by(<field>))). Unknown fields and malformed nested calls error with
// a listed-valid-set message so users see what's available.
func parseExpandArgs(args []model.FunctionArg) (field string, refsInactive bool, err error) {
	if len(args) != 1 {
		return "", false, fmt.Errorf("expand: requires exactly one field argument (e.g. expand(involvement) or expand(refs))")
	}
	a := args[0]
	switch a.Kind {
	case model.ArgKindIdent, model.ArgKindString:
		switch a.String {
		case "involvement":
			return "involvement", false, nil
		case "refs":
			return "refs", false, nil
		default:
			return "", false, fmt.Errorf("expand: unknown field %q (known: involvement, refs)", a.String)
		}
	case model.ArgKindFunc:
		// Nested-call form. Only expand(refs(inactive)) is defined;
		// involvement takes no filter argument.
		inner := a.Func
		if inner == nil || inner.Name != "refs" {
			return "", false, fmt.Errorf("expand: nested-call form is only valid as expand(refs(inactive))")
		}
		if len(inner.Args) != 1 {
			return "", false, fmt.Errorf("expand: refs(...) takes exactly one filter argument (e.g. expand(refs(inactive)))")
		}
		fa := inner.Args[0]
		if (fa.Kind != model.ArgKindIdent && fa.Kind != model.ArgKindString) || fa.String != "inactive" {
			return "", false, fmt.Errorf("expand: refs(...) filter must be inactive (e.g. expand(refs(inactive)))")
		}
		return "refs", true, nil
	default:
		return "", false, fmt.Errorf("expand: argument must be an identifier or a nested filter call, got %s", a.Kind)
	}
}

// parseStalledArgs validates the `stalled(<value>)` modifier. The
// argument is a numeric threshold; non-negative floats are accepted
// (zero is valid — "anything below zero heat is stalled" reduces to
// "stalled never fires"). Strings/idents reject so users don't pass
// names by mistake.
func parseStalledArgs(args []model.FunctionArg) (float64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("stalled: requires exactly one numeric argument (e.g. stalled(1.0))")
	}
	a := args[0]
	if a.Kind != model.ArgKindNumber {
		return 0, fmt.Errorf("stalled: argument must be a number, got %s", a.Kind)
	}
	if a.Number < 0 {
		return 0, fmt.Errorf("stalled: argument must be non-negative, got %v", a.Number)
	}
	return a.Number, nil
}

// isRenderFunction reports whether name terminates a section. Reads the
// renderFunctions map directly so adding a render is a one-entry change.
func isRenderFunction(name string) bool {
	_, ok := renderFunctions[name]
	return ok
}

// renderFunctionsList returns the comma-separated list of render names
// for inclusion in error messages. Iteration order over the map is not
// stable, so the slice is sorted for deterministic test output.
func renderFunctionsList() string {
	return strings.Join(ViewRenderNames(), ", ")
}

// parseNameArgs validates name()'s single string argument. Empty strings
// are accepted — `name("")` clears any prior name() in the section
// (last-write-wins). Identifiers are accepted interchangeably with
// strings to keep the call site flexible: `name(Aspirations)` and
// `name("Aspirations")` are equivalent.
func parseNameArgs(args []model.FunctionArg) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("name: requires exactly one string argument (e.g. name(\"Top\"))")
	}
	a := args[0]
	switch a.Kind {
	case model.ArgKindString, model.ArgKindIdent:
		return a.String, nil
	default:
		return "", fmt.Errorf("name: argument must be a string or identifier, got %s", a.Kind)
	}
}

// parseGroupArgs validates and extracts the field name from group()'s
// argument. Per d-tac-3pq, the only accepted form is `group(by(<field>))`
// — bare `group(field)`, string-arg `group("field")`, and other shapes
// error with a clear pointer to the nested by(...) form.
func parseGroupArgs(args []model.FunctionArg) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf(
			"group: requires exactly one argument of the form by(<field>) (e.g. group(by(kind)))")
	}
	a := args[0]
	if a.Kind != model.ArgKindFunc || a.Func == nil || a.Func.Name != "by" {
		return "", fmt.Errorf(
			"group: argument must be the marker call by(<field>), got %s (e.g. group(by(kind)))", a.Kind)
	}
	inner := a.Func
	if len(inner.Args) != 1 {
		return "", fmt.Errorf(
			"group: by(...) requires exactly one field name (e.g. group(by(kind)))")
	}
	fieldArg := inner.Args[0]
	switch fieldArg.Kind {
	case model.ArgKindIdent, model.ArgKindString:
		return fieldArg.String, nil
	default:
		return "", fmt.Errorf(
			"group: by()'s argument must be an identifier or string, got %s", fieldArg.Kind)
	}
}

// filterBySince returns entries whose creation time is on or after the
// cutoff. Entries with zero Time fall through (kept) so synthetic test
// fixtures without explicit times don't get silently dropped.
func filterBySince(entries []*model.Entry, cutoff time.Time) []*model.Entry {
	var out []*model.Entry
	for _, e := range entries {
		if e.Time.IsZero() || !e.Time.Before(cutoff) {
			out = append(out, e)
		}
	}
	return out
}

// parseKindArgs validates kind()'s args and returns the kind values.
// Identifier and string args are interchangeable so users can write
// either kind(plan) or kind("plan"). Returning multiple kinds models the
// disjunction: kind(plan, directive) means "plan OR directive".
// parseParticipantArgs reads the names in a participant(...) call. Each
// arg is a canonical participant string — bare idents for single-word
// names (participant(Christopher)) or quoted strings for names with
// spaces (participant("Jonathan Philipp")). Mirrors parseKindArgs.
func parseParticipantArgs(args []model.FunctionArg) ([]string, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("requires at least one participant argument (e.g. participant(Christopher) or participant(\"Jonathan Philipp\"))")
	}
	out := make([]string, 0, len(args))
	for i, a := range args {
		switch a.Kind {
		case model.ArgKindIdent, model.ArgKindString:
			out = append(out, a.String)
		default:
			return nil, fmt.Errorf("argument %d must be an identifier or string, got %s", i+1, a.Kind)
		}
	}
	return out, nil
}

func parseKindArgs(args []model.FunctionArg) ([]model.Kind, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("requires at least one kind argument (e.g. kind(plan) or kind(plan,directive))")
	}
	out := make([]model.Kind, 0, len(args))
	for i, a := range args {
		switch a.Kind {
		case model.ArgKindIdent, model.ArgKindString:
			out = append(out, model.Kind(a.String))
		default:
			return nil, fmt.Errorf("argument %d must be an identifier or string, got %s", i+1, a.Kind)
		}
	}
	return out, nil
}

// parseIntentArgs validates intent()'s args and returns the intent values.
// Mirrors parseKindArgs: idents and strings interchange (intent(guiding) or
// intent("guiding")) and multiple args model a disjunction. Each value is
// checked against the closed set so a typo fails loudly rather than silently
// matching nothing — intent has only three values, so a wrong one is almost
// always a mistake, not a yet-unseen entry kind.
func parseIntentArgs(args []model.FunctionArg) ([]model.Intent, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("requires at least one intent argument (e.g. intent(pending) or intent(pending,guiding))")
	}
	out := make([]model.Intent, 0, len(args))
	for i, a := range args {
		switch a.Kind {
		case model.ArgKindIdent, model.ArgKindString:
			if !model.IsValidIntent(a.String) {
				return nil, fmt.Errorf("argument %d %q is not a valid intent (expected pending, guiding, or settled)", i+1, a.String)
			}
			out = append(out, model.Intent(a.String))
		default:
			return nil, fmt.Errorf("argument %d must be an identifier or string, got %s", i+1, a.Kind)
		}
	}
	return out, nil
}

// parseIDArgs validates id()'s args and returns the raw ID strings.
// Identifier and string args are interchangeable so users can write either
// id(20260520-131326-d-tac-6tz) or id("d-tac-6tz"). Resolution to full IDs
// (and ambiguity/missing handling) happens at apply time against the graph.
func parseIDArgs(args []model.FunctionArg) ([]string, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("requires at least one entry ID argument (e.g. id(20260520-131326-d-tac-6tz) or id(d-tac-6tz,d-cpt-ni0))")
	}
	out := make([]string, 0, len(args))
	for i, a := range args {
		switch a.Kind {
		case model.ArgKindIdent, model.ArgKindString:
			out = append(out, a.String)
		default:
			return nil, fmt.Errorf("argument %d must be an entry ID identifier or string, got %s", i+1, a.Kind)
		}
	}
	return out, nil
}

// parseIntegerArg validates a single non-negative integer argument. Used
// by n(N) and any future paging/numeric primitives where fractional or
// negative values would be meaningless.
func parseIntegerArg(name string, args []model.FunctionArg) (int, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("%s requires exactly one argument, got %d", name, len(args))
	}
	a := args[0]
	if a.Kind != model.ArgKindNumber {
		return 0, fmt.Errorf("%s argument must be a number, got %s", name, a.Kind)
	}
	if a.Number != float64(int64(a.Number)) {
		return 0, fmt.Errorf("%s argument must be an integer, got %v", name, a.Number)
	}
	if a.Number < 0 {
		return 0, fmt.Errorf("%s argument must be non-negative, got %v", name, a.Number)
	}
	return int(a.Number), nil
}

// filterByKinds returns entries whose Kind is in the given disjunction
// set. Kept in the view executor rather than extending model.GraphFilter
// to avoid churning a model type shared with sdd search and others —
// kind disjunction is specific to view's pipeline composition.
func filterByKinds(entries []*model.Entry, kinds []model.Kind) []*model.Entry {
	set := make(map[model.Kind]struct{}, len(kinds))
	for _, k := range kinds {
		set[k] = struct{}{}
	}
	var out []*model.Entry
	for _, e := range entries {
		if _, ok := set[e.Kind]; ok {
			out = append(out, e)
		}
	}
	return out
}

// filterByParticipants keeps entries listing at least one of names in their
// participants (disjunction within a single participant() call). Matching is
// exact against the canonical strings stored on the entry — the participants
// field carries canonicals only (d-cpt-979), so a canonical identifies an
// author unambiguously without alias resolution.
func filterByParticipants(entries []*model.Entry, names []string) []*model.Entry {
	want := make(map[string]struct{}, len(names))
	for _, n := range names {
		want[n] = struct{}{}
	}
	var out []*model.Entry
	for _, e := range entries {
		for _, p := range e.Participants {
			if _, ok := want[p]; ok {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// filterUntagged returns entries whose effective topic set is empty — the
// inverse of "has any topic". Relies on the same EffectiveTopics merge as the
// topic() filter, so both inline topics and annotation memberships count as
// "tagged". Used by the `untagged` primitive to surface entries that escaped
// topic assignment — the grooming/backfill entry point.
func filterUntagged(g *model.Graph, entries []*model.Entry) []*model.Entry {
	var out []*model.Entry
	for _, e := range entries {
		if len(g.EffectiveTopics(e)) == 0 {
			out = append(out, e)
		}
	}
	return out
}

// filterByIDs resolves the raw id() arguments against the graph (short-form
// to full-form, surfacing ambiguity) and returns the subset of entries whose
// ID is in the resolved set. Order follows the input entry set, not the id()
// argument order — id() is a filter, not a re-sort. An ID that resolves to
// nothing simply matches no entry.
func filterByIDs(g *model.Graph, entries []*model.Entry, rawIDs []string) ([]*model.Entry, error) {
	resolved, err := g.ResolveIDs(rawIDs)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(resolved))
	for _, id := range resolved {
		set[id] = struct{}{}
	}
	var out []*model.Entry
	for _, e := range entries {
		if _, ok := set[e.ID]; ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// excludeByKinds returns entries whose Kind is NOT in the exclusion set.
// filterByIntents keeps entries whose Intent is in the disjunction set. Only
// directives carry an intent, so a non-empty filter implicitly narrows to
// directives — every other entry has an empty Intent and never matches a
// listed value. Mirror of filterByKinds (d-tac-n9k).
func filterByIntents(entries []*model.Entry, intents []model.Intent) []*model.Entry {
	set := make(map[model.Intent]struct{}, len(intents))
	for _, in := range intents {
		set[in] = struct{}{}
	}
	var out []*model.Entry
	for _, e := range entries {
		if _, ok := set[e.Intent]; ok {
			out = append(out, e)
		}
	}
	return out
}

// excludeByIntents drops entries whose Intent is in the set — the not(intent())
// negation. Entries without an intent (every non-directive, plus unspecified
// directives) are kept, since their empty Intent is never in the set.
func excludeByIntents(entries []*model.Entry, intents []model.Intent) []*model.Entry {
	if len(intents) == 0 {
		return entries
	}
	set := make(map[model.Intent]struct{}, len(intents))
	for _, in := range intents {
		set[in] = struct{}{}
	}
	var out []*model.Entry
	for _, e := range entries {
		if _, ok := set[e.Intent]; !ok {
			out = append(out, e)
		}
	}
	return out
}

// Mirror of filterByKinds for the not(kind(...)) negation primitive.
func excludeByKinds(entries []*model.Entry, kinds []model.Kind) []*model.Entry {
	if len(kinds) == 0 {
		return entries
	}
	set := make(map[model.Kind]struct{}, len(kinds))
	for _, k := range kinds {
		set[k] = struct{}{}
	}
	var out []*model.Entry
	for _, e := range entries {
		if _, ok := set[e.Kind]; !ok {
			out = append(out, e)
		}
	}
	return out
}

// excludeByLayer returns entries that are NOT at the given layer. Mirror
// of the positive layer() filter for the not(layer(...)) primitive.
func excludeByLayer(entries []*model.Entry, layer model.Layer) []*model.Entry {
	if layer == "" {
		return entries
	}
	var out []*model.Entry
	for _, e := range entries {
		if e.Layer != layer {
			out = append(out, e)
		}
	}
	return out
}

// parseNotArgs validates a `not(<inner-filter>)` call and dispatches the
// inner filter into the corresponding exclusion slot on spec. The argument
// must be exactly one nested function call whose name is in the supported
// inner set (kind, layer, topic). Active, since, and nested not are
// rejected with the listed-supported-set error so users see what's
// available rather than guessing.
func parseNotArgs(spec *sectionSpec, args []model.FunctionArg) error {
	if len(args) != 1 {
		return fmt.Errorf("requires exactly one filter argument, e.g. not(kind(contract,aspiration))")
	}
	a := args[0]
	if a.Kind != model.ArgKindFunc || a.Func == nil {
		// A bare identifier (e.g. not(active)) lands here too — the parser
		// treats names without parens as idents. Surface the supported-
		// inner set in the same message so users see what's accepted
		// regardless of which shape they tried.
		return fmt.Errorf("argument must be a filter call (e.g. not(kind(contract,aspiration))); supported inner: %s", strings.Join(supportedNotInner, ", "))
	}
	inner := *a.Func
	switch inner.Name {
	case "kind":
		kinds, err := parseKindArgs(inner.Args)
		if err != nil {
			return fmt.Errorf("kind: %w", err)
		}
		// Multiple not(kind(...)) calls union their exclusion sets — the
		// flat slice captures every kind to drop, regardless of how many
		// not() calls contributed which kinds.
		spec.excludeKinds = append(spec.excludeKinds, kinds...)
	case "intent":
		intents, err := parseIntentArgs(inner.Args)
		if err != nil {
			return fmt.Errorf("intent: %w", err)
		}
		spec.excludeIntents = append(spec.excludeIntents, intents...)
	case "layer":
		if len(inner.Args) != 1 {
			return fmt.Errorf("layer: requires exactly one argument (e.g. not(layer(stg)))")
		}
		s, err := argString(inner.Args[0])
		if err != nil {
			return fmt.Errorf("layer: %w", err)
		}
		if abbrev, ok := model.LayerFromAbbrev[s]; ok {
			spec.excludeLayer = abbrev
		} else {
			spec.excludeLayer = model.Layer(s)
		}
	case "topic":
		if len(inner.Args) != 1 {
			return fmt.Errorf("topic: requires exactly one argument (e.g. not(topic(\"infrastructure/cli\")))")
		}
		s, err := argString(inner.Args[0])
		if err != nil {
			return fmt.Errorf("topic: %w", err)
		}
		path, err := model.ParseTopicPath(s)
		if err != nil {
			return fmt.Errorf("topic: %w", err)
		}
		spec.excludeTopicPrefix = path
	default:
		return fmt.Errorf("unsupported inner filter %q (supported: %s)", inner.Name, strings.Join(supportedNotInner, ", "))
	}
	return nil
}
