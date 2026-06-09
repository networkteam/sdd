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
	fmt.Fprintln(w, clrHeading.Render("# body"))
	fmt.Fprint(w, renderStyledBody(g.Primary.Content, opts.Width))

	if len(g.Upstream) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, clrHeading.Render("# upstream"))
		fmt.Fprintln(w)
		for _, item := range g.Upstream {
			renderTreeItemStyled(w, item, g.Primary.ID)
		}
	}
	if len(g.Downstream) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, clrHeading.Render("# downstream"))
		fmt.Fprintln(w)
		for _, item := range g.Downstream {
			renderTreeItemStyled(w, item, g.Primary.ID)
		}
	}
}

// styleEnvelopeYAML highlights the marshaled envelope YAML by concept: keys in
// the key color, ids gold, the prominent identity values (type/kind/layer)
// white, ref kinds purple, and everything else in the body grey — so the
// scannable fields and the relationships pop while the rest recedes. It tracks
// the current top-level section (refs vs a scalar/list field) to colour values
// correctly, since the same key (`id`, `kind`) means different things inside a
// ref. Line-based — the envelope is shallow, single-line-valued YAML.
func styleEnvelopeYAML(yamlText string) string {
	lines := strings.Split(strings.TrimRight(yamlText, "\n"), "\n")
	section := ""
	for i, line := range lines {
		lines[i], section = styleEnvelopeLine(line, section)
	}
	return strings.Join(lines, "\n")
}

func styleEnvelopeLine(line, section string) (string, string) {
	trimmed := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(trimmed)]
	topLevel := indent == ""

	if trimmed == "---" {
		return indent + clrFaint.Render(trimmed), section
	}

	marker := ""
	rest := trimmed
	if strings.HasPrefix(rest, "- ") {
		marker = clrFaint.Render("- ")
		rest = rest[len("- "):]
	}

	// `key: value` — colour the key, dim the colon, colour the value by field.
	if i := strings.IndexByte(rest, ':'); i > 0 && isYAMLKey(rest[:i]) {
		key := rest[:i]
		if topLevel {
			section = key
		}
		return indent + marker + clrKey.Render(key) + clrFaint.Render(":") + styleEnvelopeValue(key, rest[i+1:], topLevel, section), section
	}
	// Bare scalar list item (value only): closes/supersedes are ids, the rest
	// (participants, topics, aliases, attachments) is body text.
	if section == "closes" || section == "supersedes" {
		return indent + marker + clrID.Render(rest), section
	}
	return indent + marker + clrBody.Render(rest), section
}

// styleEnvelopeValue colours a `key: value` value by what the field means: ids
// gold, the prominent identity fields white, a ref's kind purple, everything
// else body grey. The leading space stays outside the styled span.
func styleEnvelopeValue(key, value string, topLevel bool, section string) string {
	if strings.TrimSpace(value) == "" {
		return value // header-only line, e.g. `refs:`
	}
	v := strings.TrimPrefix(value, " ")
	lead := value[:len(value)-len(v)]

	// Status mirrors the tree qualifier: the state word white-bold, any id it
	// carries (closed-by/superseded-by target) gold like every other id.
	if topLevel && key == "status" {
		return lead + styleStatusValue(v)
	}

	var st lipgloss.Style
	switch {
	case key == "id":
		st = clrID
	case section == "refs" && key == "kind":
		st = clrRefKind
	case topLevel && key == "kind":
		st = clrQual // bold white — matches the tree qualifier's kind word
	case topLevel && (key == "type" || key == "layer"):
		st = clrIdentity
	default:
		st = clrBody
	}
	return lead + st.Render(v)
}

