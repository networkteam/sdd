package basefacts

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/viewlayout"
)

func testVocabulary() viewlayout.Vocabulary {
	return viewlayout.Vocabulary{
		Functions:  []string{"active", "indexed", "kind", "rank", "n", "as-list"},
		Renders:    []string{"as-list"},
		Algorithms: []string{"heat"},
		Decays:     []string{"exp-14d"},
		Macros:     []string{"top"},
	}
}

func TestEntriesShipViewGrammarFact(t *testing.T) {
	entries, err := Entries(testVocabulary())
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d base facts, want 1", len(entries))
	}

	fact := entries[0]
	if fact.ID != ViewGrammarFactID {
		t.Errorf("fact ID = %q, want stable %q", fact.ID, ViewGrammarFactID)
	}
	if fact.Type != model.TypeSignal || fact.Kind != model.KindFact {
		t.Errorf("fact is %s %s, want signal fact", fact.Type, fact.Kind)
	}
	if !fact.Embedded {
		t.Error("fact is not marked Embedded")
	}
	if len(fact.Participants) != 0 {
		t.Errorf("base fact carries participants %v, want none", fact.Participants)
	}
	if len(fact.Refs) != 0 {
		t.Errorf("base fact carries refs %v, want none", fact.Refs)
	}
	const title = "How to compose graph views (view tool): layout grammar, filters, ranking, quoting, and examples"
	if fact.Index == nil || fact.Index.Title != title || fact.Index.Topic.String() != "cli/view" {
		t.Errorf("fact index = %+v", fact.Index)
	}
}

func TestViewGrammarBodyIsGeneratedAndHostNeutral(t *testing.T) {
	entries, err := Entries(testVocabulary())
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	body := entries[0].Content

	for _, want := range []string{"# How to compose graph views", "## Grammar", "```text", "| Category | Syntax | Meaning |", "active", "heat", "exp-14d", "top(N)", "active:indexed:as-list"} {
		if !strings.Contains(body, want) {
			t.Errorf("generated body missing live-vocabulary token %q", want)
		}
	}
	for _, host := range []string{"sdd view", "Usage:", "--layout", "MCP", "debugging"} {
		if strings.Contains(body, host) {
			t.Errorf("base fact body contains host-specific reference %q", host)
		}
	}
}

func TestBuildRejectsNonFact(t *testing.T) {
	_, err := build("20260101-000000-d-cpt-xxx", "type: decision\nlayer: conceptual\nkind: directive\nconfidence: low\nintent: guiding\n", "body")
	if err == nil {
		t.Fatal("build accepted a non-fact base entry, want error")
	}
}

func TestBuildRejectsInvalidFactIndexEnrollment(t *testing.T) {
	frontmatter := "type: signal\nlayer: process\nkind: fact\ntopics: [cli/view]\nindex: {title: Reference, topic: agent/ux}\n"
	_, err := build("20260101-000000-s-prc-idx", frontmatter, "body")
	if err == nil || !strings.Contains(err.Error(), "must also appear in topics") {
		t.Fatalf("build error = %v", err)
	}
}
