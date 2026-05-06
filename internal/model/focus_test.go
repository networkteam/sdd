package model

import (
	"strings"
	"testing"
)

func TestEntry_IsFocus(t *testing.T) {
	cases := []struct {
		typ  EntryType
		kind Kind
		want bool
	}{
		{TypeDecision, KindFocus, true},
		{TypeDecision, KindDirective, false},
		{TypeSignal, KindFocus, false}, // wrong type
		{TypeSignal, KindAnnotation, false},
	}
	for _, c := range cases {
		e := &Entry{Type: c.typ, Kind: c.kind}
		if got := e.IsFocus(); got != c.want {
			t.Errorf("IsFocus for type=%s kind=%s: want %v, got %v", c.typ, c.kind, c.want, got)
		}
	}
}

func TestFocusWhen_Validate(t *testing.T) {
	cases := []struct {
		name    string
		w       *FocusWhen
		wantErr string
	}{
		{name: "nil ok", w: nil, wantErr: ""},
		{name: "from only", w: &FocusWhen{From: "2026-05-06"}, wantErr: ""},
		{name: "to only", w: &FocusWhen{To: "2026-05-20"}, wantErr: ""},
		{name: "both", w: &FocusWhen{From: "2026-05-06", To: "2026-05-20"}, wantErr: ""},
		{name: "both empty", w: &FocusWhen{}, wantErr: "at least one"},
		{name: "bad from", w: &FocusWhen{From: "06.05.2026"}, wantErr: "from"},
		{name: "bad to", w: &FocusWhen{From: "2026-05-06", To: "next-week"}, wantErr: "to"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.w.Validate()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("expected error containing %q, got %v", c.wantErr, err)
			}
		})
	}
}

func TestEntry_ResolveActors(t *testing.T) {
	focus := &Entry{
		Type:        TypeDecision,
		Kind:        KindFocus,
		FocusActors: []string{"Christopher", "Claude"},
	}

	t.Run("inherit when actors not set", func(t *testing.T) {
		got := focus.ResolveActors(Involvement{Target: "x"})
		if len(got) != 2 || got[0] != "Christopher" {
			t.Fatalf("want focus-level default, got %v", got)
		}
	})

	t.Run("override with explicit empty (pull-available)", func(t *testing.T) {
		got := focus.ResolveActors(Involvement{Target: "x", ActorsSet: true, Actors: nil})
		if len(got) != 0 {
			t.Fatalf("want empty (pull-available), got %v", got)
		}
	})

	t.Run("override with non-empty list", func(t *testing.T) {
		got := focus.ResolveActors(Involvement{Target: "x", ActorsSet: true, Actors: []string{"Claude"}})
		if len(got) != 1 || got[0] != "Claude" {
			t.Fatalf("want [Claude], got %v", got)
		}
	})
}

func TestEntry_ResolveWhen(t *testing.T) {
	defaultWhen := &FocusWhen{From: "2026-05-06", To: "2026-05-20"}
	focus := &Entry{
		Type:      TypeDecision,
		Kind:      KindFocus,
		FocusWhen: defaultWhen,
	}

	t.Run("inherit when not set", func(t *testing.T) {
		got := focus.ResolveWhen(Involvement{Target: "x"})
		if got != defaultWhen {
			t.Fatalf("want default focus-level when, got %+v", got)
		}
	})

	t.Run("override with per-involvement", func(t *testing.T) {
		over := &FocusWhen{To: "2026-05-13"}
		got := focus.ResolveWhen(Involvement{Target: "x", When: over})
		if got != over {
			t.Fatalf("want override, got %+v", got)
		}
	})
}

