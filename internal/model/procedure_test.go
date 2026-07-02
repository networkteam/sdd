package model

import (
	"strings"
	"testing"
)

// procedureHelper returns a kind: procedure decision at process layer
// carrying the given canonical.
func procedureHelper(id, canonical string, opts ...entryOpt) *Entry {
	e := &Entry{
		ID:        id,
		Type:      TypeDecision,
		Kind:      KindProcedure,
		Layer:     LayerProcess,
		Canonical: canonical,
		Content:   "procedure " + canonical,
	}
	parts, err := ParseID(id)
	if err != nil {
		panic(err)
	}
	e.Time = parts.Time
	for _, o := range opts {
		o(e)
	}
	return e
}

func withEmbedded() entryOpt {
	return func(e *Entry) { e.Embedded = true }
}

func TestProcedureChains_SingleEntryChain(t *testing.T) {
	p := procedureHelper("20260702-120000-d-prc-aaa", "capture")
	g := NewGraph([]*Entry{p})

	chains := g.ProcedureChains()
	if len(chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(chains))
	}
	c := chains[0]
	if c.Head == nil || c.Head.ID != p.ID {
		t.Errorf("head = %v, want %s", c.Head, p.ID)
	}
	if len(c.CanonicalHistory) != 1 || c.CanonicalHistory[0] != "capture" {
		t.Errorf("canonical history = %v, want [capture]", c.CanonicalHistory)
	}
	if c.Forked() {
		t.Error("single-entry chain must not be forked")
	}
}

func TestProcedureChains_LinearSupersession(t *testing.T) {
	p1 := procedureHelper("20260702-120000-d-prc-aaa", "capture", withEmbedded())
	p2 := procedureHelper("20260702-130000-d-prc-bbb", "capture", withSupersedes(p1.ID))
	g := NewGraph([]*Entry{p1, p2})

	chains := g.ProcedureChains()
	if len(chains) != 1 {
		t.Fatalf("expected 1 chain (linked by supersedes), got %d", len(chains))
	}
	c := chains[0]
	if c.Head == nil || c.Head.ID != p2.ID {
		t.Errorf("head = %v, want %s (successor)", c.Head, p2.ID)
	}
	if c.Forked() {
		t.Error("linear chain must not be forked")
	}
	if got := g.ResolveProcedure("capture"); got == nil || got.ID != p2.ID {
		t.Errorf("ResolveProcedure = %v, want %s", got, p2.ID)
	}
}

func TestProcedureChains_ForkProjectHeadWins(t *testing.T) {
	// Base v1 is superseded by BOTH a shipped base successor and a project
	// override — the fork scenario. The project (non-embedded) head wins for
	// execution even though the base successor is newer.
	base := procedureHelper("20260702-120000-d-prc-aaa", "capture", withEmbedded())
	override := procedureHelper("20260702-130000-d-prc-bbb", "capture", withSupersedes(base.ID))
	shipped := procedureHelper("20260702-140000-d-prc-ccc", "capture", withSupersedes(base.ID), withEmbedded())
	g := NewGraph([]*Entry{base, override, shipped})

	chains := g.ProcedureChains()
	if len(chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(chains))
	}
	c := chains[0]
	if !c.Forked() {
		t.Fatal("chain with two live heads must report forked")
	}
	if len(c.LiveHeads) != 2 {
		t.Fatalf("live heads = %d, want 2", len(c.LiveHeads))
	}
	if c.Head == nil || c.Head.ID != override.ID {
		t.Errorf("resolved head = %v, want project override %s", c.Head, override.ID)
	}
	if got := g.ResolveProcedure("capture"); got == nil || got.ID != override.ID {
		t.Errorf("ResolveProcedure = %v, want project override %s", got, override.ID)
	}
}

func TestProcedureChains_ForkTwoProjectHeadsNewestWins(t *testing.T) {
	base := procedureHelper("20260702-120000-d-prc-aaa", "capture", withEmbedded())
	o1 := procedureHelper("20260702-130000-d-prc-bbb", "capture", withSupersedes(base.ID))
	o2 := procedureHelper("20260702-140000-d-prc-ccc", "capture", withSupersedes(base.ID))
	g := NewGraph([]*Entry{base, o1, o2})

	c := g.ProcedureChains()[0]
	if !c.Forked() {
		t.Fatal("expected fork")
	}
	if c.Head == nil || c.Head.ID != o2.ID {
		t.Errorf("resolved head = %v, want newest project head %s", c.Head, o2.ID)
	}
}

