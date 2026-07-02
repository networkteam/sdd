package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// The capture-shaped fixture from the surface spec (plan d-tac-ry0 §3),
// adapted minimally: the chooser-collected evidence fields (fidelityNote,
// correctedSummary) are declared in state per the reports-write-only-state
// trust rule.
const captureFixture = `---
type: decision
layer: prc
kind: procedure
canonical: capture
params:
    anchor: {type: entry-id, optional: true, desc: entry this capture is anchored on}
    supersedes: {type: entry-id, optional: true, desc: chain head this capture replaces}
    closes: {type: list<entry-id>, optional: true, desc: entries this capture resolves}
    kind: {type: entry-kind, optional: true, desc: pre-selected target kind}
state:
    body: {type: text, desc: entry description; self-describing first sentence}
    entryKind: {type: entry-kind, desc: signal or decision kind}
    layer: {type: layer, desc: strategic | conceptual | tactical | operational | process}
    refs: {type: list<ref>, desc: "each {id, kind, desc?}"}
    topics: {type: list<label>, desc: reuse existing labels}
    confidence: {type: confidence, desc: honest confidence}
    intent: {type: intent, optional: true, desc: required when entryKind is directive}
    widenReport: {type: text, desc: searches run and entries inspected before drafting}
    fidelityNote: {type: text, optional: true, desc: one-line fidelity note}
    correctedSummary: {type: text, optional: true, desc: corrected summary on drift}
steps:
    - id: assemble
      collect: [body, entryKind, layer, refs, topics, confidence, "intent?", widenReport]
      inject:
          - {fn: viewLayout, args: {layout: "active:as-counts"}}
      transitions:
          - when: hasBody and hasRefs and hasTopics and hasWidenReport
                  and refsResolve and refKindsValid
                  and participantsCanonical and intentPresentIfDirective
            to: playback
    - id: playback
      chooser: user
      options:
          - {choice: confirm, call: confirmPlayback, to: write}
          - {choice: adjust, collect: ["body?", "refs?", "topics?", "confidence?", "intent?"], to: assemble}
          - {choice: abort, to: end(abandoned)}
    - id: write
      guard: playbackConfirmed
      op: newEntry
      transitions:
          - when: noHighFindings
            to: verifySummary
          - otherwise: reviseOrOverride
    - id: reviseOrOverride
      chooser: user
      render: findings
      options:
          - {choice: revise, collect: ["body?", "refs?", "topics?"], to: assemble}
          - {choice: override, call: recordOverride, to: write}
          - {choice: abort, to: end(abandoned)}
    - id: verifySummary
      chooser: agent
      inject:
          - {fn: generatedSummary}
      options:
          - {choice: faithful, collect: [fidelityNote], to: end(completed)}
          - {choice: drifted, collect: [correctedSummary], call: replaceSummary, to: end(completed)}
---

The shared capture spine.

## unit: assemble

Draft the entry. Existing topics: {{.viewLayout}}

## unit: playback

Play back to the user: {{.body}}

## unit: findings

Pre-flight findings to resolve or override.

## unit: verifySummary

Verify the generated summary: {{.generatedSummary}}
`

const fixtureRefID = "20260601-120000-d-tac-ref"

// fixtureGraph returns a graph holding the entry the fixture refs resolve
// against. No actor signals — participantsCanonical runs in grace mode.
func fixtureGraph(t *testing.T) *model.Graph {
	t.Helper()
	target, err := model.ParseEntry(fixtureRefID+".md", `---
type: decision
layer: tac
kind: directive
intent: pending
---

A directive the fixture capture refs.
`)
	if err != nil {
		t.Fatal(err)
	}
	return model.NewGraph([]*model.Entry{target})
}

