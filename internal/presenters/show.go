package presenters

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// ShowOptions controls optional segments of the show rendering.
type ShowOptions struct {
	// WithSummary includes the primary's stored summary in the envelope.
	// Off by default — the body renders right after the envelope, and the
	// summary is only needed for human drift-review.
	WithSummary bool
	// Width is the target wrap width for the styled renderer's glamour body
	// (terminal columns). Zero falls back to a sensible default. Ignored by
	// the plain renderer, which never reflows the body.
	Width int
}

// RenderShow writes the plain-markdown show output for a ShowResult. Each group
// renders as a YAML frontmatter envelope + raw markdown body for the primary,
// followed by compact markdown trees for the upstream and (when requested)
// downstream neighborhoods. Groups are separated by a blank line — each is a
// self-contained frontmatter+body markdown document.
//
// This is the agent / pipe / `--format text` renderer; the styled terminal
// renderer shares the same data model (see the styled renderer / slice 3).
func RenderShow(w io.Writer, result *query.ShowResult, opts ShowOptions) {
	for i, g := range result.Groups {
		if i > 0 {
			fmt.Fprintln(w)
		}
		renderShowGroup(w, g, opts)
	}
}

func renderShowGroup(w io.Writer, g query.ShowGroup, opts ShowOptions) {
	writeEnvelope(w, g, opts)
	fmt.Fprintln(w)
	fmt.Fprintln(w, g.Primary.Content)

	if len(g.Upstream) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## upstream")
		for _, item := range g.Upstream {
			renderTreeItem(w, item, g.Primary.ID)
		}
	}

	if len(g.Downstream) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## downstream")
		for _, item := range g.Downstream {
			renderTreeItem(w, item, g.Primary.ID)
		}
	}
}

// showEnvelope is the YAML frontmatter envelope for the primary entry. It
// mirrors the on-disk frontmatter (reusing model.Ref's object-form marshaling)
// and adds the filename-derived id, the entry time, discovered attachments,
// and the derived status and effective topics. Field order here is the YAML
// output order. Summary renders last (and only with --with-summary) so the
// long opt-in text never pushes the scannable fields down.
type showEnvelope struct {
	ID           string      `yaml:"id"`
	Type         string      `yaml:"type"`
	Layer        string      `yaml:"layer"`
	Kind         string      `yaml:"kind,omitempty"`
	Confidence   string      `yaml:"confidence,omitempty"`
	Participants []string    `yaml:"participants,omitempty"`
	Canonical    string      `yaml:"canonical,omitempty"`
	Aliases      []string    `yaml:"aliases,omitempty"`
	Actor        string      `yaml:"actor,omitempty"`
	Topics       []string    `yaml:"topics,omitempty"`
	Refs         []model.Ref `yaml:"refs,omitempty"`
	Closes       []string    `yaml:"closes,omitempty"`
	Supersedes   []string    `yaml:"supersedes,omitempty"`
	Attachments  []string    `yaml:"attachments,omitempty"`
	Status       string      `yaml:"status,omitempty"`
	Time         string      `yaml:"time"`
	Summary      string      `yaml:"summary,omitempty"`
}

func writeEnvelope(w io.Writer, g query.ShowGroup, opts ShowOptions) {
	e := g.Primary
	env := showEnvelope{
		ID:           e.ID,
		Type:         e.TypeLabel(),
		Layer:        e.LayerLabel(),
		Kind:         string(e.Kind),
		Confidence:   e.Confidence,
		Participants: e.Participants,
		Canonical:    e.Canonical,
		Aliases:      e.Aliases,
		Actor:        e.Actor,
		Topics:       topicLabels(g.PrimaryTopics),
		Refs:         e.Refs,
		Closes:       e.Closes,
		Supersedes:   e.Supersedes,
		Attachments:  e.Attachments,
		Status:       formatStatusTrailValue(g.PrimaryStatus, g.PrimarySupersedePath),
		Time:         e.Time.Format("2006-01-02 15:04:05"),
	}
	if opts.WithSummary {
		env.Summary = e.Summary
	}

	data, _ := yaml.Marshal(&env)
	fmt.Fprint(w, "---\n")
	fmt.Fprint(w, string(data))
	fmt.Fprint(w, "---\n")
}