func TestProcedureChains_RetiredChainStillResolves(t *testing.T) {
	// A directive closes the head — the move is retired, but the canonical
	// still resolves so status and show surfaces work.
	p := procedureHelper("20260702-120000-d-prc-aaa", "capture")
	retire := entry("20260702-130000-d-prc-bbb",
		withKind(KindDirective),
		withCloses(p.ID),
		withContent("retire the capture move"))
	g := NewGraph([]*Entry{p, retire})

	c := g.ProcedureChains()[0]
	if len(c.LiveHeads) != 0 {
		t.Fatalf("live heads = %d, want 0 (head closed)", len(c.LiveHeads))
	}
	if c.Head == nil || c.Head.ID != p.ID {
		t.Errorf("retired chain head = %v, want %s", c.Head, p.ID)
	}
	if s := g.DerivedStatus(p); s.Kind != StatusClosedBy {
		t.Errorf("closed procedure status = %q, want closed-by", s.Kind)
	}
}

func TestProcedureChainForCanonical_SeparateNamespaceFromActors(t *testing.T) {
	// An actor and a procedure may carry the same canonical string — the
	// namespaces are separate, so neither resolution sees the other.
	a := actorHelper("20260702-120000-s-prc-aaa", "capture", nil)
	p := procedureHelper("20260702-130000-d-prc-bbb", "capture")
	g := NewGraph([]*Entry{a, p})

	pc := g.ProcedureChainForCanonical("capture")
	if pc == nil || pc.Head == nil || pc.Head.ID != p.ID {
		t.Fatalf("procedure chain for canonical = %v, want head %s", pc, p.ID)
	}
	ac := g.ChainForCanonical("capture")
	if ac == nil || ac.Head == nil || ac.Head.ID != a.ID {
		t.Fatalf("actor chain for canonical = %v, want head %s", ac, a.ID)
	}
	// Neither invariant check may cross-flag the shared string.
	for _, e := range []*Entry{a, p} {
		for _, w := range e.Warnings {
			if strings.Contains(w.Message, "write-once") {
				t.Errorf("unexpected write-once warning on %s: %s", e.ID, w.Message)
			}
		}
	}
}

func TestValidateProcedureFrontmatter(t *testing.T) {
	missing := procedureHelper("20260702-120000-d-prc-aaa", "")
	wrongLayer := procedureHelper("20260702-130000-d-tac-bbb", "capture")
	wrongLayer.Layer = LayerTactical
	NewGraph([]*Entry{missing, wrongLayer})

	if !hasWarning(missing, "canonical", "missing required canonical") {
		t.Errorf("missing canonical not flagged: %v", missing.Warnings)
	}
	if !hasWarning(wrongLayer, "layer", "should live at process layer") {
		t.Errorf("non-process layer not flagged: %v", wrongLayer.Warnings)
	}
}

func TestValidateProcedureInvariant_CanonicalInTwoChains(t *testing.T) {
	// Two unrelated chains carrying the same canonical — write-once violated.
	p1 := procedureHelper("20260702-120000-d-prc-aaa", "capture")
	p2 := procedureHelper("20260702-130000-d-prc-bbb", "capture")
	NewGraph([]*Entry{p1, p2})

	for _, e := range []*Entry{p1, p2} {
		if !hasWarning(e, "canonical", "write-once invariant violated") {
			t.Errorf("write-once violation not flagged on %s: %v", e.ID, e.Warnings)
		}
	}
}

func TestValidateProcedureForks_FlagsLiveHeadsAndSkipsGenericWarning(t *testing.T) {
	base := procedureHelper("20260702-120000-d-prc-aaa", "capture", withEmbedded())
	override := procedureHelper("20260702-130000-d-prc-bbb", "capture", withSupersedes(base.ID))
	shipped := procedureHelper("20260702-140000-d-prc-ccc", "capture", withSupersedes(base.ID), withEmbedded())
	NewGraph([]*Entry{base, override, shipped})

	for _, e := range []*Entry{override, shipped} {
		if !hasWarning(e, "canonical", "wins for execution") {
			t.Errorf("fork not flagged on live head %s: %v", e.ID, e.Warnings)
		}
	}
	// The generic supersede-fork warning is owned by the procedure check —
	// the forked base entry must not carry the "ambiguous" message.
	if hasWarning(base, "supersedes", "ambiguous") {
		t.Errorf("generic fork warning leaked onto procedure entry: %v", base.Warnings)
	}
}

