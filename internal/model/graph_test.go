package model

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// entry is a test helper that builds an Entry from an ID string.
// It parses the type, layer, and time from the ID using ParseID.
func entry(id string, opts ...entryOpt) *Entry {
	parts, err := ParseID(id)
	if err != nil {
		panic(fmt.Sprintf("bad test ID %q: %v", id, err))
	}

	typ := TypeFromAbbrev[parts.TypeCode]
	kind := Kind("")
	if typ == TypeDecision {
		kind = KindDirective
	}

	e := &Entry{
		ID:      id,
		Type:    typ,
		Layer:   LayerFromAbbrev[parts.LayerCode],
		Kind:    kind,
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

func withAttachments(paths ...string) entryOpt {
	return func(e *Entry) { e.Attachments = paths }
}

func withSummary(s string) entryOpt {
	return func(e *Entry) { e.Summary = s }
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

	signals := g.Filter(GraphFilter{Type: TypeSignal})
	if len(signals) != 2 {
		t.Errorf("Filter(signal) = %d, want 2", len(signals))
	}

	tactical := g.Filter(GraphFilter{Layer: LayerTactical})
	if len(tactical) != 2 {
		t.Errorf("Filter(tactical) = %d, want 2", len(tactical))
	}

	tacSignals := g.Filter(GraphFilter{Type: TypeSignal, Layer: LayerTactical})
	if len(tacSignals) != 1 {
		t.Errorf("Filter(signal, tactical) = %d, want 1", len(tacSignals))
	}

	all := g.Filter(GraphFilter{})
	if len(all) != 3 {
		t.Errorf("Filter({}) = %d, want 3", len(all))
	}
}

func TestFilterOpenOnly(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-tac-aaa", withContent("Open signal")),
		entry("20260406-100100-s-tac-bbb", withContent("Closed signal")),
		entry("20260406-100200-d-tac-ccc", withCloses("20260406-100100-s-tac-bbb")),
		entry("20260406-100300-d-tac-ddd", withContent("Old decision")),
		entry("20260406-100400-d-tac-eee", withSupersedes("20260406-100300-d-tac-ddd")),
		entry("20260406-100500-a-tac-fff", withCloses("20260406-100200-d-tac-ccc")),
	})

	open := g.Filter(GraphFilter{OpenOnly: true})
	ids := entryIDs(open)

	// aaa (open signal), eee (active decision), fff (action) = 3
	// excluded: bbb (closed signal), ccc (closed decision), ddd (superseded decision)
	if len(open) != 3 {
		t.Fatalf("Filter(OpenOnly) = %v (len %d), want 3 entries", ids, len(open))
	}

	assertContains(t, ids, "20260406-100000-s-tac-aaa", "open signal")
	assertContains(t, ids, "20260406-100400-d-tac-eee", "superseding decision")
	assertContains(t, ids, "20260406-100500-a-tac-fff", "action")
	assertNotContains(t, ids, "20260406-100100-s-tac-bbb", "closed signal")
	assertNotContains(t, ids, "20260406-100200-d-tac-ccc", "closed decision")
	assertNotContains(t, ids, "20260406-100300-d-tac-ddd", "superseded decision")

	openSignals := g.Filter(GraphFilter{Type: TypeSignal, OpenOnly: true})
	if len(openSignals) != 1 {
		t.Errorf("Filter(signal, OpenOnly) = %d, want 1", len(openSignals))
	}

	openDecisions := g.Filter(GraphFilter{Type: TypeDecision, OpenOnly: true})
	if len(openDecisions) != 1 {
		t.Errorf("Filter(decision, OpenOnly) = %d, want 1", len(openDecisions))
	}
}