func TestParseEntry_Focus_FrontmatterRoundtrip(t *testing.T) {
	src := `---
type: decision
layer: cpt
kind: focus
participants: [Christopher, Claude]
actors: [Christopher, Claude]
when:
  from: 2026-05-06
  to: 2026-05-20
involvement:
  - target: 20260505-215340-s-cpt-rwd
  - target: 20260505-215333-s-cpt-jq7
    actors: []
  - target: 20260417-120309-s-tac-93s
    actors: [Claude]
    when:
      from: 2026-05-06
      to: 2026-05-13
---
Focus body.`
	e, err := ParseEntry("20260506-160000-d-cpt-foc.md", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !e.IsFocus() {
		t.Fatalf("not detected as focus")
	}
	if len(e.FocusActors) != 2 || e.FocusActors[0] != "Christopher" {
		t.Fatalf("FocusActors: %v", e.FocusActors)
	}
	if e.FocusWhen == nil || e.FocusWhen.From != "2026-05-06" || e.FocusWhen.To != "2026-05-20" {
		t.Fatalf("FocusWhen: %+v", e.FocusWhen)
	}
	if len(e.Involvement) != 3 {
		t.Fatalf("involvement count: want 3, got %d", len(e.Involvement))
	}

	// First triple: actors not set → ActorsSet false (inherits focus default)
	if e.Involvement[0].ActorsSet {
		t.Fatalf("[0]: actors not given but ActorsSet=true")
	}
	// Second triple: explicit empty → ActorsSet true with empty Actors
	if !e.Involvement[1].ActorsSet {
		t.Fatalf("[1]: explicit empty actors but ActorsSet=false")
	}
	if len(e.Involvement[1].Actors) != 0 {
		t.Fatalf("[1]: want empty actors, got %v", e.Involvement[1].Actors)
	}
	// Third triple: explicit list + per-involvement when
	if !e.Involvement[2].ActorsSet || len(e.Involvement[2].Actors) != 1 || e.Involvement[2].Actors[0] != "Claude" {
		t.Fatalf("[2].Actors: %+v", e.Involvement[2])
	}
	if e.Involvement[2].When == nil || e.Involvement[2].When.To != "2026-05-13" {
		t.Fatalf("[2].When: %+v", e.Involvement[2].When)
	}

	// Roundtrip the frontmatter and make sure all fields survive.
	emitted := FormatFrontmatter(e)
	e2, err := ParseEntry("20260506-160000-d-cpt-foc.md", emitted+"Focus body.")
	if err != nil {
		t.Fatalf("re-parse: %v\nemitted:\n%s", err, emitted)
	}
	if len(e2.Involvement) != 3 {
		t.Fatalf("roundtrip involvement count: want 3, got %d\nemitted:\n%s", len(e2.Involvement), emitted)
	}
	if !e2.Involvement[1].ActorsSet || len(e2.Involvement[1].Actors) != 0 {
		t.Fatalf("roundtrip [1]: pull-available distinction lost; got %+v\nemitted:\n%s", e2.Involvement[1], emitted)
	}
}

func TestParseEntry_AnnotationTopics(t *testing.T) {
	src := `---
type: signal
layer: cpt
kind: annotation
refs:
  - 20260505-215340-s-cpt-rwd
  - 20260505-215333-s-cpt-jq7
  - 20260423-203503-d-cpt-ygn
topics:
  - catch-up-scaling
  - label: type-system/kinds
    members:
      - 20260423-203503-d-cpt-ygn
---
Annotation body.`
	e, err := ParseEntry("20260506-161000-s-cpt-ann.md", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !e.IsAnnotation() {
		t.Fatalf("not detected as annotation")
	}
	if len(e.AnnotationTopics) != 2 {
		t.Fatalf("AnnotationTopics: want 2, got %d", len(e.AnnotationTopics))
	}
	if len(e.Topics) != 0 {
		t.Fatalf("annotation should not populate inline Topics, got %v", e.Topics)
	}
	if e.AnnotationTopics[0].Label != "catch-up-scaling" || len(e.AnnotationTopics[0].Members) != 0 {
		t.Fatalf("[0]: want plain catch-up-scaling, got %+v", e.AnnotationTopics[0])
	}
	if e.AnnotationTopics[1].Label != "type-system/kinds" || len(e.AnnotationTopics[1].Members) != 1 {
		t.Fatalf("[1]: want type-system/kinds with 1 member, got %+v", e.AnnotationTopics[1])
	}

	// Plain-string topic resolves to all refs via MembersFor
	plainMembers := e.MembersFor(e.AnnotationTopics[0])
	if len(plainMembers) != 3 {
		t.Fatalf("plain topic MembersFor: want all 3 refs, got %v", plainMembers)
	}
	// Mapping topic resolves to its explicit members
	mappingMembers := e.MembersFor(e.AnnotationTopics[1])
	if len(mappingMembers) != 1 || mappingMembers[0] != "20260423-203503-d-cpt-ygn" {
		t.Fatalf("mapping topic MembersFor: %v", mappingMembers)
	}
}

func TestParseEntry_InlineTopics_NonAnnotation(t *testing.T) {
	src := `---
type: signal
layer: cpt
kind: insight
participants: [Christopher]
topics:
  - catch-up-scaling
  - infrastructure/cli
---
Body text.`
	e, err := ParseEntry("20260506-162000-s-cpt-ins.md", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.IsAnnotation() {
		t.Fatalf("should not be detected as annotation")
	}
	if len(e.AnnotationTopics) != 0 {
		t.Fatalf("AnnotationTopics should be empty on non-annotation, got %v", e.AnnotationTopics)
	}
	if len(e.Topics) != 2 {
		t.Fatalf("Topics: want 2, got %d", len(e.Topics))
	}
	if e.Topics[0].String() != "catch-up-scaling" || e.Topics[1].String() != "infrastructure/cli" {
		t.Fatalf("Topics values: %v", e.Topics)
	}
}

func TestValidate_Annotation(t *testing.T) {
	target1 := &Entry{ID: "20260101-000000-s-cpt-aaa", Type: TypeSignal, Kind: KindGap}
	target2 := &Entry{ID: "20260101-000000-s-cpt-bbb", Type: TypeSignal, Kind: KindGap}
	g := NewGraph([]*Entry{target1, target2})

	t.Run("ok with all-refs string topic", func(t *testing.T) {
		e := &Entry{
			ID:               "20260506-000000-s-cpt-ann",
			Type:             TypeSignal,
			Kind:             KindAnnotation,
			Refs:             []string{target1.ID, target2.ID},
			AnnotationTopics: []AnnotationTopic{{Label: "catch-up-scaling"}},
		}
		ValidateEntry(e, g)
		if len(e.Warnings) != 0 {
			t.Fatalf("unexpected warnings: %v", e.Warnings)
		}
	})

	t.Run("ok with explicit subset members", func(t *testing.T) {
		e := &Entry{
			ID:   "20260506-000000-s-cpt-ann",
			Type: TypeSignal,
			Kind: KindAnnotation,
			Refs: []string{target1.ID, target2.ID},
			AnnotationTopics: []AnnotationTopic{
				{Label: "x", Members: []string{target1.ID}},
			},
		}
		ValidateEntry(e, g)
		if len(e.Warnings) != 0 {
			t.Fatalf("unexpected warnings: %v", e.Warnings)
		}
	})

	t.Run("flags missing refs", func(t *testing.T) {
		e := &Entry{
			ID:               "20260506-000000-s-cpt-ann",
			Type:             TypeSignal,
			Kind:             KindAnnotation,
			AnnotationTopics: []AnnotationTopic{{Label: "x"}},
		}
		ValidateEntry(e, g)
		if !hasWarningField(e, "refs") {
			t.Fatalf("expected refs warning, got %v", e.Warnings)
		}
	})

	t.Run("flags missing topics", func(t *testing.T) {
		e := &Entry{
			ID:   "20260506-000000-s-cpt-ann",
			Type: TypeSignal,
			Kind: KindAnnotation,
			Refs: []string{target1.ID},
		}
		ValidateEntry(e, g)
		if !hasWarningField(e, "topics") {
			t.Fatalf("expected topics warning, got %v", e.Warnings)
		}
	})

	t.Run("flags member outside refs", func(t *testing.T) {
		e := &Entry{
			ID:   "20260506-000000-s-cpt-ann",
			Type: TypeSignal,
			Kind: KindAnnotation,
			Refs: []string{target1.ID},
			AnnotationTopics: []AnnotationTopic{
				{Label: "x", Members: []string{target2.ID}}, // not in refs
			},
		}
		ValidateEntry(e, g)
		found := false
		for _, w := range e.Warnings {
			if w.Field == "topics" && strings.Contains(w.Message, "subset") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected subset warning, got %v", e.Warnings)
		}
	})

	t.Run("flags malformed label", func(t *testing.T) {
		e := &Entry{
			ID:   "20260506-000000-s-cpt-ann",
			Type: TypeSignal,
			Kind: KindAnnotation,
			Refs: []string{target1.ID},
			AnnotationTopics: []AnnotationTopic{
				{Label: "has spaces"},
			},
		}
		ValidateEntry(e, g)
		if !hasWarningField(e, "topics") {
			t.Fatalf("expected topics warning for malformed label, got %v", e.Warnings)
		}
	})
}

func TestValidate_Focus(t *testing.T) {
	target := &Entry{ID: "20260101-000000-s-cpt-aaa", Type: TypeSignal, Kind: KindGap}
	g := NewGraph([]*Entry{target})

	t.Run("ok minimal focus", func(t *testing.T) {
		e := &Entry{
			ID:    "20260506-000000-d-tac-foc",
			Type:  TypeDecision,
			Kind:  KindFocus,
			Layer: LayerTactical,
			Involvement: []Involvement{
				{Target: target.ID},
			},
		}
		ValidateEntry(e, g)
		if len(e.Warnings) != 0 {
			t.Fatalf("unexpected warnings: %v", e.Warnings)
		}
	})

	t.Run("ok with top-level defaults", func(t *testing.T) {
		e := &Entry{
			ID:          "20260506-000000-d-tac-foc",
			Type:        TypeDecision,
			Kind:        KindFocus,
			Layer:       LayerTactical,
			FocusActors: []string{"Christopher", "Claude"},
			FocusWhen:   &FocusWhen{From: "2026-05-06", To: "2026-05-20"},
			Involvement: []Involvement{
				{Target: target.ID},
				{Target: target.ID, ActorsSet: true, Actors: nil}, // pull-available
				{Target: target.ID, ActorsSet: true, Actors: []string{"Claude"},
					When: &FocusWhen{To: "2026-05-13"}},
			},
		}
		ValidateEntry(e, g)
		if len(e.Warnings) != 0 {
			t.Fatalf("unexpected warnings: %v", e.Warnings)
		}
	})

	t.Run("flags missing involvement", func(t *testing.T) {
		e := &Entry{
			ID:    "20260506-000000-d-tac-foc",
			Type:  TypeDecision,
			Kind:  KindFocus,
			Layer: LayerTactical,
		}
		ValidateEntry(e, g)
		if !hasWarningField(e, "involvement") {
			t.Fatalf("expected involvement warning, got %v", e.Warnings)
		}
	})

	t.Run("flags involvement target missing in graph", func(t *testing.T) {
		e := &Entry{
			ID:    "20260506-000000-d-tac-foc",
			Type:  TypeDecision,
			Kind:  KindFocus,
			Layer: LayerTactical,
			Involvement: []Involvement{
				{Target: "20260101-000000-s-cpt-zzz"},
			},
		}
		ValidateEntry(e, g)
		found := false
		for _, w := range e.Warnings {
			if w.Field == "involvement" && strings.Contains(w.Message, "does not resolve") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected unresolved-target warning, got %v", e.Warnings)
		}
	})

	t.Run("flags malformed when", func(t *testing.T) {
		e := &Entry{
			ID:        "20260506-000000-d-tac-foc",
			Type:      TypeDecision,
			Kind:      KindFocus,
			Layer:     LayerTactical,
			FocusWhen: &FocusWhen{From: "next-month"},
			Involvement: []Involvement{
				{Target: target.ID},
			},
		}
		ValidateEntry(e, g)
		if !hasWarningField(e, "when") {
			t.Fatalf("expected when warning, got %v", e.Warnings)
		}
	})
}

func hasWarningField(e *Entry, field string) bool {
	for _, w := range e.Warnings {
		if w.Field == field {
			return true
		}
	}
	return false
}

func TestParseEntry_InlineTopics_RejectsMembersForm(t *testing.T) {
	// Non-annotation entry with an object-form topic item should warn (not error
	// — preserving the load-permissive contract) and skip the offending item.
	src := `---
type: signal
layer: cpt
kind: insight
topics:
  - foo
  - label: bar
    members: [a, b]
---
Body.`
	e, err := ParseEntry("20260506-163000-s-cpt-bad.md", src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(e.Topics) != 1 {
		t.Fatalf("only valid topic should survive; got %d (%v)", len(e.Topics), e.Topics)
	}
	if e.Topics[0].String() != "foo" {
		t.Fatalf("want 'foo', got %q", e.Topics[0].String())
	}
	if len(e.Warnings) == 0 {
		t.Fatalf("expected a warning about the inline-with-members entry")
	}
}
