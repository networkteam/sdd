package presenters

import "charm.land/lipgloss/v2"

// CLI color palette. Every styled (TTY) sdd command renders through these shared
// styles so human-facing output reads consistently — one color per concept,
// prominence carried by weight (white-bold, white, body grey, faint) rather than
// extra hues, per the CLI color-scheme directive (d-cpt-n0f). Colors emit only on
// the colorprofile-wrapped TTY path; a plain io.Writer (test buffer, pipe) is
// downsampled to Ascii and gets clean text. The palette tracks glamour's dark
// style — body text 252, faint 240 — so styled chrome and the glamour-rendered
// body read as one piece.
var (
	clrHeading  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")) // section headings (bright white)
	clrIdentity = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))            // identity values: type, layer
	clrID       = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))           // every rendered id outside the body (gold)
	clrKey      = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))             // YAML keys / column labels (cyan)
	clrRefKind  = lipgloss.NewStyle().Foreground(lipgloss.Color("141"))           // ref kinds: frontmatter ref values + tree verbs (purple)
	clrBody     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))           // glamour body grey: secondary values, summary, desc
	clrQual     = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true) // kind + status words (tree qualifier and envelope) — most prominent
	clrFaint    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))           // punctuation, guides, truncation
	clrInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))           // whole node when closed/superseded (recedes)
)
