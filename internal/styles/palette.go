// Package styles is the single source of truth for sdd's CLI color palette.
// It sits below both the read-side view layer (internal/presenters) and the
// long-running-command output coordinator (internal/cliout); both import it so
// human-facing output reads as one system. One concept maps to one color;
// prominence is carried by weight (bold/plain/body/faint) rather than extra
// hues, per the CLI color-scheme directive (d-cpt-vye). Colors emit only on the
// colorprofile-wrapped TTY path; a plain io.Writer (test buffer, pipe) or
// NO_COLOR downsamples to clean text. Greys track glamour's dark style — body
// 252, faint 240 — so styled chrome and the glamour-rendered body read as one
// piece.
package styles

import "charm.land/lipgloss/v2"

var (
	Heading  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")) // section headings, operation labels (bright white)
	Identity = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))            // identity values: type, layer
	// Qualifier deliberately shares Heading's bold-white treatment (two most-
	// prominent concepts, one weight) — keep them identical if either changes.
	Qualifier = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")) // kind + status words — most prominent
	ID        = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))           // rendered entry ids outside the body (gold)
	Key       = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))             // YAML keys, attr keys, column labels (cyan)
	RefKind   = lipgloss.NewStyle().Foreground(lipgloss.Color("141"))           // ref kinds: frontmatter ref values + tree verbs (purple)
	Body      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))           // body grey: secondary values, summary, desc, message text
	Faint     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))           // punctuation, guides, truncation, counts
	Inactive  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))           // whole node when closed/superseded (recedes)

	// Log levels. Info shares cyan with Key and Warn shares gold with ID —
	// prominence by hue reuse, not new colors.
	Info  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))            // info level (cyan)
	Warn  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))          // warn level (gold)
	Error = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")) // error level (red, bold)
)
