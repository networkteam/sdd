package query

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestExpandMacros_TopWithN(t *testing.T) {
	// `top(20)` → `active:n(20):rank(heat(exp-14d)):as-list` per d-tac-uww §5.
	layout := mustParseLayoutHelper(t, "top(20)")
	got, err := ExpandMacros(layout)
	if err != nil {
		t.Fatalf("ExpandMacros: %v", err)
	}
	if len(got.Sections) != 1 {
		t.Fatalf("sections: got %d, want 1", len(got.Sections))
	}
	wantNames := []string{"active", "n", "rank", "as-list"}
	gotNames := functionNames(got.Sections[0])
	if !equalStringSlices(gotNames, wantNames) {
		t.Fatalf("function names:\n  got:  %v\n  want: %v", gotNames, wantNames)
	}
	// Verify n's argument is the user-supplied value, not a default.
	nFn := got.Sections[0].Functions[1]
	if nFn.Name != "n" || len(nFn.Args) != 1 || nFn.Args[0].Number != 20 {
		t.Errorf("n args: got %+v, want one number arg = 20", nFn.Args)
	}
	// Verify rank carries the canonical heat(exp-14d) shape.
	rankFn := got.Sections[0].Functions[2]
	if rankFn.Name != "rank" || len(rankFn.Args) != 1 {
		t.Fatalf("rank args: got %+v", rankFn.Args)
	}
	heatArg := rankFn.Args[0]
	if heatArg.Kind != model.ArgKindFunc || heatArg.Func == nil || heatArg.Func.Name != "heat" {
		t.Fatalf("rank arg: got %+v, want heat(...)", heatArg)
	}
	if len(heatArg.Func.Args) != 1 || heatArg.Func.Args[0].String != "exp-14d" {
		t.Errorf("heat arg: got %+v, want exp-14d ident", heatArg.Func.Args)
	}
}

func TestExpandMacros_TopRequiresInteger(t *testing.T) {
	// `top` without N is a user error — clear message rather than silent default.
	cases := []string{
		"top",
		"top()",
		"top(plan)",  // identifier, not number
		`top("20")`,  // string, not number
		"top(2.5)",   // non-integer
		"top(20,30)", // extra arg (no space — parser rejects whitespace)
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			_, err := ExpandMacros(mustParseLayoutHelper(t, src))
			if err == nil {
				t.Fatalf("expected error for %q, got nil", src)
			}
			if !strings.Contains(err.Error(), "top") {
				t.Errorf("error %q does not name the macro", err.Error())
			}
		})
	}
}

func TestExpandMacros_TopWithUserModifiers(t *testing.T) {
	// User modifiers append after the expansion; last-write-wins is
	// resolved later in the executor (proven by slice-3 ranking tests).
	// This test verifies the *append* mechanic — the expanded section
	// carries macro functions first, then user functions.
	layout := mustParseLayoutHelper(t, "top(10):rank(in-degree)")
	got, err := ExpandMacros(layout)
	if err != nil {
		t.Fatalf("ExpandMacros: %v", err)
	}
	wantNames := []string{"active", "n", "rank", "as-list", "rank"}
	gotNames := functionNames(got.Sections[0])
	if !equalStringSlices(gotNames, wantNames) {
		t.Errorf("function names:\n  got:  %v\n  want: %v", gotNames, wantNames)
	}
}

func TestExpandMacros_Topic(t *testing.T) {
	// `topic(L)` → `topic(L):rank(heat(exp-14d)):as-list`. The macro and
	// the primitive share a name; resolution is positional — first
	// function in the section means macro, anywhere else means primitive.
	layout := mustParseLayoutHelper(t, `topic("infrastructure/cli")`)
	got, err := ExpandMacros(layout)
	if err != nil {
		t.Fatalf("ExpandMacros: %v", err)
	}
	wantNames := []string{"topic", "rank", "as-list"}
	gotNames := functionNames(got.Sections[0])
	if !equalStringSlices(gotNames, wantNames) {
		t.Errorf("function names:\n  got:  %v\n  want: %v", gotNames, wantNames)
	}
	// Verify the topic primitive received the macro's arg verbatim.
	topicFn := got.Sections[0].Functions[0]
	if len(topicFn.Args) != 1 || topicFn.Args[0].String != "infrastructure/cli" {
		t.Errorf("topic args: got %+v, want one string arg 'infrastructure/cli'", topicFn.Args)
	}
}

