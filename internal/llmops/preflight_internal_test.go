package llmops

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/basefacts"
	"github.com/networkteam/sdd/internal/model"
)

func Test_selectCheckType(t *testing.T) {
	signal := entry("20260410-120000-s-cpt-aaa", withContent("some signal")) // default kind: empty, treated as gap
	gapSignal := entry("20260410-120100-s-cpt-gap", withKind(model.KindGap), withContent("gap signal"))
	questionSignal := entry("20260410-120200-s-cpt-qst", withKind(model.KindQuestion), withContent("question signal"))
	factSignal := entry("20260410-120300-s-cpt-fct", withKind(model.KindFact), withContent("fact signal"))
	decision := entry("20260410-120000-d-tac-bbb", withContent("some decision"))
	plan := entry("20260410-120000-d-tac-ccc", withKind(model.KindPlan), withContent("some plan"))
	contract := entry("20260410-120000-d-prc-ctr", withKind(model.KindContract), withContent("some contract"))
	aspiration := entry("20260410-120000-d-stg-asp", withKind(model.KindAspiration), withContent("some aspiration"))

	graph := model.NewGraph([]*model.Entry{signal, gapSignal, questionSignal, factSignal, decision, plan, contract, aspiration})

	tests := []struct {
		name     string
		entry    *model.Entry
		expected checkType
	}{
		{
			name:     "done signal closing decision",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindDone, Closes: []string{decision.ID}},
			expected: checkClosingDone,
		},
		{
			name:     "done signal closing plan",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindDone, Closes: []string{plan.ID}},
			expected: checkClosingDone,
		},
		{
			name:     "done signal closing gap signal (short-loop)",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindDone, Closes: []string{gapSignal.ID}},
			expected: checkShortLoop,
		},
		{
			name:     "done signal closing both decision and signal routes to closing-done",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindDone, Closes: []string{decision.ID, gapSignal.ID}},
			expected: checkClosingDone,
		},
		{
			name:     "fact signal closing question (dissolution)",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindFact, Closes: []string{questionSignal.ID}},
			expected: checkDissolution,
		},
		{
			name:     "insight signal closing question (dissolution)",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindInsight, Closes: []string{questionSignal.ID}},
			expected: checkDissolution,
		},
		{
			name:     "fact signal closing non-question routes to closing-signal",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindFact, Closes: []string{factSignal.ID}},
			expected: checkClosingSignal,
		},
		{
			name:     "insight closing question and fact (mixed targets) routes to closing-signal",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindInsight, Closes: []string{questionSignal.ID, factSignal.ID}},
			expected: checkClosingSignal,
		},
		{
			name:     "gap signal closing fact routes to closing-signal",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindGap, Closes: []string{factSignal.ID}},
			expected: checkClosingSignal,
		},
		{
			name:     "gap signal closing gap routes to closing-signal",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindGap, Closes: []string{gapSignal.ID}},
			expected: checkClosingSignal,
		},
		{
			name:     "fact closing unresolvable target routes to closing-signal",
			entry:    &model.Entry{Type: model.TypeSignal, Kind: model.KindFact, Closes: []string{"20260410-999999-s-cpt-zzz"}},
			expected: checkClosingSignal,
		},
		{
			name:     "decision closing signal",
			entry:    &model.Entry{Type: model.TypeDecision, Closes: []string{signal.ID}},
			expected: checkClosingDecision,
		},
		{
			name:     "directive closing contract (retirement)",
			entry:    &model.Entry{Type: model.TypeDecision, Kind: model.KindDirective, Closes: []string{contract.ID}},
			expected: checkClosingDecision,
		},
		{
			name:     "aspiration decision with refs routes to aspiration-capture",
			entry:    &model.Entry{Type: model.TypeDecision, Kind: model.KindAspiration, Refs: refsOf(signal.ID)},
			expected: checkAspirationCapture,
		},
		{
			name:     "aspiration decision with no refs routes to aspiration-capture",
			entry:    &model.Entry{Type: model.TypeDecision, Kind: model.KindAspiration},
			expected: checkAspirationCapture,
		},
		{
			name:     "decision with refs only",
			entry:    &model.Entry{Type: model.TypeDecision, Refs: refsOf(signal.ID)},
			expected: checkDecisionRefs,
		},
		{
			name:     "decision with no refs or closes",
			entry:    &model.Entry{Type: model.TypeDecision},
			expected: checkDecisionRefs,
		},
		{
			name:     "signal",
			entry:    &model.Entry{Type: model.TypeSignal},
			expected: checkSignalCapture,
		},
		{
			name:     "signal with refs",
			entry:    &model.Entry{Type: model.TypeSignal, Refs: refsOf(decision.ID)},
			expected: checkSignalCapture,
		},
		{
			name:     "supersedes takes priority over closes",
			entry:    &model.Entry{Type: model.TypeDecision, Supersedes: []string{decision.ID}, Closes: []string{signal.ID}},
			expected: checkSupersedes,
		},
		{
			name:     "supersedes on signal",
			entry:    &model.Entry{Type: model.TypeSignal, Supersedes: []string{signal.ID}},
			expected: checkSupersedes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectCheckType(tt.entry, graph)
			if got != tt.expected {
				t.Errorf("selectCheckType() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func Test_checkTypeString(t *testing.T) {
	tests := []struct {
		ct   checkType
		want string
	}{
		{checkClosingDone, "closing-done"},
		{checkClosingDecision, "closing-decision"},
		{checkDecisionRefs, "decision-refs"},
		{checkShortLoop, "short-loop"},
		{checkDissolution, "dissolution"},
		{checkAspirationCapture, "aspiration-capture"},
		{checkSignalCapture, "signal-capture"},
		{checkSupersedes, "supersedes"},
		{checkType(99), "unknown(99)"},
	}

	for _, tt := range tests {
		if got := tt.ct.String(); got != tt.want {
			t.Errorf("checkType(%d).String() = %q, want %q", int(tt.ct), got, tt.want)
		}
	}
}

func Test_FormatEntryForPrompt(t *testing.T) {
	e := &model.Entry{
		ID:         "20260410-120000-d-tac-xyz",
		Type:       model.TypeDecision,
		Layer:      model.LayerTactical,
		Kind:       model.KindPlan,
		Refs:       refsOf("20260410-110000-s-cpt-aaa"),
		Closes:     []string{"20260410-100000-s-stg-bbb"},
		Confidence: "high",
		Content:    "Build the validator.",
	}

	result := FormatEntryForPrompt(e)

	checks := []string{
		"ID: 20260410-120000-d-tac-xyz",
		"Type: decision",
		"Layer: tactical",
		"Kind: plan",
		"Refs:",
		"- 20260410-110000-s-cpt-aaa (kind: related)",
		"Closes: 20260410-100000-s-stg-bbb",
		"Confidence: high",
		"Build the validator.",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("FormatEntryForPrompt() missing %q\nGot:\n%s", check, result)
		}
	}
}

func Test_FormatEntryForPrompt_ShowsKindForDecisions(t *testing.T) {
	e := &model.Entry{
		ID:      "20260410-120000-d-tac-xyz",
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Kind:    model.KindDirective,
		Content: "Some directive.",
	}

	result := FormatEntryForPrompt(e)
	if !strings.Contains(result, "Kind: directive") {
		t.Errorf("FormatEntryForPrompt() should show Kind for decisions, got:\n%s", result)
	}
}

func Test_FormatEntryForPrompt_ShowsKindForSignals(t *testing.T) {
	// Regression for s-prc-y0e: pre-flight reported `missing-explicit-kind`
	// as a medium finding on insight signals that carried an explicit kind,
	// because FormatEntryForPrompt gated the Kind line on type == decision.
	// The LLM was telling the truth — the prompt genuinely didn't contain the
	// field. Under the two-type system (d-cpt-ydf), kind is structural on
	// signals too and must flow into the pre-flight prompt.
	e := &model.Entry{
		ID:      "20260420-163043-s-stg-ljq",
		Type:    model.TypeSignal,
		Layer:   model.LayerStrategic,
		Kind:    model.KindInsight,
		Content: "Some insight.",
	}

	result := FormatEntryForPrompt(e)
	if !strings.Contains(result, "Kind: insight") {
		t.Errorf("FormatEntryForPrompt() should show explicit Kind for signals, got:\n%s", result)
	}
}

func Test_FormatEntryForPrompt_OmitsEmptyFields(t *testing.T) {
	e := &model.Entry{
		ID:      "20260410-120000-s-cpt-xyz",
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "Observed something.",
	}

	result := FormatEntryForPrompt(e)
	for _, field := range []string{"Kind:", "Refs:", "Closes:", "Supersedes:", "Confidence:"} {
		if strings.Contains(result, field) {
			t.Errorf("FormatEntryForPrompt() should omit empty %s, got:\n%s", field, result)
		}
	}
}

// Test_FormatEntryForPrompt_LegacyRefsRenderFlat pins the flat-ref fallback for
// entries whose refs are all bare-string legacy (Kind == RefKindUnknown): with
// no kind or desc to show, the object form would only add empty `(kind:
// unknown)` noise, so these render as the flat `Refs: a, b` list.
func Test_FormatEntryForPrompt_LegacyRefsRenderFlat(t *testing.T) {
	e := &model.Entry{
		ID:    "20260410-120000-d-tac-xyz",
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindPlan,
		Refs: []model.Ref{
			{ID: "20260410-110000-s-cpt-aaa", Kind: model.RefKindUnknown},
			{ID: "20260410-110100-s-cpt-bbb", Kind: model.RefKindUnknown},
		},
		Content: "Plan body.",
	}

	want := "ID: 20260410-120000-d-tac-xyz\n" +
		"Type: decision\n" +
		"Layer: tactical\n" +
		"Kind: plan\n" +
		"Refs: 20260410-110000-s-cpt-aaa, 20260410-110100-s-cpt-bbb\n" +
		"\nPlan body."

	got := FormatEntryForPrompt(e)
	if got != want {
		t.Errorf("FormatEntryForPrompt() with all-legacy refs:\nwant:\n%q\n\ngot:\n%q", want, got)
	}

	// Sanity: the multi-line markers must not appear when all refs are legacy.
	if strings.Contains(got, "(kind:") {
		t.Errorf("FormatEntryForPrompt() with all-legacy refs leaked the multi-line kind marker:\n%s", got)
	}
}

// Test_FormatEntryForPrompt_ObjectRefsRenderMultiline pins the new format for
// entries authored under the per-ref-kind contract. At least one object-form
// ref triggers multi-line rendering; the LLM ref-meta consistency check sees
// the (kind, desc) metadata in its prompt context.
func Test_FormatEntryForPrompt_ObjectRefsRenderMultiline(t *testing.T) {
	e := &model.Entry{
		ID:    "20260520-120000-d-tac-zzz",
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindDirective,
		Refs: []model.Ref{
			{ID: "20260519-100000-s-cpt-aaa", Kind: model.RefKindAddresses, Desc: "responds to the gap"},
			{ID: "20260518-100000-d-stg-bbb", Kind: model.RefKindGroundedIn},
		},
		Content: "Refinement body.",
	}

	got := FormatEntryForPrompt(e)
	wantLines := []string{
		"Refs:\n",
		"  - 20260519-100000-s-cpt-aaa (kind: addresses): responds to the gap\n",
		"  - 20260518-100000-d-stg-bbb (kind: grounded-in)\n",
	}
	for _, line := range wantLines {
		if !strings.Contains(got, line) {
			t.Errorf("FormatEntryForPrompt() with object refs missing %q\nGot:\n%s", line, got)
		}
	}
	if strings.Contains(got, "Refs: 20260519-100000-s-cpt-aaa") {
		t.Errorf("FormatEntryForPrompt() with object refs leaked the flat ref line:\n%s", got)
	}
}

// Test_FormatEntryForPrompt_MixedRefsRenderMultiline verifies that the
// presence of any object-form ref triggers multi-line rendering — a single
// non-legacy ref is enough to signal a modern entry. This is rare in practice
// (entries are immutable, so mixed shapes shouldn't appear in normal use) but
// the rendering must remain unambiguous if it ever does.
func Test_FormatEntryForPrompt_MixedRefsRenderMultiline(t *testing.T) {
	e := &model.Entry{
		ID:    "20260520-120000-d-tac-mix",
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Kind:  model.KindDirective,
		Refs: []model.Ref{
			{ID: "20260410-100000-s-cpt-legacy", Kind: model.RefKindUnknown},
			{ID: "20260520-100000-d-tac-modern", Kind: model.RefKindBuildsOn},
		},
		Content: "Mixed body.",
	}

	got := FormatEntryForPrompt(e)
	if !strings.Contains(got, "  - 20260520-100000-d-tac-modern (kind: builds-on)") {
		t.Errorf("FormatEntryForPrompt() with mixed refs should render multi-line, got:\n%s", got)
	}
	if strings.Contains(got, "Refs: 20260410-100000-s-cpt-legacy, 20260520-100000-d-tac-modern\n") {
		t.Errorf("FormatEntryForPrompt() with any object-form ref should not render the flat list, got:\n%s", got)
	}
}

// Test_FormatEntryForPrompt_LegacyAliasRendersCanonical confirms the unified
// prompt renders a legacy grounds/evidence ref with its canonical resolved kind
// (grounded-in). Summary generation and pre-flight share this single render
// path now that no summary hash depends on the on-disk alias value.
func Test_FormatEntryForPrompt_LegacyAliasRendersCanonical(t *testing.T) {
	for _, onDisk := range []string{"grounds", "evidence"} {
		t.Run(onDisk, func(t *testing.T) {
			src := "---\n" +
				"type: decision\n" +
				"layer: tactical\n" +
				"kind: directive\n" +
				"refs:\n" +
				"  - id: 20260410-110000-s-cpt-aaa\n" +
				"    kind: " + onDisk + "\n" +
				"---\n" +
				"Body."
			e, err := model.ParseEntry("20260410-120000-d-tac-xyz.md", src)
			if err != nil {
				t.Fatalf("ParseEntry: %v", err)
			}
			render := FormatEntryForPrompt(e)
			if !strings.Contains(render, "(kind: grounded-in)") {
				t.Errorf("legacy %s ref must render canonical grounded-in, got:\n%s", onDisk, render)
			}
			if strings.Contains(render, "(kind: "+onDisk+")") {
				t.Errorf("legacy %s ref must not render the on-disk alias, got:\n%s", onDisk, render)
			}
		})
	}
}

func Test_assembleContext_BasicSignal(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("observed something"))
	graph := model.NewGraph([]*model.Entry{sig})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "new signal",
	}

	pctx := assembleContext(proposed, graph, checkSignalCapture, "")

	if !strings.Contains(pctx.ProposedEntry, "new signal") {
		t.Error("ProposedEntry should contain the signal content")
	}
	if pctx.ClosedEntries != "" {
		t.Error("ClosedEntries should be empty for signal capture")
	}
	if pctx.SupersededEntries != "" {
		t.Error("SupersededEntries should be empty")
	}
}

