package model

import (
	"fmt"
	"time"
)

// Involvement is one entry in a kind: focus decision's involvement: list.
// Required: Target (entry ID this involvement is about). Optional: Actors
// (canonical-only; per-involvement override of focus-level default — explicit
// empty list means "deliberately unattributed / pull-available", distinct
// from the unset case which inherits the focus-level default). Optional:
// When (per-involvement temporal scope override).
type Involvement struct {
	Target string
	// Actors carries canonical-only names. Distinguishing "unset" (inherit
	// focus-level default) from "explicit empty" (deliberately
	// pull-available) requires the ActorsSet field; YAML's natural decoding
	// merges both into a nil slice, so we capture the distinction at
	// frontmatter parse time.
	Actors    []string
	ActorsSet bool
	When      *FocusWhen
}

// FocusWhen is the temporal scope for a focus or one of its involvement
// triples. At least one of From or To must be set when the field is present;
// the absent end means "open-ended in that direction." Dates are ISO
// YYYY-MM-DD format on disk and parsed into time.Time at validation time.
type FocusWhen struct {
	From string `yaml:"from,omitempty"`
	To   string `yaml:"to,omitempty"`
}

// IsZero reports whether neither end is set. A FocusWhen with IsZero() == true
// is invalid in frontmatter (the field should have been omitted entirely);
// validators surface this as a shape error.
func (w *FocusWhen) IsZero() bool {
	return w == nil || (w.From == "" && w.To == "")
}

// Validate checks ISO date shape on each end and that at least one end is set.
// Returns a descriptive error or nil. Pure — no I/O, safe to call from
// validators and finders.
func (w *FocusWhen) Validate() error {
	if w == nil {
		return nil
	}
	if w.From == "" && w.To == "" {
		return fmt.Errorf("when: at least one of `from` or `to` is required")
	}
	if w.From != "" {
		if _, err := time.Parse("2006-01-02", w.From); err != nil {
			return fmt.Errorf("when.from: %w", err)
		}
	}
	if w.To != "" {
		if _, err := time.Parse("2006-01-02", w.To); err != nil {
			return fmt.Errorf("when.to: %w", err)
		}
	}
	return nil
}

// IsFocus reports whether this decision is a focus declaration — a
// dual-lifecycle entry carrying involvement triples.
func (e *Entry) IsFocus() bool {
	return e.Type == TypeDecision && e.Kind == KindFocus
}

// ResolveActors returns the effective actor canonicals for an involvement
// target on a focus entry: the involvement's own Actors if set (including
// the explicit empty case), else the focus-level default (e.IsActors), else
// nil. The empty case carries semantic weight — "pull-available" — so
// callers must distinguish nil from len()==0 by checking ActorsSet.
func (e *Entry) ResolveActors(inv Involvement) []string {
	if inv.ActorsSet {
		return inv.Actors
	}
	return e.FocusActors
}

// ResolveWhen returns the effective temporal scope for an involvement —
// per-involvement When if set, else the focus-level default, else nil.
func (e *Entry) ResolveWhen(inv Involvement) *FocusWhen {
	if inv.When != nil {
		return inv.When
	}
	return e.FocusWhen
}

// Focuses returns all kind: focus decisions regardless of derived status.
// Used by list/filter paths.
func (g *Graph) Focuses() []*Entry {
	var out []*Entry
	for _, e := range g.Entries {
		if e.IsFocus() {
			out = append(out, e)
		}
	}
	return out
}

// ActiveFocuses returns kind: focus decisions that are not closed and not
// superseded. Ordered by entry time, oldest first.
func (g *Graph) ActiveFocuses() []*Entry {
	closed := g.closedSet()
	superseded := g.supersededSet()
	var out []*Entry
	for _, e := range g.Entries {
		if !e.IsFocus() {
			continue
		}
		if closed[e.ID] || superseded[e.ID] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Annotations returns all kind: annotation signals regardless of derived
// status. Used by list/filter paths and by topic-membership lookup.
func (g *Graph) Annotations() []*Entry {
	var out []*Entry
	for _, e := range g.Entries {
		if e.IsAnnotation() {
			out = append(out, e)
		}
	}
	return out
}

// EffectiveTopics returns the merged topic-path set for an entry: inline
// `topics:` declared on the entry's own frontmatter unioned with topics
// declared by every kind: annotation entry whose refs (or per-topic members
// sub-selection) include this entry. Deduplicated case-insensitively with
// first-seen casing winning. Used by the topic(L) filter and by display
// rendering. Pure — uses the graph's reverse-ref index for annotation
// lookups, no I/O.
func (g *Graph) EffectiveTopics(e *Entry) []TopicPath {
	if e == nil {
		return nil
	}
	var paths []TopicPath
	paths = append(paths, e.Topics...)

	for _, refdByID := range g.RefsTo[e.ID] {
		ann, ok := g.ByID[refdByID]
		if !ok || !ann.IsAnnotation() {
			continue
		}
		for _, t := range ann.AnnotationTopics {
			if !annotationMembers(ann, t, e.ID) {
				continue
			}
			p, err := ParseTopicPath(t.Label)
			if err != nil {
				continue // malformed label ignored here; lint surfaces it
			}
			paths = append(paths, p)
		}
	}
	return CanonicalizeTopicPaths(paths)
}

// annotationMembers reports whether entryID is in the set of members an
// annotation's topic assigns. The plain-string item form (Members nil/empty)
// means "all of the annotation's refs"; the mapping form restricts to the
// listed subset.
func annotationMembers(ann *Entry, t AnnotationTopic, entryID string) bool {
	pool := t.Members
	if len(pool) == 0 {
		pool = ann.Refs
	}
	for _, m := range pool {
		if m == entryID {
			return true
		}
	}
	return false
}
