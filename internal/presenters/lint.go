package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// RenderLint writes a human-readable lint report to w. Returns nothing —
// the caller decides what to do about a non-zero issue count (typically
// returning a non-zero exit code from the CLI).
func RenderLint(w io.Writer, result *query.LintResult, g *model.Graph) {
	if len(result.Entries) == 0 {
		fmt.Fprintln(w, "No issues found.")
		return
	}

	fmt.Fprintf(w, "%d issue(s) in %d entry/entries:\n\n", result.TotalIssues, len(result.Entries))
	for _, e := range result.Entries {
		desc := e.Summary
		if desc == "" {
			desc = e.ShortContent(200)
		}
		status := FormatStatus(g.DerivedStatus(e))
		if status != "" {
			fmt.Fprintf(w, "  %s  %s  %s  %s\n", e.ID, e.TypeLabel(), status, desc)
		} else {
			fmt.Fprintf(w, "  %s  %s  %s\n", e.ID, e.TypeLabel(), desc)
		}
		for _, warning := range e.Warnings {
			fmt.Fprintf(w, "    ⚠ %s\n", warning.Message)
		}
		fmt.Fprintln(w)
	}
}
