package presenters

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/networkteam/sdd/internal/mdcompose"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/application/types"
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

// RenderShow writes the plain-markdown show output for shown entries. Each
// renders as a YAML frontmatter envelope + raw markdown body, followed by
// compact markdown trees for the upstream and downstream neighborhoods (each
// present to the depth the query set; an empty direction is omitted). Entries
// are separated by a blank line — each is a self-contained frontmatter+body
// markdown document.
//
// This is the agent / pipe / `--format text` renderer; the styled terminal
// renderer shares the same data model (see the styled renderer / slice 3).
func RenderShow(w io.Writer, entries []types.ShownEntry, opts ShowOptions) {
	for i, shown := range entries {
		if i > 0 {
			fmt.Fprintln(w)
		}
		renderShown(w, shown, opts)
	}
}

// ShowText is the plain show output as the MCP surface and engine injection
// serve it: default options, trailing newlines trimmed.
func ShowText(entries []types.ShownEntry) string {
	var b strings.Builder
	RenderShow(&b, entries, ShowOptions{})
	return strings.TrimRight(b.String(), "\n")
}

func renderShown(w io.Writer, g types.ShownEntry, opts ShowOptions) {
	writeEnvelope(w, g.Entry, opts)
	fmt.Fprintln(w)
	// Top-level `# body` heading with the body demoted beneath it, so an
	// embedded heading never sits level with the section containing it
	// (d-cpt-5wv).
	fmt.Fprintln(w, "# body")
	fmt.Fprintln(w)
	fmt.Fprintln(w, mdcompose.DemoteTo(g.Entry.Body, bodyHeadingLevel))

	if len(g.Upstream) > 0 || len(g.UpstreamTruncated) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "# upstream")
		fmt.Fprintln(w)
		for _, item := range g.Upstream {
			renderTreeItem(w, item, g.Entry.DisplayID)
		}
		renderDirectionCut(w, g.UpstreamTruncated)
	}

	if len(g.Downstream) > 0 || len(g.DownstreamTruncated) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "# downstream")
		fmt.Fprintln(w)
		for _, item := range g.Downstream {
			renderTreeItem(w, item, g.Entry.DisplayID)
		}
		renderDirectionCut(w, g.DownstreamTruncated)
	}
}

// renderDirectionCut names the primary's own children a chain budget kept out
// of the direction entirely — the direction-level honest frontier.
func renderDirectionCut(w io.Writer, refs []types.TruncatedRef) {
	if len(refs) == 0 {
		return
	}
	fmt.Fprintf(w, "+%d more entries truncated (chain budget): %s\n", len(refs), truncatedIDs(refs))
}

// showEnvelope is the YAML frontmatter envelope for the primary entry. It
// mirrors the on-disk frontmatter (reusing model.Ref's object-form marshaling)
// and adds the filename-derived id, the entry time, discovered attachments,
// and the derived status and effective topics. Field order here is the YAML
// output order. A procedure's machine part (params/state/steps/framing)
// renders after the scannable tail, and Summary last (and only with
// --with-summary), so neither pushes the scannable fields down.
type showEnvelope struct {
	ID           string               `yaml:"id"`
	Type         types.EntryType      `yaml:"type"`
	Kind         types.Kind           `yaml:"kind,omitempty"`
	Layer        types.Layer          `yaml:"layer"`
	Confidence   types.Confidence     `yaml:"confidence,omitempty"`
	Intent       types.Intent         `yaml:"intent,omitempty"`
	Participants []string             `yaml:"participants,omitempty"`
	Canonical    string               `yaml:"canonical,omitempty"`
	Aliases      []string             `yaml:"aliases,omitempty"`
	Class        types.ProcedureClass `yaml:"class,omitempty"`
	Actor        string               `yaml:"actor,omitempty"`
	Topics       []string             `yaml:"topics,omitempty"`
	Index        *model.FactIndex     `yaml:"index,omitempty"`
	Refs         []model.Ref          `yaml:"refs,omitempty"`
	Closes       []string             `yaml:"closes,omitempty"`
	Supersedes   []string             `yaml:"supersedes,omitempty"`
	Attachments  []string             `yaml:"attachments,omitempty"`
	Status       string               `yaml:"status,omitempty"`
	Time         string               `yaml:"time"`
	Params       yaml.Node            `yaml:"params,omitempty"`
	State        yaml.Node            `yaml:"state,omitempty"`
	Steps        yaml.Node            `yaml:"steps,omitempty"`
	Framing      yaml.Node            `yaml:"framing,omitempty"`
	Summary      string               `yaml:"summary,omitempty"`
}

