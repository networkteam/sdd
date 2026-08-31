package query

import (
	"fmt"
	"sort"

	"github.com/networkteam/sdd/internal/model"
)

// ExpandMacros walks each section's first function and substitutes macro
// expansions per d-tac-uww §5. Macros recognized as the section's first
// function expand into a sequence of canonical primitives; subsequent
// user-supplied functions append to that sequence and resolve via the
// executor's last-write-wins rules. Functions appearing mid-section are
// always treated as primitives — `topic(L)` at section start expands to
// the macro pipeline; the same name later in the same section is the
// filter primitive.
//
// Macro expansion is a separate query-layer pass distinct from grammar
// parsing so each can be tested in isolation. The CLI calls ParseLayout
// then ExpandMacros; tests can target either step.
func ExpandMacros(layout model.Layout) (model.Layout, error) {
	// A lone layout macro expands into several sections first; the per-section
	// pass below then finishes any section macros they introduce (participants,
	// aspirations). A layout macro only triggers when it is the whole layout —
	// one section, one function — so a stray `readiness` mid-layout falls
	// through to the section pass and the executor's unknown-function error.
	if len(layout.Sections) == 1 && len(layout.Sections[0].Functions) == 1 {
		first := layout.Sections[0].Functions[0]
		if expand, ok := layoutMacros[first.Name]; ok {
			expanded, err := expand(first.Args)
			if err != nil {
				return model.Layout{}, fmt.Errorf("%s: %w", first.Name, err)
			}
			layout = expanded
		}
	}

	out := model.Layout{Sections: make([]model.Section, 0, len(layout.Sections))}
	for i, section := range layout.Sections {
		expanded, err := expandSection(section)
		if err != nil {
			return model.Layout{}, fmt.Errorf("section %d: %w", i+1, err)
		}
		out.Sections = append(out.Sections, expanded)
	}
	return out, nil
}

