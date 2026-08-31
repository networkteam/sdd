package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/query"
)

// RenderLint writes the categorized lint report to w: findings grouped by
// category in first-appearance order, errors marked ⚠ and advisories marked
// ℹ. Returns nothing — the caller derives the exit code from
// LintResult.Errors alone (advisories never flip it, d-cpt-xc3).
func RenderLint(w io.Writer, result *query.LintResult) {
	if len(result.Findings) == 0 {
		fmt.Fprintln(w, "No issues found.")
		return
	}
	var categories []string
	grouped := map[string][]query.LintFinding{}
	for _, f := range result.Findings {
		if _, seen := grouped[f.Category]; !seen {
			categories = append(categories, f.Category)
		}
		grouped[f.Category] = append(grouped[f.Category], f)
	}
	advisories := len(result.Findings) - result.Errors()
	fmt.Fprintf(w, "%d error(s), %d advisory/advisories:\n", result.Errors(), advisories)
	for _, category := range categories {
		fmt.Fprintf(w, "\n%s:\n", category)
		for _, f := range grouped[category] {
			marker := "⚠"
			if f.Severity == query.LintAdvisory {
				marker = "ℹ"
			}
			if f.EntryID != "" {
				fmt.Fprintf(w, "  %s %s [%s] %s\n", marker, f.EntryID, f.Code, f.Message)
			} else {
				fmt.Fprintf(w, "  %s [%s] %s\n", marker, f.Code, f.Message)
			}
		}
	}
}
