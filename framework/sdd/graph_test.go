package sdd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// entry is a test helper that builds an Entry from an ID string.
// It parses the type, layer, and time from the ID using ParseID.
func entry(id string, opts ...entryOpt) *Entry {
	parts, err := ParseID(id)
	if err != nil {
		panic(fmt.Sprintf("bad test ID %q: %v", id, err))
	}

	e := &Entry{
		ID:      id,
		Type:    TypeFromAbbrev[parts.TypeCode],
		Layer:   LayerFromAbbrev[parts.LayerCode],
		Content: id,
		Time:    parts.Time,
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

type entryOpt func(*Entry)

func withRefs(refs ...string) entryOpt {
	return func(e *Entry) { e.Refs = refs }
}

func withSupersedes(ids ...string) entryOpt {
	return func(e *Entry) { e.Supersedes = ids }
}

func withCloses(ids ...string) entryOpt {
	return func(e *Entry) { e.Closes = ids }
}

func withContent(c string) entryOpt {
	return func(e *Entry) { e.Content = c }
}


func TestParseEntry(t *testing.T) {
	content := `---
type: decision
layer: strategic
refs:
  - 20260406-115516-s-stg-beh
participants:
  - Christopher
confidence: medium
---

Explore a novel process framework.`

	e, err := ParseEntry("20260406-115540-d-stg-0gh.md", content)
	if err != nil {
		t.Fatal(err)
	}

	if e.ID != "20260406-115540-d-stg-0gh" {
		t.Errorf("ID = %q, want %q", e.ID, "20260406-115540-d-stg-0gh")
	}
	if e.Type != TypeDecision {
		t.Errorf("Type = %q, want %q", e.Type, TypeDecision)
	}
	if e.Layer != LayerStrategic {
		t.Errorf("Layer = %q, want %q", e.Layer, LayerStrategic)
	}
	if len(e.Refs) != 1 || e.Refs[0] != "20260406-115516-s-stg-beh" {
		t.Errorf("Refs = %v, want [20260406-115516-s-stg-beh]", e.Refs)
	}
	if e.Confidence != "medium" {
		t.Errorf("Confidence = %q, want %q", e.Confidence, "medium")
	}
	if e.Content != "Explore a novel process framework." {
		t.Errorf("Content = %q, want %q", e.Content, "Explore a novel process framework.")
	}
	if e.Time.IsZero() {
		t.Error("Time should not be zero")
	}
}

func TestParseEntrySignal(t *testing.T) {
	content := `---
type: signal
layer: strategic
participants:
  - Christopher
---

Current practices produce overhead.`

	e, err := ParseEntry("20260406-115516-s-stg-beh.md", content)
	if err != nil {
		t.Fatal(err)
	}

	if e.Type != TypeSignal {
		t.Errorf("Type = %q, want %q", e.Type, TypeSignal)
	}
	if e.Confidence != "" {
		t.Errorf("Confidence = %q, want empty", e.Confidence)
	}
}

func TestLoadGraph(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "20260406-115516-s-stg-beh.md", `---
type: signal
layer: strategic
---

Signal one.`)

	writeFile(t, dir, "20260406-115540-d-stg-0gh.md", `---
type: decision
layer: strategic
refs:
  - 20260406-115516-s-stg-beh
closes:
  - 20260406-115516-s-stg-beh
---

Decision closing signal.`)

	writeFile(t, dir, "20260406-115559-a-cpt-f8v.md", `---
type: action
layer: conceptual
refs:
  - 20260406-115540-d-stg-0gh
closes:
  - 20260406-115540-d-stg-0gh
---

Action closing decision.`)

	g, err := LoadGraph(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(g.Entries) != 3 {
		t.Fatalf("Entries = %d, want 3", len(g.Entries))
	}

	// Signal is closed by decision
	open := g.OpenSignals()
	if len(open) != 0 {
		t.Errorf("OpenSignals = %d, want 0", len(open))
	}

	// Decision is closed by action
	active := g.ActiveDecisions()
	if len(active) != 0 {
		t.Errorf("ActiveDecisions = %d, want 0", len(active))
	}

	// Recent actions
	actions := g.RecentActions(10)
	if len(actions) != 1 {
		t.Errorf("RecentActions = %d, want 1", len(actions))
	}

	// Ref chain from action should include all three
	chain := g.RefChain("20260406-115559-a-cpt-f8v")
	if len(chain) != 3 {
		t.Errorf("RefChain = %d, want 3", len(chain))
	}
}

func TestNewGraph(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
		entry("20260406-100100-d-stg-bbb", withRefs("20260406-100000-s-stg-aaa")),
		entry("20260406-100200-a-ops-ccc", withRefs("20260406-100100-d-stg-bbb")),
	})

	if len(g.Entries) != 3 {
		t.Fatalf("Entries = %d, want 3", len(g.Entries))
	}
	if g.Entries[0].Time.After(g.Entries[1].Time) {
		t.Error("entries should be sorted by time")
	}
	if _, ok := g.ByID["20260406-100000-s-stg-aaa"]; !ok {
		t.Error("ByID missing aaa")
	}
	if refs := g.RefsTo["20260406-100000-s-stg-aaa"]; len(refs) != 1 {
		t.Errorf("RefsTo[aaa] = %v, want 1 entry", refs)
	}
}

