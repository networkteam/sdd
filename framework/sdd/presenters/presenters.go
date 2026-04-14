// Package presenters renders structured query results as text for CLI
// output. Presenters take pure data from finders plus an io.Writer and
// formatting parameters; they have no IO of their own beyond the writer.
//
// The package isolates the view layer from finders (data) and from CLI
// command plumbing — separating concerns so each can be tested in
// isolation.
package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/resonance/framework/sdd/model"
)

// EntryLine writes a single entry summary line — used by status, list, and
// other surfaces that show entries in a flat list.
func EntryLine(w io.Writer, e *model.Entry) {
	conf := ""
	if e.Confidence != "" {
		conf = fmt.Sprintf(" [%s]", e.Confidence)
	}
	desc := e.Summary
	if desc == "" {
		desc = e.ShortContent(200)
	}
	fmt.Fprintf(w, "  %s  %-8s %-12s%s  %s\n",
		e.ID, e.TypeLabel(), e.LayerLabel(), conf, desc)
}

// LayerOrder returns the display order for layers (strategic → process).
func LayerOrder() []model.Layer {
	return []model.Layer{
		model.LayerStrategic,
		model.LayerConceptual,
		model.LayerTactical,
		model.LayerOperational,
		model.LayerProcess,
	}
}

// GroupByLayer groups entries by layer for display. Returns a map from
// layer to entries; iterate with LayerOrder to render in canonical order.
func GroupByLayer(entries []*model.Entry) map[model.Layer][]*model.Entry {
	m := make(map[model.Layer][]*model.Entry)
	for _, e := range entries {
		m[e.Layer] = append(m[e.Layer], e)
	}
	return m
}
