package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// Per-procedure table tests for the embedded user-dialogue shell entry,
// driving the shipped base entry through the production loader. The shell
// names session-scoped functions the MCP shell registers in production;
// the harness stubs them.

type shellEnv struct {
	spec    *Spec
	session *Session
}

func newShellEnv(t *testing.T) *shellEnv {
	return newShellEnvWithFacts(t, []model.FactIndexRow{{ID: "20260717-110000-s-prc-vwg", Title: "How to compose graph views"}})
}

func newShellEnvWithFacts(t *testing.T, facts []model.FactIndexRow) *shellEnv {
	t.Helper()
	env := &shellEnv{}

	reg := NewRegistry()
	mustRegisterQuery(reg, Query{
		Doc: FuncDoc{Name: "sessionInfo", Doc: "fake session info"},
		Fn: func(_ *Context, _ map[string]any) (any, error) {
			return map[string]any{
				"participant": "christopher", "language": "", "search": "text",
				"recovery": "Recovery\n\n  a pending write awaits explicit recovery: mutation-1 · unknown · main",
			}, nil
		},
	})
	mustRegisterQuery(reg, Query{
		Doc: FuncDoc{Name: "procedureList", Doc: "fake move enumeration"},
		Fn: func(_ *Context, _ map[string]any) (any, error) {
			return "- capture — record a signal or decision.\n- engage — anchor on an entry.", nil
		},
	})
	mustRegisterQuery(reg, Query{
		Doc: FuncDoc{Name: "factIndex", Doc: "fake fact index"},
		Fn: func(_ *Context, _ map[string]any) (any, error) {
			return facts, nil
		},
	})
	// The shell declares its framing lanes as viewLayout injects; the MCP shell
	// registers the real query in production. Stub it (serve-safe, like the real
	// one) so the base entry loads.
	mustRegisterQuery(reg, Query{
		Doc:       FuncDoc{Name: "viewLayout", Doc: "fake view layout"},
		ServeSafe: true,
		Fn: func(_ *Context, args map[string]any) (any, error) {
			layout, _ := args["layout"].(string)
			return "view: " + layout, nil
		},
	})
	spec, err := LoadSpec(baseEntry(t, "user-dialogue"), reg)
	if err != nil {
		t.Fatal(err)
	}
	env.spec = spec

	engine := New(reg, StaticGraphs{Graph: procGraph(t)})
	ts := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	env.session = engine.NewSession("s_shell", "christopher", &memorySink{}, WithClock(func() time.Time {
		ts = ts.Add(time.Second)
		return ts
	}))
	return env
}

func TestUserDialogueOpeningRendersFactPointersFromData(t *testing.T) {
	const id = "20991231-235959-s-prc-xyz"
	env := newShellEnvWithFacts(t, []model.FactIndexRow{{ID: id, Title: "Standalone retrieval cue"}})
	serve, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	want := "- `" + id + "` — Standalone retrieval cue"
	if !strings.Contains(serve.Instructions, want) || !strings.Contains(serve.Instructions, "Pull the relevant fact in full first") {
		t.Fatalf("opening fact index missing %q:\n%s", want, serve.Instructions)
	}
	if strings.Contains(env.spec.Units["open"], id) {
		t.Fatal("opening unit hard-codes a fact ID")
	}
}

func TestUserDialogueOpeningOmitsEmptyFactIndex(t *testing.T) {
	env := newShellEnvWithFacts(t, nil)
	serve, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(serve.Instructions, "Reference facts available") {
		t.Fatalf("empty fact index rendered a block:\n%s", serve.Instructions)
	}
}

func TestUserDialogue_OpeningServeAndConclude(t *testing.T) {
	env := newShellEnv(t)

	if env.spec.Class != model.ProcedureClassShell {
		t.Fatalf("user-dialogue must load as a shell, got %q", env.spec.Class)
	}

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "junction" || sv.Chooser == nil || sv.Chooser.Kind != ChooserUser {
		t.Fatalf("the shell should rest on its user junction, got step %q chooser %+v", sv.Step, sv.Chooser)
	}
	if sv.Goal != "dialogue freely; start a move when something crystallizes" {
		t.Fatalf("the junction should carry the standing goal, got %q", sv.Goal)
	}
	// Participant/language/search now live in the engine-supplied info block
	// (application framing), not the unit — the unit keeps recovery, the move
	// list, and the standing goal.
	for _, want := range []string{"pending write awaits explicit recovery", "- capture — ", "Standing goal"} {
		if !strings.Contains(sv.Instructions, want) {
			t.Errorf("opening serve should carry %q, got %q", want, sv.Instructions)
		}
	}

	// Conclude cascades through wrap to completion in the user's one answer.
	sv, err = env.session.Answer(sv.Instance, "junction", "conclude", nil, "we're done")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("conclude should complete the shell, got %s at %q", sv.Status, sv.Step)
	}
}

// TestUserDialogue_ConcludeCarriesNoDischargeGate pins the un-gated conclude: no
// step stands between the junction's answer and the end, so a session is
// closable without first discharging what it raised — which loose ends deserve
// carrying is the user's judgment, not the engine's (d-tac-k4q).
func TestUserDialogue_ConcludeCarriesNoDischargeGate(t *testing.T) {
	env := newShellEnv(t)
	if env.spec.StepByID["threads"] != nil {
		t.Fatal("the shell must carry no thread-settling step between conclude and the end")
	}
	wrap := env.spec.StepByID["wrap"]
	if wrap == nil {
		t.Fatal("the shell should reach its end through wrap")
	}
	if len(wrap.Transitions) != 1 {
		t.Fatalf("wrap must hold exactly one transition, got %+v", wrap.Transitions)
	}
	if to := wrap.Transitions[0]; !to.Otherwise || !IsEndTarget(to.To) {
		t.Fatalf("wrap must end unconditionally, got %+v", to)
	}
	// The teaching half of leaving threads behind: the agent names what is open
	// before the offer, since the answer is final.
	for _, want := range []string{"name each of those threads", "left behind", "stays listed and resumable"} {
		if !strings.Contains(env.spec.Units["open"], want) {
			t.Errorf("the opening unit should teach ending the session; missing %q", want)
		}
	}
}
