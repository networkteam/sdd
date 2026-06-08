package presenters

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/glamour/v2"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Show styling. Colors render only through the colorprofile writer on the TTY
// path; a plain io.Writer (test buffer, pipe) is downsampled to Ascii and gets
// clean text. The concrete palette is a starting point, not a frozen spec.
var (
	showHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")) // section headings (bright white)
	envKeyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))             // YAML keys (cyan)
	envPunctStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))           // fences, list markers, colons
	showGuideStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))           // indent guides
	showVerbStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))             // relation kind
	showDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))           // secondary: (kind, status), desc, truncation
)

// defaultBodyWidth is the glamour wrap width used when no terminal width is
// available.
const defaultBodyWidth = 80

// RenderShowStyled writes the styled, human-facing show output for an
// interactive terminal: a dimmed YAML envelope, the markdown body rendered
// with glamour, and a lipgloss-styled neighborhood tree (relation-kind color,
// dimmed status, dim indent guides). It shares the read-side data model with
// the plain renderer (RenderShow) — only presentation differs.
//
// The colorprofile writer downsamples ANSI to the destination's capability and
// strips it for a non-terminal writer (d-cpt-mvb), so a plain io.Writer — a
// test buffer or a pipe — receives clean, color-free structured text.
func RenderShowStyled(dst io.Writer, result *query.ShowResult, opts ShowOptions) {
	w := colorprofile.NewWriter(dst, os.Environ())
	for i, g := range result.Groups {
		if i > 0 {
			fmt.Fprintln(w)
		}
		renderShowGroupStyled(w, g, opts)
	}
}

func renderShowGroupStyled(w io.Writer, g query.ShowGroup, opts ShowOptions) {
	// Envelope: the same YAML the plain renderer emits, lightly syntax-
	// highlighted (keys vs values vs punctuation) so it reads as structured
	// metadata above the rendered body.
	var env strings.Builder
	writeEnvelope(&env, g, opts)
	fmt.Fprintln(w, styleEnvelopeYAML(env.String()))
	fmt.Fprintln(w)

	// Body under a top-level heading so the body's own `##` sections nest
	// beneath it; glamour-rendered (raw fallback on any render error).
	fmt.Fprintln(w, showHeaderStyle.Render("# body"))
	fmt.Fprint(w, renderStyledBody(g.Primary.Content, opts.Width))

	if len(g.Upstream) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, showHeaderStyle.Render("# upstream"))
		fmt.Fprintln(w)
		for _, item := range g.Upstream {
			renderTreeItemStyled(w, item, g.Primary.ID)
		}
	}
	if len(g.Downstream) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, showHeaderStyle.Render("# downstream"))
		fmt.Fprintln(w)
		for _, item := range g.Downstream {
			renderTreeItemStyled(w, item, g.Primary.ID)
		}
	}
}

// styleEnvelopeYAML applies light syntax highlighting to the marshaled
// envelope YAML so keys read distinctly from values: keys in the key color,
// structural punctuation (fences, list markers, colons) dimmed, and values
// left in the default foreground for contrast. Line-based and deliberately
// simple — the envelope is shallow, single-line-valued YAML.
func styleEnvelopeYAML(yamlText string) string {
	lines := strings.Split(strings.TrimRight(yamlText, "\n"), "\n")
	for i, line := range lines {
		lines[i] = styleEnvelopeLine(line)
	}
	return strings.Join(lines, "\n")
}

func styleEnvelopeLine(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(trimmed)]

	if trimmed == "---" {
		return indent + envPunctStyle.Render(trimmed)
	}

	marker := ""
	rest := trimmed
	if strings.HasPrefix(rest, "- ") {
		marker = envPunctStyle.Render("- ")
		rest = rest[len("- "):]
	}

	// `key: value` — color the key, dim the colon, leave the value default.
	if i := strings.IndexByte(rest, ':'); i > 0 && isYAMLKey(rest[:i]) {
		return indent + marker + envKeyStyle.Render(rest[:i]) + envPunctStyle.Render(":") + rest[i+1:]
	}
	// Bare scalar list item (value only, e.g. a participant or topic).
	return indent + marker + rest
}

// isYAMLKey reports whether s looks like an envelope key — a single token with
// no whitespace — rather than text before a colon that happens to sit inside a
// value (e.g. a summary sentence).
func isYAMLKey(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \t")
}

// renderStyledBody renders the entry body markdown through glamour at the given
// wrap width, using the dark style with its document margin zeroed so the body
// aligns flush-left with the envelope and tree (the stock margin insets it).
// Any renderer-construction or render error falls back to the raw body so the
// command never fails on a body it can't pretty-print.
func renderStyledBody(content string, width int) string {
	if width <= 0 {
		width = defaultBodyWidth
	}
	style := glamourstyles.DarkStyleConfig
	var noMargin uint
	style.Document.Margin = &noMargin
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return content + "\n"
	}
	out, err := r.Render(content)
	if err != nil {
		return content + "\n"
	}
	return out
}

// renderTreeItemStyled is the styled analog of renderTreeItem: dim indent
// guides encode depth, the relation verb is colored, and the (kind, status)
// qualifier plus desc/truncation sub-lines are dimmed. The node shape is
// otherwise identical to the plain renderer.
func renderTreeItemStyled(w io.Writer, item model.ShowTreeItem, primaryID string) {
	guide := showGuideStyle.Render(strings.Repeat("│ ", item.Depth-1))
	subGuide := showGuideStyle.Render(strings.Repeat("│ ", item.Depth))

	var line strings.Builder
	line.WriteString(guide)
	line.WriteString("- ")
	line.WriteString(showVerbStyle.Render(treeVerb(item)))
	line.WriteString(" ")
	line.WriteString(item.Entry.ID)
	if qual := treeQualifier(item); qual != "" {
		line.WriteString(" ")
		line.WriteString(showDimStyle.Render(qual))
	}
	if sentence := treeSentence(item, primaryID); sentence != "" {
		line.WriteString(" — ")
		line.WriteString(sentence)
	}
	fmt.Fprintln(w, line.String())

	if item.RefDesc != "" {
		fmt.Fprintf(w, "%s%s\n", subGuide, showDimStyle.Render("desc: "+item.RefDesc))
	}

	if len(item.Truncated) > 0 {
		ids := make([]string, len(item.Truncated))
		for i, tr := range item.Truncated {
			ids[i] = tr.ID
		}
		trunc := fmt.Sprintf("+%d more refs truncated (depth %d): %s",
			len(item.Truncated), item.Depth, strings.Join(ids, ", "))
		fmt.Fprintf(w, "%s%s\n", subGuide, showDimStyle.Render(trunc))
	}
}
