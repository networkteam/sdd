package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/baseprocedures"
	"github.com/networkteam/sdd/internal/model"
)

// Per-procedure table tests for the embedded engage and explore entries.
// Like the capture tests, they drive the shipped base entries through the
// production loader, so spec drift between tests and the served procedure is
// impossible.

const (
	procAnchorID   = "20260601-120000-d-tac-ref"
	procNeighborID = "20260601-130000-s-tac-nbr"
	// procMissingID is well-formed but absent from the fixture graph.
	procMissingID = "20260601-140000-d-tac-gon"
)

// procEnv is the harness around one engage/explore instance: the shipped
// spec, a fake entryChains query echoing the IDs it was asked to serve, and
// a two-entry graph for resolution checks.
type procEnv struct {
	session *Session
	spec    *Spec
	// chainCalls records the ID sets entryChains served, one string per call.
	chainCalls []string
}

func baseEntry(t *testing.T, canonical string) *model.Entry {
	t.Helper()
	entries, err := baseprocedures.Entries()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Canonical == canonical {
			return e
		}
	}
	t.Fatalf("embedded base entries carry no %s procedure", canonical)
	return nil
}

func procGraph(t *testing.T) *model.Graph {
	t.Helper()
	anchor, err := model.ParseEntry(procAnchorID+".md", `---
type: decision
layer: tac
kind: directive
intent: pending
---

A directive the engagement anchors on.
`)
	if err != nil {
		t.Fatal(err)
	}
	neighbor, err := model.ParseEntry(procNeighborID+".md", `---
type: signal
layer: tac
kind: gap
---

A connected gap engaged alongside.
`)
	if err != nil {
		t.Fatal(err)
	}
	return model.NewGraph([]*model.Entry{anchor, neighbor})
}

func newProcEnv(t *testing.T, canonical string) *procEnv {
	t.Helper()
	env := &procEnv{}

	reg := NewRegistry()
	mustRegisterQuery(reg, Query{
		Doc: FuncDoc{Name: "entryChains", Doc: "fake chain render", Reads: []string{"anchor", "targets"}},
		Fn: func(ctx *Context, _ map[string]any) (any, error) {
			var ids []string
			if v, ok := ctx.Store.Get("anchor"); ok {
				if id, ok := v.(string); ok && id != "" {
					ids = append(ids, id)
				}
			}
			if v, ok := ctx.Store.Get("targets"); ok {
				ids = append(ids, asStrings(v)...)
			}
			call := "chains(" + strings.Join(ids, ",") + ")"
			env.chainCalls = append(env.chainCalls, call)
			return call, nil
		},
	})

	spec, err := LoadSpec(baseEntry(t, canonical), reg)
	if err != nil {
		t.Fatal(err)
	}
	env.spec = spec

	engine := New(reg, procGraph(t))
	ts := time.Date(2026, 7, 3, 19, 30, 0, 0, time.UTC)
	env.session = engine.NewSession("s_proc", "christopher", &memorySink{}, WithClock(func() time.Time {
		ts = ts.Add(time.Second)
		return ts
	}))
	return env
}

// --- engage -----------------------------------------------------------------