func Test_assembleContext_WithRefs(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("first signal"))
	dec := entry("20260410-130000-d-tac-bbb", withContent("decision content"), withRefs(sig.ID))
	graph := model.NewGraph([]*model.Entry{sig, dec})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Refs:    refsOf(sig.ID),
		Content: "new decision",
	}

	pctx := assembleContext(proposed, graph, checkDecisionRefs, "")

	if !strings.Contains(pctx.ReferencedEntries, "first signal") {
		t.Error("ReferencedEntries should contain the referenced signal content")
	}
}

// Test_assembleContext_RefsCarryDerivedStatus verifies that each referenced
// entry in the pre-flight context carries its derived status — the signal the
// ref-meta check needs to tell grounded-in / builds-on / refines apart.
func Test_assembleContext_RefsCarryDerivedStatus(t *testing.T) {
	gap := &model.Entry{ID: "20260410-120000-s-cpt-gap", Type: model.TypeSignal, Kind: model.KindGap, Content: "a gap"}
	done := &model.Entry{ID: "20260410-121000-s-tac-don", Type: model.TypeSignal, Kind: model.KindDone, Closes: []string{gap.ID}, Content: "did it"}
	active := &model.Entry{ID: "20260410-122000-d-tac-act", Type: model.TypeDecision, Kind: model.KindDirective, Content: "active directive"}
	graph := model.NewGraph([]*model.Entry{gap, done, active})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Refs:    refsOf(gap.ID, active.ID),
		Content: "new decision",
	}

	pctx := assembleContext(proposed, graph, checkDecisionRefs, "")

	if !strings.Contains(pctx.ReferencedEntries, "Derived status: active") {
		t.Errorf("expected active ref to carry 'Derived status: active'\n%s", pctx.ReferencedEntries)
	}
	if !strings.Contains(pctx.ReferencedEntries, "Derived status: closed by "+done.ID) {
		t.Errorf("expected closed ref to carry 'Derived status: closed by %s'\n%s", done.ID, pctx.ReferencedEntries)
	}
}

