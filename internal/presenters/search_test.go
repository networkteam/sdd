package presenters

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

func mkEntry(id string, kind model.Kind) *model.Entry {
	parts, _ := model.ParseID(id)
	return &model.Entry{
		ID:           id,
		Type:         model.TypeFromAbbrev[parts.TypeCode],
		Layer:        model.LayerFromAbbrev[parts.LayerCode],
		Kind:         kind,
		Confidence:   "medium",
		Participants: []string{"Test"},
		Time:         time.Now(),
		Summary:      "Summary text for " + id,
	}
}

func TestRenderSearch_RelativePercentages(t *testing.T) {
	t.Parallel()

	a := mkEntry("20260101-100000-s-tac-aaa", model.KindGap)
	b := mkEntry("20260101-100001-s-tac-bbb", model.KindGap)

	res := &query.SearchResult{
		Entries: []query.SearchEntry{
			{
				Entry: a,
				Score: 0.95,
				Citations: []query.Citation{
					{Snippet: "alpha-strong", IsSummary: true, Score: 0.95},
					{Snippet: "alpha-mid", Breadcrumb: []string{"Section"}, Score: 0.85},
				},
			},
			{
				Entry: b,
				Score: 0.50,
				Citations: []query.Citation{
					{Snippet: "beta-only", IsSummary: true, Score: 0.50},
				},
			},
		},
	}

	g := model.NewGraph([]*model.Entry{a, b})
	var buf bytes.Buffer
	RenderSearch(&buf, res, g)

	out := buf.String()
	// Top citation in the result set must show 100%.
	if !strings.Contains(out, "100%  ·  Summary  ·  alpha-strong") {
		t.Errorf("expected 100%% on the strongest citation; got:\n%s", out)
	}
	// Mid citation: 0.85 / 0.95 ≈ 89%.
	if !strings.Contains(out, "89%  ·  Section  ·  alpha-mid") {
		t.Errorf("expected ~89%% on the mid citation; got:\n%s", out)
	}
	// Cross-entry comparison: 0.50 / 0.95 ≈ 52%.
	if !strings.Contains(out, "52%  ·  Summary  ·  beta-only") {
		t.Errorf("expected ~52%% on entry B; got:\n%s", out)
	}
}

func TestRenderSearch_EmptyResult(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	RenderSearch(&buf, &query.SearchResult{}, model.NewGraph(nil))
	if !strings.Contains(buf.String(), "(no matches)") {
		t.Errorf("expected (no matches) marker; got %q", buf.String())
	}
}

func TestRenderSearch_AllCitationsRendered(t *testing.T) {
	t.Parallel()
	a := mkEntry("20260101-100000-s-tac-aaa", model.KindGap)
	res := &query.SearchResult{
		Entries: []query.SearchEntry{{
			Entry: a,
			Score: 1.0,
			Citations: []query.Citation{
				{Snippet: "first", IsSummary: true, Score: 1.0},
				{Snippet: "second", Breadcrumb: []string{"X"}, Score: 0.9},
				{Snippet: "third", Breadcrumb: []string{"Y"}, Score: 0.85},
			},
		}},
	}
	g := model.NewGraph([]*model.Entry{a})
	var buf bytes.Buffer
	RenderSearch(&buf, res, g)

	// Three citation lines for one entry — verify each appears with its
	// own breadcrumb/snippet.
	for _, want := range []string{"first", "second", "third"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing citation %q in output:\n%s", want, buf.String())
		}
	}
	// The number of "↳" markers equals the number of citations.
	if got := strings.Count(buf.String(), "↳"); got != 3 {
		t.Errorf("expected 3 citation markers, got %d:\n%s", got, buf.String())
	}
}

func TestFormatRelativeScore_AlignedWidth(t *testing.T) {
	t.Parallel()
	cases := []struct {
		score, max float32
		want       string
	}{
		{1.0, 1.0, "100%"},
		{0.91, 1.0, " 91%"},
		{0.05, 1.0, "  5%"},
		{0.0, 1.0, "  0%"},
		// Zero max returns blank padding so render doesn't break.
		{0.5, 0.0, "    "},
	}
	for _, c := range cases {
		if got := formatRelativeScore(c.score, c.max); got != c.want {
			t.Errorf("formatRelativeScore(%v, %v) = %q; want %q", c.score, c.max, got, c.want)
		}
	}
}
