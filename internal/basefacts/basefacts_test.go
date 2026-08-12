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
// contract every base fact shares: a kind: fact signal, embedded, carrying no
// participants and no project refs — a ref may only point at another base
// fact, since project entries do not exist in the graphs a base fact ships to.
func factByID(t *testing.T, id string) *model.Entry {
	t.Helper()
	entries, err := Entries(testVocabulary())
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	baseIDs := make(map[string]bool, len(entries))
	for _, fact := range entries {
		baseIDs[fact.ID] = true
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
		for _, ref := range fact.Refs {
			if !baseIDs[ref.ID] {
				t.Errorf("base fact refs %s, which is not a base fact — project refs never ship embedded", ref.ID)
			}
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

// TestEntriesShipDoneKindFact covers the done authoring fact: reached from the
// capture lane rather than the pull-side index, so no index enrollment, and its
// mechanics block renders from the model declarations that enforce the rules.
func TestEntriesShipDoneKindFact(t *testing.T) {
	fact := factByID(t, DoneFactID)
	if fact.Index != nil {
		t.Errorf("done fact carries index enrollment %+v, want none — authoring facts are teased from the capture lane", fact.Index)
	}
	if fact.Summary == "" {
		t.Error("done fact has no summary; every reading surface needs one")
	}
	if !strings.Contains(fact.Content, "## Mechanics") {
		t.Error("done fact body missing the rendered mechanics block")
	}
	if !strings.Contains(fact.Content, model.DoneAnchorRequirement) {
		t.Error("done fact mechanics do not carry the declared anchor rule the validator enforces")
	}
	if strings.Contains(fact.Content, "{{") {
		t.Error("done fact body contains an unrendered template placeholder")
	}
}

// TestDoneFactBodyIsSelfContained holds the done fact to the same
// framework-generic standard as the principles fact: it ships to projects that
// hold none of this repository's source or graph.
func TestDoneFactBodyIsSelfContained(t *testing.T) {
	body := factByID(t, DoneFactID).Content

	for _, want := range []string{"# Recording completed work", "Say what happened, plainly", "The doing is part of the record", "One act, one done", "names what it completes", "Evidence follows the act", "not what was chosen", "Cover the whole commitment", "The baseline goes without saying", "Status is terminal; the loop is not", "shape of the work"} {
		if !strings.Contains(body, want) {
			t.Errorf("done fact body missing %q", want)
		}
	}
	if loc := entryIDPattern.FindString(body); loc != "" {
		t.Errorf("done fact cites project entry %q; a base fact carries no graph-local references", loc)
	}
	for _, host := range []string{"sdd ", "MCP", "CLI"} {
		if strings.Contains(body, host) {
			t.Errorf("done fact body contains host-specific reference %q", host)
		}
	}
}

// TestAllBaseFactsRenderAndValidate is the table test the composition layer is
// held to: every shipped base fact must pass full entry validation, so a
// template or frontmatter mistake fails the build here rather than surfacing
// at a reader's graph load. Validation runs against the base-fact set itself,
// since fact-to-fact refs (overview → authoring facts) must resolve.
func TestAllBaseFactsRenderAndValidate(t *testing.T) {
	entries, err := Entries(testVocabulary())
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no base facts shipped")
	}
	graph := model.NewGraph(entries)
	for _, fact := range entries {
		model.ValidateEntry(fact, graph)
		for _, w := range fact.Warnings {
			t.Errorf("base fact %s invalid: %s: %s", fact.ID, w.Field, w.Message)
		}
	}
}

// TestEntriesShipOverviewFact covers the type-system overview: the indexed
// introduction whose kind lists render from the model enumeration and which
// references every authoring fact.
func TestEntriesShipOverviewFact(t *testing.T) {
	fact := factByID(t, OverviewFactID)
	if fact.Index == nil || fact.Index.Topic.String() != "type-system/kinds" {
		t.Errorf("fact index = %+v, want enrollment under type-system/kinds", fact.Index)
	}
	if len(fact.Refs) == 0 || fact.Refs[0].ID != DoneFactID {
		t.Errorf("overview refs = %v, want the done authoring fact", fact.Refs)
	}
	for _, kind := range append(model.SignalKindValues(), model.DecisionKindValues()...) {
		if !strings.Contains(fact.Content, "`"+string(kind)+"`") {
			t.Errorf("overview body missing kind %q in its generated lists", kind)
		}
	}
	if strings.Contains(fact.Content, "{{") {
		t.Error("overview body contains an unrendered template placeholder")
	}
}

// TestEveryKindHasAQuestion pins the completeness rule: a kind declared in the
// model without an authored question fails here, not at a reader's graph load.
func TestEveryKindHasAQuestion(t *testing.T) {
	for _, kind := range append(model.SignalKindValues(), model.DecisionKindValues()...) {
		if q, ok := kindQuestions[kind]; !ok || q == "" {
			t.Errorf("kind %q has no question in kindQuestions", kind)
		}
	}
}

// TestOverviewBodyIsSelfContained holds the overview to the framework-generic
// standard, with the one sanctioned exception of base-fact IDs in frontmatter
// refs (the body itself stays ID-free).
func TestOverviewBodyIsSelfContained(t *testing.T) {
	body := factByID(t, OverviewFactID).Content

	for _, want := range []string{"# The type system", "Signal kinds", "Decision kinds", "force, not completion", "WHAT vs THAT", "Standing constraints are guiding directives", "outside vs here", "Retirement follows the same split", "layer", "This is the map, not the depth"} {
		if !strings.Contains(body, want) {
			t.Errorf("overview body missing %q", want)
		}
	}
	if loc := entryIDPattern.FindString(body); loc != "" {
		t.Errorf("overview body cites entry %q; pointers to authoring facts live in refs, not prose", loc)
	}
	for _, host := range []string{"sdd ", "MCP", "CLI"} {
		if strings.Contains(body, host) {
			t.Errorf("overview body contains host-specific reference %q", host)
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