func TestFilterOpenOnlyWithLayerFilter(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-tac-aaa"),
		entry("20260406-100100-s-ops-bbb"),
	})

	tacOpen := g.Filter(GraphFilter{Type: TypeSignal, Layer: LayerTactical, OpenOnly: true})
	if len(tacOpen) != 1 {
		t.Fatalf("Filter(signal, tactical, OpenOnly) = %d, want 1", len(tacOpen))
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

func TestFilterWithKind(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-d-cpt-aaa", withKind(KindContract)),
		entry("20260406-100100-d-tac-bbb"),
		entry("20260406-100150-d-tac-ppp", withKind(KindPlan)),
		entry("20260406-100200-s-tac-ccc"),
	})

	// Kind=contract: only contract decisions
	contracts := g.Filter(GraphFilter{Kind: KindContract, OpenOnly: true})
	if len(contracts) != 1 {
		t.Fatalf("Filter(contract, OpenOnly) = %d, want 1", len(contracts))
	}
	if contracts[0].ID != "20260406-100000-d-cpt-aaa" {
		t.Errorf("Got %q, want aaa", contracts[0].ID)
	}

	// Kind=directive: only directive decisions (excludes contracts and plans)
	directives := g.Filter(GraphFilter{Kind: KindDirective, OpenOnly: true})
	if len(directives) != 1 {
		t.Fatalf("Filter(directive, OpenOnly) = %d, want 1", len(directives))
	}
	if directives[0].ID != "20260406-100100-d-tac-bbb" {
		t.Errorf("Got %q, want bbb", directives[0].ID)
	}

	// Kind=plan: only plan decisions
	plans := g.Filter(GraphFilter{Kind: KindPlan, OpenOnly: true})
	if len(plans) != 1 {
		t.Fatalf("Filter(plan, OpenOnly) = %d, want 1", len(plans))
	}
	if plans[0].ID != "20260406-100150-d-tac-ppp" {
		t.Errorf("Got %q, want ppp", plans[0].ID)
	}

	// Kind filter excludes non-decisions (signals, actions)
	contractOnly := g.Filter(GraphFilter{Kind: KindContract})
	if len(contractOnly) != 1 {
		t.Errorf("Filter(contract) = %d, want 1 (only contract, no signal)", len(contractOnly))
	}
}

func TestActiveDecisionsExcludesPlans(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-d-cpt-aaa", withKind(KindContract)),
		entry("20260406-100100-d-tac-bbb"),
		entry("20260406-100200-d-tac-ccc", withKind(KindPlan)),
	})

	active := g.ActiveDecisions()
	if len(active) != 1 {
		t.Fatalf("ActiveDecisions = %d, want 1 (contract and plan excluded)", len(active))
	}
	if active[0].ID != "20260406-100100-d-tac-bbb" {
		t.Errorf("Active = %q, want bbb", active[0].ID)
	}
}

func TestPlans(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-d-cpt-aaa", withKind(KindPlan)),
		entry("20260406-100100-d-tac-bbb"),
		entry("20260406-100200-d-cpt-ccc", withKind(KindPlan)),
		entry("20260406-100300-d-cpt-ddd", withKind(KindPlan), withContent("Superseded plan")),
		entry("20260406-100400-d-cpt-eee", withKind(KindPlan), withSupersedes("20260406-100300-d-cpt-ddd")),
		entry("20260406-100500-d-cpt-fff", withKind(KindPlan)),
		entry("20260406-100600-a-cpt-ggg", withCloses("20260406-100500-d-cpt-fff")),
	})

	plans := g.Plans()
	ids := entryIDs(plans)
	if len(plans) != 3 {
		t.Fatalf("Plans = %v (len %d), want 3", ids, len(plans))
	}
	assertContains(t, ids, "20260406-100000-d-cpt-aaa", "plan")
	assertContains(t, ids, "20260406-100200-d-cpt-ccc", "plan")
	assertContains(t, ids, "20260406-100400-d-cpt-eee", "superseding plan")
	assertNotContains(t, ids, "20260406-100100-d-tac-bbb", "directive")
	assertNotContains(t, ids, "20260406-100300-d-cpt-ddd", "superseded plan")
	assertNotContains(t, ids, "20260406-100500-d-cpt-fff", "closed plan")
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

// --- lint / validation tests ---

