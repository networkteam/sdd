package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/model"
)

// renderAsParticipantsBlock writes one `### <canonical>` per active
// actor head, with the actor's entry line followed by every derived-
// active role bound to that chain. Mirrors the Participants section in
// `sdd status` so view-side output reads consistently across surfaces —
// users learning the catch-up surface get the same grouping shape.
//
// Empty groups suppress output entirely. The CLI shell handles the
// outer `## <name>` header (when name() is set), so this function
// focuses on the per-actor bodies.
func renderAsParticipantsBlock(w io.Writer, g *model.Graph, block model.ParticipantsBlock) {
	if len(block.Groups) == 0 {
		return
	}
	for i, grp := range block.Groups {
		if grp.Actor == nil {
			continue
		}
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "### %s\n", grp.Actor.Canonical)
		EntryLine(w, grp.Actor, g)
		for _, r := range grp.Roles {
			EntryLine(w, r, g)
		}
	}
}
