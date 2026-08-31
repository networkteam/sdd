package presenters

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/networkteam/sdd/internal/mdcompose"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// bodyHeadingLevel is where an embedded entry body starts: one level below the
// `# body` section heading that contains it.
const bodyHeadingLevel = 2

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
// followed by compact markdown trees for the upstream and downstream
// neighborhoods (each present to the depth the query set; an empty direction is
// omitted). Groups are separated by a blank line — each is a self-contained
// frontmatter+body markdown document.
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
	// Top-level `# body` heading with the body demoted beneath it, so an
	// embedded heading never sits level with the section containing it
	// (d-cpt-5wv).
	fmt.Fprintln(w, "# body")
	fmt.Fprintln(w)
	fmt.Fprintln(w, mdcompose.DemoteTo(g.Primary.Content, bodyHeadingLevel))

	if len(g.Upstream) > 0 || len(g.UpstreamTruncated) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "# upstream")
		fmt.Fprintln(w)
		for _, item := range g.Upstream {
			renderTreeItem(w, item, primaryDisplayID(g))
		}
		renderDirectionCut(w, g.UpstreamTruncated)
	}

	if len(g.Downstream) > 0 || len(g.DownstreamTruncated) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "# downstream")
		fmt.Fprintln(w)
		for _, item := range g.Downstream {
			renderTreeItem(w, item, primaryDisplayID(g))
		}
		renderDirectionCut(w, g.DownstreamTruncated)
	}
}

// renderDirectionCut names the primary's own children a chain budget kept out
// of the direction entirely — the direction-level honest frontier.
func renderDirectionCut(w io.Writer, refs []model.TruncatedRef) {
	if len(refs) == 0 {
		return
	}
	fmt.Fprintf(w, "+%d more entries truncated (chain budget): %s\n", len(refs), truncatedIDs(refs))
}

// primaryDisplayID is the primary's display identity — the repo-prefixed
// form for a cross-repo primary. Falls back to the bare entry ID for
// callers that construct groups without setting PrimaryID.
func primaryDisplayID(g query.ShowGroup) string {
	if g.PrimaryID != "" {
		return g.PrimaryID
	}
	return g.Primary.ID
}

// showEnvelope is the YAML frontmatter envelope for the primary entry. It
// mirrors the on-disk frontmatter (reusing model.Ref's object-form marshaling)
// and adds the filename-derived id, the entry time, discovered attachments,
// and the derived status and effective topics. Field order here is the YAML
// output order. A procedure's machine part (params/state/steps/framing)
// renders after the scannable tail, and Summary last (and only with
// --with-summary), so neither pushes the scannable fields down.
type showEnvelope struct {
	ID           string           `yaml:"id"`
	Type         string           `yaml:"type"`
	Kind         string           `yaml:"kind,omitempty"`
	Layer        string           `yaml:"layer"`
	Confidence   string           `yaml:"confidence,omitempty"`
	Intent       string           `yaml:"intent,omitempty"`
	Participants []string         `yaml:"participants,omitempty"`
	Canonical    string           `yaml:"canonical,omitempty"`
	Aliases      []string         `yaml:"aliases,omitempty"`
	Class        string           `yaml:"class,omitempty"`
	Actor        string           `yaml:"actor,omitempty"`
	Topics       []string         `yaml:"topics,omitempty"`
	Index        *model.FactIndex `yaml:"index,omitempty"`
	Refs         []model.Ref      `yaml:"refs,omitempty"`
	Closes       []string         `yaml:"closes,omitempty"`
	Supersedes   []string         `yaml:"supersedes,omitempty"`
	Attachments  []string         `yaml:"attachments,omitempty"`
	Status       string           `yaml:"status,omitempty"`
	Time         string           `yaml:"time"`
	Params       yaml.Node        `yaml:"params,omitempty"`
	State        yaml.Node        `yaml:"state,omitempty"`
	Steps        yaml.Node        `yaml:"steps,omitempty"`
	Framing      yaml.Node        `yaml:"framing,omitempty"`
	Summary      string           `yaml:"summary,omitempty"`
}

