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

	sections := make([]query.SectionResult, 0, len(q.Layout.Sections))
	for i, section := range q.Layout.Sections {
		sr, err := executeSection(q.Graph, section)
		if err != nil {
			return nil, fmt.Errorf("section %d: %w", i+1, err)
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
// Later slices add as-focus-block (slice 7) and as-participants-block /
// as-wip-list (slice 8).
var renderFunctions = map[string]model.RenderShape{
	"as-list":    model.ShapeFlatList,
	"as-grouped": model.ShapeGrouped,
}

// knownFunctions lists every function name the executor recognizes. Used
// in the unknown-function error message so users see what's available.
var knownFunctions = []string{"active", "kind", "layer", "since", "topic", "n", "rank", "group", "as-list", "as-grouped"}

// executeSection walks one section's pipeline left-to-right, accumulating
// each function's intent into one of these buckets:
//
//   - graph filter        (active, kind, layer, since, topic)
//   - rank                (rank(<algo>), last-write-wins)
//   - pagination          (n(N), last-write-wins)
//   - aggregation         (group(by(<field>)), last-write-wins)
//   - render terminator   (as-list / as-grouped, must be last)
//
// Buckets apply in canonical filter → rank → page → group → render order
// regardless of source ordering — last-write-wins per modifier kind, per
// d-tac-uww §2. group() is mutually exclusive with rank() and n() in
// slice 5: per-group sort and per-group pagination are reserved for a
// later slice.
func executeSection(g *model.Graph, section model.Section) (query.SectionResult, error) {
	if len(section.Functions) == 0 {
		return query.SectionResult{}, fmt.Errorf("empty section")
	}

	var (
		filter      model.GraphFilter
		kindFilter  []model.Kind // accumulated disjunction across kind(...) calls
		sinceCutoff *time.Time   // pointer so we can distinguish "no since()" from "since(0d)"
		topicPrefix model.TopicPath
		rankPlan    *rankSpec // last-write-wins per d-tac-uww §2
		pageN       = -1      // -1 = no page limit
		groupField  string    // empty = no group; non-empty = group(by(<field>))
		renderName  string
	)

	for i, fn := range section.Functions {
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
			kindFilter = append(kindFilter, kinds...)

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

		case isRenderFunction(fn.Name):
			if i != len(section.Functions)-1 {
				return query.SectionResult{}, fmt.Errorf(
					"render function %q must be the last function in a section, found at position %d of %d",
					fn.Name, i+1, len(section.Functions))
			}
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

	// Mutual-exclusivity: slice 5 doesn't sort or paginate within groups.
	// rank() and n() over a grouped result both have plausible meanings
	// (per-group, across groups, capped at N groups) — error rather than
	// pick one silently. A future slice can introduce explicit per-group
	// modifiers if needed.
	if groupField != "" && rankPlan != nil {
		return query.SectionResult{}, fmt.Errorf(
			"group is mutually exclusive with rank in slice 5; per-group ranking is reserved for a future slice")
	}
	if groupField != "" && pageN >= 0 {
		return query.SectionResult{}, fmt.Errorf(
			"group is mutually exclusive with n in slice 5; per-group pagination is reserved for a future slice")
	}

	// Render-shape contract: every render expects a specific result shape.
	// as-list expects flat; as-grouped expects grouped. The check here is
	// AC 16's render-shape mismatch error — fired before computing the
	// result so the user sees the structural error first.
	expectedShape := renderFunctions[renderName]
	if groupField != "" && expectedShape != model.ShapeGrouped {
		return query.SectionResult{}, fmt.Errorf(
			"render-shape mismatch: %s expects flat-list result, but group(by(...)) produces a grouped result (use as-grouped)",
			renderName)
	}
	if groupField == "" && expectedShape == model.ShapeGrouped {
		return query.SectionResult{}, fmt.Errorf(
			"render-shape mismatch: as-grouped expects a grouped result, but the section produces a flat-list (add group(by(<field>)) before as-grouped)")
	}

	// Apply intent in canonical pipeline order: filter → rank → page →
	// group → render. Within filter: GraphFilter (active, layer) → kind
	// disjunction → since → topic. Order among post-Graph.Filter()
	// narrowings doesn't affect the result; chosen here to keep cheaper
	// structural checks before time/topic walks.
	entries := g.Filter(filter)
	if len(kindFilter) > 0 {
		entries = filterByKinds(entries, kindFilter)
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

	if groupField != "" {
		groups, err := groupBy(entries, groupField)
		if err != nil {
			return query.SectionResult{}, fmt.Errorf("group: %w", err)
		}
		return query.SectionResult{
			Render: renderName,
			Data:   model.Grouped{Field: groupField, Groups: groups},
		}, nil
	}
	return query.SectionResult{
		Render: renderName,
		Data:   model.FlatList{Entries: entries, Scores: scores},
	}, nil
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
