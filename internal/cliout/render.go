package cliout

import (
	"fmt"
	"log/slog"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/networkteam/sdd/internal/model"
)

// PhaseLabel is the footer text for a phase. Rendering lives here, not on the
// domain type, so internal/model stays dependency-free display vocabulary.
func PhaseLabel(p model.Phase) string { return string(p) }

// Log-level + progress-chrome styles; distinct surface from the presenters palette (d-cpt-n0f).
var (
	StyleLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")) // operation label (bright white)
	StyleBody  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))           // message text (body grey)
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
		st = StyleBody
	}
	return st.Render(fmt.Sprintf("%-5s", l.String()))
}

// RenderEntry formats one log entry as a styled line: level badge, message,
// then space-separated key=value attributes. It is the single line renderer for
// both the plain stderr path (dormant/armed) and the live tea.Printf path.
func RenderEntry(e LogEntry) string {
	var b strings.Builder
	b.WriteString(levelBadge(e.Level))
	b.WriteByte(' ')
	b.WriteString(StyleBody.Render(e.Message))
	for _, a := range e.Attrs {
		b.WriteByte(' ')
		b.WriteString(styleKey.Render(a.Key + "="))
		b.WriteString(styleFaint.Render(a.Value.String()))
	}
	return b.String()
}

// RenderCount formats the absolute progress count beside the spinner, e.g.
// "42/120 entries". Empty when no total is known and nothing has completed.
func RenderCount(p Progress) string {
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