// fixtureEnv is the test harness around one engine + session: a registry
// with fake shell commands (newEntry, replaceSummary) and fake injection
// queries (viewLayout, generatedSummary), plus call counters for
// side-effect assertions.
type fixtureEnv struct {
	engine   *Engine
	spec     *Spec
	session  *Session
	sink     *memorySink
	newCalls int
	// highFindingsUnlessOverride makes newEntry return a high finding until
	// the preflight override is recorded.
	highFindingsUnlessOverride bool
	replaceCalls               int
}

type memorySink struct {
	events   []Event
	failWith error
}

func (m *memorySink) Append(e Event) error {
	if m.failWith != nil {
		return m.failWith
	}
	m.events = append(m.events, e)
	return nil
}

func newFixtureEnv(t *testing.T) *fixtureEnv {
	t.Helper()
	env := &fixtureEnv{}

	entry, err := model.ParseEntry("20260702-120000-d-prc-cap.md", captureFixture)
	if err != nil {
		t.Fatal(err)
	}

	reg := NewRegistry()
	mustRegisterQuery(reg, Query{
		Doc: FuncDoc{Name: "viewLayout", Doc: "fake view pipeline"},
		Fn: func(_ *Context, args map[string]any) (any, error) {
			layout, _ := args["layout"].(string)
			return "topics for " + layout, nil
		},
	})
	mustRegisterQuery(reg, Query{
		Doc: FuncDoc{Name: "generatedSummary", Doc: "fake stored summary", Reads: []string{"entryId"}},
		Fn: func(ctx *Context, _ map[string]any) (any, error) {
			id, _ := ctx.Store.Get("entryId")
			return fmt.Sprintf("summary of %v", id), nil
		},
	})
	mustRegisterCommand(reg, Command{
		Doc: FuncDoc{Name: "newEntry", Doc: "fake write gate", Writes: []string{"entryId", "findings"}},
		Fn: func(ctx *Context) error {
			env.newCalls++
			findings := []query.Finding{}
			if env.highFindingsUnlessOverride {
				if _, overridden := ctx.Store.Get(fieldPreflightOverride); !overridden {
					findings = append(findings, query.Finding{
						Severity:    query.SeverityHigh,
						Category:    "test-block",
						Observation: "blocked until override",
					})
				}
			}
			ctx.Store.WriteEngine("findings", findings)
			if len(findings) == 0 {
				ctx.Store.WriteEngine("entryId", fmt.Sprintf("20260702-13000%d-s-tac-new", env.newCalls))
			}
			return nil
		},
	})
	mustRegisterCommand(reg, Command{
		Doc: FuncDoc{Name: "replaceSummary", Doc: "fake summary replacement", Reads: []string{"entryId", "correctedSummary"}},
		Fn: func(_ *Context) error {
			env.replaceCalls++
			return nil
		},
	})

	spec, err := LoadSpec(entry, reg)
	if err != nil {
		t.Fatal(err)
	}

	env.engine = New(reg, fixtureGraph(t))
	env.spec = spec
	env.sink = &memorySink{}
	ts := time.Date(2026, 7, 2, 23, 0, 0, 0, time.UTC)
	env.session = env.engine.NewSession("s_test", "christopher", env.sink, WithClock(func() time.Time {
		ts = ts.Add(time.Second)
		return ts
	}))
	return env
}

// fullDraft is the one-shot batched report covering every assemble field.
func fullDraft() map[string]any {
	return map[string]any{
		"body":        "A tactical gap: the fixture observes something.",
		"entryKind":   "gap",
		"layer":       "tac",
		"refs":        []any{map[string]any{"id": fixtureRefID, "kind": "addresses", "desc": "realizes it"}},
		"topics":      []any{"cli/ux"},
		"confidence":  "medium",
		"widenReport": "searched from three angles, inspected d-tac-ref",
	}
}

