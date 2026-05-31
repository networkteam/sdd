package presenters

import (
	"fmt"
	"io"
	"strconv"

	"github.com/networkteam/sdd/internal/model"
)

// renderAsCounts writes one row per aggregated topic: the entry count, the
// topic label, and the summed heat. Counts are right-aligned and labels are
// left-padded to a common width so the columns line up, matching the dense,
// scannable shape of the other view renders. Heat uses the same three-decimal
// form as the `{score: X.XXX}` segment on ranked entry lines.
//
//	12  catch-up-scaling      heat 8.421
//	 9  type-system/topics    heat 5.100
//
// An empty result (no entry carried an effective topic) renders a single
// `(no topics)` line so the section isn't silently blank.
func renderAsCounts(w io.Writer, counts model.Counts) {
	if len(counts.Rows) == 0 {
		fmt.Fprintln(w, "  (no topics)")
		return
	}

	countWidth, labelWidth := 0, 0
	for _, r := range counts.Rows {
		if n := len(strconv.Itoa(r.Count)); n > countWidth {
			countWidth = n
		}
		if n := len(r.Label); n > labelWidth {
			labelWidth = n
		}
	}

	for _, r := range counts.Rows {
		fmt.Fprintf(w, "  %*d  %-*s  heat %.3f\n", countWidth, r.Count, labelWidth, r.Label, r.Heat)
	}
}
