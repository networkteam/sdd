package presenters

import (
	"fmt"
	"io"
	"strings"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// RenderSearch writes one section per ranked entry: a header line that
// matches `sdd list` shape, then one citation line per entry-citation
// (capped by SearchQuery.MaxCitationsPerEntry on the finder side).
//
// Render format (single-citation case):
//
//	<id> <layer> <kind>? <type> [confidence: ?]? (<participants>) {status: ?}? <summary>
//	  ↳ 100%  ·  Breadcrumb > Chain  ·  <snippet>
//
// Render format (multi-citation case):
//
//	<id> ...
//	  ↳ 100%  ·  Summary  ·  <snippet>
//	  ↳  91%  ·  Approach > Storage  ·  <snippet>
//	  ↳  87%  ·  ... [attachment: ...]  ·  <snippet>
//
// The percentage is each citation's score normalized against the
// strongest score in the result set — the top citation everywhere
// renders 100%, others scale relative. This is honest about the cross-
// entry ranking without making any claims about absolute "confidence"
// (cosine values aren't calibrated across embedders, and text-mode
// match counts and hybrid RRF scores live on different scales than
// cosine — but ALL citations within a single result set share whatever
// scale the active mode uses).
func RenderSearch(w io.Writer, result *query.SearchResult, g *model.Graph) {
	if result == nil {
		return
	}
	if len(result.Entries) == 0 {
		fmt.Fprintln(w, "  (no matches)")
		return
	}

	maxScore := globalMaxCitationScore(result)
	for _, se := range result.Entries {
		EntryLine(w, se.Entry, g)
		for _, c := range se.Citations {
			renderCitation(w, c, maxScore)
		}
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

// globalMaxCitationScore returns the largest score across every
// citation in the result. Used to normalize per-citation scores into a
// relative percentage at render time. Returns 0 when the result is
// empty (the caller short-circuits this case before calling, but the
// guard keeps the helper total).
func globalMaxCitationScore(result *query.SearchResult) float32 {
	var max float32
	for _, se := range result.Entries {
		for _, c := range se.Citations {
			if c.Score > max {
				max = c.Score
			}
		}
	}
	return max
}

func renderCitation(w io.Writer, c query.Citation, maxScore float32) {
	var b strings.Builder
	b.WriteString("    ↳ ")
	b.WriteString(formatRelativeScore(c.Score, maxScore))
	b.WriteString("  ·  ")
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

// formatRelativeScore formats a citation's score as a percentage of the
// result-set max, right-padded to 4 chars so the citation lines align
// vertically (`100%`, ` 91%`, ` 5%` all occupy the same width). When
// max is zero (no scores recorded), renders blank padding so the
// pipeline doesn't break.
func formatRelativeScore(score, max float32) string {
	if max <= 0 {
		return "    "
	}
	pct := int((score / max) * 100)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf("%3d%%", pct)
}
