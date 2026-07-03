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
	"math"
	"strings"

	"github.com/networkteam/sdd/internal/model"
)

// EntryLine writes a single entry summary line — used by status, list, and
// other surfaces that show entries in a flat list.
//
// Format: `<id> <layer> <kind>? <type> [confidence: <conf>]? [intent: guiding]? (<participants>) {status: <status>}? {score: <score>}? <topics>? <summary>`
// Kind renders as a qualifier alongside layer/type — it's identity, not an
// attribute (d-cpt-omm's two-type redesign makes every entry carry a kind).
// Square brackets denote stored attributes (confidence; intent, but only its
// guiding value — pending/unspecified stay quiet and settled shows via status);
// curly braces
// denote derived attributes computed from graph relationships (d-tac-3yi);
// angle brackets denote topic membership (also derived — inline topics merged
// with annotation declarations). Participants are always present — empty is
// rendered as `()`. Status is present for signals and decisions; omitted for
// done signals (terminal). Topics are omitted entirely when the effective set
// is empty. Score is per-rendering, computed by a rank algorithm; emitted
// only via EntryLineWithScore (slice 3 — d-tac-uww).
func EntryLine(w io.Writer, e *model.Entry, g *model.Graph) {
	entryLineCore(w, e, g, math.NaN())
}

// EntryLineWithScore is EntryLine plus a `{score: X.XXX}` segment after
// status. Used by the as-list renderer when the section was ranked.
func EntryLineWithScore(w io.Writer, e *model.Entry, g *model.Graph, score float64) {
	entryLineCore(w, e, g, score)
}

// EntryLineBrief writes the compact entry line the `brief` layout modifier
// selects: identity qualifiers plus the first summary sentence, with every
// attribute segment (confidence, intent, participants, status, score,
// topics) dropped. Built for injection surfaces where full lines cost
// context without informing the decision at hand.
//
// Format: `<id> <layer> <kind>? <type> <first summary sentence>`
func EntryLineBrief(w io.Writer, e *model.Entry, g *model.Graph) {
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
	sb.WriteString(" ")
	sb.WriteString(e.FirstSummarySentence())
	sb.WriteString("\n")
	fmt.Fprint(w, sb.String())
}

func entryLineCore(w io.Writer, e *model.Entry, g *model.Graph, score float64) {
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
	if in := FormatIntent(e.Intent); in != "" {
		sb.WriteString(" ")
		sb.WriteString(in)
	}
	sb.WriteString(" (")
	sb.WriteString(strings.Join(e.Participants, ", "))
	sb.WriteString(")")
	if s := FormatStatus(g.DerivedStatus(e)); s != "" {
		sb.WriteString(" ")
		sb.WriteString(s)
	}
	if !math.IsNaN(score) {
		sb.WriteString(" ")
		sb.WriteString(FormatScore(score))
	}
	if topics := FormatTopics(g.EffectiveTopics(e)); topics != "" {
		sb.WriteString(" ")
		sb.WriteString(topics)
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

// FormatScore renders a per-rendering rank score in curly-brace notation
// alongside the other derived-attribute segments. Three decimal places
// balance precision with line length — heat scores are typically small
// floats (a few units) and three places preserves enough resolution to
// distinguish similar entries.
func FormatScore(score float64) string {
	return fmt.Sprintf("{score: %.3f}", score)
}

// FormatTopics renders an entry's effective topic set in angle-bracket
// notation (`<label1, label2>`). Returns the empty string when the set is
// empty so callers can omit the segment entirely.
func FormatTopics(paths []model.TopicPath) string {
	if len(paths) == 0 {
		return ""
	}
	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		parts = append(parts, p.String())
	}
	return "<" + strings.Join(parts, ", ") + ">"
}

// FormatConfidence renders a stored confidence attribute in square-bracket notation.
func FormatConfidence(c string) string {
	return "[confidence: " + c + "]"
}

// FormatIntent renders the stored directive intent as a square-bracket
// attribute — but only for guiding, the one posture a reader cannot read off
// the derived status. Pending and unspecified stay quiet (the action-on
// default), and settled surfaces through its derived {status: settled}.
// Returns "" when no bracket should render.
func FormatIntent(in model.Intent) string {
	if in == model.IntentGuiding {
		return "[intent: guiding]"
	}
	return ""
}

// FormatStatus renders a derived status in curly-brace notation. Returns the
// empty string for StatusNone so callers can omit the attribute. Compound
// states use a space separator in the value (`closed-by <id>`) to avoid
// ambiguity with the outer `{key: value}` delimiter.
func FormatStatus(s model.Status) string {
	if s.Kind == model.StatusNone {
		return ""
	}
	if s.By != "" {
		return "{status: " + string(s.Kind) + " " + s.By + "}"
	}
	return "{status: " + string(s.Kind) + "}"
}

// FormatStatusTrail renders the status segment for a ref sub-line, expanding a
// superseded target into its full supersede trail through to the live head:
// `{status: superseded-by <hop1> → … → <head>}`. supersedePath is the resolved
// origin→head path (model.ResolveRef(...).Path()); its first element is the
// origin (the sub-line's own entry) and is dropped, so the rendered hops are
// the superseders ending at the head. When the target is not superseded
// (path length ≤ 1) this is identical to FormatStatus — only ref sub-lines,
// where a reader is traversing a reference, surface the intermediate hops;
// flat surfaces keep the head-only form.
func FormatStatusTrail(s model.Status, supersedePath []string) string {
	v := formatStatusTrailValue(s, supersedePath)
	if v == "" {
		return ""
	}
	return "{status: " + v + "}"
}

// formatStatusTrailValue is the brace-less status value shared by the
// curly-brace flat surfaces (via FormatStatusTrail) and the bracket-less
// contexts — the show envelope's `status:` field and the show tree's
// `(<kind>, <status>)` slot. StatusNone (done signals) yields the empty
// string so callers omit the segment. A superseded target with a multi-hop
// trail expands to `superseded-by <hop1> → … → <head>` (the path's first
// element is the origin and is dropped); other compound states render
// `<kind> <head-id>`.
func formatStatusTrailValue(s model.Status, supersedePath []string) string {
	if s.Kind == model.StatusNone {
		return ""
	}
	if s.Kind == model.StatusSupersededBy && len(supersedePath) > 1 {
		return "superseded-by " + strings.Join(supersedePath[1:], " → ")
	}
	if s.By != "" {
		return string(s.Kind) + " " + s.By
	}
	return string(s.Kind)
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
