package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/query"
)

// RenderWIPList writes the active WIP markers in a fixed-width layout.
func RenderWIPList(w io.Writer, result *query.WIPListResult) {
	if len(result.Markers) == 0 {
		fmt.Fprintln(w, "No active WIP markers.")
		return
	}
	for _, m := range result.Markers {
		excl := ""
		if m.Exclusive {
			excl = " [exclusive]"
		}
		branch := ""
		if m.Branch != "" {
			branch = fmt.Sprintf("  branch:%s", m.Branch)
		}
		fmt.Fprintf(w, "  %s  %-15s%s  %s%s  %s\n",
			m.ID, m.Participant, excl, m.Entry, branch, m.ShortContent(200))
	}
}