func Test_assembleContext_RefsCarryApplicabilityLines(t *testing.T) {
	done := &model.Entry{ID: "20260410-121000-s-tac-don", Type: model.TypeSignal, Kind: model.KindDone, Content: "did it"}
	gap := &model.Entry{ID: "20260410-120000-s-cpt-gap", Type: model.TypeSignal, Kind: model.KindGap, Content: "a gap"}
	graph := model.NewGraph([]*model.Entry{done, gap})

	proposed := &model.Entry{
		Type:  model.TypeDecision,
		Layer: model.LayerTactical,
		Refs: []model.Ref{
			{ID: done.ID, Kind: model.RefKindBuildsOn},
			{ID: gap.ID, Kind: model.RefKindAddresses},
		},
		Content: "new decision",
	}

	pctx := assembleContext(proposed, graph, checkDecisionRefs, "")

	// Each ref block names the admissible set for its target class and
	// settles applicability for the chosen kind.
	if !strings.Contains(pctx.ReferencedEntries, "Admissible ref kinds for this target (terminal-done):") {
		t.Errorf("expected admissible-kind line for the terminal done target\n%s", pctx.ReferencedEntries)
	}
	if !strings.Contains(pctx.ReferencedEntries, "Admissible ref kinds for this target (live-signal):") {
		t.Errorf("expected admissible-kind line for the open gap target\n%s", pctx.ReferencedEntries)
	}
	if !strings.Contains(pctx.ReferencedEntries, "Chosen ref kind: builds-on — applicable:") {
		t.Errorf("expected chosen-kind applicable line for builds-on on terminal done\n%s", pctx.ReferencedEntries)
	}
	if !strings.Contains(pctx.ReferencedEntries, "Applicability is settled mechanically; do not flag it.") {
		t.Errorf("expected the settled-mechanically instruction\n%s", pctx.ReferencedEntries)
	}
	// The terminal-done admissible set must not offer the two excluded kinds.
	if strings.Contains(pctx.ReferencedEntries, "(terminal-done): grounded-in, builds-on, refines") {
		t.Errorf("terminal-done admissible set must exclude refines\n%s", pctx.ReferencedEntries)
	}
}

