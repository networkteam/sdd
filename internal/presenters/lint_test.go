package presenters_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

// TestRenderLintReportsLoadErrors pins that unreadable entries render in their
// own section — they carry no per-entry warning, so this is the only place
// they surface — and that a graph with only a load error is not reported clean.
func TestRenderLintReportsLoadErrors(t *testing.T) {
	g := model.NewGraphWithLoadIssues(nil, []model.LoadIssue{
		{Ref: "20260101-120001-s-prc-bad", Message: "parsing frontmatter: unexpected token"},
	})
	result := &query.LintResult{
		TotalIssues: len(g.LoadIssues),
		LoadErrors:  g.LoadIssues,
	}

	var buf bytes.Buffer
	presenters.RenderLint(&buf, result, g)
	out := buf.String()

	if strings.Contains(out, "No issues found") {
		t.Fatalf("reported clean despite a load error:\n%s", out)
	}
	if !strings.Contains(out, "unreadable") {
		t.Fatalf("missing unreadable-entries section:\n%s", out)
	}
	if !strings.Contains(out, "20260101-120001-s-prc-bad") || !strings.Contains(out, "unexpected token") {
		t.Fatalf("load-error section missing ref/message:\n%s", out)
	}
}