// MacroNames returns the section-start macro names sorted for deterministic
// help-text rendering. Exposed so the CLI's view help doesn't need to
// import the registry directly. Layout macros are excluded — they are not
// valid at section start; see LayoutMacroNames.
func MacroNames() []string {
	names := make([]string, 0, len(macros))
	for n := range macros {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// LayoutMacroNames returns the layout macro names sorted — macros used alone
// as a whole layout rather than at section start.
func LayoutMacroNames() []string {
	names := make([]string, 0, len(layoutMacros))
	for n := range layoutMacros {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func expandSection(section model.Section) (model.Section, error) {
	if len(section.Functions) == 0 {
		return section, nil
	}
	first := section.Functions[0]
	expand, ok := macros[first.Name]
	if !ok {
		return section, nil
	}
	expanded, err := expand(first.Args)
	if err != nil {
		return model.Section{}, fmt.Errorf("%s: %w", first.Name, err)
	}
	// Append remaining user functions verbatim — last-write-wins for
	// non-filter modifiers and intersection for filter() calls happens
	// downstream in the executor.
	expanded = append(expanded, section.Functions[1:]...)
	// The user's source survives expansion: a pull expression built from
	// this section stays as terse as what was typed (macros re-expand on
	// every parse, so the source is runnable as written).
	return model.Section{Functions: expanded, Source: section.Source}, nil
}

// macros maps macro names to their expand function. A macro consumes its
// own call arguments and returns the function sequence to substitute.
var macros = map[string]func(args []model.FunctionArg) ([]model.Function, error){
	"top":          expandTop,
	"topic":        expandTopicMacro,
	"focus":        expandFocus,
	"decisions":    expandDecisions,
	"signals":      expandSignals,
	"insights":     expandInsights,
	"done":         expandDone,
	"aspirations":  expandAspirations,
	"contracts":    expandContracts,
	"participants": expandParticipants,
	"wip":          expandWIP,
}

// layoutMacros expand a lone macro name into a whole multi-section layout,
// which a single-section macro cannot produce.
var layoutMacros = map[string]func(args []model.FunctionArg) (model.Layout, error){
	"readiness": expandReadinessLayout,
}

// readinessLayout is the four capped bootstrap grounding lanes: participants,
// aspirations, strategic guiding, conceptual guiding.
const readinessLayout = `participants:brief,` +
	`aspirations:active:rank(heat(exp-14d)):n(6):name("Aspirations"):brief:as-list,` +
	`kind(directive):intent(guiding):layer(strategic):active:rank(heat(exp-14d)):n(6):name("Direction — strategic guiding"):brief:as-list,` +
	`kind(directive):intent(guiding):layer(conceptual):active:rank(heat(exp-14d)):n(6):name("Shape — conceptual guiding"):brief:as-list`

func expandReadinessLayout(args []model.FunctionArg) (model.Layout, error) {
	if len(args) > 0 {
		return model.Layout{}, fmt.Errorf("takes no arguments")
	}
	return ParseLayout(readinessLayout)
}

// expandTop expands `top(N)` with a baked `name-prefix("Top")` so the
// rendered header reads "Top by heat (exp-14d)" by default and
// "Top by in-degree" (etc.) when the user overrides rank — the prefix
// stays constant while the rank suffix tracks the user's modifier.
func expandTop(args []model.FunctionArg) ([]model.Function, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("requires one integer argument N (e.g. top(20))")
	}
	a := args[0]
	if a.Kind != model.ArgKindNumber {
		return nil, fmt.Errorf("argument must be a number, got %s", a.Kind)
	}
	if a.Number != float64(int64(a.Number)) {
		return nil, fmt.Errorf("argument must be an integer, got %v", a.Number)
	}
	if a.Number < 0 {
		return nil, fmt.Errorf("argument must be non-negative, got %v", a.Number)
	}
	return []model.Function{
		{Name: "active"},
		{Name: "n", Args: []model.FunctionArg{a}},
		{Name: "rank", Args: []model.FunctionArg{funcArg("heat", identArg("exp-14d"))}},
		{Name: "name-prefix", Args: []model.FunctionArg{stringArg("Top")}},
		{Name: "as-list"},
	}, nil
}

// expandTopicMacro expands `topic(L)` with a baked `name-prefix("Topic:
// <L>")` so the rendered header reads e.g.
// "Topic: infrastructure/cli by heat (exp-14d)" — both the topic label
// and the rank suffix surface, addressing the slice-8 evaluation
// finding that the previous derive ignored L. The label arg is also
// forwarded verbatim to the topic filter primitive.
func expandTopicMacro(args []model.FunctionArg) ([]model.Function, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("requires one label argument L (e.g. topic(catch-up-scaling))")
	}
	a := args[0]
	if a.Kind != model.ArgKindIdent && a.Kind != model.ArgKindString {
		return nil, fmt.Errorf("argument must be an identifier or string, got %s", a.Kind)
	}
	return []model.Function{
		{Name: "topic", Args: []model.FunctionArg{a}},
		{Name: "rank", Args: []model.FunctionArg{funcArg("heat", identArg("exp-14d"))}},
		{Name: "name-prefix", Args: []model.FunctionArg{stringArg("Topic: " + a.String)}},
		{Name: "as-list"},
	}, nil
}

// expandDecisions, expandSignals, expandInsights, expandDone,
// expandAspirations, expandContracts — nullary macros producing fixed
// pipelines. A user passing args is a clear mistake (the macro shape
// has no parameter slot), so they error early rather than silently
// drop the args.

// Macros bake a `name-prefix("<title>")` step so the rendered section
// carries a `## <title>` header. The executor's auto-derive resolver
// (sectionSpec.resolveSectionHeader) composes the prefix with the rank
// suffix when rank is set ("Top by heat (exp-14d)", "Topic: foo by
// in-degree", "Done by date") and uses just the prefix when there's
// no rank ("Focus", "Participants", "WIP"). User-supplied `name(...)`
// always wins (final, no auto-append); user `name-prefix(...)` plays
// by the same compose rules as the macro bake. The prefix-vs-final
// split keeps macros expressing *what* the section is about while
// rank() expresses *how* it's sorted.

func expandDecisions(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "active"},
		{Name: "kind", Args: identArgs("plan", "directive", "activity", "contract", "aspiration")},
		{Name: "group", Args: []model.FunctionArg{funcArg("by", identArg("kind"))}},
		{Name: "name-prefix", Args: []model.FunctionArg{stringArg("Decisions")}},
		{Name: "as-grouped"},
	}, nil
}

func expandSignals(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "active"},
		{Name: "kind", Args: identArgs("gap", "question")},
		{Name: "group", Args: []model.FunctionArg{funcArg("by", identArg("kind"))}},
		{Name: "name-prefix", Args: []model.FunctionArg{stringArg("Signals")}},
		{Name: "as-grouped"},
	}, nil
}