func Test_assembleContext_LegacyRefKindNoApplicabilityLines(t *testing.T) {
	// Legacy bare-string refs (kind unknown) get no applicability lines —
	// the capturable-kind mechanical check rejects them; there is nothing
	// for the validator to judge.
	target := &model.Entry{ID: "20260410-120000-s-cpt-tgt", Type: model.TypeSignal, Kind: model.KindGap, Content: "target"}
	graph := model.NewGraph([]*model.Entry{target})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Refs:    []model.Ref{{ID: target.ID, Kind: model.RefKindUnknown}},
		Content: "new decision",
	}

	pctx := assembleContext(proposed, graph, checkDecisionRefs, "")
	if strings.Contains(pctx.ReferencedEntries, "Admissible ref kinds") {
		t.Errorf("unknown kind must not render applicability lines\n%s", pctx.ReferencedEntries)
	}
	if !strings.Contains(pctx.ReferencedEntries, "Derived status:") {
		t.Errorf("derived status line should still render for legacy refs\n%s", pctx.ReferencedEntries)
	}
}

// Test_assembleContext_SupersededRefSurfacesLiveHead verifies s-tac-5p5 at the
// pre-flight layer: when a ref points at a superseded entry, the literal target
// is kept (its superseded status is what the ref-meta check reasons against) and
// the live head's content is surfaced alongside, labeled, so the validator can
// reason about the current entity rather than a retired intermediate.
func Test_assembleContext_SupersededRefSurfacesLiveHead(t *testing.T) {
	old := entry("20260410-100000-d-cpt-old", withContent("old contract text"))
	head := entry("20260410-110000-d-cpt-new", withContent("live head text"), withSupersedes(old.ID))
	graph := model.NewGraph([]*model.Entry{old, head})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Refs:    refsOf(old.ID),
		Content: "new decision grounded in the old contract",
	}

	pctx := assembleContext(proposed, graph, checkDecisionRefs, "")

	if !strings.Contains(pctx.ReferencedEntries, "old contract text") {
		t.Errorf("should keep the literal (superseded) target's content\n%s", pctx.ReferencedEntries)
	}
	if !strings.Contains(pctx.ReferencedEntries, "(live head of "+old.ID+")") {
		t.Errorf("should label the resolved live head\n%s", pctx.ReferencedEntries)
	}
	if !strings.Contains(pctx.ReferencedEntries, "live head text") {
		t.Errorf("should surface the live head's content\n%s", pctx.ReferencedEntries)
	}
}

func Test_derivedStatusForPrompt(t *testing.T) {
	cases := []struct {
		status model.Status
		want   string
	}{
		{model.Status{Kind: model.StatusActive}, "active"},
		{model.Status{Kind: model.StatusOpen}, "open"},
		{model.Status{Kind: model.StatusClosedBy, By: "20260101-000000-s-tac-xyz"}, "closed by 20260101-000000-s-tac-xyz"},
		{model.Status{Kind: model.StatusSupersededBy, By: "20260101-000000-d-tac-abc"}, "superseded by 20260101-000000-d-tac-abc"},
		{model.Status{Kind: model.StatusNone}, "terminal (done signal — no lifecycle status)"},
		{model.Status{Kind: model.StatusCascadeClosedBy, By: "20260101-000000-s-prc-act"}, "role retired (bound actor chain closed by 20260101-000000-s-prc-act)"},
		{model.Status{Kind: model.StatusCascadeOrphan}, "orphan role (no matching actor chain)"},
	}
	for _, c := range cases {
		if got := derivedStatusForPrompt(c.status); got != c.want {
			t.Errorf("derivedStatusForPrompt(%+v) = %q, want %q", c.status, got, c.want)
		}
	}
}

// Test_RenderSummaryPrompt_OmitsDerivedStatus guards AC 5: status threading is
// pre-flight only. The summary prompt must NOT carry derived-status lines, so
// adding the status to the pre-flight context never drifts summary-prompt
// hashes (which would force graph-wide re-summarization).
func Test_RenderSummaryPrompt_OmitsDerivedStatus(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("first signal"))
	dec := &model.Entry{
		ID:      "20260410-130000-d-tac-bbb",
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Kind:    model.KindDirective,
		Refs:    refsOf(sig.ID),
		Content: "decision content",
	}
	graph := model.NewGraph([]*model.Entry{sig, dec})

	req, err := RenderSummaryPrompt(dec, graph, "")
	if err != nil {
		t.Fatalf("RenderSummaryPrompt: %v", err)
	}
	if strings.Contains(req.Combined(), "Derived status:") {
		t.Errorf("summary prompt must not include derived status (pre-flight only)\n%s", req.Combined())
	}
}

func Test_assembleContext_ProposedEntryShowsIntent(t *testing.T) {
	graph := model.NewGraph(nil)
	proposed := &model.Entry{
		Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective,
		Intent: model.IntentSettled, Content: "Keep X as-is; no follow-up needed.",
	}
	pctx := assembleContext(proposed, graph, checkDecisionRefs, "")
	if !strings.Contains(pctx.ProposedEntry, "Intent: settled") {
		t.Errorf("ProposedEntry should surface the directive intent for the settled rubric:\n%s", pctx.ProposedEntry)
	}
}

func Test_renderPreflightPrompt_CarriesSettledRubric(t *testing.T) {
	// The settled-justification rubric lives unconditionally in the universal
	// system preamble (cache-stable); the LLM applies it via its prose guard.
	graph := model.NewGraph(nil)
	proposed := &model.Entry{
		Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective,
		Intent: model.IntentSettled, Content: "Keep X as-is.",
	}
	pctx := assembleContext(proposed, graph, checkDecisionRefs, "")
	req, err := renderPreflightPrompt(checkDecisionRefs, pctx)
	if err != nil {
		t.Fatalf("renderPreflightPrompt: %v", err)
	}
	if !strings.Contains(req.SystemPrompt, "Settled-directive justification") {
		t.Errorf("system prompt should carry the settled-justification rubric:\n%s", req.SystemPrompt)
	}
}