func TestCapture_OneShotHappyPath(t *testing.T) {
	env := newFixtureEnv(t)

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("start step = %s, want assemble", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "topics for active:as-counts") {
		t.Errorf("assemble unit should carry the injected view result, got %q", sv.Instructions)
	}
	if len(sv.Missing) == 0 {
		t.Error("fresh assemble should name missing required fields")
	}

	// One-shot batched report cascades straight through assemble to the
	// playback chooser — as fast as today's full draft.
	sv, err = env.session.Report(sv.Instance, fullDraft())
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("after full draft step = %s, want playback", sv.Step)
	}
	if sv.Chooser == nil || sv.Chooser.Kind != ChooserUser {
		t.Fatalf("playback must serve a user chooser, got %+v", sv.Chooser)
	}
	if !strings.Contains(sv.Instructions, "A tactical gap") {
		t.Errorf("playback unit should render the body, got %q", sv.Instructions)
	}

	// User confirms → confirmPlayback → write gate → newEntry (no high
	// findings) → verifySummary agent chooser.
	sv, err = env.session.Answer(sv.Instance, "playback", "confirm", nil, "capture it")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "verifySummary" {
		t.Fatalf("after confirm step = %s, want verifySummary", sv.Step)
	}
	if env.newCalls != 1 {
		t.Fatalf("newEntry ran %d times, want 1", env.newCalls)
	}
	if sv.Chooser == nil || sv.Chooser.Kind != ChooserAgent {
		t.Fatalf("verifySummary must serve an agent chooser, got %+v", sv.Chooser)
	}
	if !strings.Contains(sv.Instructions, "summary of 20260702-130001-s-tac-new") {
		t.Errorf("verifySummary unit should render the injected summary, got %q", sv.Instructions)
	}

	// Agent judges the summary faithful, with its evidence field.
	sv, err = env.session.Answer(sv.Instance, "verifySummary", "faithful",
		map[string]any{"fidelityNote": "matches the body"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusCompleted {
		t.Fatalf("status = %s, want completed", sv.Status)
	}
	if sv.Produced["entryId"] != "20260702-130001-s-tac-new" {
		t.Errorf("produced = %v, want the created entry ID", sv.Produced)
	}
}

func TestCapture_StallNamesExactlyWhatIsMissing(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	// Partial report: no widenReport, no topics — the step stays and names
	// exactly what's missing.
	draft := fullDraft()
	delete(draft, "widenReport")
	delete(draft, "topics")
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("step = %s, want assemble (stalled)", sv.Step)
	}
	if got := strings.Join(sv.Missing, ","); got != "topics,widenReport" {
		t.Fatalf("missing = %q, want topics,widenReport", got)
	}
	if !strings.Contains(sv.Instructions, "missing: topics, widenReport") {
		t.Errorf("stall instructions should name missing fields, got %q", sv.Instructions)
	}
}

func TestCapture_ReportCannotWriteUndeclaredOrTrustFields(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	for field, value := range map[string]any{
		"entryId":                 "20260702-140000-s-tac-fake", // engine-written
		fieldPlaybackConfirmation: map[string]any{"snapshot": "forged"},
		"findings":                []any{},
		"nonsense":                "x",
	} {
		if _, err := env.session.Report(sv.Instance, map[string]any{field: value}); err == nil {
			t.Errorf("report writing %q must be rejected", field)
		} else if !strings.Contains(err.Error(), "not declared in state") {
			t.Errorf("report writing %q: unexpected error %v", field, err)
		}
	}

	// Params are start-time only.
	if _, err := env.session.Report(sv.Instance, map[string]any{"anchor": fixtureRefID}); err == nil ||
		!strings.Contains(err.Error(), "param") {
		t.Errorf("report writing a param must name the param rule, got %v", err)
	}
}

