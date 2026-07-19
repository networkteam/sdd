package model

import (
	"reflect"
	"strings"
	"testing"
)

func indexedFact(t *testing.T, id, title, topic string) *Entry {
	t.Helper()
	index, err := NewFactIndex(title, topic)
	if err != nil {
		t.Fatal(err)
	}
	path, err := ParseTopicPath(topic)
	if err != nil {
		t.Fatal(err)
	}
	return &Entry{ID: id, Type: TypeSignal, Layer: LayerTactical, Kind: KindFact, Topics: []TopicPath{path}, Index: index}
}

func TestFactIndexYAMLRoundTrip(t *testing.T) {
	const source = `---
type: signal
layer: tactical
kind: fact
topics:
    - cli/view
index:
    title: How to compose graph views
    topic: cli/view
---

Reference body.
`
	entry, err := ParseEntry("20260719-120000-s-tac-idx.md", source)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Index == nil || entry.Index.Title != "How to compose graph views" || entry.Index.Topic.String() != "cli/view" {
		t.Fatalf("index = %+v", entry.Index)
	}
	formatted := FormatFrontmatter(entry)
	if !strings.Contains(formatted, "index:\n    title: How to compose graph views\n    topic: cli/view\n") {
		t.Fatalf("nested index missing from frontmatter:\n%s", formatted)
	}
	roundTrip, err := ParseEntry(entry.ID+".md", formatted+"\n"+entry.Content)
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip.Index == nil || roundTrip.Index.Title != entry.Index.Title || !roundTrip.Index.Topic.Equal(entry.Index.Topic) {
		t.Fatalf("round-trip index = %+v, want %+v", roundTrip.Index, entry.Index)
	}
}

func TestFactIndexYAMLRejectsNonExactMappings(t *testing.T) {
	tests := []struct {
		name  string
		index string
		want  string
	}{
		{name: "scalar", index: `index: cue`, want: "must be a mapping"},
		{name: "sequence", index: "index: [title, topic]", want: "must be a mapping"},
		{name: "unknown field", index: "index: {title: Cue, topic: cli/view, extra: value}", want: `unknown field "extra"`},
		{name: "duplicate field", index: "index: {title: Cue, title: Other, topic: cli/view}", want: `duplicate field "title"`},
		{name: "non-string", index: "index: {title: 12, topic: cli/view}", want: "index.title must be a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseEntry("20260719-120000-s-tac-bad.md", "---\ntype: signal\nlayer: tactical\nkind: fact\ntopics: [cli/view]\n"+tt.index+"\n---\nbody")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseEntry error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateEntryFactIndex(t *testing.T) {
	topic, _ := ParseTopicPath("cli/view")
	other, _ := ParseTopicPath("engine/base-facts")
	tests := []struct {
		name  string
		entry *Entry
		want  string
	}{
		{
			name: "non-fact",
			entry: &Entry{ID: "20260719-120000-s-tac-non", Type: TypeSignal, Layer: LayerTactical, Kind: KindInsight,
				Topics: []TopicPath{topic}, Index: &FactIndex{Title: "Title", Topic: topic}},
			want: "only valid on kind: fact",
		},
		{
			name: "blank title",
			entry: &Entry{ID: "20260719-120001-s-tac-ttl", Type: TypeSignal, Layer: LayerTactical, Kind: KindFact,
				Topics: []TopicPath{topic}, Index: &FactIndex{Title: " ", Topic: topic}},
			want: "trimmed, non-empty",
		},
		{
			name: "missing topic",
			entry: &Entry{ID: "20260719-120002-s-tac-top", Type: TypeSignal, Layer: LayerTactical, Kind: KindFact,
				Topics: []TopicPath{topic}, Index: &FactIndex{Title: "Title"}},
			want: "valid, non-empty topic path",
		},
		{
			name: "malformed topic value",
			entry: &Entry{ID: "20260719-120004-s-tac-raw", Type: TypeSignal, Layer: LayerTactical, Kind: KindFact,
				Topics: []TopicPath{topic}, Index: &FactIndex{Title: "Title", Topic: TopicPath{Components: []string{"cli view"}}}},
			want: "invalid character",
		},
		{
			name: "topic not enrolled",
			entry: &Entry{ID: "20260719-120003-s-tac-mem", Type: TypeSignal, Layer: LayerTactical, Kind: KindFact,
				Topics: []TopicPath{other}, Index: &FactIndex{Title: "Title", Topic: topic}},
			want: "must also appear in topics",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := NewGraph([]*Entry{tt.entry})
			var found bool
			for _, warning := range graph.ByID[tt.entry.ID].Warnings {
				if warning.Field == "index" && strings.Contains(warning.Message, tt.want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("warnings = %+v, want index warning containing %q", tt.entry.Warnings, tt.want)
			}
		})
	}
}

func TestParseEntryInvalidFactIndexTopicFailsValidation(t *testing.T) {
	entry, err := ParseEntry("20260719-120000-s-tac-bad.md", `---
type: signal
layer: tactical
kind: fact
topics: [cli/view]
index: {title: Reference, topic: "cli view"}
---
body`)
	if err != nil {
		t.Fatal(err)
	}
	graph := NewGraph([]*Entry{entry})
	if len(graph.ByID[entry.ID].Warnings) == 0 || !strings.Contains(graph.ByID[entry.ID].Warnings[0].Message, "invalid character") {
		t.Fatalf("warnings = %+v", graph.ByID[entry.ID].Warnings)
	}
}

func TestIndexedFactsLifecycleOrderingAndNoInheritance(t *testing.T) {
	old := indexedFact(t, "20260719-100000-s-tac-old", "Old", "cli/view")
	successor := indexedFact(t, "20260719-100100-s-tac-new", "beta", "cli/view")
	successor.Supersedes = []string{old.ID}
	closed := indexedFact(t, "20260719-100200-s-tac-cls", "Closed", "agent/ux")
	closer := &Entry{ID: "20260719-100300-d-tac-dec", Type: TypeDecision, Layer: LayerTactical, Kind: KindDirective, Intent: IntentPending, Closes: []string{closed.ID}}
	alphaLater := indexedFact(t, "20260719-100500-s-tac-a02", "Alpha", "cli/view")
	alphaEarlier := indexedFact(t, "20260719-100400-s-tac-a01", "alpha", "cli/view")
	graph := NewGraph([]*Entry{old, successor, closed, closer, alphaLater, alphaEarlier})

	rows, err := graph.IndexedFacts()
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(rows))
	for i, row := range rows {
		got[i] = row.ID
	}
	want := []string{alphaEarlier.ID, alphaLater.ID, successor.ID}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IndexedFacts IDs = %v, want %v", got, want)
	}

	unindexedSuccessor := &Entry{ID: "20260719-100600-s-tac-off", Type: TypeSignal, Layer: LayerTactical, Kind: KindFact, Supersedes: []string{successor.ID}}
	graph = NewGraph(append(graph.Entries, unindexedSuccessor))
	rows, err = graph.IndexedFacts()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == successor.ID || row.ID == unindexedSuccessor.ID {
			t.Fatalf("index metadata inherited across supersession: %+v", rows)
		}
	}
}

