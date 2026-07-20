package finders

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

func withFactIndex(title, topic string) entryOpt {
	return func(entry *model.Entry) {
		index, err := model.NewFactIndex(title, topic)
		if err != nil {
			panic(err)
		}
		entry.Index = index
	}
}

func TestViewIndexedIsStructuralAndComposes(t *testing.T) {
	old := entry("20260101-100000-s-tac-old", withKind(model.KindFact), withTopics("cli/view"), withFactIndex("Old", "cli/view"))
	closer := entry("20260101-110000-s-tac-cls", withKind(model.KindDone), withCloses(old.ID))
	first := entry("20260101-120000-s-tac-one", withKind(model.KindFact), withTopics("cli/view"), withFactIndex("First", "cli/view"))
	second := entry("20260101-130000-s-tac-two", withKind(model.KindFact), withTopics("cli/view"), withFactIndex("Second", "cli/view"))
	other := entry("20260101-140000-s-tac-oth", withKind(model.KindFact), withTopics("agent/ux"), withFactIndex("Other", "agent/ux"))
	unindexed := entry("20260101-150000-s-tac-off", withKind(model.KindFact), withTopics("cli/view"))
	graph := model.NewGraph([]*model.Entry{old, closer, first, second, other, unindexed})
	finder := New(Options{})

	result, err := finder.View(query.ViewQuery{Graph: graph, Layout: mustParseLayout(t, "indexed:as-list")})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := idsOf(result.Sections[0].Data.(model.FlatList).Entries), []string{old.ID, first.ID, second.ID, other.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("indexed population = %v, want %v", got, want)
	}

	result, err = finder.View(query.ViewQuery{Graph: graph, Layout: mustParseLayout(t, `active:indexed:topic("cli/view"):rank(by(date)):n(1):brief:as-list`)})
	if err != nil {
		t.Fatal(err)
	}
	if got := idsOf(result.Sections[0].Data.(model.FlatList).Entries); !reflect.DeepEqual(got, []string{second.ID}) {
		t.Fatalf("composed indexed population = %v", got)
	}

	for _, layout := range []string{"indexed:group(by(layer)):as-grouped", "active:indexed:as-counts"} {
		if _, err := finder.View(query.ViewQuery{Graph: graph, Layout: mustParseLayout(t, layout)}); err != nil {
			t.Errorf("%s: %v", layout, err)
		}
	}
}

func TestActiveIndexedMatchesDerivedFactIndex(t *testing.T) {
	active := entry("20260101-100000-s-tac-act", withKind(model.KindFact), withTopics("cli/view"), withFactIndex("Active", "cli/view"))
	second := entry("20260101-090000-s-tac-two", withKind(model.KindFact), withTopics("agent/ux"), withFactIndex("Second", "agent/ux"))
	retired := entry("20260101-110000-s-tac-ret", withKind(model.KindFact), withTopics("cli/view"), withFactIndex("Retired", "cli/view"))
	successor := entry("20260101-120000-s-tac-new", withKind(model.KindFact), withSupersedes(retired.ID))
	graph := model.NewGraph([]*model.Entry{active, second, retired, successor})

	result, err := New(Options{}).View(query.ViewQuery{Graph: graph, Layout: mustParseLayout(t, "active:indexed:as-list")})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := graph.IndexedFacts()
	if err != nil {
		t.Fatal(err)
	}
	want := make([]string, len(rows))
	for i, row := range rows {
		want[i] = row.ID
	}
	got := idsOf(result.Sections[0].Data.(model.FlatList).Entries)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("active:indexed IDs = %v, derived index IDs = %v", got, want)
	}
}

func TestViewIndexedRejectsArguments(t *testing.T) {
	_, err := New(Options{}).View(query.ViewQuery{Graph: model.NewGraph(nil), Layout: mustParseLayout(t, "indexed(topic):as-list")})
	if err == nil || !strings.Contains(err.Error(), "indexed takes no arguments") {
		t.Fatalf("indexed argument error = %v", err)
	}
}