func Test_RenderSummaryPrompt_OmitsIntent(t *testing.T) {
	// Intent is a pre-flight-only addition; the summary prompt must not carry
	// it — summary generation has no use for the directive's lifecycle posture.
	dec := &model.Entry{
		ID: "20260410-130000-d-tac-bbb", Type: model.TypeDecision, Layer: model.LayerTactical,
		Kind: model.KindDirective, Intent: model.IntentSettled, Content: "decision content",
	}
	graph := model.NewGraph([]*model.Entry{dec})
	req, err := RenderSummaryPrompt(dec, graph, "")
	if err != nil {
		t.Fatalf("RenderSummaryPrompt: %v", err)
	}
	if strings.Contains(req.Combined(), "Intent: settled") {
		t.Errorf("summary prompt must not include intent (pre-flight only)\n%s", req.Combined())
	}
}

func Test_assembleContext_WithCloses(t *testing.T) {
	sig := entry("20260410-120000-s-cpt-aaa", withContent("signal to close"))
	graph := model.NewGraph([]*model.Entry{sig})

	proposed := &model.Entry{
		Type:    model.TypeDecision,
		Layer:   model.LayerConceptual,
		Closes:  []string{sig.ID},
		Content: "decision closing signal",
	}

	pctx := assembleContext(proposed, graph, checkClosingDecision, "")

	if !strings.Contains(pctx.ClosedEntries, "signal to close") {
		t.Error("ClosedEntries should contain the closed signal content")
	}
}

func Test_assembleContext_WithContracts(t *testing.T) {
	contract := entry("20260410-120000-d-prc-aaa",
		withKind(model.KindContract),
		withContent("all entries must have refs"))
	graph := model.NewGraph([]*model.Entry{contract})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Layer:   model.LayerConceptual,
		Content: "some signal",
	}

	pctx := assembleContext(proposed, graph, checkSignalCapture, "")

	if !strings.Contains(pctx.ActiveContracts, "all entries must have refs") {
		t.Error("ActiveContracts should contain contract content")
	}
}

func Test_assembleContext_WithSupersedes(t *testing.T) {
	old := entry("20260410-120000-d-tac-aaa", withContent("old decision"))
	graph := model.NewGraph([]*model.Entry{old})

	proposed := &model.Entry{
		Type:       model.TypeDecision,
		Layer:      model.LayerTactical,
		Supersedes: []string{old.ID},
		Content:    "replacement decision",
	}

	pctx := assembleContext(proposed, graph, checkSupersedes, "")

	if !strings.Contains(pctx.SupersededEntries, "old decision") {
		t.Error("SupersededEntries should contain the superseded entry content")
	}
}

func Test_assembleContext_ClosedPlanDescriptionFlowsThrough(t *testing.T) {
	// Plans carry their AC section inline in the description; a done signal
	// closing a plan flows that description through ClosedEntries via
	// FormatEntryForPrompt — no attachment extraction needed.
	plan := entry("20260410-120000-d-tac-pln",
		withKind(model.KindPlan),
		withContent("Plan body.\n\n## Acceptance criteria\n- [ ] finish X\n- [ ] finish Y\n"))
	graph := model.NewGraph([]*model.Entry{plan})

	proposed := &model.Entry{
		Type:    model.TypeSignal,
		Kind:    model.KindDone,
		Layer:   model.LayerTactical,
		Closes:  []string{plan.ID},
		Content: "done signal closing the plan",
	}

	pctx := assembleContext(proposed, graph, checkClosingDone, "")
	if !strings.Contains(pctx.ClosedEntries, "## Acceptance criteria") {
		t.Errorf("ClosedEntries should contain the plan's AC heading inline, got %q", pctx.ClosedEntries)
	}
	if !strings.Contains(pctx.ClosedEntries, "finish X") || !strings.Contains(pctx.ClosedEntries, "finish Y") {
		t.Errorf("ClosedEntries should contain checklist items from the plan description")
	}
}

func Test_renderPreflightPrompt_AllCheckTypes(t *testing.T) {
	pctx := &preflightContext{
		ProposedEntry:     "ID: test\nType: signal\n\nTest content",
		ReferencedEntries: "ID: ref\nType: signal\n\nRef content",
		ClosedEntries:     "ID: closed\nType: signal\n\nClosed content",
		SupersededEntries: "ID: old\nType: decision\n\nOld content",
		ActiveContracts:   "ID: contract\nType: decision\n\nContract content",
	}

	for ct, tmplName := range checkTypeTemplates {
		t.Run(ct.String(), func(t *testing.T) {
			result, err := renderPreflightPrompt(ct, pctx)
			if err != nil {
				t.Fatalf("renderPreflightPrompt(%s) error: %v", ct, err)
			}
			if result.Combined() == "" {
				t.Errorf("renderPreflightPrompt(%s) returned empty string", ct)
			}
			if !strings.Contains(result.Combined(), "Test content") {
				t.Errorf("renderPreflightPrompt(%s) missing proposed entry content", ct)
			}
			// Verdict partial must be embedded — the JSON output format and
			// severity semantics are the single source of truth for all checks.
			if !strings.Contains(result.Combined(), `"findings"`) {
				t.Errorf("renderPreflightPrompt(%s) missing JSON schema with findings key", ct)
			}
			if !strings.Contains(result.Combined(), `"severity"`) {
				t.Errorf("renderPreflightPrompt(%s) missing severity field in schema", ct)
			}
			// Category and severity are conclusions, not opening bids: the
			// schema must ask for the observation (reasoning) first, so a
			// finding cannot commit a verdict its own prose then retracts —
			// and category-first proved to get skipped by the model entirely.
			obsIdx := strings.Index(result.Combined(), `"observation"`)
			catIdx := strings.Index(result.Combined(), `"category"`)
			sevIdx := strings.Index(result.Combined(), `"severity"`)
			if obsIdx < 0 || catIdx < 0 || sevIdx < 0 || obsIdx > catIdx || catIdx > sevIdx {
				t.Errorf("renderPreflightPrompt(%s) schema must order observation < category < severity (obs %d, cat %d, sev %d)", ct, obsIdx, catIdx, sevIdx)
			}
			// PASS/FAIL are the legacy binary verdict — must be gone.
			if strings.Contains(result.Combined(), "\"PASS\"") || strings.Contains(result.Combined(), "\"FAIL\"") {
				t.Errorf("renderPreflightPrompt(%s) still contains legacy PASS/FAIL output", ct)
			}
			_ = tmplName
		})
	}
}