func TestLintDanglingRef(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
		entry("20260406-100100-d-stg-bbb", withRefs("20260406-100000-s-stg-aaa", "20260406-099999-s-stg-missing")),
	})

	lint := g.Lint()
	if len(lint) != 1 {
		t.Fatalf("Lint() = %d entries, want 1", len(lint))
	}
	if lint[0].ID != "20260406-100100-d-stg-bbb" {
		t.Errorf("Lint entry = %q, want bbb", lint[0].ID)
	}
	if len(lint[0].Warnings) != 1 {
		t.Fatalf("Warnings = %d, want 1", len(lint[0].Warnings))
	}
	w := lint[0].Warnings[0]
	if w.Field != "refs" {
		t.Errorf("Field = %q, want refs", w.Field)
	}
	if w.Value != "20260406-099999-s-stg-missing" {
		t.Errorf("Value = %q, want the missing ID", w.Value)
	}
}

func TestLintDanglingCloses(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-a-tac-aaa", withCloses("20260406-099999-d-tac-missing")),
	})

	lint := g.Lint()
	if len(lint) != 1 {
		t.Fatalf("Lint() = %d entries, want 1", len(lint))
	}
	if lint[0].Warnings[0].Field != "closes" {
		t.Errorf("Field = %q, want closes", lint[0].Warnings[0].Field)
	}
}

func TestLintMalformedID(t *testing.T) {
	// Simulate an entry with a short/malformed ID in refs (like the s-stg-qg0 bug)
	e := entry("20260406-100000-d-stg-aaa")
	e.Refs = []string{"d-stg-0gh"} // short suffix, not a full ID
	g := NewGraph([]*Entry{e})

	lint := g.Lint()
	if len(lint) != 1 {
		t.Fatalf("Lint() = %d entries, want 1", len(lint))
	}
	w := lint[0].Warnings[0]
	if w.Field != "refs" {
		t.Errorf("Field = %q, want refs", w.Field)
	}
	if w.Value != "d-stg-0gh" {
		t.Errorf("Value = %q, want d-stg-0gh", w.Value)
	}
}

func TestLintClosesTypeMismatch(t *testing.T) {
	tests := []struct {
		name      string
		entries   []*Entry
		wantWarns int
		wantMsg   string
	}{
		{
			name: "signal cannot close",
			entries: []*Entry{
				entry("20260406-100000-d-stg-aaa"),
				func() *Entry {
					e := entry("20260406-100100-s-stg-bbb")
					e.Closes = []string{"20260406-100000-d-stg-aaa"}
					return e
				}(),
			},
			wantWarns: 1,
			wantMsg:   "signal cannot close entries",
		},
		{
			name: "cannot close action",
			entries: []*Entry{
				entry("20260406-100000-a-tac-aaa"),
				func() *Entry {
					e := entry("20260406-100100-d-tac-bbb")
					e.Closes = []string{"20260406-100000-a-tac-aaa"}
					return e
				}(),
			},
			wantWarns: 1,
			wantMsg:   "actions cannot be closed",
		},
		{
			name: "decision cannot close decision",
			entries: []*Entry{
				entry("20260406-100000-d-tac-aaa"),
				func() *Entry {
					e := entry("20260406-100100-d-tac-bbb")
					e.Closes = []string{"20260406-100000-d-tac-aaa"}
					return e
				}(),
			},
			wantWarns: 1,
			wantMsg:   "decision cannot close another decision",
		},
		{
			name: "valid: decision closes signal",
			entries: []*Entry{
				entry("20260406-100000-s-stg-aaa"),
				entry("20260406-100100-d-stg-bbb", withCloses("20260406-100000-s-stg-aaa")),
			},
			wantWarns: 0,
		},
		{
			name: "valid: action closes decision",
			entries: []*Entry{
				entry("20260406-100000-d-tac-aaa"),
				entry("20260406-100100-a-tac-bbb", withCloses("20260406-100000-d-tac-aaa")),
			},
			wantWarns: 0,
		},
		{
			name: "valid: action closes signal",
			entries: []*Entry{
				entry("20260406-100000-s-tac-aaa"),
				entry("20260406-100100-a-tac-bbb", withCloses("20260406-100000-s-tac-aaa")),
			},
			wantWarns: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGraph(tt.entries)
			lint := g.Lint()

			totalWarns := 0
			for _, e := range lint {
				totalWarns += len(e.Warnings)
			}

			if totalWarns != tt.wantWarns {
				msgs := []string{}
				for _, e := range lint {
					for _, w := range e.Warnings {
						msgs = append(msgs, w.Message)
					}
				}
				t.Fatalf("warnings = %d (%v), want %d", totalWarns, msgs, tt.wantWarns)
			}

			if tt.wantMsg != "" && totalWarns > 0 {
				found := false
				for _, e := range lint {
					for _, w := range e.Warnings {
						if strings.Contains(w.Message, tt.wantMsg) {
							found = true
						}
					}
				}
				if !found {
					t.Errorf("expected warning containing %q", tt.wantMsg)
				}
			}
		})
	}
}

