package basefacts

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/viewlayout"
)

func testVocabulary() viewlayout.Vocabulary {
	return viewlayout.Vocabulary{
		Functions:  []string{"active", "kind", "rank", "n", "as-list"},
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
}

// The body is generated from the supplied vocabulary and must stay host-
// neutral (d-cpt-476): no fact may name the sdd CLI or a command to run.
func TestViewGrammarBodyIsGeneratedAndHostNeutral(t *testing.T) {
	entries, err := Entries(testVocabulary())
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	body := entries[0].Content

	for _, want := range []string{"active", "heat", "exp-14d", "top(N)", "Grammar:"} {
		if !strings.Contains(body, want) {
			t.Errorf("generated body missing live-vocabulary token %q", want)
		}
	}
	for _, host := range []string{"sdd view", "Usage:", "--layout"} {
		if strings.Contains(body, host) {
			t.Errorf("base fact body contains host-specific reference %q", host)
		}
	}
}

// A base fact that is not a kind: fact signal is a build mistake and must
// fail construction rather than reach a graph.
func TestBuildRejectsNonFact(t *testing.T) {
	_, err := build("20260101-000000-d-cpt-xxx", "type: decision\nlayer: conceptual\nkind: directive\nconfidence: low\nintent: guiding\n", "body")
	if err == nil {
		t.Fatal("build accepted a non-fact base entry, want error")
	}
}
