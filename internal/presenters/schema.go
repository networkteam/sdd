package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/model"
)

// RenderSchemaError writes a user-facing explanation of a compatibility
// failure to w. The CompatibilityResult.Reason is already human-readable;
// this presenter frames it with a fixed prefix and exit hint so output
// reads the same across every write command.
func RenderSchemaError(w io.Writer, compat model.CompatibilityResult) {
	if compat.Compatible {
		return
	}
	fmt.Fprintln(w, "sdd: refusing to write — graph incompatible with this binary")
	fmt.Fprintf(w, "  %s\n", compat.Reason)
}
