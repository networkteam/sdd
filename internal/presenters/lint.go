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
	} else {
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
	renderIndexLint(w, result)
}

// renderIndexLint surfaces the search index's drift count under the
// configured embedder. Suppressed when no embedder is configured so
// projects without vector search don't see a noisy "index" section.
func renderIndexLint(w io.Writer, result *query.LintResult) {
	if !result.IndexConfigured {
		return
	}
	fmt.Fprintln(w, "Index:")
	fmt.Fprintf(w, "  fingerprint: %s\n", result.IndexFingerprint)
	fmt.Fprintf(w, "  entries indexed: %d\n", result.IndexEntryCount)
	if result.IndexDriftCount > 0 {
		fmt.Fprintf(w, "  ⚠ %d entry/entries indexed under a different fingerprint — run `sdd index --force` to re-embed (or let `sdd search` lazy-fill)\n", result.IndexDriftCount)
	} else if result.IndexEntryCount > 0 {
		fmt.Fprintln(w, "  no fingerprint drift")
	}
}
