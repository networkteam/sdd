package presenters

import (
	"fmt"
	"io"
	"strings"

	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/query"
)

// RenderShow writes hierarchical output for a Show result. Groups are
// separated by "---". Within a group, depth controls heading level
// (depth 0 = #, depth 1 = ##, etc.). Items flagged ShownAbove/ShownBelow
// render the heading plus the appropriate marker only.
func RenderShow(w io.Writer, result *query.ShowResult) {
	for i, g := range result.Groups {
		if i > 0 {
			fmt.Fprintln(w, "---")
			fmt.Fprintln(w)
		}
		for _, item := range g.Items {
			renderShowItem(w, item)
		}
	}
}

func renderShowItem(w io.Writer, item query.ShowItem) {
	hashes := strings.Repeat("#", item.Depth+1)
	if item.Relation != "" {
		fmt.Fprintf(w, "%s %s: %s\n", hashes, item.Relation, item.Entry.ID)
	} else {
		fmt.Fprintf(w, "%s %s\n", hashes, item.Entry.ID)
	}
	fmt.Fprintln(w)

	switch {
	case item.ShownAbove:
		fmt.Fprintln(w, "(shown above)")
		fmt.Fprintln(w)
	case item.ShownBelow:
		fmt.Fprintln(w, "(shown below)")
		fmt.Fprintln(w)
	default:
		WriteEntryFull(w, item.Entry)
	}
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