func writeEnvelope(w io.Writer, g query.ShowGroup, opts ShowOptions) {
	e := g.Primary
	env := showEnvelope{
		ID:           primaryDisplayID(g),
		Type:         e.TypeLabel(),
		Layer:        e.LayerLabel(),
		Kind:         string(e.Kind),
		Confidence:   e.Confidence,
		Intent:       string(e.Intent),
		Participants: e.Participants,
		Canonical:    e.Canonical,
		Aliases:      e.Aliases,
		Class:        string(e.Class),
		Actor:        e.Actor,
		Topics:       topicLabels(g.PrimaryTopics),
		Index:        e.Index,
		Refs:         e.Refs,
		Closes:       e.Closes,
		Supersedes:   e.Supersedes,
		Attachments:  e.Attachments,
		Status:       formatStatusTrailValue(g.PrimaryStatus, g.PrimarySupersedePath),
		Time:         e.Time.Format("2006-01-02 15:04:05"),
	}
	if e.ProcedureSpec != nil {
		env.Params = e.ProcedureSpec.Params
		env.State = e.ProcedureSpec.State
		env.Steps = e.ProcedureSpec.Steps
		env.Framing = e.ProcedureSpec.Framing
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
// `- <verb> <full-id> (<kind>, <status>) — <first-sentence>`. An `↳` why
// sub-line follows when the ref carries a desc, and unexpanded children at the
// depth boundary render as an indented child-level truncation line — never
// inline on this node.
func renderTreeItem(w io.Writer, item model.ShowTreeItem, primaryID string) {
	bulletIndent := strings.Repeat("  ", item.Depth-1)
	subIndent := strings.Repeat("  ", item.Depth)

	var line strings.Builder
	line.WriteString(bulletIndent)
	line.WriteString("- ")
	line.WriteString(treeVerb(item))
	line.WriteString(" ")
	line.WriteString(item.NodeID())
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
		fmt.Fprintf(w, "%s↳ %s\n", subIndent, item.RefDesc)
	}

	if len(item.Truncated) > 0 {
		reason := fmt.Sprintf("depth %d", item.Depth)
		if item.TruncatedReason != "" {
			reason = item.TruncatedReason
		}
		fmt.Fprintf(w, "%s+%d more refs truncated (%s): %s\n",
			subIndent, len(item.Truncated), reason, truncatedIDs(item.Truncated))
	}
}

func truncatedIDs(refs []model.TruncatedRef) string {
	ids := make([]string, len(refs))
	for i, tr := range refs {
		ids[i] = tr.ID
	}
	return strings.Join(ids, ", ")
}

// treeQualifier is the parenthesized `(<kind>, <status>)` slot for a tree
// node. Empty parts collapse so a node with no derived status renders
// `(<kind>)` and a legacy entry with neither renders no parens at all —
// never a bare `()` or a leading-comma `(, …)`. An unresolved cross-repo
// node takes the bracketed unresolved marker in this slot instead.
func treeQualifier(item model.ShowTreeItem) string {
	if repo := unresolvedRepo(item); repo != "" {
		return "[unresolved: repo " + repo + "]"
	}
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
// body — derived at render time without touching the stored summary. An
// unresolved cross-repo node has no entry to summarize.
func treeSentence(item model.ShowTreeItem, primaryID string) string {
	switch {
	case item.ShownAbove && item.NodeID() == primaryID:
		return "(this entry)"
	case item.ShownAbove:
		return "(see above)"
	case item.ShownBelow:
		return "(see below)"
	case item.Entry == nil:
		return ""
	}
	return item.Entry.FirstSummarySentence()
}

// unresolvedRepo returns the target repo-id when the node is a cross-repo
// reference whose graph is not available locally, else "".
func unresolvedRepo(item model.ShowTreeItem) string {
	if item.Entry != nil || item.CrossRepoID == "" {
		return ""
	}
	repo, _, _ := model.SplitCrossRepoID(item.CrossRepoID)
	return repo
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
