package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/model"
)

// renderAsParticipantsBlock writes one `### <canonical>` per active
// actor head, with the actor's entry line followed by every derived-
// active role bound to that chain — the Participants block surfaced by
// `sdd view --layout='participants'`.
//
// Empty groups suppress output entirely. The CLI shell handles the
// outer `## <name>` header (when name() is set), so this function
// focuses on the per-actor bodies.
func renderAsParticipantsBlock(w io.Writer, g *model.Graph, block model.ParticipantsBlock, brief bool) {
	if len(block.Groups) == 0 {
		return
	}
	line := EntryLine
	if brief {
		line = EntryLineBrief
	}
	for i, grp := range block.Groups {
		if grp.Actor == nil {
			continue
		}
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "### %s\n", grp.Actor.Canonical)
		line(w, grp.Actor, g)
		for _, r := range grp.Roles {
			line(w, r, g)
		}
	}
}
