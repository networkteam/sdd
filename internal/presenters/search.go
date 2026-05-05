package presenters

import (
	"fmt"
	"io"
	"strings"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// RenderSearch writes one section per ranked entry: a header line that
// matches `sdd list` shape, then a citation line carrying the breadcrumb
// (when present) and a snippet of the matched chunk.
//
// Render format:
//
//   <id> <layer> <kind>? <type> [confidence: ?]? (<participants>) {status: ?}? <summary>
//     ↳ Breadcrumb > Chain > Here  ·  <snippet>
//
// The leading "↳" marks the citation as derived from the entry, not as a
// separate entry of its own. When breadcrumb is empty the citation
// renders as just the snippet (with a leading "↳" still). When the
// citation comes from an attachment, the source path renders alongside
// the breadcrumb.
func RenderSearch(w io.Writer, result *query.SearchResult, g *model.Graph) {
	if result == nil {
		return
	}
	if len(result.Entries) == 0 {
		fmt.Fprintln(w, "  (no matches)")
		return
	}
	for _, se := range result.Entries {
		EntryLine(w, se.Entry, g)
		renderCitation(w, se.Citation)
	}
}

// FormatSearchCapability renders the capability suffix shown in
// `sdd status`'s header: `text` when only text mode is available;
// `vector,text` when a vector embedder is configured. Used by callers
// that want to surface the search surface alongside other status fields.
func FormatSearchCapability(vectorAvailable bool) string {
	if vectorAvailable {
		return "vector,text"
	}
	return "text"
}

func renderCitation(w io.Writer, c query.Citation) {
	var b strings.Builder
	b.WriteString("    ↳ ")
	if c.IsSummary {
		b.WriteString("Summary")
	} else if len(c.Breadcrumb) > 0 {
		b.WriteString(strings.Join(c.Breadcrumb, " > "))
	} else {
		b.WriteString("Body")
	}
	if c.IsAttachment && c.SourceAttachmentPath != "" {
		b.WriteString(" [attachment: ")
		b.WriteString(c.SourceAttachmentPath)
		b.WriteString("]")
	}
	if c.Snippet != "" {
		b.WriteString("  ·  ")
		b.WriteString(c.Snippet)
	}
	b.WriteString("\n")
	fmt.Fprint(w, b.String())
}
