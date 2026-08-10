package model

// ResolvedRef is the resolution of a target ID to its live head by walking
// supersession transitively. The path runs origin-first to head-last; a target
// that nothing supersedes resolves to a single-element path whose head is the
// origin itself. path is unexported so callers cannot index an intermediate and
// mistake a superseded link for the live head — Head() is the accessor every
// reasoning consumer should reach for, with the stale trail available via
// Path() for rendering only.
type ResolvedRef struct {
	path []string
}

// Head returns the live head of the supersession chain — the entry the
// reference resolves to now. For a non-superseded target this is the target
// itself.
func (r ResolvedRef) Head() string {
	if len(r.path) == 0 {
		return ""
	}
	return r.path[len(r.path)-1]
}

// Origin returns the ID the resolution started from — the literal target of the
// reference, which may be a superseded (stale) entry.
func (r ResolvedRef) Origin() string {
	if len(r.path) == 0 {
		return ""
	}
	return r.path[0]
}

// IsStale reports whether the origin was superseded — the reference points at
// an entry that has since been replaced, so the head differs from the origin.
func (r ResolvedRef) IsStale() bool {
	return len(r.path) > 1
}

// Hops returns the number of supersede steps between origin and head — zero
// when the reference already points at a live entry.
func (r ResolvedRef) Hops() int {
	if len(r.path) == 0 {
		return 0
	}
	return len(r.path) - 1
}

// InboundRef is one incoming reference after supersede resolution: who made it,
// and how far the target it named sits from the live head.
type InboundRef struct {
	Source string
	Hops   int
}

// Path returns the ordered supersession trail from origin to head (origin
// first, head last). Rendering-only — reasoning consumers want Head(). The
// returned slice is a copy; mutating it does not affect the resolution.
func (r ResolvedRef) Path() []string {
	out := make([]string, len(r.path))
	copy(out, r.path)
	return out
}

// ResolveRef walks the supersession chain from the given target ID to its live
// head and returns the ordered path (origin first, head last). A target that is
// not superseded resolves to a single-element path (Head == Origin). This is
// the general read-time generalization of the actor-identity head-walk
// (ActorChain.Head): any reference into a multiply-superseded chain resolves to
// the current entry rather than leaving the reader to hop link by link.
//
// Forks (an entry with more than one superseder) are an anomaly flagged by
// lint; the walk follows the first superseder at each hop and guards against
// cycles, so it always terminates. Distinct from ResolveRefIDs, which resolves
// short-form ID strings to full form — this resolves a full ID forward through
// supersession.
func (g *Graph) ResolveRef(id string) ResolvedRef {
	path := []string{id}
	seen := map[string]bool{id: true}
	current := id
	for {
		supers := g.SupersededBy[current]
		if len(supers) == 0 {
			break
		}
		next := supers[0]
		if seen[next] {
			break // cycle guard — supersession should never cycle
		}
		seen[next] = true
		path = append(path, next)
		current = next
	}
	return ResolvedRef{path: path}
}
