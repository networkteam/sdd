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

// MacroNames returns the registered macro names sorted for deterministic
// help-text rendering. Exposed so the CLI's view help doesn't need to
// import the registry directly.
func MacroNames() []string {
	names := make([]string, 0, len(macros))
	for n := range macros {
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
	return model.Section{Functions: expanded}, nil
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

// expandTop expands `top(N)` to `active:n(N):rank(heat(exp-14d)):as-list`.
// N is required and must be a non-negative integer; failures here would
// otherwise surface as obscure n() errors after expansion, so the macro
// catches them with a top-prefixed message.
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
		{Name: "as-list"},
	}, nil
}

// expandTopicMacro expands `topic(L)` to `topic(L):rank(heat(exp-14d)):as-list`.
// The label arg is forwarded verbatim to the topic filter primitive — both
// bare identifiers (catch-up-scaling) and quoted strings (paths with /)
// pass through unchanged.
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
		{Name: "as-list"},
	}, nil
}

// expandDecisions, expandSignals, expandInsights, expandDone,
// expandAspirations, expandContracts — nullary macros producing fixed
// pipelines. A user passing args is a clear mistake (the macro shape
// has no parameter slot), so they error early rather than silently
// drop the args.

// Macros that don't terminate in a ranked as-list section bake a default
// `name("<title>")` so their rendered output carries a `## <title>`
// header symmetric with rank-based auto-derive. Without baked names,
// non-rank shapes (focus-block, participants-block, wip-list, grouped)
// rendered headerless because auto-derive (per d-tac-jgi) only fires
// when rank is configured. Users override via `<macro>:name("Custom")`
// — last-write-wins keeps the override path identical to what works on
// ranked sections. Ranked macros (top, topic, insights, done) skip the
// baked name so auto-derive's more informative output ("Top by heat
// (exp-14d)", "Most recent") still shows.

func expandDecisions(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "active"},
		{Name: "kind", Args: identArgs("plan", "directive", "activity", "contract", "aspiration")},
		{Name: "group", Args: []model.FunctionArg{funcArg("by", identArg("kind"))}},
		{Name: "name", Args: []model.FunctionArg{stringArg("Decisions")}},
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
		{Name: "name", Args: []model.FunctionArg{stringArg("Signals")}},
		{Name: "as-grouped"},
	}, nil
}

func expandInsights(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "active"},
		{Name: "kind", Args: identArgs("insight")},
		{Name: "since", Args: []model.FunctionArg{stringArg("30d")}},
		{Name: "rank", Args: []model.FunctionArg{funcArg("by", identArg("date"))}},
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
		{Name: "name", Args: []model.FunctionArg{stringArg("Aspirations")}},
		{Name: "as-list"},
	}, nil
}

// expandFocus expands `focus` to `kind(focus):active:expand(involvement):
// name("Focus"):as-focus-block` per d-tac-uww §5 plus the baked-name
// pattern. The state derivation algorithm and stalled threshold live
// downstream in the executor; the macro wires the canonical pipeline
// and the default header. Users override via `focus:stalled(<value>)`
// or `focus:name("<title>")` (last-write-wins).
func expandFocus(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "kind", Args: identArgs("focus")},
		{Name: "active"},
		{Name: "expand", Args: []model.FunctionArg{identArg("involvement")}},
		{Name: "name", Args: []model.FunctionArg{stringArg("Focus")}},
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
		{Name: "name", Args: []model.FunctionArg{stringArg("Contracts")}},
		{Name: "as-list"},
	}, nil
}

// expandParticipants expands `participants` to
// `active:kind(actor):name("Participants"):as-participants-block` per
// d-tac-uww §5 plus the baked-name pattern. The active+kind(actor)
// filters narrow which actors surface; the renderer derives the role
// cascade from full chain history per d-cpt-d34 (within-chain canonical
// corrections still bind to the current head).
func expandParticipants(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "active"},
		{Name: "kind", Args: identArgs("actor")},
		{Name: "name", Args: []model.FunctionArg{stringArg("Participants")}},
		{Name: "as-participants-block"},
	}, nil
}

// expandWIP expands `wip` to `source(wip):name("WIP"):as-wip-list` per
// d-tac-uww §5 plus the baked-name pattern. Markers come from disk
// (the wip/ subdirectory of the graph) rather than the graph itself,
// so the macro switches sources before terminating in as-wip-list.
// Filter primitives are not part of the expansion — slice 8 surfaces
// every active marker; user-supplied modifiers like name() append.
func expandWIP(args []model.FunctionArg) ([]model.Function, error) {
	if err := requireNoArgs(args); err != nil {
		return nil, err
	}
	return []model.Function{
		{Name: "source", Args: []model.FunctionArg{identArg("wip")}},
		{Name: "name", Args: []model.FunctionArg{stringArg("WIP")}},
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
