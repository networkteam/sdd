// Package basefacts assembles the base facts shipped with the sdd binary.
// Base facts are framework reference knowledge merged into every graph as
// Embedded `fact` signals — the pull layer that answers the engine's
// push-only knowledge gap (d-cpt-dtv). They mirror the base-procedures
// pattern: always loaded, no participants, no project refs, stable IDs, and
// overridable per-project by a superseding entry on the same ID — except the
// type-system facts, which declare `override: closed` (model.OverrideClosed)
// because their content renders from the running version's declarations and a
// frozen project copy would silently outrank that truth (d-tac-9be).
//
// Unlike base procedures — static .md files embedded at compile time — a base
// fact body may be rendered at load from live executor vocabularies, so the
// fact tracks the code with zero manual sync. To keep this package free of a
// dependency on the read stack (finders assembles the vocabulary and merges
// these entries, so importing it here would cycle), the caller supplies the
// vocabulary as data — the same dependency inversion viewlayout uses.
package basefacts

import (
	"fmt"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/viewlayout"
)

// Entries returns the base facts to merge into a graph, rendered against the
// supplied live vocabulary. The set is compile-time-shaped, so a construction
// error means a broken build — callers fail hard, exactly as with base
// procedures.
func Entries(vocab viewlayout.Vocabulary) ([]*model.Entry, error) {
	entries := []*model.Entry{}

	viewGrammar, err := build(ViewGrammarFactID, viewGrammarFrontmatter, viewGrammarBody(vocab))
	if err != nil {
		return nil, err
	}
	entries = append(entries, viewGrammar)

	principles, err := build(PrinciplesFactID, principlesFrontmatter, principlesBody)
	if err != nil {
		return nil, err
	}
	entries = append(entries, principles)

	doneContent, err := doneFactContent()
	if err != nil {
		return nil, err
	}
	doneFact, err := buildContent(DoneFactID, doneContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, doneFact)

	overviewContent, err := overviewFactContent()
	if err != nil {
		return nil, err
	}
	overviewFact, err := buildContent(OverviewFactID, overviewContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, overviewFact)

	procedureContent, err := procedureFactContent()
	if err != nil {
		return nil, err
	}
	procedureFact, err := buildContent(ProcedureFactID, procedureContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, procedureFact)

	gapContent, err := gapFactContent()
	if err != nil {
		return nil, err
	}
	gapFact, err := buildContent(GapFactID, gapContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, gapFact)

	directiveContent, err := directiveFactContent()
	if err != nil {
		return nil, err
	}
	directiveFact, err := buildContent(DirectiveFactID, directiveContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, directiveFact)

	insightContent, err := insightFactContent()
	if err != nil {
		return nil, err
	}
	insightFact, err := buildContent(InsightFactID, insightContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, insightFact)

	factContent, err := factFactContent()
	if err != nil {
		return nil, err
	}
	factFact, err := buildContent(FactFactID, factContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, factFact)

	questionContent, err := questionFactContent()
	if err != nil {
		return nil, err
	}
	questionFact, err := buildContent(QuestionFactID, questionContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, questionFact)

	planContent, err := planFactContent()
	if err != nil {
		return nil, err
	}
	planFact, err := buildContent(PlanFactID, planContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, planFact)

	actorContent, err := actorFactContent()
	if err != nil {
		return nil, err
	}
	actorFact, err := buildContent(ActorFactID, actorContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, actorFact)

	roleContent, err := roleFactContent()
	if err != nil {
		return nil, err
	}
	roleFact, err := buildContent(RoleFactID, roleContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, roleFact)

	activityContent, err := activityFactContent()
	if err != nil {
		return nil, err
	}
	activityFact, err := buildContent(ActivityFactID, activityContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, activityFact)

	focusContent, err := focusFactContent()
	if err != nil {
		return nil, err
	}
	focusFact, err := buildContent(FocusFactID, focusContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, focusFact)

	aspirationContent, err := aspirationFactContent()
	if err != nil {
		return nil, err
	}
	aspirationFact, err := buildContent(AspirationFactID, aspirationContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, aspirationFact)

	annotationContent, err := annotationFactContent()
	if err != nil {
		return nil, err
	}
	annotationFact, err := buildContent(AnnotationFactID, annotationContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, annotationFact)

	procedureSpecContent, err := procedureSpecFactContent()
	if err != nil {
		return nil, err
	}
	procedureSpecFact, err := buildContent(ProcedureSpecFactID, procedureSpecContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, procedureSpecFact)

	return entries, nil
}