func TestLintSupersedesTypeMismatch(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
		entry("20260406-100100-d-stg-bbb", withSupersedes("20260406-100000-s-stg-aaa")),
	})

	lint := g.Lint()
	if len(lint) != 1 {
		t.Fatalf("Lint() = %d entries, want 1", len(lint))
	}
	w := lint[0].Warnings[0]
	if w.Field != "supersedes" {
		t.Errorf("Field = %q, want supersedes", w.Field)
	}
}

func TestLintSupersedesSameTypeValid(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-d-tac-aaa"),
		entry("20260406-100100-d-tac-bbb", withSupersedes("20260406-100000-d-tac-aaa")),
	})

	lint := g.Lint()
	if len(lint) != 0 {
		t.Errorf("Lint() = %d entries, want 0 for valid same-type supersedes", len(lint))
	}
}

func TestLintCleanGraph(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
		entry("20260406-100100-d-stg-bbb", withKind(KindDirective), withRefs("20260406-100000-s-stg-aaa"), withCloses("20260406-100000-s-stg-aaa")),
		entry("20260406-100200-a-tac-ccc", withRefs("20260406-100100-d-stg-bbb"), withCloses("20260406-100100-d-stg-bbb")),
	})

	lint := g.Lint()
	if len(lint) != 0 {
		for _, e := range lint {
			for _, w := range e.Warnings {
				t.Logf("unexpected warning on %s: %s", e.ID, w.Message)
			}
		}
		t.Fatalf("Lint() = %d entries, want 0 for clean graph", len(lint))
	}
}

func TestLintMultipleWarningsOnOneEntry(t *testing.T) {
	e := entry("20260406-100000-d-stg-aaa")
	e.Refs = []string{"bad-id"}
	e.Closes = []string{"also-bad"}
	g := NewGraph([]*Entry{e})

	lint := g.Lint()
	if len(lint) != 1 {
		t.Fatalf("Lint() = %d entries, want 1", len(lint))
	}
	if len(lint[0].Warnings) != 2 {
		t.Errorf("Warnings = %d, want 2", len(lint[0].Warnings))
	}
}

func TestLintBrokenAttachmentLink(t *testing.T) {
	e := entry("20260406-115516-s-stg-beh",
		withContent("See [design](./06-115516-s-stg-beh/design.md) for details."),
	)
	// No attachments — the link is broken
	g := NewGraph([]*Entry{e})

	lint := g.Lint()
	if len(lint) != 1 {
		t.Fatalf("Lint() = %d entries, want 1", len(lint))
	}
	w := lint[0].Warnings[0]
	if w.Field != "attachments" {
		t.Errorf("Field = %q, want attachments", w.Field)
	}
	if !strings.Contains(w.Message, "design.md") {
		t.Errorf("Message = %q, want mention of design.md", w.Message)
	}
}

func TestLintValidAttachmentLink(t *testing.T) {
	e := entry("20260406-115516-s-stg-beh",
		withContent("See [design](./06-115516-s-stg-beh/design.md) for details."),
		withAttachments("2026/04/06-115516-s-stg-beh/design.md"),
	)
	g := NewGraph([]*Entry{e})

	lint := g.Lint()
	if len(lint) != 0 {
		for _, e := range lint {
			for _, w := range e.Warnings {
				t.Logf("unexpected warning: %s", w.Message)
			}
		}
		t.Fatalf("Lint() = %d entries, want 0 for valid attachment link", len(lint))
	}
}

