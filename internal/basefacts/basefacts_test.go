package basefacts

import (
	"regexp"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/engine"
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

// TestEntriesShipProcedureKindFact covers the procedure authoring fact: like
// the done fact it is teased from the capture lane (no index enrollment), and
// its generated blocks render from the declarations that enforce them — the
// model's class enumeration and the engine's domain-type vocabulary.
func TestEntriesShipProcedureKindFact(t *testing.T) {
	fact := factByID(t, ProcedureFactID)
	if fact.Index != nil {
		t.Errorf("procedure fact carries index enrollment %+v, want none — authoring facts are teased from the capture lane", fact.Index)
	}
	if fact.Summary == "" {
		t.Error("procedure fact has no summary; every reading surface needs one")
	}
	if !strings.Contains(fact.Content, "## Mechanics") {
		t.Error("procedure fact body missing the rendered mechanics block")
	}
	for _, class := range model.ProcedureClassValues() {
		if !strings.Contains(fact.Content, "`"+string(class)+"` — "+class.Description()) {
			t.Errorf("procedure fact body missing class %q with its declared description", class)
		}
	}
	if len(fact.Refs) == 0 || fact.Refs[0].ID != ProcedureSpecFactID {
		t.Errorf("procedure fact refs = %v, want the spec reference fact", fact.Refs)
	}
	if strings.Contains(fact.Content, "{{") {
		t.Error("procedure fact body contains an unrendered template placeholder")
	}
}

// TestEntriesShipProcedureSpecFact covers the spec reference — the indexed
// how-to-write-it fact beside the kind's authoring fact. Its variable types
// and end targets render from the engine declarations; the ability inventory
// is deliberately a pull cue for the live registry, never a baked list.
func TestEntriesShipProcedureSpecFact(t *testing.T) {
	fact := factByID(t, ProcedureSpecFactID)
	if fact.Index != nil {
		t.Errorf("spec reference carries index enrollment %+v, want none — it is reached through the procedure fact and lane teasers", fact.Index)
	}
	for _, baseType := range engine.BaseTypeValues() {
		if !strings.Contains(fact.Content, string(baseType)) {
			t.Errorf("spec reference missing domain type %q", baseType)
		}
	}
	for _, want := range []string{"# Writing a procedure spec", "`params`", "`state`", "`steps`", "end(completed)", "end(abandoned)", "## A worked example", "```yaml", "registry", "## unit:", "## Dispatching another procedure"} {
		if !strings.Contains(fact.Content, want) {
			t.Errorf("spec reference missing %q", want)
		}
	}
	if !strings.Contains(fact.Content, engine.ExampleSpecFrontmatter) {
		t.Error("spec reference does not embed the engine's worked example verbatim")
	}
	for _, pair := range engine.PresencePairs() {
		if !strings.Contains(fact.Content, "`"+pair.Field+"` — checked by `"+pair.Predicate+"`") {
			t.Errorf("spec reference missing gateable pair %s/%s", pair.Field, pair.Predicate)
		}
	}
	if len(fact.Refs) == 0 || fact.Refs[0].ID != ProcedureFactID {
		t.Errorf("spec reference refs = %v, want the procedure authoring fact", fact.Refs)
	}
	// The body teaches a literal lowercase placeholder ({{.anchor}}); only the
	// template's own capitalized data fields would mark a failed render.
	if regexp.MustCompile(`\{\{\s*\.[A-Z]`).MatchString(fact.Content) {
		t.Error("spec reference body contains an unrendered template placeholder")
	}
	body := fact.Content
	if loc := entryIDPattern.FindString(body); loc != "" {
		t.Errorf("spec reference cites entry %q in its body; pointers live in refs", loc)
	}
	for _, host := range []string{"sdd ", "MCP", "CLI"} {
		if strings.Contains(body, host) {
			t.Errorf("spec reference body contains host-specific reference %q", host)
		}
	}
}

// TestEveryProcedureClassHasADescription pins the completeness rule: a class
// declared in the model without a description fails here (and the render
// errors), not at a reader's graph load.
func TestEveryProcedureClassHasADescription(t *testing.T) {
	for _, class := range model.ProcedureClassValues() {
		if class.Description() == "" {
			t.Errorf("procedure class %q has no description beside its declaration", class)
		}
	}
}

// baseFactIDs is the sanctioned exception to the no-IDs-in-prose rule: a base
// fact's ID resolves in every graph the binary serves, so a base fact may
// point at another one by ID without stranding any reader.
func baseFactIDs() map[string]bool {
	return map[string]bool{
		ViewGrammarFactID: true, PrinciplesFactID: true, DoneFactID: true,
		OverviewFactID: true, ProcedureFactID: true, ProcedureSpecFactID: true,
	}
}

// TestProcedureFactBodyIsSelfContained holds the procedure fact to the
// framework-generic standard: no graph-local entry IDs (base-fact IDs are the
// sanctioned exception — they resolve everywhere), no host-specific surface
// names.
func TestProcedureFactBodyIsSelfContained(t *testing.T) {
	body := factByID(t, ProcedureFactID).Content

	for _, want := range []string{"# Extending the process with workflows", "Every other entry is read; a procedure is also run", "canonical is the identity", "Class places how it enters", "workflow is frontmatter", "Ask where the choice is real", "Shipped and project procedures", "Retire deliberately", "Validation waits for the engine", "spec reference fact `" + ProcedureSpecFactID + "`"} {
		if !strings.Contains(body, want) {
			t.Errorf("procedure fact body missing %q", want)
		}
	}
	for _, id := range entryIDPattern.FindAllString(body, -1) {
		if !baseFactIDs()[id] {
			t.Errorf("procedure fact cites project entry %q; a base fact's prose may only cite other base facts", id)
		}
	}
	for _, host := range []string{"sdd ", "MCP", "CLI"} {
		if strings.Contains(body, host) {
			t.Errorf("procedure fact body contains host-specific reference %q", host)
		}
	}
}

// TestEveryBaseTypeHasADescription mirrors the class rule: a domain type
// declared in the engine without a description fails here (and the spec
// reference render errors), not at a reader's graph load.
func TestEveryBaseTypeHasADescription(t *testing.T) {
	for _, bt := range engine.BaseTypeValues() {
		if bt.Description() == "" {
			t.Errorf("domain type %q has no description", bt)
		}
	}
}

// TestTypeSystemFactsAreOverrideClosed pins the declared property (d-tac-9be):
// facts whose content renders from the running version's declarations refuse
// supersession, and the marker — not an ID list — is what the write path reads.
func TestTypeSystemFactsAreOverrideClosed(t *testing.T) {
	closed := []string{OverviewFactID, DoneFactID, ProcedureFactID, ProcedureSpecFactID, GapFactID, DirectiveFactID}
	for _, id := range closed {
		if fact := factByID(t, id); fact.Override != model.OverrideClosed {
			t.Errorf("fact %s: Override = %q, want %q", id, fact.Override, model.OverrideClosed)
		}
	}
	if fact := factByID(t, PrinciplesFactID); fact.Override != "" {
		t.Errorf("principles fact must stay project-overridable, got Override = %q", fact.Override)
	}
}

// TestEntriesShipGapKindFact covers the gap authoring fact: unindexed like
// every authoring fact, with mechanics rendered from the model declarations.
func TestEntriesShipGapKindFact(t *testing.T) {
	fact := factByID(t, GapFactID)
	if fact.Index != nil {
		t.Errorf("gap fact carries index enrollment %+v, want none — authoring facts are teased from the capture lane", fact.Index)
	}
	if fact.Summary == "" {
		t.Error("gap fact has no summary; every reading surface needs one")
	}
	for _, want := range []string{"## Mechanics", model.SignalCloseRule, "Both sides, each with its source", "The world input is the substance", "Reasoning enters only under its own name", "Observed, never decided", "Record the act, not the readings", "Frame the level that needs to change", "How a gap resolves", "Choosing gap at all"} {
		if !strings.Contains(fact.Content, want) {
			t.Errorf("gap fact body missing %q", want)
		}
	}
	assertFactSelfContained(t, fact)
}

// TestEntriesShipDirectiveKindFact covers the directive authoring fact.
func TestEntriesShipDirectiveKindFact(t *testing.T) {
	fact := factByID(t, DirectiveFactID)
	if fact.Index != nil {
		t.Errorf("directive fact carries index enrollment %+v, want none — authoring facts are teased from the capture lane", fact.Index)
	}
	if fact.Summary == "" {
		t.Error("directive fact has no summary; every reading surface needs one")
	}
	for _, want := range []string{"## Mechanics", model.DirectiveIntentRequirement, model.SettledCloseRule, "The choice carries its why", "Intent is chosen, never defaulted", "A guiding directive binds", "Retirement follows the posture", "Refining without replacing", "Choosing directive at all"} {
		if !strings.Contains(fact.Content, want) {
			t.Errorf("directive fact body missing %q", want)
		}
	}
	assertFactSelfContained(t, fact)
}

// assertFactSelfContained holds a fact body to the framework-generic
// standard: no graph-local entry IDs, no host-specific tool references, no
// unrendered template placeholders.
func assertFactSelfContained(t *testing.T, fact *model.Entry) {
	t.Helper()
	if loc := entryIDPattern.FindString(fact.Content); loc != "" {
		t.Errorf("fact %s cites project entry %q; a base fact carries no graph-local references", fact.ID, loc)
	}
	for _, host := range []string{"sdd ", "MCP", "CLI"} {
		if strings.Contains(fact.Content, host) {
			t.Errorf("fact %s body contains host-specific reference %q", fact.ID, host)
		}
	}
	if strings.Contains(fact.Content, "{{") {
		t.Errorf("fact %s body contains an unrendered template placeholder", fact.ID)
	}
}
