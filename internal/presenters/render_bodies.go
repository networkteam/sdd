package presenters

import (
	"fmt"
	"io"
	"strings"

	"github.com/networkteam/sdd/internal/mdcompose"
	"github.com/networkteam/sdd/internal/model"
)

// renderAsBodies writes each entry's full body into the document's heading
// hierarchy at entryLevel — one level below the section header, so a served
// body never outranks the structure serving it (d-cpt-5wv).
//
// Per entry the render emits an identity header and then the body. The summary
// sentence is deliberately absent: it derives from the body, and serving both
// would say the same thing twice. A body that opens with a heading contributes
// that heading as the entry's title, with the identity line as its byline; a
// body opening with prose takes the identity line as its header.
func renderAsBodies(w io.Writer, bodies model.Bodies, entryLevel int) {
	marker := strings.Repeat("#", entryLevel)
	for i, e := range bodies.Entries {
		if i > 0 {
			fmt.Fprintln(w)
		}
		identity := bodyIdentity(e)
		title, body := mdcompose.SplitLeadingHeading(strings.TrimSpace(e.Content))
		if title != "" {
			fmt.Fprintf(w, "%s %s\n\n%s\n", marker, title, identity)
		} else {
			fmt.Fprintf(w, "%s %s\n", marker, identity)
		}
		if body = strings.TrimSpace(mdcompose.DemoteTo(body, entryLevel+1)); body != "" {
			fmt.Fprintf(w, "\n%s\n", body)
		}
	}
	if bodies.Dropped > 0 {
		fmt.Fprintln(w)
		writeSectionCut(w, bodies.Dropped, bodies.Pull)
	}
}

// bodyIdentity renders an entry's identity line: the full ID a reader cites or
// passes to a tool, plus the layer/kind/type words that place it in the graph.
func bodyIdentity(e *model.Entry) string {
	words := []string{e.LayerLabel()}
	if e.Kind != "" {
		words = append(words, string(e.Kind))
	}
	words = append(words, e.TypeLabel())
	return fmt.Sprintf("`%s` — %s", e.ID, strings.Join(words, " "))
}
