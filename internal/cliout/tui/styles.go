package tui

import (
	"fmt"
	"log/slog"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/networkteam/sdd/internal/cliout"
)

// Styles for the transient view. They follow the CLI color-scheme philosophy
// (d-cpt-n0f) — one color per concept, prominence by weight — but cover a
// different surface (log levels, progress chrome) than the entry-rendering
// palette in internal/presenters, so they live here rather than being shared.
// Colors emit only on the colorprofile-capable TTY the view runs on.
var (
	styleLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")) // operation label (bright white)
	styleBody  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))           // message text (body grey)
	styleKey   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))             // attr keys (cyan)
	styleFaint = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))           // attr values, counts (faint)

	levelStyles = map[slog.Level]lipgloss.Style{
		slog.LevelDebug: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),          // faint
		slog.LevelInfo:  lipgloss.NewStyle().Foreground(lipgloss.Color("6")),            // cyan
		slog.LevelWarn:  lipgloss.NewStyle().Foreground(lipgloss.Color("220")),          // gold
		slog.LevelError: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")), // red, bold
	}
)

// levelBadge renders a fixed-width, colored level tag.
func levelBadge(l slog.Level) string {
	st, ok := levelStyles[l]
	if !ok {
		st = styleBody
	}
	return st.Render(fmt.Sprintf("%-5s", l.String()))
}

// renderEntry formats one log entry as a styled line: level badge, message,
// then space-separated key=value attributes. Structured to the end — attrs
// are styled here, not flattened upstream.
func renderEntry(e cliout.LogEntry) string {
	var b strings.Builder
	b.WriteString(levelBadge(e.Level))
	b.WriteByte(' ')
	b.WriteString(styleBody.Render(e.Message))
	for _, a := range e.Attrs {
		b.WriteByte(' ')
		b.WriteString(styleKey.Render(a.Key + "="))
		b.WriteString(styleFaint.Render(a.Value.String()))
	}
	return b.String()
}

// renderCount formats the absolute progress count beside the spinner, e.g.
// "42/120 entries". Empty when no total is known and nothing has completed.
func renderCount(p cliout.Progress) string {
	switch {
	case p.Total > 0:
		unit := p.Unit
		if unit != "" {
			unit = " " + unit
		}
		return styleFaint.Render(fmt.Sprintf("%d/%d%s", p.Done, p.Total, unit))
	case p.Done > 0:
		return styleFaint.Render(fmt.Sprintf("%d", p.Done))
	default:
		return ""
	}
}
