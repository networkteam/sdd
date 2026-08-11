package basefacts

import (
	"regexp"
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

// factByID returns the shipped base fact with the given ID, asserting the
// contract every base fact shares: a kind: fact signal, embedded, carrying
// neither participants nor project refs.
func factByID(t *testing.T, id string) *model.Entry {
	t.Helper()
	entries, err := Entries(testVocabulary())
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	for _, fact := range entries {
		if fact.ID != id {
			continue
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
		return fact
	}
	t.Fatalf("no base fact shipped with stable ID %q", id)
	return nil
}

func TestEntriesShipViewGrammarFact(t *testing.T) {
	fact := factByID(t, ViewGrammarFactID)
	const title = "How to compose graph views (view tool): layout grammar, filters, ranking, quoting, and examples"
	if fact.Index == nil || fact.Index.Title != title || fact.Index.Topic.String() != "cli/view" {
		t.Errorf("fact index = %+v", fact.Index)
	}
}

// TestEntriesShipPrinciplesFact covers the fact the session shell primes with:
// selectable by topic, and deliberately absent from the pull-side index, since
// its words are pushed in full at every session open.
func TestEntriesShipPrinciplesFact(t *testing.T) {
	fact := factByID(t, PrinciplesFactID)
	if len(fact.Topics) != 1 || fact.Topics[0].String() != "principles/interactive" {
		t.Errorf("topics = %v, want the single selector topic principles/interactive", fact.Topics)
	}
	if fact.Index != nil {
		t.Errorf("principles fact carries index enrollment %+v, want none — it is pushed, not pulled", fact.Index)
	}
	if fact.Summary == "" {
		t.Error("principles fact has no summary; every reading surface needs one")
	}
}

// TestPrinciplesBodyIsSelfContained keeps the served words framework-generic:
// the posture must read for a project SDD knows nothing about, so it cites no
// entry of the graph it was drawn from and names no host tool.
func TestPrinciplesBodyIsSelfContained(t *testing.T) {
	body := factByID(t, PrinciplesFactID).Content

	for _, want := range []string{"# The way of thinking", "Goal first", "Misfit is a signal", "prepares the dialogue", "A correction is a contradiction too", "Expect novelty", "step up"} {
		if !strings.Contains(body, want) {
			t.Errorf("served posture missing %q", want)
		}
	}
	if loc := entryIDPattern.FindString(body); loc != "" {
		t.Errorf("served posture cites project entry %q; a base fact carries no graph-local references", loc)
	}
	for _, host := range []string{"sdd ", "MCP", "CLI"} {
		if strings.Contains(body, host) {
			t.Errorf("served posture contains host-specific reference %q", host)
		}
	}
}

// entryIDPattern matches a full graph entry ID — what a base fact's prose must
// never carry, since the entries of the graph it was authored in do not exist
// in the graphs it ships to.
var entryIDPattern = regexp.MustCompile(`\d{8}-\d{6}-[sd]-(stg|cpt|tac|ops|prc)-[a-z0-9]+`)

func TestViewGrammarBodyIsGeneratedAndHostNeutral(t *testing.T) {
	body := factByID(t, ViewGrammarFactID).Content

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
