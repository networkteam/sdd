package model

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAnnotationTopic_UnmarshalYAML_String(t *testing.T) {
	var topics []AnnotationTopic
	src := `
- catch-up-scaling
- infrastructure/cli
`
	if err := yaml.Unmarshal([]byte(src), &topics); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(topics) != 2 {
		t.Fatalf("want 2 topics, got %d", len(topics))
	}
	if topics[0].Label != "catch-up-scaling" {
		t.Fatalf("[0].Label: want 'catch-up-scaling', got %q", topics[0].Label)
	}
	if len(topics[0].Members) != 0 {
		t.Fatalf("[0].Members: want empty for plain string form, got %v", topics[0].Members)
	}
	if topics[1].Label != "infrastructure/cli" {
		t.Fatalf("[1].Label: want 'infrastructure/cli', got %q", topics[1].Label)
	}
}

func TestAnnotationTopic_UnmarshalYAML_Mapping(t *testing.T) {
	var topics []AnnotationTopic
	src := `
- label: type-system/kinds
  members:
    - 20260423-203503-d-cpt-ygn
    - 20260506-151849-d-tac-gvn
`
	if err := yaml.Unmarshal([]byte(src), &topics); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(topics) != 1 {
		t.Fatalf("want 1 topic, got %d", len(topics))
	}
	if topics[0].Label != "type-system/kinds" {
		t.Fatalf("Label: want 'type-system/kinds', got %q", topics[0].Label)
	}
	if len(topics[0].Members) != 2 {
		t.Fatalf("Members: want 2, got %v", topics[0].Members)
	}
	if topics[0].Members[0] != "20260423-203503-d-cpt-ygn" {
		t.Fatalf("Members[0]: want first ID, got %q", topics[0].Members[0])
	}
}

func TestAnnotationTopic_UnmarshalYAML_Mixed(t *testing.T) {
	var topics []AnnotationTopic
	src := `
- catch-up-scaling
- label: type-system/kinds
  members: [a, b]
- infrastructure/cli
`
	if err := yaml.Unmarshal([]byte(src), &topics); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(topics) != 3 {
		t.Fatalf("want 3 topics, got %d", len(topics))
	}
	if topics[0].Label != "catch-up-scaling" || len(topics[0].Members) != 0 {
		t.Fatalf("[0]: want plain 'catch-up-scaling', got %+v", topics[0])
	}
	if topics[1].Label != "type-system/kinds" || len(topics[1].Members) != 2 {
		t.Fatalf("[1]: want type-system with 2 members, got %+v", topics[1])
	}
	if topics[2].Label != "infrastructure/cli" || len(topics[2].Members) != 0 {
		t.Fatalf("[2]: want plain 'infrastructure/cli', got %+v", topics[2])
	}
}

func TestAnnotationTopic_UnmarshalYAML_MissingLabel(t *testing.T) {
	var topics []AnnotationTopic
	src := `
- members: [a, b]
`
	err := yaml.Unmarshal([]byte(src), &topics)
	if err == nil {
		t.Fatal("expected error for missing label")
	}
	if !strings.Contains(err.Error(), "label") {
		t.Fatalf("expected error about missing label, got %v", err)
	}
}

func TestAnnotationTopic_MarshalYAML_RoundTrip(t *testing.T) {
	in := []AnnotationTopic{
		{Label: "catch-up-scaling"},                                    // string form
		{Label: "type-system/kinds", Members: []string{"a", "b", "c"}}, // mapping form
	}
	out, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed []AnnotationTopic
	if err := yaml.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("re-unmarshal: %v\nyaml:\n%s", err, out)
	}
	if len(parsed) != 2 {
		t.Fatalf("want 2 after roundtrip, got %d", len(parsed))
	}
	if parsed[0].Label != "catch-up-scaling" || len(parsed[0].Members) != 0 {
		t.Fatalf("[0] after roundtrip: %+v\nyaml:\n%s", parsed[0], out)
	}
	if parsed[1].Label != "type-system/kinds" || len(parsed[1].Members) != 3 {
		t.Fatalf("[1] after roundtrip: %+v\nyaml:\n%s", parsed[1], out)
	}
}

func TestEntry_IsAnnotation(t *testing.T) {
	cases := []struct {
		typ  EntryType
		kind Kind
		want bool
	}{
		{TypeSignal, KindAnnotation, true},
		{TypeSignal, KindGap, false},
		{TypeDecision, KindAnnotation, false}, // wrong type
		{TypeDecision, KindFocus, false},
	}
	for _, c := range cases {
		e := &Entry{Type: c.typ, Kind: c.kind}
		if got := e.IsAnnotation(); got != c.want {
			t.Errorf("IsAnnotation for type=%s kind=%s: want %v, got %v", c.typ, c.kind, c.want, got)
		}
	}
}

func TestEntry_MembersFor(t *testing.T) {
	annotation := &Entry{
		Type: TypeSignal,
		Kind: KindAnnotation,
		Refs: []string{"a", "b", "c"},
	}

	t.Run("plain string applies to all refs", func(t *testing.T) {
		got := annotation.MembersFor(AnnotationTopic{Label: "foo"})
		if len(got) != 3 || got[0] != "a" || got[2] != "c" {
			t.Fatalf("want all refs [a b c], got %v", got)
		}
	})

	t.Run("explicit members override refs", func(t *testing.T) {
		got := annotation.MembersFor(AnnotationTopic{Label: "foo", Members: []string{"b", "c"}})
		if len(got) != 2 || got[0] != "b" {
			t.Fatalf("want [b c], got %v", got)
		}
	})
}