func Test_renderPreflightPrompt_DecisionRefsNamesACCheck(t *testing.T) {
	// decision_refs.tmpl describes the AC-presence check for plan decisions.
	// The LLM reads kind from .ProposedEntry and applies the check contextually;
	// the check text is always rendered.
	pctx := &preflightContext{
		ProposedEntry: "ID: d\nType: decision\nKind: plan\n\nplan body",
	}
	result, err := renderPreflightPrompt(checkDecisionRefs, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Combined(), "Acceptance criteria") {
		t.Errorf("decision_refs template should name the acceptance criteria check")
	}
}

func Test_renderPreflightPrompt_ClosingDoneNamesACCheck(t *testing.T) {
	// closing_done.tmpl describes per-AC coverage when the closed entry is
	// a plan. The LLM reads the closed entry's kind and description.
	pctx := &preflightContext{
		ProposedEntry: "ID: s\nType: signal\nKind: done\n\nclosing the plan",
		ClosedEntries: "ID: p\nType: decision\nKind: plan\n\nplan description with AC section",
	}
	result, err := renderPreflightPrompt(checkClosingDone, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Combined(), "Acceptance criteria") {
		t.Errorf("closing_done template should name the acceptance criteria coverage check")
	}
}

func Test_renderPreflightPrompt_CompletionTemplatesIncludeDurabilityCheck(t *testing.T) {
	// Regression: the durability check must be present on every completion-record
	// template (closing_done, short_loop) so the LLM validates artifact durability
	// for any entry claiming completion.
	pctx := &preflightContext{
		ProposedEntry: "ID: s\nType: signal\nKind: done\n\nclosing something",
		ClosedEntries: "ID: x\nType: decision\n\nsome decision",
	}

	for _, ct := range []checkType{checkClosingDone, checkShortLoop} {
		t.Run(ct.String(), func(t *testing.T) {
			result, err := renderPreflightPrompt(ct, pctx)
			if err != nil {
				t.Fatalf("renderPreflightPrompt(%s) error: %v", ct, err)
			}
			if !strings.Contains(result.Combined(), "Artifact durability") {
				t.Errorf("renderPreflightPrompt(%s) should include the durability check", ct)
			}
		})
	}
}

func Test_renderPreflightPrompt_CapturesIncludeUnrelatedRefsCheck(t *testing.T) {
	// Every capture template must invoke the shared unrelated_refs partial so
	// topically-disconnected refs get surfaced regardless of transaction type.
	pctx := &preflightContext{
		ProposedEntry:     "ID: test\n\nproposed",
		ReferencedEntries: "ID: ref\n\nreferenced entry",
		ClosedEntries:     "ID: closed\n\nclosed entry",
	}

	captureTypes := []checkType{
		checkSignalCapture,
		checkDecisionRefs,
		checkAspirationCapture,
		checkClosingDecision,
		checkClosingDone,
		checkShortLoop,
		checkDissolution,
	}
	for _, ct := range captureTypes {
		t.Run(ct.String(), func(t *testing.T) {
			result, err := renderPreflightPrompt(ct, pctx)
			if err != nil {
				t.Fatalf("renderPreflightPrompt(%s) error: %v", ct, err)
			}
			if !strings.Contains(result.Combined(), "Unrelated references check") {
				t.Errorf("renderPreflightPrompt(%s) should include the unrelated_refs partial", ct)
			}
		})
	}
}

func Test_renderPreflightPrompt_CloseCarryingTemplatesIncludeUnusualClose(t *testing.T) {
	// Every close-carrying template must invoke the shared unusual_close partial.
	pctx := &preflightContext{
		ProposedEntry: "ID: test\n\nproposed",
		ClosedEntries: "ID: closed\n\nclosed entry",
	}

	closeTypes := []checkType{
		checkClosingDecision,
		checkClosingDone,
		checkShortLoop,
		checkDissolution,
	}
	for _, ct := range closeTypes {
		t.Run(ct.String(), func(t *testing.T) {
			result, err := renderPreflightPrompt(ct, pctx)
			if err != nil {
				t.Fatalf("renderPreflightPrompt(%s) error: %v", ct, err)
			}
			if !strings.Contains(result.Combined(), "Unusual close-pattern check") {
				t.Errorf("renderPreflightPrompt(%s) should include the unusual_close partial", ct)
			}
		})
	}
}

func Test_renderPreflightPrompt_ClosingDecisionNamesRetirementRationale(t *testing.T) {
	// closing_decision.tmpl carries the retirement-rationale check for stable-kind targets.
	pctx := &preflightContext{
		ProposedEntry: "ID: d\nType: decision\nKind: directive\n\nretiring the old contract",
		ClosedEntries: "ID: c\nType: decision\nKind: contract\n\ncontract to retire",
	}
	result, err := renderPreflightPrompt(checkClosingDecision, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Combined(), "Retirement-rationale calibration") {
		t.Errorf("closing_decision template should name the retirement-rationale calibration")
	}
}

func Test_renderPreflightPrompt_DecisionRefsNamesDirectiveShapeCheck(t *testing.T) {
	// decision_refs.tmpl flags directives at stg/cpt that read as aspirations.
	pctx := &preflightContext{
		ProposedEntry: "ID: d\nType: decision\nKind: directive\nLayer: strategic\n\nperpetual pull toward X",
	}
	result, err := renderPreflightPrompt(checkDecisionRefs, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Combined(), "Directive-reads-aspiration-shaped") {
		t.Errorf("decision_refs template should name the directive-reads-aspiration-shaped calibration")
	}
}

func Test_renderPreflightPrompt_AspirationCaptureNeverHigh(t *testing.T) {
	// aspiration_capture.tmpl must state that findings are never high severity.
	pctx := &preflightContext{
		ProposedEntry: "ID: d\nType: decision\nKind: aspiration\nLayer: strategic\n\nperpetual pull toward X",
	}
	result, err := renderPreflightPrompt(checkAspirationCapture, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Combined(), "Never `high`") && !strings.Contains(result.Combined(), "never `high`") {
		t.Errorf("aspiration_capture template should state findings are never high severity")
	}
}

