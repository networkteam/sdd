package model

import "github.com/networkteam/sdd/pkg/application/types"

// GraphDiff is what changed between two revisions of one graph: entries
// added, and entries present in both whose derived status (its kind or the
// entry it names) or stored summary differs. Each list follows the later graph's time order.
type GraphDiff struct {
	Added          []types.AddedEntry
	StatusChanged  []string
	SummaryChanged []string
}

// DiffGraphs compares from with the later graph to. Entries are immutable
// apart from their summary, so removed entries are not tracked.
func DiffGraphs(from, to *Graph) GraphDiff {
	var diff GraphDiff
	for _, e := range to.Entries {
		before, ok := from.ByID[e.ID]
		if !ok {
			diff.Added = append(diff.Added, types.AddedEntry{ID: e.ID, RefIDs: referencedIDs(e)})
			continue
		}
		if from.DerivedStatus(before) != to.DerivedStatus(e) {
			diff.StatusChanged = append(diff.StatusChanged, e.ID)
		}
		if before.Summary != e.Summary {
			diff.SummaryChanged = append(diff.SummaryChanged, e.ID)
		}
	}
	return diff
}

// referencedIDs lists the IDs e points at through refs, closes and
// supersedes, each once in declaration order.
func referencedIDs(e *Entry) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, r := range e.Refs {
		add(r.ID)
	}
	for _, id := range e.Closes {
		add(id)
	}
	for _, id := range e.Supersedes {
		add(id)
	}
	return out
}