func TestActiveDecisionsSuperseded(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-d-tac-aaa"),
		entry("20260406-110000-d-tac-bbb", withSupersedes("20260406-100000-d-tac-aaa")),
	})

	active := g.ActiveDecisions()
	if len(active) != 1 {
		t.Fatalf("ActiveDecisions = %d, want 1", len(active))
	}
	if active[0].ID != "20260406-110000-d-tac-bbb" {
		t.Errorf("Active decision = %q, want the superseding one", active[0].ID)
	}
}

func TestOpenSignalsWithOpenAndAddressed(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-tac-aaa"),
		entry("20260406-100100-s-tac-bbb"),
		entry("20260406-100200-d-tac-ccc", withCloses("20260406-100000-s-tac-aaa")),
	})

	open := g.OpenSignals()
	if len(open) != 1 {
		t.Fatalf("OpenSignals = %d, want 1", len(open))
	}
	if open[0].ID != "20260406-100100-s-tac-bbb" {
		t.Errorf("Open signal = %q, want bbb", open[0].ID)
	}
}

func TestRefsDoNotClose(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-ops-aaa"),
		entry("20260406-100100-d-ops-bbb", withRefs("20260406-100000-s-ops-aaa")),
	})

	open := g.OpenSignals()
	if len(open) != 1 {
		t.Fatalf("OpenSignals = %d, want 1 (refs alone don't close signals)", len(open))
	}
}

func TestRecentActionsTruncation(t *testing.T) {
	var entries []*Entry
	for i := range 5 {
		id := fmt.Sprintf("20260406-10%02d00-a-ops-%03d", i, i)
		entries = append(entries, entry(id))
	}

	g := NewGraph(entries)

	actions := g.RecentActions(3)
	if len(actions) != 3 {
		t.Fatalf("RecentActions(3) = %d, want 3", len(actions))
	}
	if actions[0].ID != "20260406-100200-a-ops-002" {
		t.Errorf("First action = %q, want 002", actions[0].ID)
	}
	if actions[2].ID != "20260406-100400-a-ops-004" {
		t.Errorf("Last action = %q, want 004", actions[2].ID)
	}
}

func TestRefChainBranching(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
		entry("20260406-100100-s-stg-bbb"),
		entry("20260406-100200-d-stg-ccc", withRefs("20260406-100000-s-stg-aaa", "20260406-100100-s-stg-bbb")),
	})

	chain := g.RefChain("20260406-100200-d-stg-ccc")
	if len(chain) != 3 {
		t.Fatalf("RefChain = %d, want 3", len(chain))
	}
	if chain[2].ID != "20260406-100200-d-stg-ccc" {
		t.Errorf("Last in chain = %q, want ccc (the root of the walk)", chain[2].ID)
	}
}

func TestRefChainMissingRef(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-d-tac-aaa", withRefs("20260406-099999-s-tac-missing")),
	})

	chain := g.RefChain("20260406-100000-d-tac-aaa")
	if len(chain) != 1 {
		t.Fatalf("RefChain with missing ref = %d, want 1", len(chain))
	}
}

