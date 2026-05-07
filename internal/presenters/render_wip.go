package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/model"
)

// renderAsWipList writes one line per active WIP marker. Mirrors the
// existing `sdd wip list` rendering so a section that ends in
// as-wip-list reads identically to a standalone wip-list invocation —
// users learning the catch-up surface aren't faced with two different
// formats for the same data.
//
// Empty marker sets render a single explanatory line so a focus-block
// followed by `wip` still produces a visible section even when no work
// is in flight (otherwise the section header lands above blank space).
func renderAsWipList(w io.Writer, list model.WipList) {
	if len(list.Markers) == 0 {
		fmt.Fprintln(w, "No active WIP markers.")
		return
	}
	for _, m := range list.Markers {
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