func Test_renderPreflightPrompt_DissolutionNamesContextPresence(t *testing.T) {
	// dissolution.tmpl checks for dialogue-captured context connecting the closing entry
	// to the question, not the correctness of the reasoning.
	pctx := &preflightContext{
		ProposedEntry: "ID: s\nType: signal\nKind: fact\n\nfact resolving a question",
		ClosedEntries: "ID: q\nType: signal\nKind: question\n\nthe question being resolved",
	}
	result, err := renderPreflightPrompt(checkDissolution, pctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Combined(), "dialogue-captured context") {
		t.Errorf("dissolution template should name dialogue-captured context as the test")
	}
}

func Test_renderPreflightPrompt_UniversalSystemIsByteStable(t *testing.T) {
	// The universal system preamble must be byte-identical across substantive
	// check types and across repeated assembly of the same graph. That
	// byte-stability is precisely what lets Anthropic's prompt cache hit on
	// sequential captures within a session (d-tac-fah). Contracts live in the
	// preamble, so this also guards the deterministic contract sort in
	// assembleContext — a non-deterministic order would silently kill cache reuse
	// even when the contract set never changed.
	contractA := entry("20260101-120000-d-prc-aaa", withKind(model.KindContract), withContent("Contract A body"))
	contractB := entry("20260102-120000-d-prc-bbb", withKind(model.KindContract), withContent("Contract B body"))
	// Insert in non-sorted order to prove assembleContext sorts deterministically.
	graph := model.NewGraph([]*model.Entry{contractB, contractA})

	decision := &model.Entry{Type: model.TypeDecision, Layer: model.LayerTactical, Content: "a decision with refs"}
	signal := &model.Entry{Type: model.TypeSignal, Layer: model.LayerTactical, Content: "a signal"}

	reqDecision, err := renderPreflightPrompt(checkDecisionRefs, assembleContext(decision, graph, checkDecisionRefs, ""))
	if err != nil {
		t.Fatal(err)
	}
	reqSignal, err := renderPreflightPrompt(checkSignalCapture, assembleContext(signal, graph, checkSignalCapture, ""))
	if err != nil {
		t.Fatal(err)
	}

	if reqDecision.SystemPrompt != reqSignal.SystemPrompt {
		t.Errorf("universal system prompt differs across check types — prompt cache will miss.\n--- decision_refs system ---\n%s\n--- signal_capture system ---\n%s", reqDecision.SystemPrompt, reqSignal.SystemPrompt)
	}

	// Re-assemble the same graph (contracts re-sorted from scratch) and confirm
	// the system prompt is still byte-identical.
	reqDecision2, err := renderPreflightPrompt(checkDecisionRefs, assembleContext(decision, graph, checkDecisionRefs, ""))
	if err != nil {
		t.Fatal(err)
	}
	if reqDecision.SystemPrompt != reqDecision2.SystemPrompt {
		t.Error("universal system prompt not stable across repeated assembly of the same graph — contract sort is non-deterministic")
	}

	// Sanity: the contracts must actually appear, or the byte-stability assertion
	// above is vacuous.
	if !strings.Contains(reqDecision.SystemPrompt, "Contract A body") || !strings.Contains(reqDecision.SystemPrompt, "Contract B body") {
		t.Error("expected active contracts to appear in the universal system preamble")
	}
}

