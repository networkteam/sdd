package presenters

import (
	"fmt"
	"io"
	"strings"

	"github.com/networkteam/sdd/internal/model"
)

// renderAsList writes one entry-line per entry in the flat list. Reuses
// the canonical EntryLine format shared with `sdd list` and the section
// helpers in `sdd status` so output stays consistent across surfaces.
// When the section is ranked (FlatList.Scores populated and aligned),
// each line carries a `{score: X.XXX}` segment via EntryLineWithScore.
// by(date) leaves Scores nil, falling back to plain EntryLine — sort
// without per-entry rendering noise.
//
// When the section used expand(refs) (FlatList.RefExpansions populated and
// aligned), each entry's resolved outgoing refs render as indented sub-lines
// beneath it.
func renderAsList(w io.Writer, g *model.Graph, flat model.FlatList) {
	scored := len(flat.Scores) == len(flat.Entries) && len(flat.Scores) > 0
	expanded := len(flat.RefExpansions) == len(flat.Entries)
	for i, e := range flat.Entries {
		if scored {
			EntryLineWithScore(w, e, g, flat.Scores[i])
		} else {
			EntryLine(w, e, g)
		}
		if expanded {
			for _, ref := range flat.RefExpansions[i] {
				writeRefExpansion(w, ref)
			}
		}
	}
}

// writeRefExpansion renders one expand(refs) sub-line beneath its parent
// entry. The per-ref kind becomes the verb (grounded-in, builds-on, addresses,
// …); legacy bare-string refs (kind unknown) render with the generic verb
// `refs`. The referenced entry's derived status follows, then the optional
// per-ref desc as a quoted clause. Done-signal targets carry no status (they
// are terminal facts of execution) — matching how done signals render on
// every other surface, the status segment is simply omitted.
//
//	→ grounds 20260101-100000-d-cpt-aaa {status: active}
//	→ depends-on 20260101-100000-d-tac-bbb {status: closed-by <id>}: "why this ref exists"
func writeRefExpansion(w io.Writer, ref model.RefExpansion) {
	var sb strings.Builder
	sb.WriteString("    → ")
	sb.WriteString(refVerb(ref.Kind))
	sb.WriteString(" ")
	sb.WriteString(ref.ID)
	if s := FormatStatusTrail(ref.Status, ref.SupersedePath); s != "" {
		sb.WriteString(" ")
		sb.WriteString(s)
	}
	if ref.Desc != "" {
		sb.WriteString(`: "`)
		sb.WriteString(ref.Desc)
		sb.WriteString(`"`)
	}
	sb.WriteString("\n")
	fmt.Fprint(w, sb.String())
}

// refVerb maps a ref kind to its sub-line verb. Unknown (legacy bare-string)
// refs render with the generic `refs` verb so they stay legible without a
// captured kind; every capturable kind renders as its own verb.
func refVerb(k model.RefKind) string {
	if k == model.RefKindUnknown || k == "" {
		return "refs"
	}
	return string(k)
}
