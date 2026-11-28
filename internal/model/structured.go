package model

import (
	"path"

	"gopkg.in/yaml.v3"

	"github.com/networkteam/sdd/pkg/application/types"
)

var statusKindOrder = []StatusKind{StatusNone, StatusActive, StatusSettled, StatusOpen, StatusClosedBy, StatusSupersededBy, StatusCascadeClosedBy, StatusCascadeOrphan}

// Label is the status kind on the structured surface, where StatusNone has
// its own name.
func (k StatusKind) Label() types.StatusKind {
	if k == StatusNone {
		return types.StatusNone
	}
	return types.StatusKind(k)
}

// ParseStatusLabel resolves a structured-surface status kind.
func ParseStatusLabel(s types.StatusKind) (StatusKind, bool) {
	for _, k := range statusKindOrder {
		if k.Label() == s {
			return k, true
		}
	}
	return "", false
}

// FilterByParticipants keeps entries listing at least one of names. Matching
// is exact against the stored canonicals (d-cpt-979).
func FilterByParticipants(entries []*Entry, names []string) []*Entry {
	want := make(map[string]struct{}, len(names))
	for _, n := range names {
		want[n] = struct{}{}
	}
	var out []*Entry
	for _, e := range entries {
		for _, p := range e.Participants {
			if _, ok := want[p]; ok {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// qualifyIn prefixes id for display outside repo's graph, leaving embedded
// entries bare (see nodeKeyFor).
func (g *Graph) qualifyIn(repo RepoID, id string) string {
	if repo == "" {
		return id
	}
	if e, ok := g.ByID[id]; ok && e.Embedded {
		return id
	}
	return CrossRepoID(repo, id)
}

// StructuredEntry maps e, owned by g, to its exported form with the derived
// status and effective topics the presenters render. repo qualifies display
// IDs for a member graph loaded standalone; empty uses g's own repo.
// summaryOnly leaves the body out.
func (g *Graph) StructuredEntry(e *Entry, repo RepoID, summaryOnly bool) types.GraphEntry {
	if repo == "" {
		repo = g.repoID
	}
	if e.Embedded {
		repo = ""
	}
	status, trail := g.itemStatus(e)
	out := types.GraphEntry{
		ID:           e.ID,
		DisplayID:    g.qualifyIn(repo, e.ID),
		ShortID:      ShortID(e.ID),
		RepoID:       repo,
		Type:         types.EntryType(e.TypeLabel()),
		Kind:         types.Kind(e.Kind),
		Layer:        types.Layer(e.LayerLabel()),
		Status:       types.Status{Kind: status.Kind.Label()},
		Created:      e.Time,
		Participants: append([]string(nil), e.Participants...),
		Confidence:   types.Confidence(e.Confidence),
		Intent:       types.Intent(e.Intent),
		Summary:      e.Summary,
		Lead:         e.FirstSummarySentence(),
		Closes:       append([]string(nil), e.Closes...),
		Supersedes:   append([]string(nil), e.Supersedes...),
		Canonical:    e.Canonical,
		Aliases:      append([]string(nil), e.Aliases...),
		Class:        types.ProcedureClass(e.Class),
		Actor:        e.Actor,
	}
	if e.Index != nil {
		out.Index = &types.FactIndex{Title: e.Index.Title, Topic: e.Index.Topic.Structured()}
	}
	if spec := e.ProcedureSpec; spec != nil {
		out.Procedure = &types.ProcedureSpec{Params: yamlText(spec.Params), State: yamlText(spec.State), Steps: yamlText(spec.Steps), Framing: yamlText(spec.Framing)}
	}
	if repo != "" {
		out.ShortID = CrossRepoID(repo, out.ShortID)
	}
	if status.By != "" {
		out.Status.By = g.qualifyIn(repo, status.By)
	}
	for _, id := range trail {
		out.Status.SupersedePath = append(out.Status.SupersedePath, g.qualifyIn(repo, id))
	}
	if !summaryOnly {
		out.Body = e.Content
	}
	for _, t := range g.EffectiveTopics(e) {
		out.Topics = append(out.Topics, t.Structured())
	}
	for _, r := range e.Refs {
		out.Refs = append(out.Refs, types.EntryRef{ID: r.ID, Kind: types.RefKind(r.Kind), Desc: r.Desc})
	}
	for _, a := range e.Attachments {
		out.Attachments = append(out.Attachments, path.Base(a))
	}
	return out
}

// StructuredTreeItems maps show tree items built on g to their exported form,
// each resolved entry derived in its owning graph.
func (g *Graph) StructuredTreeItems(items []ShowTreeItem) []types.ShowTreeItem {
	out := make([]types.ShowTreeItem, 0, len(items))
	for _, item := range items {
		mapped := types.ShowTreeItem{
			Depth:           item.Depth,
			Relations:       structuredRelations(item.Relations),
			RefKind:         types.RefKind(item.RefKind),
			RefDesc:         item.RefDesc,
			LinkKind:        LinkKind(item.RefKind, item.Relations),
			TargetID:        item.NodeID(),
			ShownAbove:      item.ShownAbove,
			ShownBelow:      item.ShownBelow,
			Truncated:       StructuredTruncatedRefs(item.Truncated),
			TruncatedReason: item.TruncatedReason,
		}
		if item.Entry != nil {
			owner := g
			if _, o, ok := g.ResolveAcross(item.NodeID()); ok {
				owner = o
			}
			entry := owner.StructuredEntry(item.Entry, "", item.SummaryOnly)
			mapped.Entry = &entry
		}
		out = append(out, mapped)
	}
	return out
}

// StructuredTruncatedRefs maps truncated refs to their exported form.
func StructuredTruncatedRefs(refs []TruncatedRef) []types.TruncatedRef {
	if len(refs) == 0 {
		return nil
	}
	out := make([]types.TruncatedRef, len(refs))
	for i, r := range refs {
		out[i] = types.TruncatedRef{ID: r.ID, Relations: structuredRelations(r.Relations), Kind: types.Kind(r.Kind), RefKind: types.RefKind(r.RefKind), RefDesc: r.RefDesc, LinkKind: LinkKind(r.RefKind, r.Relations)}
	}
	return out
}

// LinkKind names a show edge as the citing entry sets it: the stored ref kind
// (legacy aliases are resolved at parse time), else closes or supersedes for
// those relations in either direction, else refs — also for a legacy ref
// without a kind.
func LinkKind(refKind RefKind, relations []string) types.LinkKind {
	if refKind != "" && refKind != RefKindUnknown {
		return types.LinkKind(refKind)
	}
	for _, relation := range relations {
		switch types.Relation(relation) {
		case types.RelationCloses, types.RelationClosedBy:
			return types.LinkCloses
		case types.RelationSupersedes, types.RelationSupersededBy:
			return types.LinkSupersedes
		}
	}
	return types.LinkRefs
}

func structuredRelations(relations []string) []types.Relation {
	out := make([]types.Relation, len(relations))
	for i, r := range relations {
		out[i] = types.Relation(r)
	}
	return out
}

// DirectRefCount counts a show direction's direct refs: its depth-1 items
// plus the refs its budget kept out of the direction.
func DirectRefCount(items []ShowTreeItem, truncated []TruncatedRef) int {
	n := len(truncated)
	for _, item := range items {
		if item.Depth == 1 {
			n++
		}
	}
	return n
}

// yamlText encodes a stored frontmatter value back to YAML; a decoded node
// always encodes, so the error is unreachable.
func yamlText(node yaml.Node) string {
	if node.Kind == 0 {
		return ""
	}
	out, _ := yaml.Marshal(&node)
	return string(out)
}
