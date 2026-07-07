package finders

import "github.com/networkteam/sdd/internal/model"

// expandRefs resolves each entry's outgoing refs into RefExpansion rows for
// expand(refs) output. The result is parallel to entries: result[i] holds
// the sub-line rows for entries[i] in the entry's stored ref order. Each row
// carries the ref's stored kind (the presenter renders it as the verb), the
// referenced entry's derived status resolved from the loaded graph, and the
// optional per-ref desc.
//
// A ref whose target is absent from the graph (dangling — lint surfaces it
// elsewhere) keeps a zero Status, which renders without a status segment
// rather than panicking on a nil lookup.
//
// When inactiveOnly is true, only refs whose target is currently inactive
// survive — the lean catch-up mode (expand(refs(inactive))). "Inactive" is
// the exact inverse of the `active` view filter: closed, superseded, or a
// role whose bound actor chain is no longer active. This is a current-state
// filter, not a changelog — it reports the referent's state now, not whether
// it transitioned since being referenced.
func expandRefs(g *model.Graph, entries []*model.Entry, inactiveOnly bool) [][]model.RefExpansion {
	out := make([][]model.RefExpansion, len(entries))
	for i, e := range entries {
		var rows []model.RefExpansion
		for _, ref := range e.Refs {
			var status model.Status
			var unresolvedRepo string
			if repoID, _, isCross := model.SplitCrossRepoID(ref.ID); isCross {
				// Cross-repo target: no cached remote graph is wired in, so
				// the status stays unknown and the row carries the
				// unresolved marker. Remote resolution plugs in here when
				// the multi-graph source lands.
				unresolvedRepo = repoID
			} else if target := g.ByID[ref.ID]; target != nil {
				status = g.DerivedStatus(target)
			}
			if inactiveOnly && !isInactiveStatus(status.Kind) {
				continue
			}
			// For a superseded target, carry the trail to the live head so
			// the presenter can render the supersede path; nil otherwise.
			var supersedePath []string
			if status.Kind == model.StatusSupersededBy {
				supersedePath = g.ResolveRef(ref.ID).Path()
			}
			rows = append(rows, model.RefExpansion{
				Kind:           ref.Kind,
				ID:             ref.ID,
				Status:         status,
				Desc:           ref.Desc,
				SupersedePath:  supersedePath,
				UnresolvedRepo: unresolvedRepo,
			})
		}
		out[i] = rows
	}
	return out
}

// isInactiveStatus reports whether a derived status means the referenced
// entry is no longer active — the inverse of what the `active` view filter
// keeps. The filter drops closed and superseded entries plus roles whose
// derived status isn't active (cascade-closed when the bound actor is
// retired, cascade-orphan when the canonical resolves to no chain). Matching
// the same set here keeps expand(refs(inactive)) a true complement of
// active: a ref the active view would drop shows up in the inactive view,
// never falling through both. Open signals, active decisions, and terminal
// done signals (StatusNone) are active/in-scope and don't match.
func isInactiveStatus(k model.StatusKind) bool {
	switch k {
	case model.StatusClosedBy, model.StatusSupersededBy,
		model.StatusCascadeClosedBy, model.StatusCascadeOrphan:
		return true
	default:
		return false
	}
}