func TestLintMultipleBrokenAttachmentLinks(t *testing.T) {
	e := entry("20260409-110000-d-tac-abc",
		withContent("See [a](./09-110000-d-tac-abc/a.md) and [b](./09-110000-d-tac-abc/b.png)."),
		withAttachments("2026/04/09-110000-d-tac-abc/a.md"), // only a.md exists
	)
	g := NewGraph([]*Entry{e})

	lint := g.Lint()
	if len(lint) != 1 {
		t.Fatalf("Lint() = %d entries, want 1", len(lint))
	}
	if len(lint[0].Warnings) != 1 {
		t.Fatalf("Warnings = %d, want 1 (only b.png is broken)", len(lint[0].Warnings))
	}
	if !strings.Contains(lint[0].Warnings[0].Message, "b.png") {
		t.Errorf("Message = %q, want mention of b.png", lint[0].Warnings[0].Message)
	}
}

func TestLintDecisionMissingKind(t *testing.T) {
	e := entry("20260406-100000-d-stg-aaa", withKind(""))
	g := NewGraph([]*Entry{e})

	lint := g.Lint()
	if len(lint) != 1 {
		t.Fatalf("Lint() = %d entries, want 1", len(lint))
	}
	found := false
	for _, w := range lint[0].Warnings {
		if w.Field == "kind" {
			found = true
		}
	}
	if !found {
		t.Error("expected warning on 'kind' field for decision without kind")
	}
}

func TestLintDecisionWithKindNoWarning(t *testing.T) {
	e := entry("20260406-100000-d-stg-aaa", withKind(KindDirective))
	g := NewGraph([]*Entry{e})

	lint := g.Lint()
	if len(lint) != 0 {
		for _, e := range lint {
			for _, w := range e.Warnings {
				t.Logf("unexpected warning: %s", w.Message)
			}
		}
		t.Fatalf("Lint() = %d entries, want 0 for decision with kind", len(lint))
	}
}

func TestLintSignalWithoutKindNoWarning(t *testing.T) {
	e := entry("20260406-100000-s-stg-aaa")
	g := NewGraph([]*Entry{e})

	lint := g.Lint()
	if len(lint) != 0 {
		t.Fatalf("Lint() = %d entries, want 0 for signal without kind", len(lint))
	}
}

func TestLintNoAttachmentLinksNoWarning(t *testing.T) {
	e := entry("20260406-100000-s-stg-aaa",
		withContent("No attachment links here."),
	)
	g := NewGraph([]*Entry{e})

	lint := g.Lint()
	if len(lint) != 0 {
		t.Errorf("Lint() = %d entries, want 0 for entry without attachment links", len(lint))
	}
}

func TestResolveIDFullIDPassesThrough(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
	})

	got, err := g.ResolveID("20260406-100000-s-stg-aaa")
	if err != nil {
		t.Fatalf("ResolveID full ID: %v", err)
	}
	if got != "20260406-100000-s-stg-aaa" {
		t.Errorf("ResolveID = %q, want full ID unchanged", got)
	}
}

func TestResolveIDFullIDNotInGraphPassesThrough(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
	})

	// Full-shaped ID not present in graph — caller's existence check
	// is what should surface the not-found error, not ResolveID.
	got, err := g.ResolveID("20260406-100000-s-stg-zzz")
	if err != nil {
		t.Fatalf("ResolveID missing full ID: %v", err)
	}
	if got != "20260406-100000-s-stg-zzz" {
		t.Errorf("ResolveID = %q, want missing full ID passed through", got)
	}
}

func TestResolveIDShortFormUniqueMatch(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
		entry("20260406-100100-d-tac-bbb", withKind(KindDirective)),
	})

	got, err := g.ResolveID("d-tac-bbb")
	if err != nil {
		t.Fatalf("ResolveID short unique: %v", err)
	}
	if got != "20260406-100100-d-tac-bbb" {
		t.Errorf("ResolveID = %q, want full ID", got)
	}
}

