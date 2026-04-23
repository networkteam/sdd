package model

import "testing"

// actorHelper returns a kind: actor signal at process layer carrying the
// given canonical and optional aliases.
func actorHelper(id, canonical string, aliases []string, opts ...entryOpt) *Entry {
	e := &Entry{
		ID:        id,
		Type:      TypeSignal,
		Kind:      KindActor,
		Layer:     LayerProcess,
		Canonical: canonical,
		Aliases:   aliases,
		Content:   "actor " + canonical,
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

// roleHelper returns a kind: role decision at process layer bound to the
// given actor canonical, with refs pre-populated.
func roleHelper(id, actorCanonical, headID string) *Entry {
	e := &Entry{
		ID:      id,
		Type:    TypeDecision,
		Kind:    KindRole,
		Layer:   LayerProcess,
		Actor:   actorCanonical,
		Refs:    []string{headID},
		Content: "role for " + actorCanonical,
	}
	parts, err := ParseID(id)
	if err != nil {
		panic(err)
	}
	e.Time = parts.Time
	return e
}

func TestActorChains_SingleEntryChain(t *testing.T) {
	a := actorHelper("20260410-120000-s-prc-aaa", "Christopher", nil)
	g := NewGraph([]*Entry{a})

	chains := g.ActorChains()
	if len(chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(chains))
	}
	if chains[0].Head == nil || chains[0].Head.ID != a.ID {
		t.Errorf("head = %v, want %s", chains[0].Head, a.ID)
	}
	if len(chains[0].CanonicalHistory) != 1 || chains[0].CanonicalHistory[0] != "Christopher" {
		t.Errorf("canonical history = %v, want [Christopher]", chains[0].CanonicalHistory)
	}
}

func TestActorChains_TypoCorrectionWithinChain(t *testing.T) {
	// Original entry carried typo "Chritopher"; supersession corrects to "Christopher".
	// Both canonicals sit in the chain's history; cascade derivation works for either.
	a1 := actorHelper("20260410-120000-s-prc-aaa", "Chritopher", nil)
	a2 := actorHelper("20260410-130000-s-prc-bbb", "Christopher", nil, withSupersedes(a1.ID))
	g := NewGraph([]*Entry{a1, a2})

	chains := g.ActorChains()
	if len(chains) != 1 {
		t.Fatalf("expected 1 chain (linked by supersedes), got %d", len(chains))
	}
	c := chains[0]
	if c.Head == nil || c.Head.ID != a2.ID {
		t.Errorf("head = %v, want %s (newer entry)", c.Head, a2.ID)
	}
	if !c.HasCanonical("Chritopher") || !c.HasCanonical("Christopher") {
		t.Errorf("chain history must include both canonicals, got %v", c.CanonicalHistory)
	}
}

func TestRoleStatus_ActiveChain(t *testing.T) {
	a := actorHelper("20260410-120000-s-prc-aaa", "Christopher", nil)
	r := roleHelper("20260410-130000-d-prc-bbb", "Christopher", a.ID)
	g := NewGraph([]*Entry{a, r})

	s := g.DerivedStatus(r)
	if s.Kind != StatusActive {
		t.Errorf("role status = %q (by=%q), want active", s.Kind, s.By)
	}
}

func TestRoleStatus_CascadeClosureFromChainRetirement(t *testing.T) {
	// Chain is retired by a directive closing the head actor. The role
	// binding to the chain must derive-close cascade-style — no entry on
	// the role itself changed.
	a := actorHelper("20260410-120000-s-prc-aaa", "Christopher", nil)
	r := roleHelper("20260410-130000-d-prc-bbb", "Christopher", a.ID)
	retire := entry("20260410-140000-d-prc-ccc",
		withKind(KindDirective),
		withCloses(a.ID),
		withContent("retire actor"))
	g := NewGraph([]*Entry{a, r, retire})

	s := g.DerivedStatus(r)
	if s.Kind != StatusCascadeClosedBy {
		t.Errorf("role status = %q, want cascade-closed", s.Kind)
	}
	if s.By != retire.ID {
		t.Errorf("cascade-closed-by = %q, want %q", s.By, retire.ID)
	}
}

func TestRoleStatus_WithinChainCorrectionKeepsOldRoleActive(t *testing.T) {
	// A role was captured when the chain's canonical was "Chritopher".
	// A later actor supersession corrects the canonical to "Christopher".
	// The cascade walks full canonical history — the old role's Actor
	// "Chritopher" still resolves to the active chain, so the role stays
	// derived-active despite the capture-time canonical no longer being
	// the current head.
	a1 := actorHelper("20260410-120000-s-prc-aaa", "Chritopher", nil)
	rOld := roleHelper("20260410-130000-d-prc-bbb", "Chritopher", a1.ID)
	a2 := actorHelper("20260410-140000-s-prc-ccc", "Christopher", nil, withSupersedes(a1.ID))
	g := NewGraph([]*Entry{a1, rOld, a2})

	s := g.DerivedStatus(rOld)
	if s.Kind != StatusActive {
		t.Errorf("old role after within-chain correction = %q, want active (cascade walks history)", s.Kind)
	}
}

func TestRoleStatus_Orphan(t *testing.T) {
	// Role's Actor references a canonical no chain has ever carried.
	r := roleHelper("20260410-130000-d-prc-bbb", "NeverSeen", "20260410-120000-s-prc-fake")
	g := NewGraph([]*Entry{r})

	s := g.DerivedStatus(r)
	if s.Kind != StatusCascadeOrphan {
		t.Errorf("role status = %q, want cascade-orphan", s.Kind)
	}
}

func TestActiveActorHeads_ExcludesClosedChains(t *testing.T) {
	a := actorHelper("20260410-120000-s-prc-aaa", "Christopher", nil)
	retire := entry("20260410-130000-d-prc-bbb",
		withKind(KindDirective),
		withCloses(a.ID),
		withContent("retire actor"))
	g := NewGraph([]*Entry{a, retire})

	heads := g.ActiveActorHeads()
	if len(heads) != 0 {
		t.Errorf("expected 0 active heads (chain retired), got %d: %+v", len(heads), heads)
	}
}

func TestActiveRoles_OrdersByActorThenID(t *testing.T) {
	a1 := actorHelper("20260410-120000-s-prc-aaa", "Alice", nil)
	a2 := actorHelper("20260410-120100-s-prc-bbb", "Bob", nil)
	rBob := roleHelper("20260410-130000-d-prc-ccc", "Bob", a2.ID)
	rAliceB := roleHelper("20260410-140000-d-prc-ddd", "Alice", a1.ID)
	rAliceA := roleHelper("20260410-140100-d-prc-eee", "Alice", a1.ID)
	g := NewGraph([]*Entry{a1, a2, rBob, rAliceB, rAliceA})

	roles := g.ActiveRoles()
	if len(roles) != 3 {
		t.Fatalf("expected 3 active roles, got %d", len(roles))
	}
	if roles[0].Actor != "Alice" || roles[1].Actor != "Alice" || roles[2].Actor != "Bob" {
		t.Errorf("roles not sorted by actor: %v", []string{roles[0].Actor, roles[1].Actor, roles[2].Actor})
	}
	// Alice roles sub-sorted by ID
	if roles[0].ID > roles[1].ID {
		t.Errorf("alice roles not sub-sorted by ID: %s > %s", roles[0].ID, roles[1].ID)
	}
}

func TestLint_ParticipantCoverage_UnknownSurfaces(t *testing.T) {
	a := actorHelper("20260410-120000-s-prc-aaa", "Christopher", nil)
	e := entry("20260410-130000-s-cpt-bbb",
		withContent("observation"),
		withParticipants("Claude"))
	_ = NewGraph([]*Entry{a, e})

	found := false
	for _, w := range e.Warnings {
		if w.Field == "participants" && w.Value == "Claude" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected participant-coverage warning for unknown canonical, got %+v", e.Warnings)
	}
}

func TestLint_ParticipantCoverage_GraceMode(t *testing.T) {
	// No active actor signals → check skipped.
	e := entry("20260410-130000-s-cpt-bbb",
		withContent("first entry"),
		withParticipants("Christopher"))
	_ = NewGraph([]*Entry{e})

	for _, w := range e.Warnings {
		if w.Field == "participants" {
			t.Errorf("grace mode should skip participant check, got warning %+v", w)
		}
	}
}

func TestLint_AliasAmbiguity_FlagsCollision(t *testing.T) {
	a1 := actorHelper("20260410-120000-s-prc-aaa", "Christopher", []string{"Chris"})
	a2 := actorHelper("20260410-120100-s-prc-bbb", "Christian", []string{"Chris"})
	g := NewGraph([]*Entry{a1, a2})
	_ = g

	flagged := 0
	for _, a := range []*Entry{a1, a2} {
		for _, w := range a.Warnings {
			if w.Field == "aliases" && w.Value == "Chris" {
				flagged++
			}
		}
	}
	if flagged != 2 {
		t.Errorf("expected alias ambiguity on both actors, got %d", flagged)
	}
}

func TestLint_ActorInvariant_CrossChainReuse(t *testing.T) {
	a1 := actorHelper("20260410-120000-s-prc-aaa", "Christopher", nil)
	a2 := actorHelper("20260410-130000-s-prc-bbb", "Christopher", nil) // no supersedes — new chain
	g := NewGraph([]*Entry{a1, a2})
	_ = g

	flagged := 0
	for _, a := range []*Entry{a1, a2} {
		for _, w := range a.Warnings {
			if w.Field == "canonical" && w.Value == "Christopher" {
				flagged++
			}
		}
	}
	if flagged < 2 {
		t.Errorf("expected invariant warnings on both actors sharing canonical, got %d", flagged)
	}
}

func TestLint_OrphanRole_FlagsUnresolvedActor(t *testing.T) {
	r := roleHelper("20260410-130000-d-prc-bbb", "Ghost", "20260410-120000-s-prc-fake")
	g := NewGraph([]*Entry{r})
	_ = g

	found := false
	for _, w := range r.Warnings {
		if w.Field == "actor" && w.Value == "Ghost" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected orphan-role warning, got %+v", r.Warnings)
	}
}

func TestActorFrontmatter_MissingCanonical(t *testing.T) {
	a := &Entry{
		ID:    "20260410-120000-s-prc-aaa",
		Type:  TypeSignal,
		Kind:  KindActor,
		Layer: LayerProcess,
	}
	parts, _ := ParseID(a.ID)
	a.Time = parts.Time
	g := NewGraph([]*Entry{a})
	_ = g

	found := false
	for _, w := range a.Warnings {
		if w.Field == "canonical" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing-canonical warning, got %+v", a.Warnings)
	}
}

func TestRoleFrontmatter_MissingActor(t *testing.T) {
	r := &Entry{
		ID:    "20260410-120000-d-prc-aaa",
		Type:  TypeDecision,
		Kind:  KindRole,
		Layer: LayerProcess,
	}
	parts, _ := ParseID(r.ID)
	r.Time = parts.Time
	g := NewGraph([]*Entry{r})
	_ = g

	found := false
	for _, w := range r.Warnings {
		if w.Field == "actor" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing-actor warning, got %+v", r.Warnings)
	}
}

func TestOpenFilter_CascadeClosedRoleIsHidden(t *testing.T) {
	a := actorHelper("20260410-120000-s-prc-aaa", "Christopher", nil)
	r := roleHelper("20260410-130000-d-prc-bbb", "Christopher", a.ID)
	retire := entry("20260410-140000-d-prc-ccc",
		withKind(KindDirective),
		withCloses(a.ID),
		withContent("retire actor"))
	g := NewGraph([]*Entry{a, r, retire})

	open := g.Filter(GraphFilter{Type: TypeDecision, Kind: KindRole, OpenOnly: true})
	if len(open) != 0 {
		t.Errorf("cascade-closed role should be hidden from OpenOnly filter, got %d entries", len(open))
	}
}