func TestIndexedFactsOrderingAndValueIsolation(t *testing.T) {
	agent := indexedFact(t, "20260719-101000-s-tac-top", "Later title", "agent/ux")
	sigma := indexedFact(t, "20260719-101001-s-tac-sig", "Σ", "cli/view")
	finalSigma := indexedFact(t, "20260719-101002-s-tac-fin", "ς", "cli/view")
	graph := NewGraph([]*Entry{finalSigma, sigma, agent})

	rows, err := graph.IndexedFacts()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{rows[0].ID, rows[1].ID, rows[2].ID}
	want := []string{agent.ID, sigma.ID, finalSigma.ID}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IndexedFacts IDs = %v, want %v", got, want)
	}
	rows[0].Topic.Components[0] = "mutated"
	if agent.Index.Topic.String() != "agent/ux" {
		t.Fatalf("derived row mutated entry topic to %q", agent.Index.Topic.String())
	}
}

func TestIndexedFactsFailsOnActiveMalformedEnrollment(t *testing.T) {
	topic, _ := ParseTopicPath("cli/view")
	malformed := &Entry{
		ID: "20260719-102000-s-tac-bad", Type: TypeSignal, Layer: LayerTactical, Kind: KindInsight,
		Topics: []TopicPath{topic}, Index: &FactIndex{Title: "Invalid enrollment", Topic: topic},
	}
	graph := NewGraph([]*Entry{malformed})
	_, err := graph.IndexedFacts()
	if err == nil || !strings.Contains(err.Error(), malformed.ID) || !strings.Contains(err.Error(), "only valid on kind: fact") {
		t.Fatalf("IndexedFacts error = %v", err)
	}
}

func TestIndexedFactsUsesDiskOverrideAndExcludesDependencies(t *testing.T) {
	const id = "20260719-110000-s-tac-ovr"
	embedded := indexedFact(t, id, "Embedded", "cli/view")
	embedded.Embedded = true
	retained := indexedFact(t, "20260719-110050-s-tac-base", "Retained base fact", "engine/base-facts")
	retained.Embedded = true
	disk := &Entry{ID: id, Type: TypeSignal, Layer: LayerTactical, Kind: KindFact}
	local := NewGraph(MergeEmbedded([]*Entry{disk}, map[string]bool{id: true}, []*Entry{embedded, retained}))
	remote := NewGraph([]*Entry{indexedFact(t, "20260719-110100-s-tac-rem", "Remote", "cli/view")})
	NewMultiGraph(local, []string{"example.com/team/remote"}, func(string) (*Graph, error) { return remote, nil })
	if member, err := local.MemberGraph("example.com/team/remote"); err != nil || member == nil {
		t.Fatalf("loading dependency: member=%v err=%v", member, err)
	}
	rows, err := local.IndexedFacts()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != retained.ID {
		t.Fatalf("IndexedFacts = %+v, want retained embedded fact only", rows)
	}
}
