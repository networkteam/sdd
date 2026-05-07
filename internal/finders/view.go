package finders

import (
	"fmt"
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
// Currently implemented vocabulary (d-tac-uww, slices 1-4):
//
//   - Filters: active, kind(K[, K2, ...]), layer(L), since(spec), topic(L)
//   - Rank:    rank(<algorithm>) with heat/in-degree/mult/add/log/by(date)
//     and decay names exp-/linear-{7,14,30}d, none
//   - Page:    n(N)
//   - Render:  as-list
//
// Unknown function names return an error listing the valid set so users
// (and future-slice tests) get a clear signal.
func (f *Finder) View(q query.ViewQuery) (*query.ViewResult, error) {
	if q.Graph == nil {
		return nil, fmt.Errorf("graph is required")
	}

	// shownIDs tracks every entry surfaced in an as-focus-block section so
	// far. Subsequent as-list sections strip these IDs out — AC 13's
	// "as-list deduplicates entries already shown in any focus block in
	// the same layout." Sections execute in source order so a focus block
	// appearing later can't influence an earlier as-list (the focus macro
	// is conventionally first; users who place it after as-list sections
	// see the natural ordering reflected).
	shownIDs := make(map[string]struct{})
	now := time.Now()

	sections := make([]query.SectionResult, 0, len(q.Layout.Sections))
	for i, section := range q.Layout.Sections {
		sr, err := executeSection(q.Graph, section, shownIDs, now)
		if err != nil {
			return nil, fmt.Errorf("section %d: %w", i+1, err)
		}
		// Update shownIDs from this section's focus-block targets so
		// later as-list sections can strip them.
		if fb, ok := sr.Data.(model.FocusBlock); ok {
			for id := range fb.TargetIDs() {
				shownIDs[id] = struct{}{}
			}
		}
		sections = append(sections, sr)
	}

	return &query.ViewResult{Graph: q.Graph, Sections: sections}, nil
}

// renderFunctions enumerates the function names that terminate a section,
// mapped to the shape they expect. Mismatch (e.g. as-grouped over a flat
// result, as-list over a grouped result) is the AC 16 render-shape error
// the executor emits before the presenter sees the data.
//
// Slice 8 will add as-participants-block / as-wip-list.
var renderFunctions = map[string]model.RenderShape{
	"as-list":        model.ShapeFlatList,
	"as-grouped":     model.ShapeGrouped,
	"as-focus-block": model.ShapeFocusBlock,
}

// knownFunctions lists every function name the executor recognizes. Used
// in the unknown-function error message so users see what's available.
var knownFunctions = []string{"active", "kind", "layer", "since", "topic", "n", "rank", "group", "expand", "name", "stalled", "as-list", "as-grouped", "as-focus-block"}

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
// shownIDs is the running set of entry IDs already surfaced in earlier
// as-focus-block sections; flat-list sections drop these entries (AC 13
// dedup). now is the clock used for focus-block heat scoring (test
// determinism via injection).
func executeSection(g *model.Graph, section model.Section, shownIDs map[string]struct{}, now time.Time) (query.SectionResult, error) {
	if len(section.Functions) == 0 {
		return query.SectionResult{}, fmt.Errorf("empty section")
	}

	var (
		filter           model.GraphFilter
		kindFilters      [][]model.Kind // each kind() call is a disjunction set; multiple calls intersect (d-tac-uww §2)
		sinceCutoff      *time.Time     // pointer so we can distinguish "no since()" from "since(0d)"
		topicPrefix      model.TopicPath
		rankPlan         *rankSpec // last-write-wins per d-tac-uww §2
		pageN            = -1      // -1 = no page limit
		groupField       string    // empty = no group; non-empty = group(by(<field>))
		expandField      string    // empty = no expand; non-empty = expand(<field>) (slice 7: only "involvement")
		stalledThreshold = DefaultStalledThreshold
		stalledSet       bool   // user-supplied threshold via stalled(value)
		nameSet          bool   // tracks whether name(...) was called (allows last-write-wins on empty string)
		nameValue        string // section header from name(<string>)
		renderName       string
	)

	for _, fn := range section.Functions {
		switch {
		case fn.Name == "active":
			if len(fn.Args) > 0 {
				return query.SectionResult{}, fmt.Errorf("active takes no arguments")
			}
			filter.OpenOnly = true

		case fn.Name == "kind":
			kinds, err := parseKindArgs(fn.Args)
			if err != nil {
				return query.SectionResult{}, fmt.Errorf("kind: %w", err)
			}
			// Each kind() call is a disjunction; storing them as separate
			// sets lets the application stage intersect across calls per
			// d-tac-uww §2.
			kindFilters = append(kindFilters, kinds)

		case fn.Name == "layer":
			if len(fn.Args) != 1 {
				return query.SectionResult{}, fmt.Errorf("layer: requires exactly one argument (e.g. layer(tac) or layer(tactical))")
			}
			s, err := argString(fn.Args[0])
			if err != nil {
				return query.SectionResult{}, fmt.Errorf("layer: %w", err)
			}
			if abbrev, ok := model.LayerFromAbbrev[s]; ok {
				filter.Layer = abbrev
			} else {
				filter.Layer = model.Layer(s)
			}

		case fn.Name == "since":
			if len(fn.Args) != 1 {
				return query.SectionResult{}, fmt.Errorf("since: requires exactly one argument (e.g. since(\"7d\") or since(\"2026-04-01\"))")
			}
			s, err := argString(fn.Args[0])
			if err != nil {
				return query.SectionResult{}, fmt.Errorf("since: %w", err)
			}
			cutoff, err := model.ResolveSinceSpec(s, time.Now())
			if err != nil {
				return query.SectionResult{}, err
			}
			sinceCutoff = &cutoff

		case fn.Name == "topic":
			if len(fn.Args) != 1 {
				return query.SectionResult{}, fmt.Errorf("topic: requires exactly one argument (e.g. topic(catch-up-scaling) or topic(\"infrastructure/cli\"))")
			}
			s, err := argString(fn.Args[0])
			if err != nil {
				return query.SectionResult{}, fmt.Errorf("topic: %w", err)
			}
			path, err := model.ParseTopicPath(s)
			if err != nil {
				return query.SectionResult{}, fmt.Errorf("topic: %w", err)
			}
			topicPrefix = path

		case fn.Name == "rank":
			spec, err := parseRankArg(fn.Args)
			if err != nil {
				return query.SectionResult{}, err
			}
			rankPlan = spec

		case fn.Name == "n":
			page, err := parseIntegerArg("n", fn.Args)
			if err != nil {
				return query.SectionResult{}, err
			}
			pageN = page

		case fn.Name == "group":
			field, err := parseGroupArgs(fn.Args)
			if err != nil {
				return query.SectionResult{}, err
			}
			groupField = field

		case fn.Name == "name":
			s, err := parseNameArgs(fn.Args)
			if err != nil {
				return query.SectionResult{}, err
			}
			nameValue = s
			nameSet = true

		case fn.Name == "expand":
			field, err := parseExpandArgs(fn.Args)
			if err != nil {
				return query.SectionResult{}, err
			}
			expandField = field

		case fn.Name == "stalled":
			v, err := parseStalledArgs(fn.Args)
			if err != nil {
				return query.SectionResult{}, err
			}
			stalledThreshold = v
			stalledSet = true

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
				return query.SectionResult{}, fmt.Errorf("%s takes no arguments", fn.Name)
			}
			renderName = fn.Name

		default:
			return query.SectionResult{}, fmt.Errorf(
				"unknown function %q (known: %s)",
				fn.Name, strings.Join(knownFunctions, ", "))
		}
	}

	if renderName == "" {
		return query.SectionResult{}, fmt.Errorf(
			"section must end with a render function (one of: %s)", renderFunctionsList())
	}

	// Mutual-exclusivity: per-group/per-target sort and pagination are not
	// shipped in slice 5/7. group() and expand() both produce non-flat
	// shapes that don't compose with rank()/n() the way a flat list does
	// — error rather than pick a meaning silently. A future slice can
	// introduce explicit per-bucket modifiers if needed.
	if groupField != "" && rankPlan != nil {
		return query.SectionResult{}, fmt.Errorf(
			"group is mutually exclusive with rank in slice 5; per-group ranking is reserved for a future slice")
	}
	if groupField != "" && pageN >= 0 {
		return query.SectionResult{}, fmt.Errorf(
			"group is mutually exclusive with n in slice 5; per-group pagination is reserved for a future slice")
	}
	if expandField != "" && rankPlan != nil {
		return query.SectionResult{}, fmt.Errorf(
			"expand is mutually exclusive with rank; focus-block targets render in involvement order, and heat for stalled classification is fixed at heat(exp-14d)")
	}
	if expandField != "" && pageN >= 0 {
		return query.SectionResult{}, fmt.Errorf(
			"expand is mutually exclusive with n; focus-block target lists are bounded by the focus's involvement frontmatter, not by pagination")
	}
	if expandField != "" && groupField != "" {
		return query.SectionResult{}, fmt.Errorf(
			"expand is mutually exclusive with group; focus-block has its own per-focus grouping shape")
	}
	if stalledSet && expandField == "" {
		return query.SectionResult{}, fmt.Errorf(
			"stalled() applies only to focus-block sections; pair it with expand(involvement):as-focus-block")
	}

	// Render-shape contract: every render expects a specific result shape.
	// The check fires before computing the result so the user sees the
	// structural error first (AC 16).
	expectedShape := renderFunctions[renderName]
	if groupField != "" && expectedShape != model.ShapeGrouped {
		return query.SectionResult{}, fmt.Errorf(
			"render-shape mismatch: %s expects flat-list result, but group(by(...)) produces a grouped result (use as-grouped)",
			renderName)
	}
	if expandField != "" && expectedShape != model.ShapeFocusBlock {
		return query.SectionResult{}, fmt.Errorf(
			"render-shape mismatch: %s expects flat-list result, but expand(involvement) produces a focus-block result (use as-focus-block)",
			renderName)
	}
	if groupField == "" && expectedShape == model.ShapeGrouped {
		return query.SectionResult{}, fmt.Errorf(
			"render-shape mismatch: as-grouped expects a grouped result, but the section produces a flat-list (add group(by(<field>)) before as-grouped)")
	}
	if expandField == "" && expectedShape == model.ShapeFocusBlock {
		return query.SectionResult{}, fmt.Errorf(
			"render-shape mismatch: as-focus-block expects a focus-block result, but the section produces a flat-list (add expand(involvement) before as-focus-block, with kind(focus) filter)")
	}

	// Apply intent in canonical pipeline order: filter → rank → page →
	// group → render. Within filter: GraphFilter (active, layer) → kind
	// disjunction → since → topic. Order among post-Graph.Filter()
	// narrowings doesn't affect the result; chosen here to keep cheaper
	// structural checks before time/topic walks.
	entries := g.Filter(filter)
	for _, kinds := range kindFilters {
		entries = filterByKinds(entries, kinds)
	}
	if sinceCutoff != nil {
		entries = filterBySince(entries, *sinceCutoff)
	}
	if !topicPrefix.IsZero() {
		entries = TopicFilter{Prefix: topicPrefix}.FilterEntries(g, entries)
	}
	var scores []float64
	if rankPlan != nil {
		entries, scores = applyRanking(g, entries, rankPlan, time.Now())
	}
	if pageN >= 0 && len(entries) > pageN {
		entries = entries[:pageN]
		if scores != nil {
			scores = scores[:pageN]
		}
	}

	var sectionName string
	if nameSet {
		sectionName = nameValue
	}
	if groupField != "" {
		groups, err := groupBy(entries, groupField)
		if err != nil {
			return query.SectionResult{}, fmt.Errorf("group: %w", err)
		}
		return query.SectionResult{
			Render: renderName,
			Name:   sectionName,
			Data:   model.Grouped{Field: groupField, Groups: groups},
		}, nil
	}
	if expandField != "" {
		// `entries` here is the filtered focus list; expand it into a
		// FocusBlock by walking each focus's involvement and resolving
		// targets. Score is fixed at heat(exp-14d) per slice-7 design;
		// stalled threshold takes the user-supplied value if set.
		block := expandInvolvement(g, entries, focusBlockScorer(g, now), stalledThreshold)
		return query.SectionResult{
			Render: renderName,
			Name:   sectionName,
			Data:   block,
		}, nil
	}
	// Flat-list output: apply the focus-target dedup pass (AC 13) before
	// returning. Entries surfaced in any earlier as-focus-block in the
	// same layout are skipped so the as-list section shows what's
	// "warm but not yet declared in focus."
	if len(shownIDs) > 0 {
		entries, scores = stripShown(entries, scores, shownIDs)
	}
	return query.SectionResult{
		Render: renderName,
		Name:   sectionName,
		Data:   model.FlatList{Entries: entries, Scores: scores},
	}, nil
}

