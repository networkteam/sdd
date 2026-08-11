package finders

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// TestView_AsBodies_SelectsThroughTheOrdinaryGrammar checks the render adds no
// selection vocabulary of its own: filters pick the entries, and the section
// hands them over in pipeline order.
func TestView_AsBodies_SelectsThroughTheOrdinaryGrammar(t *testing.T) {
	principle := entry("20260101-100000-s-prc-aaa",
		withKind(model.KindFact),
		withTopics("principles/interactive"),
		withContent("# The posture\n\nGoal first.\n"))
	other := entry("20260101-110000-s-prc-bbb",
		withKind(model.KindFact),
		withTopics("cli/view"),
		withContent("# Grammar\n\nColon-chained.\n"))
	g := model.NewGraph([]*model.Entry{principle, other})

	result, err := New(Options{}).OnGraph(g).View(query.ViewQuery{
		Layout: mustParseLayout(t, `active:kind(fact):topic("principles/interactive"):as-bodies:name("Working principles")`),
	})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if len(result.Sections) != 1 {
		t.Fatalf("expected one section, got %d", len(result.Sections))
	}
	section := result.Sections[0]
	if section.Render != "as-bodies" || section.Name != "Working principles" {
		t.Fatalf("unexpected section shape: render=%q name=%q", section.Render, section.Name)
	}
	bodies, ok := section.Data.(model.Bodies)
	if !ok {
		t.Fatalf("as-bodies must produce model.Bodies, got %T", section.Data)
	}
	if len(bodies.Entries) != 1 || bodies.Entries[0].ID != principle.ID {
		t.Fatalf("topic filter should select only the principles fact, got %+v", bodies.Entries)
	}
}

// TestView_AsBodies_EmptyMatchProducesAnEmptySection keeps the framing-lane
// contract: nothing selected means nothing to serve, which the presenter turns
// into no block at all.
func TestView_AsBodies_EmptyMatchProducesAnEmptySection(t *testing.T) {
	g := model.NewGraph(nil)
	result, err := New(Options{}).OnGraph(g).View(query.ViewQuery{
		Layout: mustParseLayout(t, `active:kind(fact):topic("principles/interactive"):as-bodies:name("Working principles")`),
	})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if got := result.Sections[0].Data.Count(); got != 0 {
		t.Fatalf("expected an empty section, got count %d", got)
	}
}

func TestView_AsBodies_RejectsShapesItCannotConsume(t *testing.T) {
	tests := []struct {
		name   string
		layout string
		want   []string
	}{
		{
			name:   "grouped",
			layout: "kind(fact):group(by(layer)):as-bodies",
			want:   []string{"as-bodies", "grouped"},
		},
		{
			name:   "focus involvement",
			layout: "kind(focus):expand(involvement):as-bodies",
			want:   []string{"as-bodies", "focus-block"},
		},
		{
			name:   "ref expansion",
			layout: "kind(fact):expand(refs):as-bodies",
			want:   []string{"as-bodies", "flat-list"},
		},
		{
			name:   "brief",
			layout: "kind(fact):brief:as-bodies",
			want:   []string{"as-bodies", "brief"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(Options{}).OnGraph(model.NewGraph(nil)).View(query.ViewQuery{
				Layout: mustParseLayout(t, tc.layout),
			})
			if err == nil {
				t.Fatalf("expected %s to be rejected, got nil", tc.layout)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q must mention %q", err.Error(), want)
				}
			}
		})
	}
}

// TestView_AsBodies_ComposesWithRankAndPage is what keeps the render free of its
// own selection vocabulary: order and cap come from the grammar.
func TestView_AsBodies_ComposesWithRankAndPage(t *testing.T) {
	first := entry("20260101-100000-s-prc-aaa", withKind(model.KindFact), withContent("# A\n"))
	second := entry("20260102-100000-s-prc-bbb", withKind(model.KindFact), withContent("# B\n"))
	g := model.NewGraph([]*model.Entry{first, second})

	result, err := New(Options{}).OnGraph(g).View(query.ViewQuery{
		Layout: mustParseLayout(t, "kind(fact):rank(by(date)):n(1):as-bodies"),
	})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	bodies := result.Sections[0].Data.(model.Bodies)
	if len(bodies.Entries) != 1 || bodies.Entries[0].ID != second.ID {
		t.Fatalf("rank(by(date)):n(1) should keep the newest fact alone, got %+v", bodies.Entries)
	}
}