func TestExpandMacros_TopicAsFilter_NotExpanded(t *testing.T) {
	// `active:topic(L):as-list` — topic is mid-section, treated as
	// primitive, not the macro. ExpandMacros is a no-op here.
	src := `active:topic("infrastructure/cli"):as-list`
	layout := mustParseLayoutHelper(t, src)
	got, err := ExpandMacros(layout)
	if err != nil {
		t.Fatalf("ExpandMacros: %v", err)
	}
	wantNames := []string{"active", "topic", "as-list"}
	gotNames := functionNames(got.Sections[0])
	if !equalStringSlices(gotNames, wantNames) {
		t.Errorf("function names:\n  got:  %v\n  want: %v", gotNames, wantNames)
	}
}

func TestExpandMacros_Decisions(t *testing.T) {
	// `decisions` → `active:kind(plan,directive,activity,contract,aspiration):group(by(kind)):as-grouped`.
	// Per d-tac-3pq, group uses the nested-call form by(<field>).
	layout := mustParseLayoutHelper(t, "decisions")
	got, err := ExpandMacros(layout)
	if err != nil {
		t.Fatalf("ExpandMacros: %v", err)
	}
	wantNames := []string{"active", "kind", "group", "as-grouped"}
	gotNames := functionNames(got.Sections[0])
	if !equalStringSlices(gotNames, wantNames) {
		t.Fatalf("function names:\n  got:  %v\n  want: %v", gotNames, wantNames)
	}
	// kind disjunction set
	kindFn := got.Sections[0].Functions[1]
	wantKinds := []string{"plan", "directive", "activity", "contract", "aspiration"}
	for i, k := range wantKinds {
		if i >= len(kindFn.Args) || kindFn.Args[i].String != k {
			t.Errorf("kind arg %d: got %+v, want %q", i, kindFn.Args, k)
		}
	}
	// group(by(kind)) shape
	groupFn := got.Sections[0].Functions[2]
	if groupFn.Name != "group" || len(groupFn.Args) != 1 {
		t.Fatalf("group args: got %+v", groupFn.Args)
	}
	byArg := groupFn.Args[0]
	if byArg.Kind != model.ArgKindFunc || byArg.Func == nil || byArg.Func.Name != "by" {
		t.Fatalf("group arg: got %+v, want by(...)", byArg)
	}
	if len(byArg.Func.Args) != 1 || byArg.Func.Args[0].String != "kind" {
		t.Errorf("by arg: got %+v, want kind ident", byArg.Func.Args)
	}
}

func TestExpandMacros_Signals(t *testing.T) {
	layout := mustParseLayoutHelper(t, "signals")
	got, err := ExpandMacros(layout)
	if err != nil {
		t.Fatalf("ExpandMacros: %v", err)
	}
	wantNames := []string{"active", "kind", "group", "as-grouped"}
	gotNames := functionNames(got.Sections[0])
	if !equalStringSlices(gotNames, wantNames) {
		t.Errorf("function names:\n  got:  %v\n  want: %v", gotNames, wantNames)
	}
	kindFn := got.Sections[0].Functions[1]
	wantKinds := []string{"gap", "question"}
	for i, k := range wantKinds {
		if i >= len(kindFn.Args) || kindFn.Args[i].String != k {
			t.Errorf("kind arg %d: got %+v, want %q", i, kindFn.Args, k)
		}
	}
}

