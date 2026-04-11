package sdd

import (
	"fmt"
	"io"
	"strings"
)

// refTreeItem represents an entry at a specific depth in the ref tree.
type refTreeItem struct {
	entry    *Entry
	depth    int
	relation string // "", "ref", "closes", "supersedes", "downstream"
}

// buildRefTree walks the reference chain of an entry in pre-order, tracking depth.
// The root entry is at depth 0, its direct refs/closes/supersedes at depth 1, etc.
// When an entry was already rendered, is a future primary, or was already visited
// in this tree walk, it appears in the result but its subtree is not traversed.
func buildRefTree(g *Graph, id string, depth int, relation string, visited map[string]bool, rendered map[string]bool, primaries map[string]bool) []refTreeItem {
	e, ok := g.ByID[id]
	if !ok {
		return nil
	}

	result := []refTreeItem{{entry: e, depth: depth, relation: relation}}

	if visited[id] || rendered[id] || (primaries[id] && depth > 0) {
		// Already walked, already rendered, or will be shown as its own primary — don't recurse
		return result
	}
	visited[id] = true

	for _, ref := range e.Refs {
		result = append(result, buildRefTree(g, ref, depth+1, "ref", visited, rendered, primaries)...)
	}
	for _, c := range e.Closes {
		result = append(result, buildRefTree(g, c, depth+1, "closes", visited, rendered, primaries)...)
	}
	for _, s := range e.Supersedes {
		result = append(result, buildRefTree(g, s, depth+1, "supersedes", visited, rendered, primaries)...)
	}
	return result
}

// RenderShow writes hierarchical output for the given entry IDs.
// Primary entries (requested IDs) get # headers, their refs/closes/supersedes nest
// with increasing heading depth. Deduped entries print only the heading plus
// "(shown above)". Entries that are future primaries print "(shown below)".
// In downstream mode, the target is the primary and downstream entries are flat at ##.
func RenderShow(w io.Writer, g *Graph, ids []string, downstream bool) error {
	seen := make(map[string]bool)

	// Build set of primary IDs so we can detect "shown below" cases
	primaries := make(map[string]bool, len(ids))
	for _, id := range ids {
		primaries[id] = true
	}

	for i, id := range ids {
		if i > 0 {
			fmt.Fprintln(w, "---")
			fmt.Fprintln(w)
		}

		if downstream {
			target, ok := g.ByID[id]
			if !ok {
				return fmt.Errorf("entry not found: %s", id)
			}
			renderEntry(w, target, 0, "", seen, primaries)
			for _, e := range g.Downstream(id) {
				renderEntry(w, e, 1, "downstream", seen, primaries)
			}
		} else {
			if _, ok := g.ByID[id]; !ok {
				return fmt.Errorf("entry not found: %s", id)
			}
			tree := buildRefTree(g, id, 0, "", make(map[string]bool), seen, primaries)
			if len(tree) == 0 {
				return fmt.Errorf("entry not found: %s", id)
			}
			for _, item := range tree {
				renderEntry(w, item.entry, item.depth, item.relation, seen, primaries)
			}
		}
	}
	return nil
}

// renderEntry writes a single entry with a markdown heading at the given depth.
// depth 0 = #, depth 1 = ##, etc. label is prepended to the ID in the heading
// (e.g. "ref", "closes", "supersedes", "downstream") — empty for primaries.
func renderEntry(w io.Writer, e *Entry, depth int, label string, seen map[string]bool, primaries map[string]bool) {
	hashes := strings.Repeat("#", depth+1)

	if label != "" {
		fmt.Fprintf(w, "%s %s: %s\n", hashes, label, e.ID)
	} else {
		fmt.Fprintf(w, "%s %s\n", hashes, e.ID)
	}
	fmt.Fprintln(w)

	if seen[e.ID] {
		fmt.Fprintln(w, "(shown above)")
		fmt.Fprintln(w)
	} else if primaries[e.ID] && depth > 0 {
		fmt.Fprintln(w, "(shown below)")
		fmt.Fprintln(w)
	} else {
		WriteEntryFull(w, e)
		seen[e.ID] = true
	}
}

// WriteEntryFull writes the full metadata and content of an entry.
func WriteEntryFull(w io.Writer, e *Entry) {
	fmt.Fprintf(w, "ID:     %s\n", e.ID)
	fmt.Fprintf(w, "Type:   %s\n", e.TypeLabel())
	fmt.Fprintf(w, "Layer:  %s\n", e.LayerLabel())
	if e.Kind != "" && e.Kind != KindDirective {
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
