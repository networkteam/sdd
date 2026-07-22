package model

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

func TestFactIndexYAMLRejectsPresentNull(t *testing.T) {
	for _, index := range []string{"index: null", "index: ~", "index:", "index: Null"} {
		source := "---\ntype: signal\nlayer: tactical\nkind: fact\ntopics: [cli/view]\n" + index + "\n---\nbody"
		_, err := ParseEntry("20260719-120000-s-tac-nul.md", source)
		if err == nil || !strings.Contains(err.Error(), "index cannot be null") {
			t.Errorf("%q error = %v", index, err)
		}
	}
	aliased := "---\ntype: signal\nlayer: tactical\nkind: fact\ntopics: [cli/view]\nsummary: &empty null\nindex: *empty\n---\nbody"
	if _, err := ParseEntry("20260719-120000-s-tac-als.md", aliased); err == nil || !strings.Contains(err.Error(), "index cannot be null") {
		t.Errorf("aliased null error = %v", err)
	}
}

func TestFactIndexYAMLAbsentRemainsAbsent(t *testing.T) {
	entry, err := ParseEntry("20260719-120000-s-tac-off.md", "---\ntype: signal\nlayer: tactical\nkind: fact\n---\nbody")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Index != nil || strings.Contains(FormatFrontmatter(entry), "index:") {
		t.Fatalf("absent index round-trip = %+v\n%s", entry.Index, FormatFrontmatter(entry))
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
			name: "multiline title",
			entry: &Entry{ID: "20260719-120005-s-tac-lin", Type: TypeSignal, Layer: LayerTactical, Kind: KindFact,
				Topics: []TopicPath{topic}, Index: &FactIndex{Title: "Title\n## Injected", Topic: topic}},
			want: "control or line-separator",
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

func TestNewFactIndexRejectsControlsAndLineSeparators(t *testing.T) {
	for _, title := range []string{"First\nSecond", "First\rSecond", "First\x00Second", "First\u0085Second", "First\u2028Second", "First\u2029Second"} {
		if _, err := NewFactIndex(title, "cli/view"); err == nil || !strings.Contains(err.Error(), "control or line-separator") {
			t.Fatalf("NewFactIndex(%q) error = %v", title, err)
		}
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

// TestFactIndexTopicRawRoundTrip pins the topicRaw fallback branch: when a
// topic fails to parse, Topic stays zero and the raw string is retained, so
// marshal must emit the raw topic (never silently drop it) while validation
// still surfaces the parse error.
func TestFactIndexTopicRawRoundTrip(t *testing.T) {
	index := &FactIndex{Title: "Reference", topicRaw: "cli view"}
	if !index.Topic.IsZero() {
		t.Fatalf("precondition: Topic should be zero, got %q", index.Topic.String())
	}
	marshaled, err := index.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	out, err := yaml.Marshal(marshaled)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "topic: cli view") {
		t.Fatalf("marshal dropped unparsed raw topic:\n%s", out)
	}
	if err := index.Validate(); err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("Validate error = %v, want invalid character", err)
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

	rows := graph.IndexedFacts()
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
	rows = graph.IndexedFacts()
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

	rows := graph.IndexedFacts()
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

// TestMalformedActiveEnrollmentExcludedWithParity pins the single-rule
// contract: an active entry with a present-but-invalid index (here, index on a
// non-fact kind) is not a member of the indexed population on either surface —
// FilterIndexed nor IndexedFacts — yet it still loads carrying an index
// warning rather than failing the read.
func TestMalformedActiveEnrollmentExcludedWithParity(t *testing.T) {
	topic, _ := ParseTopicPath("cli/view")
	malformed := &Entry{
		ID: "20260719-102000-s-tac-bad", Type: TypeSignal, Layer: LayerTactical, Kind: KindInsight,
		Topics: []TopicPath{topic}, Index: &FactIndex{Title: "Invalid enrollment", Topic: topic},
	}
	valid := indexedFact(t, "20260719-102001-s-tac-ok", "Valid", "cli/view")
	graph := NewGraph([]*Entry{malformed, valid})

	if got := FilterIndexed(graph.Entries); len(got) != 1 || got[0].ID != valid.ID {
		t.Fatalf("FilterIndexed = %+v, want only %s", got, valid.ID)
	}
	rows := graph.IndexedFacts()
	if len(rows) != 1 || rows[0].ID != valid.ID {
		t.Fatalf("IndexedFacts = %+v, want only %s", rows, valid.ID)
	}

	var warned bool
	for _, warning := range graph.ByID[malformed.ID].Warnings {
		if warning.Field == "index" {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("malformed enrollment did not load with an index warning: %+v", graph.ByID[malformed.ID].Warnings)
	}
}

// TestMalformedEnrollmentOnRetiredFactExcluded pins that a malformed index on a
// superseded (retired) fact is silently excluded from both surfaces — the
// invalid enrollment is dropped before lifecycle even enters the picture.
func TestMalformedEnrollmentOnRetiredFactExcluded(t *testing.T) {
	topic, _ := ParseTopicPath("cli/view")
	other, _ := ParseTopicPath("engine/base-facts")
	retired := &Entry{
		ID: "20260719-102300-s-tac-old", Type: TypeSignal, Layer: LayerTactical, Kind: KindFact,
		Topics: []TopicPath{other}, Index: &FactIndex{Title: "Unenrolled topic", Topic: topic},
	}
	successor := &Entry{ID: "20260719-102400-s-tac-new", Type: TypeSignal, Layer: LayerTactical, Kind: KindFact, Supersedes: []string{retired.ID}}
	graph := NewGraph([]*Entry{retired, successor})

	if got := FilterIndexed(graph.Entries); len(got) != 0 {
		t.Fatalf("FilterIndexed = %+v, want empty", got)
	}
	if rows := graph.IndexedFacts(); len(rows) != 0 {
		t.Fatalf("IndexedFacts = %+v, want empty", rows)
	}
}

func TestFilterIndexedSelectsMetadataWithoutLifecycleSemantics(t *testing.T) {
	indexed := indexedFact(t, "20260719-102100-s-tac-idx", "Indexed", "cli/view")
	plain := &Entry{ID: "20260719-102200-s-tac-off", Type: TypeSignal, Layer: LayerTactical, Kind: KindFact}
	if got := FilterIndexed([]*Entry{plain, indexed}); len(got) != 1 || got[0] != indexed {
		t.Fatalf("FilterIndexed = %+v", got)
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
	rows := local.IndexedFacts()
	if len(rows) != 1 || rows[0].ID != retained.ID {
		t.Fatalf("IndexedFacts = %+v, want retained embedded fact only", rows)
	}
}
