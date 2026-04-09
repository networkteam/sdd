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

func withKind(k Kind) entryOpt {
	return func(e *Entry) { e.Kind = k }
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

func TestResolveAttachmentLinks(t *testing.T) {
	tests := []struct {
		content string
		id      string
		want    string
	}{
		{
			"See [notes]({{attachments}}/design.md) for details.",
			"20260409-110000-d-tac-abc",
			"See [notes](./09-110000-d-tac-abc/design.md) for details.",
		},
		{
			"Two links: [a]({{attachments}}/a.md) and [b]({{attachments}}/b.png).",
			"20260409-110000-d-tac-abc",
			"Two links: [a](./09-110000-d-tac-abc/a.md) and [b](./09-110000-d-tac-abc/b.png).",
		},
		{
			"No placeholders here.",
			"20260409-110000-d-tac-abc",
			"No placeholders here.",
		},
	}

	for _, tt := range tests {
		got := ResolveAttachmentLinks(tt.content, tt.id)
		if got != tt.want {
			t.Errorf("ResolveAttachmentLinks(%q, %q) = %q, want %q", tt.content, tt.id, got, tt.want)
		}
	}
}

func TestLoadGraphWithAttachments(t *testing.T) {
	dir := t.TempDir()

	writeGraphEntry(t, dir, "20260406-115516-s-stg-beh", `---
type: signal
layer: strategic
---

See [design](./06-115516-s-stg-beh/design.md) for details.`)

	// Create attachment directory and file
	attachDir := filepath.Join(dir, "2026", "04", "06-115516-s-stg-beh")
	if err := os.MkdirAll(attachDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attachDir, "design.md"), []byte("# Design"), 0644); err != nil {
		t.Fatal(err)
	}

	g, err := LoadGraph(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(g.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(g.Entries))
	}
	e := g.Entries[0]
	if len(e.Attachments) != 1 {
		t.Fatalf("Attachments = %v, want [design.md]", e.Attachments)
	}
	if e.Attachments[0] != "design.md" {
		t.Errorf("Attachments[0] = %q, want %q", e.Attachments[0], "design.md")
	}
}

