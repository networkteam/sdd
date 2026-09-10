package presenters

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/charmbracelet/colorprofile"

	"github.com/networkteam/sdd/internal/query"
)

// RenderIndexVersions writes the `sdd index gc` report: one block per store
// with its size and entry count, then a table of version groups. The current
// group is marked kept so the reader sees what a drop would never touch.
func RenderIndexVersions(dst io.Writer, results []*query.IndexVersionsResult) {
	w := colorprofile.NewWriter(dst, os.Environ())
	for i, r := range results {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%s %s\n", clrQual.Render("store "+r.Label), clrFaint.Render(fmt.Sprintf("%s · %d entries · %s", HumanBytes(r.Bytes), r.Entries, r.IndexDir)))
		if len(r.Groups) == 0 {
			fmt.Fprintln(w, "  no stored versions")
			continue
		}
		tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "  group\tversions\tentries\toldest\tnewest\tsize\t")
		for _, g := range r.Groups {
			note := ""
			if !g.Droppable {
				note = "kept"
			}
			fmt.Fprintf(tw, "  %s\t%d\t%d\t%s\t%s\t%s\t%s\n", g.Name, g.Versions, g.Entries,
				g.Oldest.Format("2006-01-02"), g.Newest.Format("2006-01-02"), HumanBytes(g.Bytes), note)
		}
		_ = tw.Flush()
	}
}

// HumanBytes formats a byte count in the unit that keeps it under four digits.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit {
			if value < 10 {
				return fmt.Sprintf("%.1f %s", value, suffix)
			}
			return fmt.Sprintf("%.0f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.0f PB", value/unit)
}