func TestExpandMacros_Insights(t *testing.T) {
	// `insights` → `active:kind(insight):since("30d"):rank(by(date)):as-list`.
	// since() must carry a string-arg "30d" because the duration parser
	// requires quoted strings (bare 30d would lex as number+ident).
	layout := mustParseLayoutHelper(t, "insights")
	got, err := ExpandMacros(layout)
	if err != nil {
		t.Fatalf("ExpandMacros: %v", err)
	}
	wantNames := []string{"active", "kind", "since", "rank", "as-list"}
	gotNames := functionNames(got.Sections[0])
	if !equalStringSlices(gotNames, wantNames) {
		t.Errorf("function names:\n  got:  %v\n  want: %v", gotNames, wantNames)
	}
	sinceFn := got.Sections[0].Functions[2]
	if len(sinceFn.Args) != 1 || sinceFn.Args[0].Kind != model.ArgKindString || sinceFn.Args[0].String != "30d" {
		t.Errorf("since arg: got %+v, want string '30d'", sinceFn.Args)
	}
	rankFn := got.Sections[0].Functions[3]
	if rankFn.Name != "rank" || len(rankFn.Args) != 1 {
		t.Fatalf("rank: got %+v", rankFn)
	}
	byDate := rankFn.Args[0]
	if byDate.Kind != model.ArgKindFunc || byDate.Func == nil || byDate.Func.Name != "by" {
		t.Errorf("rank arg: got %+v, want by(date)", byDate)
	}
}

func TestExpandMacros_NullaryMacrosRejectArgs(t *testing.T) {
	// Nullary macros (decisions, signals, etc.) must error if called with
	// args — `decisions(plan)` could mean "filter to plan" but the current
	// shape has no parameter slot. Surface as error so the user reaches
	// for `decisions:kind(plan)` (modifier append) or composes the
	// pipeline manually.
	for _, name := range []string{"decisions", "signals", "insights", "done", "aspirations", "contracts"} {
		t.Run(name, func(t *testing.T) {
			_, err := ExpandMacros(mustParseLayoutHelper(t, name+"(plan)"))
			if err == nil {
				t.Fatalf("expected error for %s(plan), got nil", name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error %q does not name the macro", err.Error())
			}
		})
	}
}

func TestExpandMacros_NoMacro_PassThrough(t *testing.T) {
	// A plain pipeline with no macro names is unchanged by expansion —
	// the layout returned is byte-equivalent to the input for non-macro
	// invocations.
	src := "active:kind(plan):rank(heat):n(5):as-list"
	layout := mustParseLayoutHelper(t, src)
	got, err := ExpandMacros(layout)
	if err != nil {
		t.Fatalf("ExpandMacros: %v", err)
	}
	wantNames := []string{"active", "kind", "rank", "n", "as-list"}
	gotNames := functionNames(got.Sections[0])
	if !equalStringSlices(gotNames, wantNames) {
		t.Errorf("function names:\n  got:  %v\n  want: %v", gotNames, wantNames)
	}
}

func TestExpandMacros_TopicRequiresLabel(t *testing.T) {
	// topic(L) without a label is a clear error rather than expanding to
	// a malformed topic() primitive call.
	_, err := ExpandMacros(mustParseLayoutHelper(t, "topic"))
	if err == nil {
		t.Fatalf("expected error for bare topic, got nil")
	}
	if !strings.Contains(err.Error(), "topic") {
		t.Errorf("error %q does not name the macro", err.Error())
	}
}

func TestExpandMacros_MultipleSections(t *testing.T) {
	// Each section is expanded independently. Macros at section[0] and
	// section[1] both expand; primitives untouched.
	layout := mustParseLayoutHelper(t, "decisions,signals")
	got, err := ExpandMacros(layout)
	if err != nil {
		t.Fatalf("ExpandMacros: %v", err)
	}
	if len(got.Sections) != 2 {
		t.Fatalf("sections: got %d, want 2", len(got.Sections))
	}
	for i, sec := range got.Sections {
		gotNames := functionNames(sec)
		wantNames := []string{"active", "kind", "group", "as-grouped"}
		if !equalStringSlices(gotNames, wantNames) {
			t.Errorf("section %d names:\n  got:  %v\n  want: %v", i, gotNames, wantNames)
		}
	}
}

// --- helpers ---

func mustParseLayoutHelper(t *testing.T, s string) model.Layout {
	t.Helper()
	l, err := ParseLayout(s)
	if err != nil {
		t.Fatalf("ParseLayout(%q): %v", s, err)
	}
	return l
}

func functionNames(s model.Section) []string {
	names := make([]string, len(s.Functions))
	for i, fn := range s.Functions {
		names[i] = fn.Name
	}
	return names
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