// styleStatusValue colours a status string the same way wherever it appears:
// the leading state word (active / open / closed-by / superseded-by) white-bold,
// and any trailing ids gold with faint `→` separators for a supersede trail.
func styleStatusValue(status string) string {
	word, rest, hasRest := strings.Cut(status, " ")
	out := clrQual.Render(word)
	if !hasRest {
		return out
	}
	ids := strings.Split(rest, " → ")
	for i, id := range ids {
		ids[i] = clrID.Render(id)
	}
	return out + " " + strings.Join(ids, clrFaint.Render(" → "))
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

// renderTreeItemStyled is the styled analog of renderTreeItem: faint indent
// guides encode depth, the relation verb is purple, the id gold, and the
// (kind, status) qualifier plus summary and `↳` why sub-line sit in the body
// grey. A closed/superseded node is rendered whole in the receded grey with no
// colour accents, so the live entries stand out. The node shape is otherwise
// identical to the plain renderer.
func renderTreeItemStyled(w io.Writer, item model.ShowTreeItem, primaryID string) {
	guide := clrFaint.Render(strings.Repeat("│ ", item.Depth-1))
	subGuide := clrFaint.Render(strings.Repeat("│ ", item.Depth))
	live := isLiveStatus(item.Status)

	verb := treeVerb(item)
	qual := treeQualifier(item)
	sentence := treeSentence(item, primaryID)

	var line strings.Builder
	line.WriteString(guide)
	line.WriteString("- ")
	if live {
		line.WriteString(clrRefKind.Render(verb))
		line.WriteString(" ")
		line.WriteString(clrID.Render(item.Entry.ID))
		if q := styledQualifier(item); q != "" {
			line.WriteString(" " + q)
		}
		if sentence != "" {
			line.WriteString(" — " + clrBody.Render(sentence))
		}
	} else {
		// Closed/superseded: recede the whole node, no colour accents.
		text := verb + " " + item.Entry.ID
		if qual != "" {
			text += " " + qual
		}
		if sentence != "" {
			text += " — " + sentence
		}
		line.WriteString(clrInactive.Render(text))
	}
	fmt.Fprintln(w, line.String())

	if item.RefDesc != "" {
		why := clrBody
		if !live {
			why = clrInactive
		}
		fmt.Fprintf(w, "%s%s\n", subGuide, why.Render("↳ "+item.RefDesc))
	}

	if len(item.Truncated) > 0 {
		ids := make([]string, len(item.Truncated))
		for i, tr := range item.Truncated {
			ids[i] = tr.ID
		}
		trunc := fmt.Sprintf("+%d more refs truncated (depth %d): %s",
			len(item.Truncated), item.Depth, strings.Join(ids, ", "))
		fmt.Fprintf(w, "%s%s\n", subGuide, clrFaint.Render(trunc))
	}
}

// styledQualifier renders a live node's `(<kind>, <status>)` slot with the kind
// and status words white-bold so they read at a glance, while the parens and
// comma stay in the body grey. Returns "" when there is nothing to qualify.
// Live nodes never carry an id in the status (active/open/done only), so the
// words are safe to bold wholesale.
func styledQualifier(item model.ShowTreeItem) string {
	kind := entryKindLabel(item.Entry)
	status := formatStatusTrailValue(item.Status, item.SupersedePath)
	switch {
	case kind != "" && status != "":
		return clrBody.Render("(") + clrQual.Render(kind) + clrBody.Render(", ") + clrQual.Render(status) + clrBody.Render(")")
	case kind != "":
		return clrBody.Render("(") + clrQual.Render(kind) + clrBody.Render(")")
	case status != "":
		return clrBody.Render("(") + clrQual.Render(status) + clrBody.Render(")")
	default:
		return ""
	}
}

// isLiveStatus reports whether a node is still in play — an active decision, an
// open signal, or a terminal done — versus closed/superseded/cascade-retired,
// which the styled tree dims so live entries draw the eye.
func isLiveStatus(s model.Status) bool {
	switch s.Kind {
	case model.StatusClosedBy, model.StatusSupersededBy, model.StatusCascadeClosedBy, model.StatusCascadeOrphan:
		return false
	default:
		return true
	}
}