func TestEngage_HappyPath(t *testing.T) {
	env := newProcEnv(t, "engage")

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "anchor" {
		t.Fatalf("start step = %s, want anchor", sv.Step)
	}
	if got := strings.Join(sv.Missing, ","); got != "anchor" {
		t.Fatalf("missing = %q, want anchor (targets and goal are optional)", got)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"anchor": procAnchorID,
		"goal":   "implement it",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "brief" {
		t.Fatalf("after anchor step = %s, want brief", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "chains("+procAnchorID+")") {
		t.Errorf("brief unit should carry the injected chains, got %q", sv.Instructions)
	}
	if !strings.Contains(sv.Instructions, "implement it") {
		t.Errorf("brief unit should render the goal, got %q", sv.Instructions)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"brief":       "AC-status: 1 done, 2 remaining.",
		"widenReport": "searched three angles, nothing beyond the chain",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "moves" {
		t.Fatalf("after brief step = %s, want moves", sv.Step)
	}
	if sv.Chooser == nil || sv.Chooser.Kind != ChooserUser {
		t.Fatalf("moves must serve a user chooser, got %+v", sv.Chooser)
	}
	if !strings.Contains(sv.Instructions, procAnchorID) {
		t.Errorf("moves unit should name the anchor, got %q", sv.Instructions)
	}

	sv, err = env.session.Answer(sv.Instance, "moves", "move",
		map[string]any{"selectedMove": "implement slice 5"}, "let's build it")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("status = %s, want completed", sv.Status)
	}
}

func TestEngage_AnchorMustResolve(t *testing.T) {
	env := newProcEnv(t, "engage")
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{"anchor": procMissingID})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "anchor" {
		t.Fatalf("step = %s, want anchor (stalled on resolution)", sv.Step)
	}
	found := false
	for _, f := range sv.Failing {
		if f.Name == "anchorsResolve" {
			found = true
		}
	}
	if !found {
		t.Fatalf("failing = %+v, want anchorsResolve", sv.Failing)
	}
	// The injection never ran on the unresolved anchor.
	if len(env.chainCalls) != 0 {
		t.Errorf("entryChains ran despite unresolved anchor: %v", env.chainCalls)
	}
}

func TestEngage_TargetsServedAlongside(t *testing.T) {
	env := newProcEnv(t, "engage")
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"anchor":  procAnchorID,
		"targets": []any{procNeighborID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "brief" {
		t.Fatalf("step = %s, want brief", sv.Step)
	}
	want := "chains(" + procAnchorID + "," + procNeighborID + ")"
	if !strings.Contains(sv.Instructions, want) {
		t.Errorf("brief should serve anchor and target chains, got %q", sv.Instructions)
	}

	// A target that doesn't resolve holds the anchor gate the same way.
	env2 := newProcEnv(t, "engage")
	sv2, err := env2.session.Start(env2.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	sv2, err = env2.session.Report(sv2.Instance, map[string]any{
		"anchor":  procAnchorID,
		"targets": []any{procMissingID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv2.Step != "anchor" {
		t.Fatalf("step = %s, want anchor (stalled on target resolution)", sv2.Step)
	}
}

func TestEngage_OneShotBatchCascades(t *testing.T) {
	env := newProcEnv(t, "engage")
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"anchor":      procAnchorID,
		"brief":       "narrative brief",
		"widenReport": "widened from two angles",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "moves" {
		t.Fatalf("batched report should cascade to moves, got %s", sv.Step)
	}
}

func TestEngage_ConcludeAndAbort(t *testing.T) {
	for choice, want := range map[string]InstanceStatus{
		"conclude": StatusCompleted,
		"abort":    StatusAbandoned,
	} {
		env := newProcEnv(t, "engage")
		sv, err := env.session.Start(env.spec, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		sv, err = env.session.Report(sv.Instance, map[string]any{
			"anchor":      procAnchorID,
			"brief":       "brief",
			"widenReport": "widened",
		})
		if err != nil {
			t.Fatal(err)
		}
		sv, err = env.session.Answer(sv.Instance, "moves", choice, nil, "user said so")
		if err != nil {
			t.Fatalf("%s: %v", choice, err)
		}
		if sv.Status != want {
			t.Errorf("%s: status = %s, want %s", choice, sv.Status, want)
		}
	}
}

// --- explore ----------------------------------------------------------------

func exploreParams() map[string]any {
	return map[string]any{
		"targets": []any{procAnchorID, procNeighborID},
		"goal":    "overview: how do these connect",
	}
}

func TestExplore_HappyPath(t *testing.T) {
	env := newProcEnv(t, "explore")

	sv, err := env.session.Start(env.spec, exploreParams(), "")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "inspect" {
		t.Fatalf("start step = %s, want inspect", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "chains("+procAnchorID+","+procNeighborID+")") {
		t.Errorf("inspect unit should carry the injected target chains, got %q", sv.Instructions)
	}
	if !strings.Contains(sv.Instructions, "overview: how do these connect") {
		t.Errorf("inspect unit should render the goal verbatim, got %q", sv.Instructions)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"widenReport":  "angle 1: goal phrase; angle 2: target concept — one neighbor kept",
		"inspectedIds": []any{procAnchorID, procNeighborID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "compress" {
		t.Fatalf("after inspect step = %s, want compress", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "## Goal") {
		t.Errorf("compress unit should carry the briefing structure, got %q", sv.Instructions)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"briefing": "## Goal\noverview: how do these connect\n## Targets\n…",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("status = %s, want completed", sv.Status)
	}
}

func TestExplore_ParamsRequiredAtStart(t *testing.T) {
	env := newProcEnv(t, "explore")

	if _, err := env.session.Start(env.spec, map[string]any{"targets": []any{procAnchorID}}, ""); err == nil ||
		!strings.Contains(err.Error(), "goal") {
		t.Errorf("start without goal must fail naming it, got %v", err)
	}
	if _, err := env.session.Start(env.spec, map[string]any{"goal": "overview"}, ""); err == nil ||
		!strings.Contains(err.Error(), "targets") {
		t.Errorf("start without targets must fail naming them, got %v", err)
	}
}

func TestExplore_InspectedIdsMustResolve(t *testing.T) {
	env := newProcEnv(t, "explore")
	sv, err := env.session.Start(env.spec, exploreParams(), "")
	if err != nil {
		t.Fatal(err)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"widenReport":  "widened",
		"inspectedIds": []any{procAnchorID, procMissingID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "inspect" {
		t.Fatalf("step = %s, want inspect (stalled on evidence resolution)", sv.Step)
	}
	found := false
	for _, f := range sv.Failing {
		if f.Name == "inspectedIdsResolve" {
			found = true
		}
	}
	if !found {
		t.Fatalf("failing = %+v, want inspectedIdsResolve", sv.Failing)
	}
}

func TestExplore_OneShotBatchCompletes(t *testing.T) {
	env := newProcEnv(t, "explore")
	sv, err := env.session.Start(env.spec, exploreParams(), "")
	if err != nil {
		t.Fatal(err)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"widenReport":  "one pass saturated the goal",
		"inspectedIds": []any{procAnchorID},
		"briefing":     "## Goal\noverview…",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("batched mission report should complete, got step %s status %s", sv.Step, sv.Status)
	}
}