func Test_renderPreflightPrompt_StructuralChecksKeepLightSystem(t *testing.T) {
	// annotation and focus deliberately stay off the universal preamble (d-tac-fah,
	// option b): they are rare and their bespoke rubrics would over-flag under the
	// heavy universal partials (e.g. unrelated_refs flagging an annotation's
	// membership refs). Their system block must NOT carry the heavy partials.
	pctx := &preflightContext{ProposedEntry: "ID: test\nType: signal\n\nproposed"}

	for _, ct := range []checkType{checkAnnotationCapture, checkFocusCapture} {
		t.Run(ct.String(), func(t *testing.T) {
			req, err := renderPreflightPrompt(ct, pctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, heavy := range []string{"Artifact durability", "Unrelated references check", "Unusual close-pattern check", "Reference metadata consistency"} {
				if strings.Contains(req.SystemPrompt, heavy) {
					t.Errorf("%s system block should stay light but contains %q", ct, heavy)
				}
			}
			// It must still carry the verdict format, which every check needs.
			if !strings.Contains(req.SystemPrompt, `"findings"`) {
				t.Errorf("%s system block should still carry the verdict output format", ct)
			}
		})
	}
}

func Test_refKindVocabulary_GraphBacked(t *testing.T) {
	// Regression for 20260824-165218-s-tac-7w6: the vocabulary renders from
	// the graph's ref-kind fact and a failed load blocks instead of quietly
	// weakening the rubric.
	if _, err := refKindVocabulary(model.NewGraph(nil)); err == nil {
		t.Fatal("missing ref-kind fact must fail loud, got nil error")
	}

	fact := entry(basefacts.RefKindsFactID, withKind(model.KindFact), withContent("# Connecting entries — stub\n\nvocabulary body"))
	vocab, err := refKindVocabulary(model.NewGraph([]*model.Entry{fact}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(vocab, "### Connecting entries") || !strings.Contains(vocab, "vocabulary body") {
		t.Errorf("vocabulary not rendered from the graph fact with demoted headings:\n%s", vocab)
	}
}

func Test_renderPreflightPrompt_JSONStringRulesShared(t *testing.T) {
	// Regression for 20260827-224853-s-tac-giv: the output format teaches
	// standard JSON escaping, never quote avoidance, on every system template.
	pctx := &preflightContext{ProposedEntry: "ID: test\nType: signal\n\nproposed"}
	for _, ct := range []checkType{checkSignalCapture, checkAnnotationCapture, checkFocusCapture} {
		t.Run(ct.String(), func(t *testing.T) {
			req, err := renderPreflightPrompt(ct, pctx)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(req.SystemPrompt, "**JSON strings.**") || !strings.Contains(req.SystemPrompt, "write a literal `\"` as `\\\"`") {
				t.Errorf("%s system block missing the shared JSON escaping rule:\n%s", ct, req.SystemPrompt)
			}
			if strings.Contains(req.SystemPrompt, "never a literal") {
				t.Errorf("%s system block still teaches quote avoidance", ct)
			}
		})
	}
}

func Test_renderPreflightPrompt_InvalidCheckType(t *testing.T) {
	pctx := &preflightContext{ProposedEntry: "test"}
	_, err := renderPreflightPrompt(checkType(99), pctx)
	if err == nil {
		t.Error("renderPreflightPrompt with invalid check type should return error")
	}
}

func Test_renderPreflightPrompt_ConditionalSections(t *testing.T) {
	pctx := &preflightContext{
		ProposedEntry: "ID: test\nType: signal\n\nTest content",
	}

	result, err := renderPreflightPrompt(checkSignalCapture, pctx)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result.Combined(), "## Referenced entries") {
		t.Error("Should not include Referenced entries section when empty")
	}
	if strings.Contains(result.Combined(), "## Active contracts") {
		t.Error("Should not include Active contracts section when empty")
	}
}

func Test_parsePreflightResult(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantFindings []Finding
		wantErr      bool
	}{
		{
			name:         "empty findings array",
			input:        `{"findings": []}`,
			wantFindings: nil,
		},
		{
			name:  "single high finding",
			input: `{"findings": [{"severity": "high", "category": "type-mismatch", "observation": "signal contains imperative commitment"}]}`,
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "type-mismatch", Observation: "signal contains imperative commitment"},
			},
		},
		{
			name: "mixed severities",
			input: `{"findings": [
				{"severity": "high", "category": "missing-ref", "observation": "directly-answered signal not referenced"},
				{"severity": "medium", "category": "plan-coverage-ambiguity", "observation": "test behavior unstated"},
				{"severity": "low", "category": "opening-reference-dependent", "observation": "first sentence relies on ref"}
			]}`,
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "missing-ref", Observation: "directly-answered signal not referenced"},
				{Severity: SeverityMedium, Category: "plan-coverage-ambiguity", Observation: "test behavior unstated"},
				{Severity: SeverityLow, Category: "opening-reference-dependent", Observation: "first sentence relies on ref"},
			},
		},
		{
			name:  "severity case insensitive",
			input: `{"findings": [{"severity": "HIGH", "category": "t", "observation": "x"}]}`,
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "t", Observation: "x"},
			},
		},
		{
			// The reordered output contract: reasoning first, severity as the
			// conclusion. encoding/json is field-order independent, but the
			// contract order is pinned here so a parser change can't silently
			// regress it.
			name:  "severity last (reordered contract)",
			input: `{"findings": [{"category": "ref-kind-sharpness", "observation": "the body frames the target as a prerequisite", "severity": "low"}]}`,
			wantFindings: []Finding{
				{Severity: SeverityLow, Category: "ref-kind-sharpness", Observation: "the body frames the target as a prerequisite"},
			},
		},
		{
			name:  "JSON wrapped in code fence",
			input: "```json\n{\"findings\": [{\"severity\": \"high\", \"category\": \"a\", \"observation\": \"one\"}]}\n```",
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "a", Observation: "one"},
			},
		},
		{
			name:         "JSON with preamble and postamble prose",
			input:        "Let me validate this entry:\n\n{\"findings\": []}\n\nThat's my assessment.",
			wantFindings: nil,
		},
		{
			name:  "braces inside observation string do not confuse balance",
			input: `{"findings": [{"severity": "high", "category": "a", "observation": "The entry says {x: y} which is odd"}]}`,
			wantFindings: []Finding{
				{Severity: SeverityHigh, Category: "a", Observation: "The entry says {x: y} which is odd"},
			},
		},
		{
			name:    "unknown severity",
			input:   `{"findings": [{"severity": "critical", "category": "t", "observation": "x"}]}`,
			wantErr: true,
		},
		{
			name:    "missing category",
			input:   `{"findings": [{"severity": "high", "observation": "x"}]}`,
			wantErr: true,
		},
		{
			name:    "missing observation",
			input:   `{"findings": [{"severity": "high", "category": "t"}]}`,
			wantErr: true,
		},
		{
			name:    "no JSON object",
			input:   "PASS\nSome prose without JSON.",
			wantErr: true,
		},
		{
			name:    "unbalanced braces",
			input:   `{"findings": [{"severity": "high"`,
			wantErr: true,
		},
		{
			name:    "malformed JSON",
			input:   `{"findings": [{severity: high}]}`,
			wantErr: true,
		},
		{
			name:    "empty response",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePreflightResult(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("parsePreflightResult() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePreflightResult() unexpected error: %v", err)
			}
			if len(result.Findings) != len(tt.wantFindings) {
				t.Fatalf("parsePreflightResult().Findings len = %d, want %d\ngot: %+v",
					len(result.Findings), len(tt.wantFindings), result.Findings)
			}
			for i, f := range result.Findings {
				want := tt.wantFindings[i]
				if f.Severity != want.Severity {
					t.Errorf("finding[%d].Severity = %q, want %q", i, f.Severity, want.Severity)
				}
				if f.Category != want.Category {
					t.Errorf("finding[%d].Category = %q, want %q", i, f.Category, want.Category)
				}
				if f.Observation != want.Observation {
					t.Errorf("finding[%d].Observation = %q, want %q", i, f.Observation, want.Observation)
				}
			}
		})
	}
}

func Test_PreflightResult_HasBlocking(t *testing.T) {
	tests := []struct {
		name     string
		findings []Finding
		want     bool
	}{
		{"empty", nil, false},
		{"only medium", []Finding{{Severity: SeverityMedium}}, false},
		{"only low", []Finding{{Severity: SeverityLow}}, false},
		{"medium and low", []Finding{{Severity: SeverityMedium}, {Severity: SeverityLow}}, false},
		{"single high", []Finding{{Severity: SeverityHigh}}, true},
		{"high among others", []Finding{{Severity: SeverityLow}, {Severity: SeverityHigh}, {Severity: SeverityMedium}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &PreflightResult{Findings: tt.findings}
			if got := r.HasBlocking(); got != tt.want {
				t.Errorf("HasBlocking() = %v, want %v", got, tt.want)
			}
		})
	}
}