// expandInsights / expandDone bake `name-prefix("Insights")` and
// `name-prefix("Done")`. Combined with `rank(by(date))`, the resolver
// produces "Insights by date" and "Done by date" — uniform "by <thing>"
// suffix shape across all rank algorithms.
func expandInsights(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "active"},
		{Name: "kind", Args: identArgs("insight")},
		{Name: "since", Args: []model.FunctionArg{stringArg("30d")}},
		{Name: "rank", Args: []model.FunctionArg{funcArg("by", identArg("date"))}},
		{Name: "name-prefix", Args: []model.FunctionArg{stringArg("Insights")}},
		{Name: "as-list"},
	}, nil
}

func expandDone(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "kind", Args: identArgs("done")},
		{Name: "since", Args: []model.FunctionArg{stringArg("30d")}},
		{Name: "rank", Args: []model.FunctionArg{funcArg("by", identArg("date"))}},
		{Name: "name-prefix", Args: []model.FunctionArg{stringArg("Done")}},
		{Name: "as-list"},
	}, nil
}

func expandAspirations(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "active"},
		{Name: "kind", Args: identArgs("aspiration")},
		{Name: "name-prefix", Args: []model.FunctionArg{stringArg("Aspirations")}},
		{Name: "as-list"},
	}, nil
}

// expandFocus expands `focus` with a baked `name-prefix("Focus")`.
// No rank applies (focus-block has its own scoring), so the resolver
// uses the prefix alone — header reads "## Focus".
func expandFocus(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "kind", Args: identArgs("focus")},
		{Name: "active"},
		{Name: "expand", Args: []model.FunctionArg{identArg("involvement")}},
		{Name: "name-prefix", Args: []model.FunctionArg{stringArg("Focus")}},
		{Name: "as-focus-block"},
	}, nil
}

func expandContracts(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "active"},
		{Name: "kind", Args: identArgs("contract")},
		{Name: "name-prefix", Args: []model.FunctionArg{stringArg("Contracts")}},
		{Name: "as-list"},
	}, nil
}

// expandParticipants bakes a `name-prefix("Participants")`. No rank,
// so the resolver uses the prefix alone — header reads "## Participants".
// The active+kind(actor) filters narrow which actors surface; the
// renderer derives the role cascade from full chain history per
// d-cpt-d34 (within-chain canonical corrections still bind to the
// current head).
func expandParticipants(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "active"},
		{Name: "kind", Args: identArgs("actor")},
		{Name: "name-prefix", Args: []model.FunctionArg{stringArg("Participants")}},
		{Name: "as-participants-block"},
	}, nil
}

// expandWIP bakes a `name-prefix("WIP")`. Markers come from disk (the
// wip/ subdirectory of the graph) rather than the graph itself, so
// the macro switches sources before terminating in as-wip-list.
// Filter primitives are not part of the expansion — slice 8 surfaces
// every active marker; user-supplied modifiers like name() append.
func expandWIP(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "source", Args: []model.FunctionArg{identArg("wip")}},
		{Name: "name-prefix", Args: []model.FunctionArg{stringArg("WIP")}},
		{Name: "as-wip-list"},
	}, nil
}

func requireNoArgs(args []model.FunctionArg) error {
	if len(args) > 0 {
		return fmt.Errorf("takes no arguments (compose with modifiers via colon, e.g. decisions:layer(tac))")
	}
	return nil
}

// identArg / stringArg / funcArg / identArgs construct FunctionArg values
// for macro expansion. Macros build ASTs directly rather than re-parsing
// expansion strings — that way `since(30d)` works inside a macro even
// though the parser requires quoted strings for duration specs.

func identArg(name string) model.FunctionArg {
	return model.FunctionArg{Kind: model.ArgKindIdent, String: name}
}

func stringArg(s string) model.FunctionArg {
	return model.FunctionArg{Kind: model.ArgKindString, String: s}
}

func funcArg(name string, innerArgs ...model.FunctionArg) model.FunctionArg {
	fn := model.Function{Name: name, Args: innerArgs}
	return model.FunctionArg{Kind: model.ArgKindFunc, Func: &fn}
}

func identArgs(names ...string) []model.FunctionArg {
	args := make([]model.FunctionArg, len(names))
	for i, n := range names {
		args[i] = identArg(n)
	}
	return args
}
