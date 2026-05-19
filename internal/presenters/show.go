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
// Format: `<indent>- <relations>(<refKind>)? <id> <kind>? {status: S}?: "<summary>"`
// Per-ref kind decorates the relation when one of the relations is "refs"
// or "refd-by" and the edge carries kind metadata; closes/supersedes (and
// their inverses) never carry kind. When a desc is present it renders as
// an indented sub-line beneath the summary.
func renderSummaryItem(w io.Writer, graph *model.Graph, item model.ShowTreeItem, primaryID string) {
	indent := strings.Repeat("  ", item.Depth)
	relations := formatRelations(item.Relations, item.RefKind)

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

	if item.RefDesc != "" {
		descIndent := strings.Repeat("  ", item.Depth+1)
		fmt.Fprintf(w, "%sdesc: %s\n", descIndent, item.RefDesc)
	}

	if len(item.Truncated) > 0 {
		childIndent := strings.Repeat("  ", item.Depth+1)
		parts := make([]string, len(item.Truncated))
		for i, tr := range item.Truncated {
			rels := formatRelations(tr.Relations, tr.RefKind)
			k := ""
			if tr.Kind != "" {
				k = " " + string(tr.Kind)
			}
			parts[i] = rels + " " + tr.ID + k
		}
		fmt.Fprintf(w, "%s[truncated: %s]\n", childIndent, strings.Join(parts, ", "))
	}
}

// formatRelations builds the comma-joined relation label and decorates
// the "refs" or "refd-by" entry with its per-ref kind in parens when set.
// Other relations are left unannotated — closes/supersedes (and inverses)
// carry uniform meaning and never accept per-edge metadata.
func formatRelations(relations []string, refKind model.RefKind) string {
	if refKind == "" {
		return strings.Join(relations, ",")
	}
	out := make([]string, len(relations))
	for i, r := range relations {
		if r == "refs" || r == "refd-by" {
			out[i] = r + " (" + string(refKind) + ")"
		} else {
			out[i] = r
		}
	}
	return strings.Join(out, ",")
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

// writeRefsBlock renders the metadata-block refs section. Single-kind no-desc
// runs collapse to the legacy single-line "Refs: id1, id2" shape; once any ref
// carries a kind or desc, each ref renders on its own line so readers can
// scan kind and desc per reference.
func writeRefsBlock(w io.Writer, refs []model.Ref) {
	allLegacy := true
	for _, r := range refs {
		if r.Kind != "" && r.Kind != model.RefKindUnknown {
			allLegacy = false
			break
		}
		if r.Desc != "" {
			allLegacy = false
			break
		}
	}
	if allLegacy {
		fmt.Fprintf(w, "Refs:   %s\n", strings.Join(model.RefIDs(refs), ", "))
		return
	}
	fmt.Fprintln(w, "Refs:")
	for _, r := range refs {
		kind := string(r.Kind)
		if kind == "" {
			kind = "(unset)"
		}
		fmt.Fprintf(w, "  - %s (%s)\n", r.ID, kind)
		if r.Desc != "" {
			fmt.Fprintf(w, "    desc: %s\n", r.Desc)
		}
	}
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
		writeRefsBlock(w, e.Refs)
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
	if e.Summary != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Summary:")
		fmt.Fprintln(w, e.Summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, e.Content)
	fmt.Fprintln(w)
}

// writeDerivedSection writes a "Derived:" block with graph-computed attributes,
// omitted entirely when the entry has no derived state (e.g. actions). Status
// always renders when present; Topics renders when the entry has a non-empty
// effective topic set (inline ∪ annotation memberships).
func writeDerivedSection(w io.Writer, e *model.Entry, graph *model.Graph) {
	status := graph.DerivedStatus(e)
	topics := graph.EffectiveTopics(e)
	if status.Kind == model.StatusNone && len(topics) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Derived:")
	if status.Kind != model.StatusNone {
		fmt.Fprintf(w, "  Status: %s\n", formatStatusValue(status))
	}
	if len(topics) > 0 {
		labels := make([]string, 0, len(topics))
		for _, t := range topics {
			labels = append(labels, t.String())
		}
		fmt.Fprintf(w, "  Topics: %s\n", strings.Join(labels, ", "))
	}
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
