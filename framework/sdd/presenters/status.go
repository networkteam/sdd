package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/query"
)

// RenderStatus writes the status view: top-line counts, then sections for
// contracts, plans, active decisions, open signals, and recent actions.
// Each section is grouped by layer using LayerOrder.
func RenderStatus(w io.Writer, result *query.StatusResult) {
	g := result.Graph
	decisions := g.Filter(model.GraphFilter{Type: model.TypeDecision})
	signals := g.Filter(model.GraphFilter{Type: model.TypeSignal})
	actions := g.Filter(model.GraphFilter{Type: model.TypeAction})
	fmt.Fprintf(w, "Graph: %d entries (%d decisions, %d signals, %d actions)\n\n",
		len(g.Entries), len(decisions), len(signals), len(actions))

	renderLayeredSection(w, "Contracts", result.Contracts)
	renderLayeredSection(w, "Plans", result.Plans)
	renderLayeredSection(w, "Active Decisions", result.Active)
	renderFlatSection(w, "Open Signals", result.Open)
	renderFlatSection(w, "Recent Actions", result.Recent)
}

func renderLayeredSection(w io.Writer, title string, entries []*model.Entry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(w, "## %s\n\n", title)
	byLayer := GroupByLayer(entries)
	for _, layer := range LayerOrder() {
		group, ok := byLayer[layer]
		if !ok {
			continue
		}
		fmt.Fprintf(w, "### %s\n", layer)
		for _, e := range group {
			EntryLine(w, e)
		}
		fmt.Fprintln(w)
	}
}

func renderFlatSection(w io.Writer, title string, entries []*model.Entry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(w, "## %s\n\n", title)
	for _, e := range entries {
		EntryLine(w, e)
	}
	fmt.Fprintln(w)
}
