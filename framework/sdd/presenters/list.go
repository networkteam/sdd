package presenters

import (
	"io"

	"github.com/networkteam/resonance/framework/sdd/query"
)

// RenderList writes one EntryLine per matched entry.
func RenderList(w io.Writer, result *query.ListResult) {
	for _, e := range result.Entries {
		EntryLine(w, e)
	}
}
