package engine

import (
	"strings"
	"testing"
)

// Per-procedure table tests for the embedded evaluate entry, driving the
// shipped base entry through the production loader (see engage_explore_test
// for the shared harness).

func TestEvaluate_HappyPath(t *testing.T) {
	env := newProcEnv(t, "evaluate")

	sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assess" {
		t.Fatalf("start step = %s, want assess", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "chains("+procAnchorID+")") {
		t.Errorf("assess unit should serve the anchor's chains, got %q", sv.Instructions)
	}
	if !strings.Contains(sv.Instructions, "Two lenses — always both") {
		t.Errorf("assess unit should carry the two-lens contract, got %q", sv.Instructions)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"evaluation":  "Inner: matches the ACs. Outer: smoke test passed; one rough edge in teardown.",
		"widenReport": "searched for post-landing signals; one teardown gap nearby",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "junction" || sv.Chooser == nil || sv.Chooser.Kind != ChooserUser {
		t.Fatalf("assessment should reach the junction user chooser, got step %q", sv.Step)
	}

	sv, err = env.session.Answer(sv.Instance, "junction", "record",
		map[string]any{"selectedFindings": "the teardown rough edge"}, "record the teardown finding")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("record should complete the evaluation, got %s at %q", sv.Status, sv.Step)
	}
}

func TestEvaluate_AnchorResolvedByResolver(t *testing.T) {
	env := newProcEnv(t, "evaluate")

	// The uniform anchor contract: a cold start with no anchor does not fail —
	// it stalls at the resolver step, naming the anchor as what advances it,
	// where the agent resolves the user's pointer. A required param would
	// reject instead; the resolver keeps resolution in dialogue.
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatalf("start without anchor should stall at the resolver, not error: %v", err)
	}
	if sv.Step != "anchor" {
		t.Fatalf("cold start step = %s, want the anchor resolver", sv.Step)
	}
	missingAnchor := false
	for _, m := range sv.Missing {
		if m == "anchor" {
			missingAnchor = true
		}
	}
	if !missingAnchor {
		t.Errorf("resolver should name anchor as missing, got %v", sv.Missing)
	}

	// A resolved anchor advances to assess — the same place a seeded anchor
	// auto-advances to on entry.
	sv, err = env.session.Report(sv.Instance, map[string]any{"anchor": procAnchorID})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assess" {
		t.Fatalf("after resolving the anchor, step = %s, want assess", sv.Step)
	}
}

func TestEvaluate_CleanPassConcludes(t *testing.T) {
	env := newProcEnv(t, "evaluate")

	sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"evaluation":  "Both lenses clean.",
		"widenReport": "nothing new since landing",
	})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Answer(sv.Instance, "junction", "conclude", nil, "all good, nothing to record")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("conclude should complete, got %s at %q", sv.Status, sv.Step)
	}
}