// authoringFactIDs maps each kind to its shipped authoring fact — the
// per-kind depth behind the overview. Kinds absent here have no fact yet;
// consumers treat absence as "no section", so coverage grows as the sweep
// ships each kind.
var authoringFactIDs = map[model.Kind]string{
	model.KindDone:       DoneFactID,
	model.KindProcedure:  ProcedureFactID,
	model.KindGap:        GapFactID,
	model.KindDirective:  DirectiveFactID,
	model.KindInsight:    InsightFactID,
	model.KindFact:       FactFactID,
	model.KindQuestion:   QuestionFactID,
	model.KindPlan:       PlanFactID,
	model.KindActor:      ActorFactID,
	model.KindRole:       RoleFactID,
	model.KindActivity:   ActivityFactID,
	model.KindFocus:      FocusFactID,
	model.KindAspiration: AspirationFactID,
	model.KindAnnotation: AnnotationFactID,
}

// AuthoringFactID returns the stable ID of the kind's authoring fact, or ""
// when none ships yet.
func AuthoringFactID(kind model.Kind) string {
	return authoringFactIDs[kind]
}

// build materializes one base fact from its frontmatter and rendered body
// through the same ParseEntry path every on-disk entry uses, then marks it
// Embedded so write-side surfaces (summary regeneration, lint, rewrite) skip
// it. A base fact must be exactly a kind: fact signal — anything else is a
// build mistake, caught here so it never reaches a graph.
func build(id, frontmatter, body string) (*model.Entry, error) {
	return buildContent(id, "---\n"+frontmatter+"---\n\n"+body)
}

// buildContent is build for facts authored as whole-entry templates, where
// frontmatter and body arrive already rendered as one document. Structural
// validity goes through the construction boundary like every other write
// surface; base facts stand alone, so the graph-dependent edge rules are
// out of reach here and covered by the package's table test instead.
func buildContent(id, content string) (*model.Entry, error) {
	entry, err := model.ParseEntry(id+".md", content)
	if err != nil {
		return nil, fmt.Errorf("parsing base fact %s: %w", id, err)
	}
	if entry.Type != model.TypeSignal || entry.Kind != model.KindFact {
		return nil, fmt.Errorf("base fact %s is %s %s — base facts ship as kind: fact signals", id, entry.Type, entry.Kind)
	}
	construction, findings := model.ConstructFromEntry(entry)
	findings = append(findings, construction.Validate(nil)...)
	if len(findings) > 0 {
		return nil, fmt.Errorf("base fact %s: %s", id, findings[0].Message)
	}
	entry.Embedded = true
	return entry, nil
}

// ViewGrammarFactID is the stable identity of the view-layout-grammar fact. It
// never changes across releases: readers cite it, and the first-hit view hint
// (mcpapp) points at it. Its timestamp is a fixed authoring stamp, not a live
// clock. Exported so the hint producer references one shared constant.
const ViewGrammarFactID = "20260717-110000-s-prc-vwg"

// viewGrammarFrontmatter is the fact's static envelope. The body is rendered
// separately from live vocabulary; everything a summary surface reads lives
// here. No participants and no project refs, per the base-entry contract.
const viewGrammarFrontmatter = `type: signal
layer: process
kind: fact
confidence: high
topics:
    - engine/base-facts
    - cli/view
index:
    title: 'How to compose graph views (view tool): layout grammar, filters, ranking, quoting, and examples'
    topic: cli/view
summary: >-
    The view layout language composes graph views from a colon-chained
    pipeline — filters, then ranking, paging, and transforms, ending in a
    render terminator — with named macros as shortcuts; this fact is the
    grammar and vocabulary reference for building a layout, including the
    quoting rule for multi-word, date, and duration arguments.
`

func viewGrammarBody(vocab viewlayout.Vocabulary) string {
	return viewlayout.Markdown(vocab)
}
