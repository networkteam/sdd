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
	if result.TotalIssues == 0 {
		fmt.Fprintln(w, "No issues found.")
		renderIndexLint(w, result)
		return
	}
	if len(result.Entries) > 0 {
		entryIssues := result.TotalIssues - len(result.LoadErrors)
		fmt.Fprintf(w, "%d issue(s) in %d entry/entries:\n\n", entryIssues, len(result.Entries))
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
	renderLoadErrors(w, result)
	renderIndexLint(w, result)
}

// renderLoadErrors surfaces entries the loader could not parse. They are
// excluded from the in-memory graph, so they carry no per-entry warnings —
// this section is the only place they appear.
func renderLoadErrors(w io.Writer, result *query.LintResult) {
	if len(result.LoadErrors) == 0 {
		return
	}
	fmt.Fprintf(w, "%d unreadable entry/entries (parse failed, excluded from the graph):\n\n", len(result.LoadErrors))
	for _, le := range result.LoadErrors {
		fmt.Fprintf(w, "  %s\n    ⚠ %s\n\n", le.Ref, le.Message)
	}
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
	for _, drift := range result.RepoIndexDrift {
		fmt.Fprintf(w, "  ⚠ repo %s: %d of %d cached entries indexed under a different fingerprint than the global embedder — the next cross-graph search re-embeds them\n",
			drift.RepoID, drift.DriftCount, drift.EntryCount)
	}
}