// renderTreeItem renders one neighborhood node as a markdown bullet. Indent
// encodes depth (depth 1 sits flush-left); the line shape is
// `- <verb> <full-id> (<kind>, <status>) — <first-sentence>`. A `desc:`
// sub-line follows when the ref carries one, and unexpanded children at the
// max-depth boundary render as an indented child-level truncation line — never
// inline on this node.
func renderTreeItem(w io.Writer, item model.ShowTreeItem, primaryID string) {
	bulletIndent := strings.Repeat("  ", item.Depth-1)
	subIndent := strings.Repeat("  ", item.Depth)

	var line strings.Builder
	line.WriteString(bulletIndent)
	line.WriteString("- ")
	line.WriteString(treeVerb(item))
	line.WriteString(" ")
	line.WriteString(item.Entry.ID)
	if qual := treeQualifier(item); qual != "" {
		line.WriteString(" ")
		line.WriteString(qual)
	}
	if sentence := treeSentence(item, primaryID); sentence != "" {
		line.WriteString(" — ")
		line.WriteString(sentence)
	}
	fmt.Fprintln(w, line.String())

	if item.RefDesc != "" {
		fmt.Fprintf(w, "%sdesc: %s\n", subIndent, item.RefDesc)
	}

	if len(item.Truncated) > 0 {
		ids := make([]string, len(item.Truncated))
		for i, tr := range item.Truncated {
			ids[i] = tr.ID
		}
		fmt.Fprintf(w, "%s+%d more refs truncated (depth %d): %s\n",
			subIndent, len(item.Truncated), item.Depth, strings.Join(ids, ", "))
	}
}

// treeQualifier is the parenthesized `(<kind>, <status>)` slot for a tree
// node. Empty parts collapse so a node with no derived status renders
// `(<kind>)` and a legacy entry with neither renders no parens at all —
// never a bare `()` or a leading-comma `(, …)`.
func treeQualifier(item model.ShowTreeItem) string {
	kind := entryKindLabel(item.Entry)
	status := formatStatusTrailValue(item.Status, item.SupersedePath)
	switch {
	case kind != "" && status != "":
		return "(" + kind + ", " + status + ")"
	case kind != "":
		return "(" + kind + ")"
	case status != "":
		return "(" + status + ")"
	default:
		return ""
	}
}

// treeSentence is the trailing micro-summary for a node. Dedup markers take the
// slot when the node was already shown (or is a later primary); otherwise it's
// the entry's first sentence — from the stored summary when present, else the
// body — derived at render time without touching the stored summary.
func treeSentence(item model.ShowTreeItem, primaryID string) string {
	switch {
	case item.ShownAbove && item.Entry.ID == primaryID:
		return "(this entry)"
	case item.ShownAbove:
		return "(see above)"
	case item.ShownBelow:
		return "(see below)"
	}
	src := item.Entry.Summary
	if src == "" {
		src = item.Entry.Content
	}
	return firstSentence(src)
}

// treeVerb is the leading relation word for a tree node. A refs/refd-by edge
// carrying a kind renders as that kind (grounded-in, addresses, …) — the
// meaningful relationship; legacy refs without a kind stay "refs"/"refd-by".
// closes/supersedes (and their downstream inverses) render verbatim — they
// carry uniform meaning and never accept per-edge kinds. Combined relations
// join with a comma.
func treeVerb(item model.ShowTreeItem) string {
	parts := make([]string, len(item.Relations))
	for i, r := range item.Relations {
		if (r == "refs" || r == "refd-by") && item.RefKind != "" && item.RefKind != model.RefKindUnknown {
			parts[i] = string(item.RefKind)
		} else {
			parts[i] = r
		}
	}
	return strings.Join(parts, ",")
}

// entryKindLabel is the `<kind>` shown in a tree node's `(<kind>, <status>)`
// slot: the display kind when set (directive for kindless decisions), falling
// back to the type label for kindless signals.
func entryKindLabel(e *model.Entry) string {
	if k := kindForDisplay(e); k != "" {
		return string(k)
	}
	return e.TypeLabel()
}

// kindForDisplay returns the kind to render for an entry. Decisions fall back
// to "directive" when Kind is empty (legacy default); other types show kind
// only when explicitly set.
func kindForDisplay(e *model.Entry) model.Kind {
	if e.Kind != "" {
		return e.Kind
	}
	if e.Type == model.TypeDecision {
		return model.KindDirective
	}
	return ""
}

// topicLabels renders an effective topic set as plain label strings for the
// envelope's `topics:` field.
func topicLabels(paths []model.TopicPath) []string {
	if len(paths) == 0 {
		return nil
	}
	labels := make([]string, len(paths))
	for i, p := range paths {
		labels[i] = p.String()
	}
	return labels
}

// firstSentence returns the leading sentence of s — text up to the first
// sentence-ending ". " or the first line break, whichever comes first. Used
// for the compact depth-level micro-summary in the show tree; stored summaries
// are never modified.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}