func TestValidateCloses_DirectiveMayCloseProcedure(t *testing.T) {
	p := procedureHelper("20260702-120000-d-prc-aaa", "capture")
	retire := entry("20260702-130000-d-prc-bbb",
		withKind(KindDirective),
		withCloses(p.ID),
		withContent("retire the capture move"))
	plan := entry("20260702-140000-d-tac-ccc",
		withKind(KindPlan),
		withCloses(p.ID),
		withContent("plan closing a procedure — invalid"))
	NewGraph([]*Entry{p, retire, plan})

	if hasWarning(retire, "closes", "cannot close") {
		t.Errorf("directive→procedure close wrongly rejected: %v", retire.Warnings)
	}
	if !hasWarning(plan, "closes", "use supersedes instead") {
		t.Errorf("plan→procedure close not rejected: %v", plan.Warnings)
	}
}

func TestParticipantCoverage_SkipsEmbeddedEntries(t *testing.T) {
	// An active actor exists (no grace mode), and an embedded base entry
	// carries a participant unknown to this graph — it must not be flagged.
	a := actorHelper("20260702-120000-s-prc-aaa", "Christopher", nil)
	p := procedureHelper("20260702-130000-d-prc-bbb", "capture", withEmbedded())
	p.Participants = []string{"SomeoneElse"}
	NewGraph([]*Entry{a, p})

	if hasWarning(p, "participants", "does not match") {
		t.Errorf("participant coverage flagged embedded entry: %v", p.Warnings)
	}
}

// hasWarning reports whether the entry carries a warning on the given field
// whose message contains the given substring.
func hasWarning(e *Entry, field, substr string) bool {
	for _, w := range e.Warnings {
		if w.Field == field && strings.Contains(w.Message, substr) {
			return true
		}
	}
	return false
}

func TestProcedureSpecRoundTrip(t *testing.T) {
	content := `---
type: decision
layer: prc
kind: procedure
canonical: capture
params:
    anchor: {type: entry-id, optional: true, desc: entry this capture is anchored on}
state:
    body: {type: text, desc: entry description}
    refs: {type: list<ref>, desc: "each {id, kind, desc?}"}
steps:
    - id: assemble
      collect: [body, refs]
      transitions:
        - when: hasBody and hasRefs
          to: playback
    - id: playback
      chooser: user
      options:
        - {choice: confirm, call: confirmPlayback, to: end(completed)}
---

Procedure body.

## unit: assemble

Draft the entry.
`
	e, err := ParseEntry("20260702-120000-d-prc-rtp.md", content)
	if err != nil {
		t.Fatalf("ParseEntry: %v", err)
	}
	if e.ProcedureSpec == nil {
		t.Fatal("ProcedureSpec not retained on procedure entry")
	}
	if e.ProcedureSpec.Params.IsZero() || e.ProcedureSpec.State.IsZero() || e.ProcedureSpec.Steps.IsZero() {
		t.Fatal("expected params, state, and steps nodes to be retained")
	}

	// Round trip: format, re-parse, and check the machine part survives.
	out := FormatFrontmatter(e) + "\n" + e.Content
	e2, err := ParseEntry("20260702-120000-d-prc-rtp.md", out)
	if err != nil {
		t.Fatalf("ParseEntry (round trip): %v", err)
	}
	if e2.ProcedureSpec == nil {
		t.Fatal("ProcedureSpec lost in round trip")
	}
	var steps []map[string]any
	if err := e2.ProcedureSpec.Steps.Decode(&steps); err != nil {
		t.Fatalf("decoding round-tripped steps: %v", err)
	}
	if len(steps) != 2 || steps[0]["id"] != "assemble" || steps[1]["id"] != "playback" {
		t.Fatalf("round-tripped steps lost structure: %v", steps)
	}
}

func TestProcedureSpecIgnoredOnNonProcedure(t *testing.T) {
	content := `---
type: decision
layer: tac
kind: plan
steps:
    - id: stray
---

A plan with a stray steps key.
`
	e, err := ParseEntry("20260702-120000-d-tac-npx.md", content)
	if err != nil {
		t.Fatalf("ParseEntry: %v", err)
	}
	if e.ProcedureSpec != nil {
		t.Error("ProcedureSpec must only be routed for kind: procedure entries")
	}
}