func TestIDToRelPath(t *testing.T) {
	tests := []struct {
		id      string
		want    string
		wantErr bool
	}{
		{"20260406-115516-s-stg-beh", "2026/04/06-115516-s-stg-beh.md", false},
		{"20260407-214507-s-ops-jpb", "2026/04/07-214507-s-ops-jpb.md", false},
		{"20251231-235959-d-cpt-abc", "2025/12/31-235959-d-cpt-abc.md", false},
		{"short", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, err := IDToRelPath(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Errorf("IDToRelPath(%q) = %q, want error", tt.id, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("IDToRelPath(%q) error: %v", tt.id, err)
			}
			// Normalize to forward slashes for comparison
			got = filepath.ToSlash(got)
			if got != tt.want {
				t.Errorf("IDToRelPath(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestRelPathToID(t *testing.T) {
	tests := []struct {
		rel     string
		want    string
		wantErr bool
	}{
		{"2026/04/06-115516-s-stg-beh.md", "20260406-115516-s-stg-beh", false},
		{"2025/12/31-235959-d-cpt-abc.md", "20251231-235959-d-cpt-abc", false},
		{"flat-file.md", "", true},
		{"too/many/levels/deep.md", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			got, err := RelPathToID(tt.rel)
			if tt.wantErr {
				if err == nil {
					t.Errorf("RelPathToID(%q) = %q, want error", tt.rel, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("RelPathToID(%q) error: %v", tt.rel, err)
			}
			if got != tt.want {
				t.Errorf("RelPathToID(%q) = %q, want %q", tt.rel, got, tt.want)
			}
		})
	}
}

func TestLoadGraph(t *testing.T) {
	dir := t.TempDir()

	writeGraphEntry(t, dir, "20260406-115516-s-stg-beh", `---
type: signal
layer: strategic
---

Signal one.`)

	writeGraphEntry(t, dir, "20260406-115540-d-stg-0gh", `---
type: decision
layer: strategic
refs:
  - 20260406-115516-s-stg-beh
closes:
  - 20260406-115516-s-stg-beh
---

Decision closing signal.`)

	writeGraphEntry(t, dir, "20260406-115559-a-cpt-f8v", `---
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

	open := g.FilterOpen("", "", "")
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

	openSignals := g.FilterOpen(TypeSignal, "", "")
	if len(openSignals) != 1 {
		t.Errorf("FilterOpen(signal, '') = %d, want 1", len(openSignals))
	}

	openDecisions := g.FilterOpen(TypeDecision, "", "")
	if len(openDecisions) != 1 {
		t.Errorf("FilterOpen(decision, '') = %d, want 1", len(openDecisions))
	}
}

func TestFilterOpenWithLayerFilter(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-tac-aaa"),
		entry("20260406-100100-s-ops-bbb"),
	})

	tacOpen := g.FilterOpen(TypeSignal, LayerTactical, "")
	if len(tacOpen) != 1 {
		t.Fatalf("FilterOpen(signal, tactical) = %d, want 1", len(tacOpen))
	}
	if tacOpen[0].ID != "20260406-100000-s-tac-aaa" {
		t.Errorf("Got %q, want aaa", tacOpen[0].ID)
	}
}

func TestActiveDecisionsExcludesContracts(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-d-cpt-aaa", withKind(KindContract)),
		entry("20260406-100100-d-tac-bbb"),
	})

	active := g.ActiveDecisions()
	if len(active) != 1 {
		t.Fatalf("ActiveDecisions = %d, want 1 (contract excluded)", len(active))
	}
	if active[0].ID != "20260406-100100-d-tac-bbb" {
		t.Errorf("Active = %q, want bbb", active[0].ID)
	}
}

func TestContracts(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-d-cpt-aaa", withKind(KindContract)),
		entry("20260406-100100-d-tac-bbb"),
		entry("20260406-100200-d-cpt-ccc", withKind(KindContract)),
		entry("20260406-100300-d-cpt-ddd", withKind(KindContract), withContent("Superseded contract")),
		entry("20260406-100400-d-cpt-eee", withKind(KindContract), withSupersedes("20260406-100300-d-cpt-ddd")),
	})

	contracts := g.Contracts()
	ids := entryIDs(contracts)
	if len(contracts) != 3 {
		t.Fatalf("Contracts = %v (len %d), want 3", ids, len(contracts))
	}
	assertContains(t, ids, "20260406-100000-d-cpt-aaa", "contract")
	assertContains(t, ids, "20260406-100200-d-cpt-ccc", "contract")
	assertContains(t, ids, "20260406-100400-d-cpt-eee", "superseding contract")
	assertNotContains(t, ids, "20260406-100100-d-tac-bbb", "directive")
	assertNotContains(t, ids, "20260406-100300-d-cpt-ddd", "superseded contract")
}

func TestFilterOpenWithKind(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-d-cpt-aaa", withKind(KindContract)),
		entry("20260406-100100-d-tac-bbb"),
		entry("20260406-100200-s-tac-ccc"),
	})

	// Kind=contract: only contracts
	contracts := g.FilterOpen(TypeDecision, "", KindContract)
	if len(contracts) != 1 {
		t.Fatalf("FilterOpen(decision, '', contract) = %d, want 1", len(contracts))
	}
	if contracts[0].ID != "20260406-100000-d-cpt-aaa" {
		t.Errorf("Got %q, want aaa", contracts[0].ID)
	}

	// Kind=directive: only directives
	directives := g.FilterOpen(TypeDecision, "", KindDirective)
	if len(directives) != 1 {
		t.Fatalf("FilterOpen(decision, '', directive) = %d, want 1", len(directives))
	}
	if directives[0].ID != "20260406-100100-d-tac-bbb" {
		t.Errorf("Got %q, want bbb", directives[0].ID)
	}

	// Kind filter doesn't affect signals
	all := g.FilterOpen("", "", KindContract)
	if len(all) != 2 {
		t.Errorf("FilterOpen('', '', contract) = %d, want 2 (signal + contract)", len(all))
	}
}

func TestShortContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		maxLen  int
		want    string
	}{
		{
			name:    "short enough",
			content: "Hello world.",
			maxLen:  80,
			want:    "Hello world.",
		},
		{
			name:    "truncate at sentence boundary shows ellipsis",
			content: "First sentence. Second sentence. Third sentence that is longer.",
			maxLen:  50,
			want:    "First sentence. Second sentence. ...",
		},
		{
			name:    "single long sentence falls back to words",
			content: "This is a very long single sentence without any period at the end",
			maxLen:  40,
			want:    "This is a very long single sentence ...",
		},
		{
			name:    "first sentence too long falls back to words",
			content: "This entire first sentence is way too long for the limit. Short.",
			maxLen:  30,
			want:    "This entire first sentence ...",
		},
		{
			name:    "multiline uses first line only",
			content: "First line.\nSecond line.",
			maxLen:  80,
			want:    "First line.",
		},
		{
			name:    "accumulates sentences, no room for ellipsis",
			content: "One. Two. Three. Four. Five.",
			maxLen:  24,
			want:    "One. Two. Three. Four.",
		},
		{
			name:    "accumulates sentences with ellipsis when room",
			content: "One. Two. Three. Four. Five. Six.",
			maxLen:  32,
			want:    "One. Two. Three. Four. Five. ...",
		},
		{
			name:    "exact fit",
			content: "Exact.",
			maxLen:  6,
			want:    "Exact.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Entry{Content: tt.content}
			got := e.ShortContent(tt.maxLen)
			if got != tt.want {
				t.Errorf("ShortContent(%d) = %q, want %q", tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestDownstream(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
		entry("20260406-100100-d-stg-bbb", withRefs("20260406-100000-s-stg-aaa")),
		entry("20260406-100200-a-tac-ccc", withRefs("20260406-100100-d-stg-bbb"), withCloses("20260406-100100-d-stg-bbb")),
		entry("20260406-100300-d-tac-ddd"), // unrelated
	})

	downstream := g.Downstream("20260406-100000-s-stg-aaa")
	ids := entryIDs(downstream)
	if len(downstream) != 1 {
		t.Fatalf("Downstream(aaa) = %v (len %d), want 1", ids, len(downstream))
	}
	assertContains(t, ids, "20260406-100100-d-stg-bbb", "decision referencing signal")

	downstream = g.Downstream("20260406-100100-d-stg-bbb")
	ids = entryIDs(downstream)
	if len(downstream) != 1 {
		t.Fatalf("Downstream(bbb) = %v (len %d), want 1", ids, len(downstream))
	}
	assertContains(t, ids, "20260406-100200-a-tac-ccc", "action referencing+closing decision")
}

func TestDownstreamSupersedes(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-d-tac-aaa"),
		entry("20260406-100100-d-tac-bbb", withSupersedes("20260406-100000-d-tac-aaa")),
	})

	downstream := g.Downstream("20260406-100000-d-tac-aaa")
	ids := entryIDs(downstream)
	if len(downstream) != 1 {
		t.Fatalf("Downstream(aaa) = %v (len %d), want 1", ids, len(downstream))
	}
	assertContains(t, ids, "20260406-100100-d-tac-bbb", "superseding decision")
}

func TestDownstreamDeduplicates(t *testing.T) {
	// Entry both refs and closes the same target — should appear once
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-tac-aaa"),
		entry("20260406-100100-d-tac-bbb", withRefs("20260406-100000-s-tac-aaa"), withCloses("20260406-100000-s-tac-aaa")),
	})

	downstream := g.Downstream("20260406-100000-s-tac-aaa")
	if len(downstream) != 1 {
		t.Fatalf("Downstream(aaa) = %d, want 1 (deduplicated)", len(downstream))
	}
}

func TestDownstreamEmpty(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-tac-aaa"),
	})

	downstream := g.Downstream("20260406-100000-s-tac-aaa")
	if len(downstream) != 0 {
		t.Errorf("Downstream(leaf) = %d, want 0", len(downstream))
	}

	downstream = g.Downstream("nonexistent")
	if len(downstream) != 0 {
		t.Errorf("Downstream(nonexistent) = %d, want 0", len(downstream))
	}
}

func TestDownstreamSortedByTime(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
		entry("20260406-100300-d-tac-ddd", withRefs("20260406-100000-s-stg-aaa")),
		entry("20260406-100100-s-cpt-bbb", withRefs("20260406-100000-s-stg-aaa")),
		entry("20260406-100200-d-stg-ccc", withRefs("20260406-100000-s-stg-aaa")),
	})

	downstream := g.Downstream("20260406-100000-s-stg-aaa")
	if len(downstream) != 3 {
		t.Fatalf("Downstream = %d, want 3", len(downstream))
	}
	if downstream[0].ID != "20260406-100100-s-cpt-bbb" {
		t.Errorf("First = %q, want bbb (earliest)", downstream[0].ID)
	}
	if downstream[2].ID != "20260406-100300-d-tac-ddd" {
		t.Errorf("Last = %q, want ddd (latest)", downstream[2].ID)
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

// writeGraphEntry writes a graph entry file in the hierarchical YYYY/MM/ layout.
// The id is the full entry ID (e.g. "20260406-115516-s-stg-beh").
func writeGraphEntry(t *testing.T, dir, id, content string) {
	t.Helper()
	relPath, err := IDToRelPath(id)
	if err != nil {
		t.Fatalf("IDToRelPath(%q): %v", id, err)
	}
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