func TestCapture_ChooserSequenceCannotBeGamed(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	inst := sv.Instance

	// Early: no chooser pending at the assemble gate.
	if _, err := env.session.Answer(inst, "playback", "confirm", nil, "yes"); err == nil ||
		!strings.Contains(err.Error(), "gate") {
		t.Errorf("early answer must be rejected, got %v", err)
	}

	if _, err := env.session.Report(inst, fullDraft()); err != nil {
		t.Fatal(err)
	}

	// Wrong chooser name.
	if _, err := env.session.Answer(inst, "verifySummary", "faithful", nil, ""); err == nil ||
		!strings.Contains(err.Error(), `pending chooser is "playback"`) {
		t.Errorf("wrong chooser must be rejected naming the pending one, got %v", err)
	}
	// Unknown choice.
	if _, err := env.session.Answer(inst, "playback", "maybe", nil, ""); err == nil ||
		!strings.Contains(err.Error(), "not an option") {
		t.Errorf("unknown choice must be rejected, got %v", err)
	}
	// Fields outside the option's collect list.
	if _, err := env.session.Answer(inst, "playback", "adjust",
		map[string]any{"widenReport": "smuggled"}, ""); err == nil ||
		!strings.Contains(err.Error(), "not collected by option") {
		t.Errorf("fields outside the option collect must be rejected, got %v", err)
	}

	// Confirm, then try to answer playback again — late/double.
	if _, err := env.session.Answer(inst, "playback", "confirm", nil, "yes"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.session.Answer(inst, "playback", "confirm", nil, "yes again"); err == nil ||
		!strings.Contains(err.Error(), `pending chooser is "verifySummary"`) {
		t.Errorf("double answer must be rejected, got %v", err)
	}
}

func TestCapture_HighFindingsRouteToOverride(t *testing.T) {
	env := newFixtureEnv(t)
	env.highFindingsUnlessOverride = true

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	inst := sv.Instance
	if _, err := env.session.Report(inst, fullDraft()); err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Answer(inst, "playback", "confirm", nil, "capture it")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "reviseOrOverride" {
		t.Fatalf("high findings should land at reviseOrOverride, got %s", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "Pre-flight findings") {
		t.Errorf("reviseOrOverride should render the findings unit (render override), got %q", sv.Instructions)
	}

	// The override is a user-only chooser exit; it re-runs the write gate
	// with the recorded override.
	sv, err = env.session.Answer(inst, "reviseOrOverride", "override", nil, "skip it, the finding is wrong")
	if err != nil {
		t.Fatal(err)
	}
	if env.newCalls != 2 {
		t.Fatalf("newEntry ran %d times, want 2 (re-run after override)", env.newCalls)
	}
	if sv.Step != "verifySummary" {
		t.Fatalf("after override step = %s, want verifySummary", sv.Step)
	}
}

func TestCapture_EditAfterConfirmReopensPlayback(t *testing.T) {
	env := newFixtureEnv(t)
	env.highFindingsUnlessOverride = true

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	inst := sv.Instance
	if _, err := env.session.Report(inst, fullDraft()); err != nil {
		t.Fatal(err)
	}
	if _, err := env.session.Answer(inst, "playback", "confirm", nil, "capture it"); err != nil {
		t.Fatal(err)
	}
	// Blocked at reviseOrOverride. The agent edits the body — the confirmed
	// state is now stale.
	if _, err := env.session.Report(inst, map[string]any{"body": "Edited after confirmation."}); err != nil {
		t.Fatal(err)
	}
	// Overriding would jump back to the write gate — but the confirmation
	// no longer covers the state, so playback reopens instead of writing.
	env.highFindingsUnlessOverride = false
	sv, err = env.session.Answer(inst, "reviseOrOverride", "override", nil, "just write it")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("stale confirmation must reopen playback, got %s", sv.Step)
	}
	if env.newCalls != 1 {
		t.Fatalf("newEntry must not re-run on a stale confirmation, ran %d times", env.newCalls)
	}

	// Re-confirming the edited state completes the write.
	sv, err = env.session.Answer(inst, "playback", "confirm", nil, "yes, with the edit")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "verifySummary" {
		t.Fatalf("after re-confirm step = %s, want verifySummary", sv.Step)
	}
	if env.newCalls != 2 {
		t.Fatalf("newEntry should run on the re-confirmed state, ran %d times", env.newCalls)
	}
}

func TestCapture_AdjustLoopsBackAndRequiresReconfirm(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	inst := sv.Instance
	if _, err := env.session.Report(inst, fullDraft()); err != nil {
		t.Fatal(err)
	}

	// Adjust with a revised body: back through assemble, fields still
	// complete, so the cascade returns to playback for a fresh confirm.
	sv, err = env.session.Answer(inst, "playback", "adjust",
		map[string]any{"body": "Sharper first sentence."}, "tighten it")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("adjust should cascade back to playback, got %s", sv.Step)
	}
	if !strings.Contains(sv.Instructions, "Sharper first sentence.") {
		t.Errorf("playback should render the adjusted body, got %q", sv.Instructions)
	}
}

