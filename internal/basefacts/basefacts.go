// Package basefacts assembles the base facts shipped with the sdd binary.
// Base facts are framework reference knowledge merged into every graph as
// Embedded `fact` signals — the pull layer that answers the engine's
// push-only knowledge gap (d-cpt-dtv). They mirror the base-procedures
// pattern: always loaded, no participants, no project refs, stable IDs, and
// overridable per-project by a superseding entry on the same ID.
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

	return entries, nil
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
// frontmatter and body arrive already rendered as one document.
func buildContent(id, content string) (*model.Entry, error) {
	entry, err := model.ParseEntry(id+".md", content)
	if err != nil {
		return nil, fmt.Errorf("parsing base fact %s: %w", id, err)
	}
	if entry.Type != model.TypeSignal || entry.Kind != model.KindFact {
		return nil, fmt.Errorf("base fact %s is %s %s — base facts ship as kind: fact signals", id, entry.Type, entry.Kind)
	}
	if err := entry.Index.ValidateForEntry(entry.Kind, entry.Topics); err != nil {
		return nil, fmt.Errorf("base fact %s index: %w", id, err)
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
