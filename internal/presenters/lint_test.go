package presenters_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

// TestRenderLintGroupsByCategory pins the categorized report: findings render
// under their category, errors and advisories are marked apart, and a result
// with findings is never reported clean.
func TestRenderLintGroupsByCategory(t *testing.T) {
	result := &query.LintResult{Findings: []query.LintFinding{
		{Category: "graph", Code: "load-error", Severity: query.LintError,
			EntryID: "20260101-120001-s-prc-bad", Message: "parsing frontmatter: unexpected token"},
		{Category: "procedure-runtime", Code: "serve-budget", Severity: query.LintAdvisory,
			EntryID: "20260101-120002-d-prc-big", Message: "step \"draft\" sizes past the budget"},
	}}

	var buf bytes.Buffer
	presenters.RenderLint(&buf, result)
	out := buf.String()

	if strings.Contains(out, "No issues found") {
		t.Fatalf("reported clean despite findings:\n%s", out)
	}
	if !strings.Contains(out, "graph:") || !strings.Contains(out, "procedure-runtime:") {
		t.Fatalf("missing category sections:\n%s", out)
	}
	if !strings.Contains(out, "20260101-120001-s-prc-bad") || !strings.Contains(out, "unexpected token") {
		t.Fatalf("finding missing entry/message:\n%s", out)
	}
	if !strings.Contains(out, "1 error(s), 1 advisory") {
		t.Fatalf("missing severity counts:\n%s", out)
	}
}

func TestRenderLintClean(t *testing.T) {
	var buf bytes.Buffer
	presenters.RenderLint(&buf, &query.LintResult{})
	if !strings.Contains(buf.String(), "No issues found") {
		t.Fatalf("empty result should report clean, got:\n%s", buf.String())
	}
}
