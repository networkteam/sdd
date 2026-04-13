package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/resonance/framework/sdd/query"
)

// RenderLint writes a human-readable lint report to w. Returns nothing —
// the caller decides what to do about a non-zero issue count (typically
// returning a non-zero exit code from the CLI).
func RenderLint(w io.Writer, result *query.LintResult, width int) {
	if len(result.Entries) == 0 {
		fmt.Fprintln(w, "No issues found.")
		return
	}

	fmt.Fprintf(w, "%d issue(s) in %d entry/entries:\n\n", result.TotalIssues, len(result.Entries))
	for _, e := range result.Entries {
		fmt.Fprintf(w, "  %s  %s  %s\n", e.ID, e.TypeLabel(), e.ShortContent(width))
		for _, warning := range e.Warnings {
			fmt.Fprintf(w, "    ⚠ %s\n", warning.Message)
		}
		fmt.Fprintln(w)
	}
}