// stripShown removes entries whose ID is in shownIDs, keeping scores
// aligned. Returned slices are fresh — input is not mutated.
func stripShown(entries []*model.Entry, scores []float64, shownIDs map[string]struct{}) ([]*model.Entry, []float64) {
	out := make([]*model.Entry, 0, len(entries))
	var outScores []float64
	if scores != nil {
		outScores = make([]float64, 0, len(entries))
	}
	for i, e := range entries {
		if _, hit := shownIDs[e.ID]; hit {
			continue
		}
		out = append(out, e)
		if outScores != nil {
			outScores = append(outScores, scores[i])
		}
	}
	return out, outScores
}

// parseExpandArgs validates the `expand(<field>)` primitive's argument.
// Slice 7 recognizes only `expand(involvement)` — the focus-block
// transform. Other field names error with a listed-valid-set message
// so users see what's available without grepping the executor.
func parseExpandArgs(args []model.FunctionArg) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("expand: requires exactly one field argument (e.g. expand(involvement))")
	}
	a := args[0]
	if a.Kind != model.ArgKindIdent && a.Kind != model.ArgKindString {
		return "", fmt.Errorf("expand: argument must be an identifier or string, got %s", a.Kind)
	}
	switch a.String {
	case "involvement":
		return a.String, nil
	default:
		return "", fmt.Errorf("expand: unknown field %q (known: involvement)", a.String)
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
	names := make([]string, 0, len(renderFunctions))
	for n := range renderFunctions {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
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
// to avoid churning a type used by sdd list, sdd status, and others —
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
