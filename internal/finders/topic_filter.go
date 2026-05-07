package finders

import (
	"github.com/networkteam/sdd/internal/model"
)

// TopicFilter selects entries whose effective topic set contains a label
// matching the given prefix path component-wise (case-insensitive).
//
// "Effective topics" of an entry combine inline `topics:` declared on the
// entry's own frontmatter with topics declared by `kind: annotation` entries
// whose refs (or per-topic members sub-selection) include the entry. This
// merge is what makes annotations a first-class clustering mechanism — an
// entry is "in topic X" whether it tagged itself or some annotation tagged it.
//
// Owned by Plan 1 (d-tac-gvn) per the augmenting directive d-tac-9q1; Plan 2
// (d-tac-uww, sdd view) consumes this primitive for its `topic(L)` filter
// vocabulary in the pipeline grammar.
type TopicFilter struct {
	Prefix model.TopicPath
}

// MatchEntry reports whether the entry has any effective topic with prefix.
// Pure — uses the graph's reverse index for annotation lookups, no I/O.
func (f TopicFilter) MatchEntry(g *model.Graph, e *model.Entry) bool {
	if f.Prefix.IsZero() {
		return false
	}
	for _, t := range g.EffectiveTopics(e) {
		if t.HasPrefix(f.Prefix) {
			return true
		}
	}
	return false
}

// FilterEntries returns the subset of entries whose effective topic set
// matches the prefix. Order is preserved; nil input returns nil.
func (f TopicFilter) FilterEntries(g *model.Graph, entries []*model.Entry) []*model.Entry {
	if len(entries) == 0 || f.Prefix.IsZero() {
		return nil
	}
	var out []*model.Entry
	for _, e := range entries {
		if f.MatchEntry(g, e) {
			out = append(out, e)
		}
	}
	return out
}

// ExcludeEntries returns the subset of entries whose effective topic set
// does NOT match the prefix. Mirror of FilterEntries for the
// not(topic(...)) negation primitive (d-tac-e1s). A zero prefix is
// treated as "no exclusion" — every entry passes through unchanged.
func (f TopicFilter) ExcludeEntries(g *model.Graph, entries []*model.Entry) []*model.Entry {
	if f.Prefix.IsZero() {
		return entries
	}
	var out []*model.Entry
	for _, e := range entries {
		if !f.MatchEntry(g, e) {
			out = append(out, e)
		}
	}
	return out
}
