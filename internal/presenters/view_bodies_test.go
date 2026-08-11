package presenters_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// bodiesSection wraps entries in the section shape as-bodies consumes, named
// like a framing lane so the nesting under a `## <name>` header is exercised.
func bodiesSection(name string, entries ...*model.Entry) *query.ViewResult {
	return &query.ViewResult{
		Graph: model.NewGraph(entries),
		Sections: []query.SectionResult{{
			Render: "as-bodies",
			Name:   name,
			Data:   model.Bodies{Entries: entries},
		}},
	}
}

func TestRenderView_AsBodies_LeadingHeadingBecomesTitle(t *testing.T) {
	e := entry("20260101-100000-s-prc-aaa",
		withKind(model.KindFact),
		withSummary("summary that must not be served alongside the body"),
		withContent("# The posture\n\nGoal first.\n\n## Misfit\n\nMisfit is a signal.\n"))

	got := renderView(bodiesSection("Working principles", e))
	want := "## Working principles\n\n" +
		"### The posture\n\n" +
		"`20260101-100000-s-prc-aaa` — process fact signal\n\n" +
		"Goal first.\n\n" +
		"#### Misfit\n\n" +
		"Misfit is a signal.\n"
	if got != want {
		t.Errorf("as-bodies render mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "summary that must not be served") {
		t.Error("the identity header must not carry the summary — it derives from the body already served")
	}
}

func TestRenderView_AsBodies_HeadinglessBodyTakesIdentityAsHeader(t *testing.T) {
	e := entry("20260101-100000-s-prc-aaa",
		withKind(model.KindFact),
		withContent("Opening prose with no heading.\n\n## Section\n\nunder it\n"))

	got := renderView(bodiesSection("Working principles", e))
	want := "## Working principles\n\n" +
		"### `20260101-100000-s-prc-aaa` — process fact signal\n\n" +
		"Opening prose with no heading.\n\n" +
		"#### Section\n\n" +
		"under it\n"
	if got != want {
		t.Errorf("as-bodies render mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestRenderView_AsBodies_NoBodyHeadingOutranksTheStructure is the invariant
// itself (d-cpt-5wv): whatever levels a body was authored at, nothing it
// contributes may sit at or above the lane and entry headers serving it.
func TestRenderView_AsBodies_NoBodyHeadingOutranksTheStructure(t *testing.T) {
	e := entry("20260101-100000-s-prc-aaa",
		withKind(model.KindFact),
		withContent("# Top\n\n## Mid\n\n### Deep\n\n```sh\n# a comment, not a heading\n```\n"))

	got := renderView(bodiesSection("Working principles", e))
	inFence := false
	for line := range strings.SplitSeq(got, "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(line, "#") {
			continue
		}
		level := len(line) - len(strings.TrimLeft(line, "#"))
		switch {
		case strings.HasPrefix(line, "## Working principles"), strings.HasPrefix(line, "### Top"):
		case level <= 3:
			t.Errorf("body heading %q outranks the entry header it belongs under", line)
		}
	}
	if !strings.Contains(got, "# a comment, not a heading") || strings.Contains(got, "#### a comment") {
		t.Errorf("fenced content must survive verbatim, got:\n%s", got)
	}
}

func TestRenderView_AsBodies_UnnamedSectionNestsOneLevelHigher(t *testing.T) {
	e := entry("20260101-100000-s-prc-aaa",
		withKind(model.KindFact),
		withContent("# The posture\n\nGoal first.\n"))

	got := renderView(bodiesSection("", e))
	want := "## The posture\n\n`20260101-100000-s-prc-aaa` — process fact signal\n\nGoal first.\n"
	if got != want {
		t.Errorf("without a section header the entry header is the top level:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderView_AsBodies_EmptySelectionServesNothing(t *testing.T) {
	if got := renderView(bodiesSection("Working principles")); got != "" {
		t.Errorf("an empty selection must serve no block at all, got %q", got)
	}
}

func TestRenderView_AsBodies_MultipleEntriesInSectionOrder(t *testing.T) {
	first := entry("20260101-100000-s-prc-aaa",
		withKind(model.KindFact),
		withContent("# First\n\none\n"))
	second := entry("20260101-110000-s-prc-bbb",
		withKind(model.KindFact),
		withContent("# Second\n\ntwo\n"))

	got := renderView(bodiesSection("Working principles", first, second))
	if i, j := strings.Index(got, "### First"), strings.Index(got, "### Second"); i < 0 || j < 0 || i > j {
		t.Errorf("entries render in the order the section produced them, got:\n%s", got)
	}
	if !strings.Contains(got, "one\n\n### Second") {
		t.Errorf("entries are separated by a blank line, got:\n%s", got)
	}
}
