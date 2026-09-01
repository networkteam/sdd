package model

import "github.com/networkteam/sdd/pkg/application/types"

// Involvement and FocusWhen are defined in pkg/application/types — the
// exported surface names them, so the definitions live in the cycle-free
// public leaf (s-tac-ah2).
type (
	Involvement = types.Involvement
	FocusWhen   = types.FocusWhen
)

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

	// An annotation's own declared labels are its own topics — it is a
	// member of every topic it assigns. Without this, an annotation that
	// tags other entries with "catch-up-scaling" would not itself surface
	// under topic("catch-up-scaling"), an asymmetry that left annotation
	// entries invisible to topic filters and renders (d-tac-6tz). Members
	// sub-selections only narrow which *refs* get a label; the annotation
	// owns every label it declares regardless.
	if e.IsAnnotation() {
		for _, t := range e.AnnotationTopics {
			p, err := ParseTopicPath(t.Label)
			if err != nil {
				continue // malformed label ignored here; lint surfaces it
			}
			paths = append(paths, p)
		}
	}

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
		pool = RefIDs(ann.Refs)
	}
	for _, m := range pool {
		if m == entryID {
			return true
		}
	}
	return false
}
