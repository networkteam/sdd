package presenters

import (
	"fmt"
	"io"
	"strings"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// RenderShow writes the show output for a ShowResult. Each group renders:
// the primary entry at full detail, then an upstream section (summary lines),
// then a downstream section (summary lines). Groups are separated by "---".
func RenderShow(w io.Writer, result *query.ShowResult) {
	for i, g := range result.Groups {
		if i > 0 {
			fmt.Fprintln(w, "---")
			fmt.Fprintln(w)
		}
		renderShowGroup(w, result.Graph, g)
	}
}

func renderShowGroup(w io.Writer, graph *model.Graph, g query.ShowGroup) {
	// Primary entry: full content.
	fmt.Fprintf(w, "# %s\n\n", g.Primary.ID)
	WriteEntryFull(w, g.Primary, graph)

	if len(g.Upstream) > 0 {
		fmt.Fprintln(w, "## upstream:")
		for _, item := range g.Upstream {
			renderSummaryItem(w, graph, item, g.Primary.ID)
		}
		fmt.Fprintln(w)
	}

	if len(g.Downstream) > 0 {
		fmt.Fprintln(w, "## downstream:")
		for _, item := range g.Downstream {
			renderSummaryItem(w, graph, item, g.Primary.ID)
		}
		fmt.Fprintln(w)
	}
}

// renderSummaryItem renders a single summary line at the appropriate indent.
// Format: `<indent>- <relations> <id> <kind>? {status: S}?: "<summary>"`
// Kind renders as a plain qualifier after the ID (identity, not attribute).
func renderSummaryItem(w io.Writer, graph *model.Graph, item model.ShowTreeItem, primaryID string) {
	indent := strings.Repeat("  ", item.Depth)
	relations := strings.Join(item.Relations, ",")

	var sb strings.Builder
	sb.WriteString(item.Entry.ID)
	if k := kindForDisplay(item.Entry); k != "" {
		sb.WriteString(" ")
		sb.WriteString(string(k))
	}
	if s := FormatStatus(graph.DerivedStatus(item.Entry)); s != "" {
		sb.WriteString(" ")
		sb.WriteString(s)
	}
	idPart := sb.String()

	switch {
	case item.ShownAbove:
		if item.Entry.ID == primaryID {
			fmt.Fprintf(w, "%s- %s %s: (this entry)\n", indent, relations, idPart)
		} else {
			fmt.Fprintf(w, "%s- %s %s: (see above)\n", indent, relations, idPart)
		}
	case item.ShownBelow:
		fmt.Fprintf(w, "%s- %s %s: (see below)\n", indent, relations, idPart)
	default:
		summary := item.Entry.Summary
		if summary == "" {
			summary = firstSentence(item.Entry.Content)
		}
		fmt.Fprintf(w, "%s- %s %s: %q\n", indent, relations, idPart, summary)
	}

	if len(item.Truncated) > 0 {
		childIndent := strings.Repeat("  ", item.Depth+1)
		parts := make([]string, len(item.Truncated))
		for i, tr := range item.Truncated {
			rels := strings.Join(tr.Relations, ",")
			k := ""
			if tr.Kind != "" {
				k = " " + string(tr.Kind)
			}
			parts[i] = rels + " " + tr.ID + k
		}
		fmt.Fprintf(w, "%s[truncated: %s]\n", childIndent, strings.Join(parts, ", "))
	}
}

// kindForDisplay returns the kind to render for an entry. Decisions fall back
// to "directive" when Kind is empty (legacy default); other types show kind
// only when explicitly set. Presenter expansion for the full kind vocabulary
// lands in a later session.
func kindForDisplay(e *model.Entry) model.Kind {
	if e.Kind != "" {
		return e.Kind
	}
	if e.Type == model.TypeDecision {
		return model.KindDirective
	}
	return ""
}

// firstSentence extracts the first sentence from content as a fallback summary.
func firstSentence(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		content = content[:idx]
	}
	if len(content) > 120 {
		content = content[:117] + "..."
	}
	return content
}

// WriteEntryFull writes the full metadata and content of an entry. Stored
// frontmatter fields render first, then a "Derived:" section lists attributes
// computed from graph relationships (d-tac-3yi). The curly-brace inline
// notation (`{status: ...}`) is reserved for flat contexts like status, list,
// and summary chains — the labeled block here keeps the stored/derived split
// explicit without redundant wrapping.
func WriteEntryFull(w io.Writer, e *model.Entry, graph *model.Graph) {
	fmt.Fprintf(w, "ID:     %s\n", e.ID)
	fmt.Fprintf(w, "Type:   %s\n", e.TypeLabel())
	fmt.Fprintf(w, "Layer:  %s\n", e.LayerLabel())
	if e.Kind != "" && e.Kind != model.KindDirective {
		fmt.Fprintf(w, "Kind:   %s\n", e.Kind)
	}
	if e.Confidence != "" {
		fmt.Fprintf(w, "Conf:   %s\n", e.Confidence)
	}
	if len(e.Participants) > 0 {
		fmt.Fprintf(w, "Who:    %s\n", strings.Join(e.Participants, ", "))
	}
	if len(e.Refs) > 0 {
		fmt.Fprintf(w, "Refs:   %s\n", strings.Join(e.Refs, ", "))
	}
	if len(e.Closes) > 0 {
		fmt.Fprintf(w, "Closes: %s\n", strings.Join(e.Closes, ", "))
	}
	if len(e.Supersedes) > 0 {
		fmt.Fprintf(w, "Supersedes: %s\n", strings.Join(e.Supersedes, ", "))
	}
	for _, a := range e.Attachments {
		fmt.Fprintf(w, "Attachment: %s\n", a)
	}
	if len(e.Warnings) > 0 {
		for _, warn := range e.Warnings {
			fmt.Fprintf(w, "⚠ %s\n", warn.Message)
		}
	}
	fmt.Fprintf(w, "Time:   %s\n", e.Time.Format("2006-01-02 15:04:05"))
	writeDerivedSection(w, e, graph)
	fmt.Fprintln(w)
	fmt.Fprintln(w, e.Content)
	fmt.Fprintln(w)
}

// writeDerivedSection writes a "Derived:" block with graph-computed attributes,
// omitted entirely when the entry has no derived state (e.g. actions).
func writeDerivedSection(w io.Writer, e *model.Entry, graph *model.Graph) {
	status := graph.DerivedStatus(e)
	if status.Kind == model.StatusNone {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Derived:")
	fmt.Fprintf(w, "  Status: %s\n", formatStatusValue(status))
}

// formatStatusValue renders a Status as a plain value (no curly braces) for
// use inside the labeled "Derived:" block. Compound states use a space
// separator: "closed-by <id>", "superseded-by <id>".
func formatStatusValue(s model.Status) string {
	if s.By != "" {
		return string(s.Kind) + " " + s.By
	}
	return string(s.Kind)
}
