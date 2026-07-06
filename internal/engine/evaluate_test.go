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
	if sv.Step != "scope" {
		t.Fatalf("start step = %s, want scope", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "chains("+procAnchorID+")") {
		t.Errorf("scope unit should serve the anchor's chains, got %q", sv.Instructions)
	}
	if !strings.Contains(sv.Instructions, "verification") || !strings.Contains(sv.Instructions, "validation") {
		t.Errorf("scope unit should name the lens postures, got %q", sv.Instructions)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"plan":        "Inner only: check the done's claims against the ACs and the project's Go guidelines.",
		"widenReport": "searched for post-landing signals and prior evaluation dones; none recorded yet",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "carryOut" {
		t.Fatalf("after scope step = %s, want carryOut", sv.Step)
	}

	sv, err = env.session.Report(sv.Instance, map[string]any{
		"innerEvidence":   "read the diff against the ACs; go test ./... green",
		"innerEvaluation": "sound — matches the ACs; one rough edge in teardown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "junction" || sv.Chooser == nil || sv.Chooser.Kind != ChooserUser {
		t.Fatalf("carried-out evaluation should reach the junction user chooser, got step %q", sv.Step)
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

	// A resolved anchor advances to scope — the same place a seeded anchor
	// auto-advances to on entry.
	sv, err = env.session.Report(sv.Instance, map[string]any{"anchor": procAnchorID})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "scope" {
		t.Fatalf("after resolving the anchor, step = %s, want scope", sv.Step)
	}
}

func TestEvaluate_LensGate(t *testing.T) {
	env := newProcEnv(t, "evaluate")

	sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"plan":        "both lenses",
		"widenReport": "widened",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "carryOut" {
		t.Fatalf("step = %s, want carryOut", sv.Step)
	}

	// Evidence alone is not a judgment — the gate holds until at least one
	// lens evaluation lands (evidence is instructed, never gated).
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"innerEvidence": "ran the suite",
		"outerEvidence": "smoke test output",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "carryOut" {
		t.Fatalf("evidence without a judgment advanced to %s, want carryOut held", sv.Step)
	}

	// A single lens judgment satisfies the gate — outer alone here; coverage
	// completeness is a graph property, not a per-run gate.
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"outerEvaluation": "works in use; the user attested the flow end to end",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "junction" {
		t.Fatalf("one lens judgment should reach the junction, got %s", sv.Step)
	}
}

func TestEvaluate_CleanPassRecordsWithoutFindings(t *testing.T) {
	env := newProcEnv(t, "evaluate")

	sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"plan":        "inner only",
		"widenReport": "nothing new since landing",
	})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"innerEvaluation": "clean — claims match the commitment",
	})
	if err != nil {
		t.Fatal(err)
	}
	// A clean pass records the evaluation done alone: record with no
	// selectedFindings is a valid answer (the field is optional).
	sv, err = env.session.Answer(sv.Instance, "junction", "record", nil, "record the evaluation, no findings")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("record without findings should complete, got %s at %q", sv.Status, sv.Step)
	}
}

func TestEvaluate_Conclude(t *testing.T) {
	env := newProcEnv(t, "evaluate")

	sv, err := env.session.Start(env.spec, map[string]any{"anchor": procAnchorID}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"plan":        "outer only",
		"widenReport": "widened",
	})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, map[string]any{
		"outerEvaluation": "fine in use",
	})
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Answer(sv.Instance, "junction", "conclude", nil, "don't record this one")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("conclude should complete, got %s at %q", sv.Status, sv.Step)
	}
}

func TestEvaluate_RecordDeclaresCaptureHandoff(t *testing.T) {
	env := newProcEnv(t, "evaluate")

	// The record option's declared dispatch is the contract the evaluation-done
	// and finding captures ride: procedure-guarded to capture, seeding the
	// evaluation's own widenReport. Spec-level assertion so drift between the
	// shipped entry and the seeding machinery cannot pass unnoticed.
	var junction *Step
	for i := range env.spec.Steps {
		if env.spec.Steps[i].ID == "junction" {
			junction = env.spec.Steps[i]
		}
	}
	if junction == nil {
		t.Fatal("evaluate spec has no junction step")
	}
	var record *Option
	for i := range junction.Options {
		if junction.Options[i].Choice == "record" {
			record = &junction.Options[i]
		}
	}
	if record == nil {
		t.Fatal("junction has no record option")
	}
	if record.Dispatch == nil {
		t.Fatal("record declares no dispatch — the capture handoff is gone")
	}
	if record.Dispatch.Procedure != "capture" {
		t.Errorf("record dispatch procedure = %q, want capture", record.Dispatch.Procedure)
	}
	if got := record.Dispatch.Seed["widenReport"]; got != "widenReport" {
		t.Errorf("record dispatch seed widenReport = %q, want widenReport", got)
	}
}
