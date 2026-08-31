package engine

import (
	"fmt"
	"strings"

	"github.com/networkteam/sdd/internal/truncate"
)

// renderCuts is the engine-owned cuts lane: every bound that fired on this
// serve, what remains, and the pull for the remainder. It rides as its own
// lane beside the draft lane, so authored instruction text never carries a
// notice and a template cannot swallow one (d-tac-qwc).
func renderCuts(cuts []truncate.Cut) string {
	var b strings.Builder
	b.WriteString("Cut for size:")
	for _, c := range cuts {
		if c.Dropped > 0 {
			fmt.Fprintf(&b, "\n- %s: %d of %d items dropped", c.Part, c.Dropped, c.Total)
		} else {
			fmt.Fprintf(&b, "\n- %s: kept %d of %d bytes", c.Part, c.KeptBytes, c.TotalBytes)
		}
		if c.Pull != "" {
			fmt.Fprintf(&b, " — pull the rest: %s", c.Pull)
		}
	}
	return b.String()
}
