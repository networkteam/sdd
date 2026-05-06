package finders

import (
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func mustParse(t *testing.T, s string) model.TopicPath {
	t.Helper()
	p, err := model.ParseTopicPath(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return p
}

func TestTopicFilter_InlineMatching(t *testing.T) {
	a := &model.Entry{
		ID:     "20260101-000000-s-cpt-aaa",
		Type:   model.TypeSignal,
		Kind:   model.KindGap,
		Topics: []model.TopicPath{mustParse(t, "Catch-Up")},
	}
	b := &model.Entry{
		ID:     "20260101-000000-s-cpt-bbb",
		Type:   model.TypeSignal,
		Kind:   model.KindGap,
		Topics: []model.TopicPath{mustParse(t, "infrastructure/cli")},
	}
	c := &model.Entry{
		ID:   "20260101-000000-s-cpt-ccc",
		Type: model.TypeSignal,
		Kind: model.KindGap,
	}
	g := model.NewGraph([]*model.Entry{a, b, c})

	t.Run("matches case-insensitive prefix", func(t *testing.T) {
		f := TopicFilter{Prefix: mustParse(t, "catch-up")}
		got := f.FilterEntries(g, g.Entries)
		if len(got) != 1 || got[0].ID != a.ID {
			t.Fatalf("want a, got %v", got)
		}
	})

	t.Run("matches deeper prefix component-wise", func(t *testing.T) {
		f := TopicFilter{Prefix: mustParse(t, "Infrastructure")}
		got := f.FilterEntries(g, g.Entries)
		if len(got) != 1 || got[0].ID != b.ID {
			t.Fatalf("want b, got %v", got)
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		f := TopicFilter{Prefix: mustParse(t, "Nope")}
		got := f.FilterEntries(g, g.Entries)
		if len(got) != 0 {
			t.Fatalf("want empty, got %v", got)
		}
	})
}

func TestTopicFilter_AnnotationMembership(t *testing.T) {
	a := &model.Entry{ID: "20260101-000000-s-cpt-aaa", Type: model.TypeSignal, Kind: model.KindGap}
	b := &model.Entry{ID: "20260101-000000-s-cpt-bbb", Type: model.TypeSignal, Kind: model.KindGap}
	c := &model.Entry{ID: "20260101-000000-s-cpt-ccc", Type: model.TypeSignal, Kind: model.KindGap}

	// Annotation tags a, b, c with "type-system/kinds" via plain string topic
	// (applies to all refs).
	ann1 := &model.Entry{
		ID:               "20260506-000000-s-cpt-an1",
		Type:             model.TypeSignal,
		Kind:             model.KindAnnotation,
		Refs:             []string{a.ID, b.ID, c.ID},
		AnnotationTopics: []model.AnnotationTopic{{Label: "type-system/kinds"}},
	}

	// Second annotation tags only a and b with "catch-up-scaling" via explicit members.
	ann2 := &model.Entry{
		ID:   "20260506-000001-s-cpt-an2",
		Type: model.TypeSignal,
		Kind: model.KindAnnotation,
		Refs: []string{a.ID, b.ID, c.ID},
		AnnotationTopics: []model.AnnotationTopic{
			{Label: "catch-up-scaling", Members: []string{a.ID, b.ID}},
		},
	}
	g := model.NewGraph([]*model.Entry{a, b, c, ann1, ann2})

	t.Run("plain-string topic applies to all refs", func(t *testing.T) {
		f := TopicFilter{Prefix: mustParse(t, "type-system")}
		got := f.FilterEntries(g, []*model.Entry{a, b, c})
		if len(got) != 3 {
			t.Fatalf("want all 3, got %v", got)
		}
	})

	t.Run("explicit members restrict membership", func(t *testing.T) {
		f := TopicFilter{Prefix: mustParse(t, "catch-up-scaling")}
		got := f.FilterEntries(g, []*model.Entry{a, b, c})
		if len(got) != 2 {
			t.Fatalf("want 2 (a, b), got %v", got)
		}
		ids := map[string]bool{got[0].ID: true, got[1].ID: true}
		if !ids[a.ID] || !ids[b.ID] {
			t.Fatalf("want a and b, got %v", got)
		}
	})

	t.Run("annotations themselves are not matched by their own topics", func(t *testing.T) {
		// Annotations tag refs, not themselves — only the targeted entries
		// match the filter, not the annotation entry.
		f := TopicFilter{Prefix: mustParse(t, "type-system")}
		got := f.FilterEntries(g, g.Entries)
		for _, e := range got {
			if e.IsAnnotation() {
				t.Fatalf("annotation %s should not appear in filter result, got %v", e.ID, got)
			}
		}
	})
}

func TestEffectiveTopics_MergesInlineAndAnnotation(t *testing.T) {
	target := &model.Entry{
		ID:     "20260101-000000-s-cpt-aaa",
		Type:   model.TypeSignal,
		Kind:   model.KindGap,
		Topics: []model.TopicPath{mustParse(t, "Inline-Topic")},
	}
	ann := &model.Entry{
		ID:   "20260506-000000-s-cpt-ann",
		Type: model.TypeSignal,
		Kind: model.KindAnnotation,
		Refs: []string{target.ID},
		AnnotationTopics: []model.AnnotationTopic{
			{Label: "annotation-topic"},
			{Label: "Inline-Topic"}, // duplicate of inline; should fold-dedupe
		},
	}
	g := model.NewGraph([]*model.Entry{target, ann})

	got := g.EffectiveTopics(target)
	if len(got) != 2 {
		t.Fatalf("want 2 deduped topics, got %d (%v)", len(got), got)
	}
	// First-seen casing wins (inline came first → preserves "Inline-Topic")
	labels := []string{got[0].String(), got[1].String()}
	have := map[string]bool{}
	for _, l := range labels {
		have[l] = true
	}
	if !have["Inline-Topic"] || !have["annotation-topic"] {
		t.Fatalf("want {Inline-Topic, annotation-topic}, got %v", labels)
	}
}
