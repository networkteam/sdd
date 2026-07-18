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
// the harness stubs them with a toggle for quiescence.

type shellEnv struct {
	spec      *Spec
	session   *Session
	quiescent bool
}

func newShellEnv(t *testing.T) *shellEnv {
	t.Helper()
	env := &shellEnv{quiescent: true}

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
	// The shell declares its framing lanes as viewLayout injects; the MCP shell
	// registers the real query in production. Stub it so the base entry loads.
	mustRegisterQuery(reg, Query{
		Doc: FuncDoc{Name: "viewLayout", Doc: "fake view layout"},
		Fn: func(_ *Context, args map[string]any) (any, error) {
			layout, _ := args["layout"].(string)
			return "view: " + layout, nil
		},
	})
	mustRegisterPredicate(reg, Predicate{
		Doc: FuncDoc{Name: "sessionQuiescent", Doc: "fake quiescence"},
		Fn: func(_ *Context) (bool, error) {
			return env.quiescent, nil
		},
		FailMessage: "open threads remain",
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

func TestUserDialogue_OpeningServeAndQuietConclude(t *testing.T) {
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
	for _, want := range []string{"Participant: christopher", "pending write awaits explicit recovery", "- capture — ", "Standing goal"} {
		if !strings.Contains(sv.Instructions, want) {
			t.Errorf("opening serve should carry %q, got %q", want, sv.Instructions)
		}
	}

	// Conclude on a quiescent session cascades through wrap to completion.
	sv, err = env.session.Answer(sv.Instance, "junction", "conclude", nil, "we're done")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("conclude on a quiescent session should complete, got %s at %q", sv.Status, sv.Step)
	}
}

func TestUserDialogue_ConcludeWalksThreadsThenParks(t *testing.T) {
	env := newShellEnv(t)
	env.quiescent = false

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	// Open threads hold the wrap gate: conclude routes to the threads step.
	sv, err = env.session.Answer(sv.Instance, "junction", "conclude", nil, "wrap it up")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "threads" || sv.Chooser == nil || sv.Chooser.Kind != ChooserUser {
		t.Fatalf("conclude with open threads should reach the threads chooser, got %s at %q", sv.Status, sv.Step)
	}
	if !strings.Contains(sv.Instructions, "settle each") {
		t.Fatalf("threads unit should guide per-thread decisions, got %q", sv.Instructions)
	}

	// Park returns to the resident junction, which re-serves.
	sv, err = env.session.Answer(sv.Instance, "threads", "park", nil, "keep it for later")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "junction" || sv.Status != StatusRunning {
		t.Fatalf("park should return to the junction, got %s at %q", sv.Status, sv.Step)
	}

	// Settled with the threads actually closed completes through wrap.
	sv, err = env.session.Answer(sv.Instance, "junction", "conclude", nil, "trying again")
	if err != nil {
		t.Fatal(err)
	}
	env.quiescent = true
	sv, err = env.session.Answer(sv.Instance, "threads", "settled", nil, "all closed out")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("settled threads should complete the shell, got %s at %q", sv.Status, sv.Step)
	}
}
