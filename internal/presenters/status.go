package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// RenderStatus writes the status view: top-line counts, then one section per
// decision kind (Aspirations, Contracts, Plans, Activities, Directives),
// followed by the signal sections (Gaps and Questions, Recent Insights,
// Recent Done Signals). Decision-kind sections are grouped by layer; signal
// sections are flat activity streams. Empty sections are suppressed — the
// section header implies membership, so "open" / "active" prefixes are not
// needed on the headers.
func RenderStatus(w io.Writer, result *query.StatusResult) {
	g := result.Graph
	decisions := g.Filter(model.GraphFilter{Type: model.TypeDecision})
	signals := g.Filter(model.GraphFilter{Type: model.TypeSignal})
	fmt.Fprintf(w, "Graph: %d entries (%d decisions, %d signals)\n\n",
		len(g.Entries), len(decisions), len(signals))

	renderLayeredSection(w, g, "Aspirations", result.Aspirations)
	renderLayeredSection(w, g, "Contracts", result.Contracts)
	renderLayeredSection(w, g, "Plans", result.Plans)
	renderLayeredSection(w, g, "Activities", result.Activities)
	renderLayeredSection(w, g, "Directives", result.Directives)
	renderFlatSection(w, g, "Gaps and Questions", result.Open)
	renderFlatSection(w, g, "Recent Insights", result.Insights)
	renderFlatSection(w, g, "Recent Done Signals", result.Recent)
}

func renderLayeredSection(w io.Writer, g *model.Graph, title string, entries []*model.Entry) {
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
			EntryLine(w, e, g)
		}
		fmt.Fprintln(w)
	}
}

func renderFlatSection(w io.Writer, g *model.Graph, title string, entries []*model.Entry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(w, "## %s\n\n", title)
	for _, e := range entries {
		EntryLine(w, e, g)
	}
	fmt.Fprintln(w)
}