func TestResolveIDShortFormAmbiguousErrors(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-xyz"),
		entry("20260407-110000-s-stg-xyz"),
	})

	got, err := g.ResolveID("s-stg-xyz")
	if err == nil {
		t.Fatalf("ResolveID ambiguous: want error, got %q", got)
	}
	msg := err.Error()
	if !strings.Contains(msg, "ambiguous") {
		t.Errorf("error should mention ambiguous, got: %s", msg)
	}
	if !strings.Contains(msg, "20260406-100000-s-stg-xyz") ||
		!strings.Contains(msg, "20260407-110000-s-stg-xyz") {
		t.Errorf("error should list all candidates, got: %s", msg)
	}
}

func TestResolveIDShortFormNoMatchPassesThrough(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
	})

	got, err := g.ResolveID("s-stg-zzz")
	if err != nil {
		t.Fatalf("ResolveID short no-match: %v", err)
	}
	if got != "s-stg-zzz" {
		t.Errorf("ResolveID = %q, want input unchanged for zero matches", got)
	}
}

func TestResolveIDShortFormUnknownTypeAbbrev(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
	})

	// 'x' is not a valid type abbrev — treat as unresolvable, pass through.
	got, err := g.ResolveID("x-stg-aaa")
	if err != nil {
		t.Fatalf("ResolveID bad type: %v", err)
	}
	if got != "x-stg-aaa" {
		t.Errorf("ResolveID = %q, want input unchanged for unknown type abbrev", got)
	}
}

func TestResolveIDShortFormUnknownLayerAbbrev(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
	})

	got, err := g.ResolveID("s-xxx-aaa")
	if err != nil {
		t.Fatalf("ResolveID bad layer: %v", err)
	}
	if got != "s-xxx-aaa" {
		t.Errorf("ResolveID = %q, want input unchanged for unknown layer abbrev", got)
	}
}

func TestResolveIDEmptyString(t *testing.T) {
	g := NewGraph([]*Entry{})
	got, err := g.ResolveID("")
	if err != nil {
		t.Fatalf("ResolveID empty: %v", err)
	}
	if got != "" {
		t.Errorf("ResolveID(\"\") = %q, want empty", got)
	}
}

func TestResolveIDSingleSegmentPassesThrough(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
	})

	// "aaa" has no dashes — not a recognized short form; caller surfaces not-found.
	got, err := g.ResolveID("aaa")
	if err != nil {
		t.Fatalf("ResolveID suffix-only: %v", err)
	}
	if got != "aaa" {
		t.Errorf("ResolveID = %q, want input unchanged (suffix-only not supported)", got)
	}
}

func TestResolveIDsAllFullIDs(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
		entry("20260406-100100-d-tac-bbb", withKind(KindDirective)),
	})

	ids := []string{"20260406-100000-s-stg-aaa", "20260406-100100-d-tac-bbb"}
	got, err := g.ResolveIDs(ids)
	if err != nil {
		t.Fatalf("ResolveIDs: %v", err)
	}
	if !slices.Equal(got, ids) {
		t.Errorf("ResolveIDs = %v, want %v", got, ids)
	}
}

func TestResolveIDsMixedShortAndFull(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-aaa"),
		entry("20260406-100100-d-tac-bbb", withKind(KindDirective)),
	})

	got, err := g.ResolveIDs([]string{"s-stg-aaa", "20260406-100100-d-tac-bbb"})
	if err != nil {
		t.Fatalf("ResolveIDs: %v", err)
	}
	want := []string{"20260406-100000-s-stg-aaa", "20260406-100100-d-tac-bbb"}
	if !slices.Equal(got, want) {
		t.Errorf("ResolveIDs = %v, want %v", got, want)
	}
}

func TestResolveIDsPropagatesAmbiguousError(t *testing.T) {
	g := NewGraph([]*Entry{
		entry("20260406-100000-s-stg-xyz"),
		entry("20260407-110000-s-stg-xyz"),
	})

	_, err := g.ResolveIDs([]string{"s-stg-xyz"})
	if err == nil {
		t.Fatal("ResolveIDs: want error for ambiguous short ID")
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