func TestRefChainNonexistentRoot(t *testing.T) {
	g := NewGraph([]*Entry{})

	chain := g.RefChain("nonexistent")
	if len(chain) != 0 {
		t.Errorf("RefChain for nonexistent ID = %d, want 0", len(chain))
	}
}

func TestFilter(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
		entry("20260406-100100-s-tac-bbb"),
		entry("20260406-100200-d-tac-ccc"),
	})

	signals := g.Filter(TypeSignal, "")
	if len(signals) != 2 {
		t.Errorf("Filter(signal, '') = %d, want 2", len(signals))
	}

	tactical := g.Filter("", LayerTactical)
	if len(tactical) != 2 {
		t.Errorf("Filter('', tactical) = %d, want 2", len(tactical))
	}

	tacSignals := g.Filter(TypeSignal, LayerTactical)
	if len(tacSignals) != 1 {
		t.Errorf("Filter(signal, tactical) = %d, want 1", len(tacSignals))
	}

	all := g.Filter("", "")
	if len(all) != 3 {
		t.Errorf("Filter('', '') = %d, want 3", len(all))
	}
}

func TestFilterOpen(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-tac-aaa", withContent("Open signal")),
		entry("20260406-100100-s-tac-bbb", withContent("Closed signal")),
		entry("20260406-100200-d-tac-ccc", withCloses("20260406-100100-s-tac-bbb")),
		entry("20260406-100300-d-tac-ddd", withContent("Old decision")),
		entry("20260406-100400-d-tac-eee", withSupersedes("20260406-100300-d-tac-ddd")),
		entry("20260406-100500-a-tac-fff", withCloses("20260406-100200-d-tac-ccc")),
	})

	open := g.FilterOpen("", "")
	ids := entryIDs(open)

	// aaa (open signal), eee (active decision), fff (action) = 3
	// excluded: bbb (closed signal), ccc (closed decision), ddd (superseded decision)
	if len(open) != 3 {
		t.Fatalf("FilterOpen('', '') = %v (len %d), want 3 entries", ids, len(open))
	}

	assertContains(t, ids, "20260406-100000-s-tac-aaa", "open signal")
	assertContains(t, ids, "20260406-100400-d-tac-eee", "superseding decision")
	assertContains(t, ids, "20260406-100500-a-tac-fff", "action")
	assertNotContains(t, ids, "20260406-100100-s-tac-bbb", "closed signal")
	assertNotContains(t, ids, "20260406-100200-d-tac-ccc", "closed decision")
	assertNotContains(t, ids, "20260406-100300-d-tac-ddd", "superseded decision")

	openSignals := g.FilterOpen(TypeSignal, "")
	if len(openSignals) != 1 {
		t.Errorf("FilterOpen(signal, '') = %d, want 1", len(openSignals))
	}

	openDecisions := g.FilterOpen(TypeDecision, "")
	if len(openDecisions) != 1 {
		t.Errorf("FilterOpen(decision, '') = %d, want 1", len(openDecisions))
	}
}

func TestFilterOpenWithLayerFilter(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-tac-aaa"),
		entry("20260406-100100-s-ops-bbb"),
	})

	tacOpen := g.FilterOpen(TypeSignal, LayerTactical)
	if len(tacOpen) != 1 {
		t.Fatalf("FilterOpen(signal, tactical) = %d, want 1", len(tacOpen))
	}
	if tacOpen[0].ID != "20260406-100000-s-tac-aaa" {
		t.Errorf("Got %q, want aaa", tacOpen[0].ID)
	}
}

// --- test helpers ---

func entryIDs(entries []*Entry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}

func assertContains(t *testing.T, ids []string, want, label string) {
	t.Helper()
	if !slices.Contains(ids, want) {
		t.Errorf("expected %s (%s) in results, got %v", label, want, ids)
	}
}

func assertNotContains(t *testing.T, ids []string, unwanted, label string) {
	t.Helper()
	if slices.Contains(ids, unwanted) {
		t.Errorf("expected %s (%s) NOT in results, but it was", label, unwanted)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
