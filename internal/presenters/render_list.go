package presenters

import (
	"io"

	"github.com/networkteam/sdd/internal/model"
)

// renderAsList writes one EntryLine per entry in the flat list. Reuses the
// canonical entry-line format shared with `sdd list` and the section
// helpers in `sdd status` so output stays consistent across surfaces.
//
// When score becomes meaningful (slice 2 lands ranking), an extra `[score:
// X.XXX]` segment will inject before the participants — until then,
// EntryLine's existing format covers the AC's "ID, type/kind/layer,
// summary" requirements as a strict superset.
func renderAsList(w io.Writer, g *model.Graph, flat model.FlatList) {
	for _, e := range flat.Entries {
		EntryLine(w, e, g)
	}
}
