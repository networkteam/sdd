package presenters

import (
	"fmt"
	"io"
	"strings"

	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/query"
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
		renderShowGroup(w, g)
	}
}

func renderShowGroup(w io.Writer, g query.ShowGroup) {
	// Primary entry: full content.
	fmt.Fprintf(w, "# %s\n\n", g.Primary.ID)
	WriteEntryFull(w, g.Primary)

	if len(g.Upstream) > 0 {
		fmt.Fprintln(w, "## upstream:")
		for _, item := range g.Upstream {
			renderSummaryItem(w, item, g.Primary.ID)
		}
		fmt.Fprintln(w)
	}

	if len(g.Downstream) > 0 {
		fmt.Fprintln(w, "## downstream:")
		for _, item := range g.Downstream {
			renderSummaryItem(w, item, g.Primary.ID)
		}
		fmt.Fprintln(w)
	}
}

// renderSummaryItem renders a single summary line at the appropriate indent.
func renderSummaryItem(w io.Writer, item model.ShowTreeItem, primaryID string) {
	indent := strings.Repeat("  ", item.Depth)
	relations := strings.Join(item.Relations, ",")
	kindStr := kindLabel(item.Entry)

	var idPart string
	if kindStr != "" {
		idPart = fmt.Sprintf("%s (%s)", item.Entry.ID, kindStr)
	} else {
		idPart = item.Entry.ID
	}

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

	if len(item.TruncatedIDs) > 0 {
		childIndent := strings.Repeat("  ", item.Depth+1)
		fmt.Fprintf(w, "%s[truncated: %s]\n", childIndent, strings.Join(item.TruncatedIDs, ", "))
	}
}

// kindLabel returns the kind for display. Decisions always show their kind
// (defaulting to "directive" when empty). Non-decisions show kind only if set.
func kindLabel(e *model.Entry) string {
	if e.Kind != "" {
		return string(e.Kind)
	}
	if e.Type == model.TypeDecision {
		return string(model.KindDirective)
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

// WriteEntryFull writes the full metadata and content of an entry.
func WriteEntryFull(w io.Writer, e *model.Entry) {
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
	fmt.Fprintln(w)
	fmt.Fprintln(w, e.Content)
	fmt.Fprintln(w)
}