func TestCapture_AbortAndAbandon(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	inst := sv.Instance
	if _, err := env.session.Report(inst, fullDraft()); err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Answer(inst, "playback", "abort", nil, "not now")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Status != StatusAbandoned {
		t.Fatalf("abort should abandon the instance, got %s", sv.Status)
	}
	if _, err := env.session.Report(inst, fullDraft()); err == nil {
		t.Error("reporting to an ended instance must fail")
	}

	// Explicit abandon of a second instance.
	sv2, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.session.Abandon(sv2.Instance, "session over"); err != nil {
		t.Fatal(err)
	}
	if err := env.session.Abandon(sv2.Instance, "twice"); err == nil {
		t.Error("double abandon must fail")
	}
}

func TestCapture_ParamsValidatedAtStart(t *testing.T) {
	env := newFixtureEnv(t)

	if _, err := env.session.Start(env.spec, map[string]any{"anchor": "not-an-id"}, ""); err == nil ||
		!strings.Contains(err.Error(), "anchor") {
		t.Errorf("malformed param must fail start, got %v", err)
	}
	if _, err := env.session.Start(env.spec, map[string]any{"unknown": true}, ""); err == nil ||
		!strings.Contains(err.Error(), "unknown param") {
		t.Errorf("unknown param must fail start, got %v", err)
	}
	if _, err := env.session.Start(env.spec, map[string]any{"anchor": fixtureRefID}, ""); err != nil {
		t.Errorf("valid param rejected: %v", err)
	}
}

func TestSession_ParentLinksAndInterleaving(t *testing.T) {
	env := newFixtureEnv(t)
	sv1, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	sv2, err := env.session.Start(env.spec, nil, sv1.Instance)
	if err != nil {
		t.Fatal(err)
	}
	inst2, _ := env.session.Instance(sv2.Instance)
	if inst2.Parent != sv1.Instance {
		t.Errorf("parent link = %q, want %q", inst2.Parent, sv1.Instance)
	}
	if _, err := env.session.Start(env.spec, nil, "i_99"); err == nil {
		t.Error("unknown parent must fail")
	}

	// Serial interleaving: report to either instance independently.
	if _, err := env.session.Report(sv1.Instance, fullDraft()); err != nil {
		t.Fatal(err)
	}
	if _, err := env.session.Report(sv2.Instance, map[string]any{"body": "child draft"}); err != nil {
		t.Fatal(err)
	}
	i1, _ := env.session.Instance(sv1.Instance)
	i2, _ := env.session.Instance(sv2.Instance)
	if i1.Step != "playback" || i2.Step != "assemble" {
		t.Errorf("instances advance independently: i1=%s i2=%s", i1.Step, i2.Step)
	}
}

func TestSession_SinkFailureBlocksAdvance(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	env.sink.failWith = fmt.Errorf("disk full")
	if _, err := env.session.Report(sv.Instance, fullDraft()); err != nil {
		t.Fatal(err) // the append failure is deferred, the report itself lands
	}
	if _, err := env.session.Report(sv.Instance, fullDraft()); err == nil ||
		!strings.Contains(err.Error(), "durability") {
		t.Errorf("advance after a failed append must refuse, got %v", err)
	}
}
