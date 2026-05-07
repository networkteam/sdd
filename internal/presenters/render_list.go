package presenters

import (
	"io"

	"github.com/networkteam/sdd/internal/model"
)

// renderAsList writes one entry-line per entry in the flat list. Reuses
// the canonical EntryLine format shared with `sdd list` and the section
// helpers in `sdd status` so output stays consistent across surfaces.
// When the section is ranked (FlatList.Scores populated and aligned),
// each line carries a `{score: X.XXX}` segment via EntryLineWithScore.
// by(date) leaves Scores nil, falling back to plain EntryLine — sort
// without per-entry rendering noise.
func renderAsList(w io.Writer, g *model.Graph, flat model.FlatList) {
	scored := len(flat.Scores) == len(flat.Entries) && len(flat.Scores) > 0
	for i, e := range flat.Entries {
		if scored {
			EntryLineWithScore(w, e, g, flat.Scores[i])
		} else {
			EntryLine(w, e, g)
		}
	}
}