func writeEnvelope(w io.Writer, e types.GraphEntry, opts ShowOptions) {
	env := showEnvelope{
		ID:           e.DisplayID,
		Type:         e.Type,
		Layer:        e.Layer,
		Kind:         e.Kind,
		Confidence:   e.Confidence,
		Intent:       e.Intent,
		Participants: e.Participants,
		Canonical:    e.Canonical,
		Aliases:      e.Aliases,
		Class:        e.Class,
		Actor:        e.Actor,
		Topics:       topicLabels(e.Topics),
		Index:        envelopeIndex(e.Index),
		Refs:         envelopeRefs(e.Refs),
		Closes:       e.Closes,
		Supersedes:   e.Supersedes,
		Attachments:  e.Attachments,
		Status:       statusValue(e.Status),
		Time:         e.Created.Format("2006-01-02 15:04:05"),
	}
	if p := e.Procedure; p != nil {
		env.Params = yamlValue(p.Params)
		env.State = yamlValue(p.State)
		env.Steps = yamlValue(p.Steps)
		env.Framing = yamlValue(p.Framing)
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
func renderTreeItem(w io.Writer, item types.ShowTreeItem, primaryID string) {
	bulletIndent := strings.Repeat("  ", item.Depth-1)
	subIndent := strings.Repeat("  ", item.Depth)

	var line strings.Builder
	line.WriteString(bulletIndent)
	line.WriteString("- ")
	line.WriteString(treeVerb(item))
	line.WriteString(" ")
	line.WriteString(item.TargetID)
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

func truncatedIDs(refs []types.TruncatedRef) string {
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
func treeQualifier(item types.ShowTreeItem) string {
	if repo := unresolvedRepo(item); repo != "" {
		return "[unresolved: repo " + string(repo) + "]"
	}
	kind := entryKindLabel(item.Entry)
	status := statusValue(item.Entry.Status)
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
// the entry's Lead. An unresolved cross-repo node has no entry to summarize.
func treeSentence(item types.ShowTreeItem, primaryID string) string {
	switch {
	case item.ShownAbove && item.TargetID == primaryID:
		return "(this entry)"
	case item.ShownAbove:
		return "(see above)"
	case item.ShownBelow:
		return "(see below)"
	case item.Entry == nil:
		return ""
	}
	return item.Entry.Lead
}

// unresolvedRepo returns the target repo-id when the node is a cross-repo
// reference whose graph is not available locally, else "".
func unresolvedRepo(item types.ShowTreeItem) model.RepoID {
	if item.Entry != nil {
		return ""
	}
	repo, _, _ := model.SplitCrossRepoID(item.TargetID)
	return repo
}

// treeVerb is the leading relation word for a tree node. A refs/refd-by edge
// carrying a kind renders as that kind (grounded-in, addresses, …) — the
// meaningful relationship; legacy refs without a kind stay "refs"/"refd-by".
// closes/supersedes (and their downstream inverses) render verbatim — they
// carry uniform meaning and never accept per-edge kinds. Combined relations
// join with a comma.
func treeVerb(item types.ShowTreeItem) string {
	parts := make([]string, len(item.Relations))
	for i, r := range item.Relations {
		if (r == types.RelationRefs || r == types.RelationRefdBy) && item.RefKind != "" && item.RefKind != types.RefKindUnknown {
			parts[i] = string(item.RefKind)
		} else {
			parts[i] = string(r)
		}
	}
	return strings.Join(parts, ",")
}

// entryKindLabel is the `<kind>` shown in a tree node's `(<kind>, <status>)`
// slot: the kind when set, directive for kindless decisions, else the type.
func entryKindLabel(e *types.GraphEntry) string {
	switch {
	case e.Kind != "":
		return string(e.Kind)
	case e.Type == types.TypeDecision:
		return string(types.KindDirective)
	default:
		return string(e.Type)
	}
}

// statusValue is the brace-less status value of s (see
// formatStatusTrailValue).
func statusValue(s types.Status) string {
	return statusTrailValue(s.Kind, s.By, s.SupersedePath)
}

func topicLabels(paths []types.TopicPath) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = p.String()
	}
	return out
}

func envelopeIndex(index *types.FactIndex) *model.FactIndex {
	if index == nil {
		return nil
	}
	return &model.FactIndex{Title: index.Title, Topic: model.TopicPath{Components: index.Topic}}
}

func envelopeRefs(refs []types.EntryRef) []model.Ref {
	if len(refs) == 0 {
		return nil
	}
	out := make([]model.Ref, len(refs))
	for i, r := range refs {
		out[i] = model.Ref{ID: r.ID, Kind: model.RefKind(r.Kind), Desc: r.Desc}
	}
	return out
}

// yamlValue decodes a procedure field's stored YAML text back to the node the
// envelope embeds; the text was encoded from a decoded node, so it decodes.
func yamlValue(text string) yaml.Node {
	var doc yaml.Node
	if text == "" || yaml.Unmarshal([]byte(text), &doc) != nil || len(doc.Content) == 0 {
		return yaml.Node{}
	}
	return *doc.Content[0]
}
