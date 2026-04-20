package presenters_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

// TestRenderStatusSections verifies that each decision kind surfaces in its
// own section with the expected ordering (Aspirations → Contracts → Plans →
// Activities → Directives), and that signals render under "Gaps and
// Questions" (allow-list of gap + question) rather than a generic "Open
// Signals" heading.
func TestRenderStatusSections(t *testing.T) {
	g := model.NewGraph([]*model.Entry{
		entry("20260406-100000-d-stg-asp", withKind(model.KindAspiration), withContent("An aspiration.")),
		entry("20260406-100100-d-cpt-ctr", withKind(model.KindContract), withContent("A contract.")),
		entry("20260406-100200-d-cpt-pln", withKind(model.KindPlan), withContent("A plan.\n\n## Acceptance criteria\n- [ ] do the thing")),
		entry("20260406-100300-d-tac-act", withKind(model.KindActivity), withContent("An activity.")),
		entry("20260406-100400-d-tac-dir", withKind(model.KindDirective), withContent("A directive.")),
		entry("20260406-100500-s-tac-gap", withKind(model.KindGap), withContent("A gap.")),
		entry("20260406-100600-s-tac-que", withKind(model.KindQuestion), withContent("A question.")),
		entry("20260406-100700-s-stg-ins", withKind(model.KindInsight), withContent("An insight.")),
		entry("20260406-100800-s-tac-don", withKind(model.KindDone), withRefs("20260406-100400-d-tac-dir"), withContent("A done signal.")),
	})

	result := &query.StatusResult{
		Graph:       g,
		Aspirations: g.Aspirations(),
		Contracts:   g.Contracts(),
		Plans:       g.Plans(),
		Activities:  g.Activities(),
		Directives:  g.Directives(),
		Open:        g.OpenSignals(),
		Insights:    g.RecentInsights(10),
		Recent:      g.RecentDone(10),
	}

	var buf bytes.Buffer
	presenters.RenderStatus(&buf, result)
	out := buf.String()

	// Ordered section headers: aspirations first, then contracts, plans,
	// activities, directives, then the signal sections.
	order := []string{
		"## Aspirations",
		"## Contracts",
		"## Plans",
		"## Activities",
		"## Directives",
		"## Gaps and Questions",
		"## Recent Insights",
		"## Recent Done Signals",
	}
	var idx []int
	for _, header := range order {
		i := strings.Index(out, header)
		if i < 0 {
			t.Fatalf("missing section %q:\n%s", header, out)
		}
		idx = append(idx, i)
	}
	for i := 1; i < len(idx); i++ {
		if idx[i-1] >= idx[i] {
			t.Fatalf("section order broken at %q (idx %d) vs %q (idx %d):\n%s",
				order[i-1], idx[i-1], order[i], idx[i], out)
		}
	}

	// The prior "Open Signals" / "Active Decisions" headings must not render.
	if strings.Contains(out, "## Open Signals") {
		t.Errorf("legacy 'Open Signals' heading should be gone; expected 'Gaps and Questions':\n%s", out)
	}
	if strings.Contains(out, "## Active Decisions") {
		t.Errorf("legacy 'Active Decisions' heading should be gone; directives and activities render separately:\n%s", out)
	}

	// Sanity: directives and activities land in their own sections.
	dirIdx := strings.Index(out, "## Directives")
	gapsIdx := strings.Index(out, "## Gaps and Questions")
	directivesBlock := out[dirIdx:gapsIdx]
	if !strings.Contains(directivesBlock, "20260406-100400-d-tac-dir") {
		t.Errorf("directives section missing the directive entry:\n%s", directivesBlock)
	}
	if strings.Contains(directivesBlock, "20260406-100300-d-tac-act") {
		t.Errorf("directives section should not include the activity entry:\n%s", directivesBlock)
	}

	actIdx := strings.Index(out, "## Activities")
	activitiesBlock := out[actIdx:dirIdx]
	if !strings.Contains(activitiesBlock, "20260406-100300-d-tac-act") {
		t.Errorf("activities section missing the activity entry:\n%s", activitiesBlock)
	}

	// Gaps and Questions must carry both gap and question but not insight.
	insIdx := strings.Index(out, "## Recent Insights")
	gapsBlock := out[gapsIdx:insIdx]
	if !strings.Contains(gapsBlock, "20260406-100500-s-tac-gap") {
		t.Errorf("Gaps and Questions missing gap:\n%s", gapsBlock)
	}
	if !strings.Contains(gapsBlock, "20260406-100600-s-tac-que") {
		t.Errorf("Gaps and Questions missing question:\n%s", gapsBlock)
	}
	if strings.Contains(gapsBlock, "20260406-100700-s-stg-ins") {
		t.Errorf("Gaps and Questions should not contain insight:\n%s", gapsBlock)
	}
}
