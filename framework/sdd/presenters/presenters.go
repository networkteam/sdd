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
	"strings"

	"github.com/networkteam/resonance/framework/sdd/model"
)

// EntryLine writes a single entry summary line — used by status, list, and
// other surfaces that show entries in a flat list.
//
// Format: `<id> <layer> <kind>? <type> [confidence: <conf>]? (<participants>) <summary>`
// Kind is present only when the entry has one (decisions today; format accommodates
// other types in the future). Confidence is present only when set (decisions + signals).
// Participants are always present — empty is rendered as `()`.
func EntryLine(w io.Writer, e *model.Entry) {
	var sb strings.Builder
	sb.WriteString("  ")
	sb.WriteString(e.ID)
	sb.WriteString(" ")
	sb.WriteString(e.LayerLabel())
	if e.Kind != "" {
		sb.WriteString(" ")
		sb.WriteString(string(e.Kind))
	}
	sb.WriteString(" ")
	sb.WriteString(e.TypeLabel())
	if e.Confidence != "" {
		sb.WriteString(" [confidence: ")
		sb.WriteString(e.Confidence)
		sb.WriteString("]")
	}
	sb.WriteString(" (")
	sb.WriteString(strings.Join(e.Participants, ", "))
	sb.WriteString(") ")
	desc := e.Summary
	if desc == "" {
		desc = e.ShortContent(200)
	}
	sb.WriteString(desc)
	sb.WriteString("\n")
	fmt.Fprint(w, sb.String())
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
