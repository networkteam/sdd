package finders

import (
	"fmt"
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
// Slice 2 vocabulary: `active`, `kind(K[, K2, ...])`, `n(N)` (filters and
// paging) plus `as-list` (render). Unknown function names return an error
// listing the valid set so users (and future-slice tests) get a clear
// signal.
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

// renderFunctions enumerates the function names that terminate a section.
// Slice 2 has only as-list; later slices add as-grouped, as-focus-block,
// as-participants-block, as-wip-list.
var renderFunctions = map[string]bool{
	"as-list": true,
}

// knownFunctions lists every function name the executor recognizes. Used
// in the unknown-function error message so users see what's available.
var knownFunctions = []string{"active", "kind", "n", "rank", "as-list"}

// executeSection walks one section's pipeline left-to-right. Each
// non-render function contributes to one of three intent buckets:
// graph filter (active, kind), pagination (n), or render terminator
// (as-list). The buckets apply in canonical filter→page→render order
// regardless of source ordering — last-write-wins per modifier kind.
func executeSection(g *model.Graph, section model.Section) (query.SectionResult, error) {
	if len(section.Functions) == 0 {
		return query.SectionResult{}, fmt.Errorf("empty section")
	}

	var (
		filter     model.GraphFilter
		kindFilter []model.Kind // accumulated disjunction across kind(...) calls
		rankPlan   *rankSpec    // last-write-wins per d-tac-uww §2
		pageN      = -1         // -1 = no page limit
		renderName string
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

		case renderFunctions[fn.Name]:
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
			"section must end with a render function (one of: as-list)")
	}

	// Apply intent in canonical pipeline order: filter → rank → page → render.
	entries := g.Filter(filter)
	if len(kindFilter) > 0 {
		entries = filterByKinds(entries, kindFilter)
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

	return query.SectionResult{
		Render: renderName,
		Data:   model.FlatList{Entries: entries, Scores: scores},
	}, nil
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
