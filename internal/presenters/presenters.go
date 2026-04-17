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

	"github.com/networkteam/sdd/internal/model"
)

// EntryLine writes a single entry summary line — used by status, list, and
// other surfaces that show entries in a flat list.
//
// Format: `<id> <layer> <kind>? <type> [confidence: <conf>]? (<participants>) {status: <status>}? <summary>`
// Kind renders as a qualifier alongside layer/type — it's identity, not an
// attribute (d-cpt-omm's two-type redesign makes every entry carry a kind).
// Square brackets denote stored attributes (today: confidence); curly braces
// denote derived attributes computed from graph relationships (d-tac-3yi).
// Participants are always present — empty is rendered as `()`. Status is
// present for signals and decisions; omitted for actions.
func EntryLine(w io.Writer, e *model.Entry, g *model.Graph) {
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
		sb.WriteString(" ")
		sb.WriteString(FormatConfidence(e.Confidence))
	}
	sb.WriteString(" (")
	sb.WriteString(strings.Join(e.Participants, ", "))
	sb.WriteString(")")
	if s := FormatStatus(g.DerivedStatus(e)); s != "" {
		sb.WriteString(" ")
		sb.WriteString(s)
	}
	sb.WriteString(" ")
	desc := e.Summary
	if desc == "" {
		desc = e.ShortContent(200)
	}
	sb.WriteString(desc)
	sb.WriteString("\n")
	fmt.Fprint(w, sb.String())
}

// FormatConfidence renders a stored confidence attribute in square-bracket notation.
func FormatConfidence(c string) string {
	return "[confidence: " + c + "]"
}

// FormatStatus renders a derived status in curly-brace notation. Returns the
// empty string for StatusNone (actions) so callers can omit the attribute.
// Compound states use a space separator in the value (`closed-by <id>`) to
// avoid ambiguity with the outer `{key: value}` delimiter.
func FormatStatus(s model.Status) string {
	if s.Kind == model.StatusNone {
		return ""
	}
	if s.By != "" {
		return "{status: " + string(s.Kind) + " " + s.By + "}"
	}
	return "{status: " + string(s.Kind) + "}"
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
