package model

import "sort"

// ActorChain represents a supersession chain of kind: actor signals — all
// actor entries linked by supersedes (transitively) plus their shared
// canonical history. Canonicals within a chain may change across entries
// (e.g., typo correction supersedes an earlier spelling); across chains
// each canonical is write-once (see validateActorInvariant in lint).
type ActorChain struct {
	// Entries are all actor signals in the chain, ordered by time (oldest
	// first).
	Entries []*Entry
	// Head is the chain's current head — the actor signal not superseded
	// by any other entry in the chain. Capture-time checks resolve against
	// the head's canonical; derivation-time checks walk CanonicalHistory.
	Head *Entry
	// CanonicalHistory is the ordered list of distinct canonicals ever
	// carried by entries in this chain, oldest first.
	CanonicalHistory []string
	// canonicalSet is an internal lookup for membership checks.
	canonicalSet map[string]bool
}

// HasCanonical reports whether the chain has ever held canonical c.
func (c *ActorChain) HasCanonical(name string) bool {
	if c == nil {
		return false
	}
	return c.canonicalSet[name]
}

// ActorChains groups all kind: actor signals into supersession chains.
// Each chain surfaces its head, full entry list, and canonical history.
// Pure computation — no I/O. Returned in deterministic order by head ID.
func (g *Graph) ActorChains() []*ActorChain {
	superseded := g.supersededSet()
	byRoot := make(map[string]*ActorChain)

	rootOf := func(e *Entry) *Entry {
		current := e
		for {
			var parent *Entry
			for _, id := range current.Supersedes {
				p, ok := g.ByID[id]
				if ok && p.IsActor() {
					parent = p
					break
				}
			}
			if parent == nil {
				return current
			}
			current = parent
		}
	}

	for _, e := range g.Entries {
		if !e.IsActor() {
			continue
		}
		root := rootOf(e)
		chain, ok := byRoot[root.ID]
		if !ok {
			chain = &ActorChain{canonicalSet: make(map[string]bool)}
			byRoot[root.ID] = chain
		}
		chain.Entries = append(chain.Entries, e)
		if e.Canonical != "" && !chain.canonicalSet[e.Canonical] {
			chain.canonicalSet[e.Canonical] = true
			chain.CanonicalHistory = append(chain.CanonicalHistory, e.Canonical)
		}
	}

	for _, chain := range byRoot {
		sort.SliceStable(chain.Entries, func(i, j int) bool {
			return chain.Entries[i].Time.Before(chain.Entries[j].Time)
		})
		// Head = the one not superseded by any other entry.
		for _, e := range chain.Entries {
			if !superseded[e.ID] {
				chain.Head = e
				break
			}
		}
	}

	result := make([]*ActorChain, 0, len(byRoot))
	for _, chain := range byRoot {
		result = append(result, chain)
	}
	sort.Slice(result, func(i, j int) bool {
		switch {
		case result[i].Head == nil && result[j].Head == nil:
			return false
		case result[i].Head == nil:
			return true
		case result[j].Head == nil:
			return false
		default:
			return result[i].Head.ID < result[j].Head.ID
		}
	})
	return result
}

// ActiveActorHeads returns the head actor signal of every active actor-identity
// chain — the head must not be closed. Ordered by head ID.
func (g *Graph) ActiveActorHeads() []*Entry {
	closed := g.closedSet()
	var active []*Entry
	for _, chain := range g.ActorChains() {
		if chain.Head == nil {
			continue
		}
		if closed[chain.Head.ID] {
			continue
		}
		active = append(active, chain.Head)
	}
	return active
}

// ChainForCanonical returns the actor-identity chain that has ever held the
// given canonical. Returns nil when no chain matches. The write-once-across-
// chains invariant guarantees at most one match in a well-formed graph;
// callers that care about violations should use ChainsForCanonical.
func (g *Graph) ChainForCanonical(canonical string) *ActorChain {
	if canonical == "" {
		return nil
	}
	for _, chain := range g.ActorChains() {
		if chain.HasCanonical(canonical) {
			return chain
		}
	}
	return nil
}

// ChainsForCanonical returns every actor-identity chain that has ever held
// the given canonical. In a well-formed graph this is zero or one; multiple
// results indicate a violation of the write-once-across-chains invariant
// and are surfaced by lint.
func (g *Graph) ChainsForCanonical(canonical string) []*ActorChain {
	if canonical == "" {
		return nil
	}
	var matches []*ActorChain
	for _, chain := range g.ActorChains() {
		if chain.HasCanonical(canonical) {
			matches = append(matches, chain)
		}
	}
	return matches
}

// ResolveRoleChain returns the actor-identity chain a role binds to via the
// role's Actor canonical. Returns nil when the role is not kind: role, has
// no Actor, or the canonical resolves to no chain (orphan — surfaced by
// lint). Used by both DerivedStatus and finders that need to render the
// resolved chain alongside the role.
func (g *Graph) ResolveRoleChain(role *Entry) *ActorChain {
	if !role.IsRole() {
		return nil
	}
	return g.ChainForCanonical(role.Actor)
}

// ActiveRoles returns all kind: role decisions that are derived-active by
// the cascade (DerivedStatus == StatusActive). Ordered by bound actor
// canonical then role entry ID so presenters can group without re-sorting.
func (g *Graph) ActiveRoles() []*Entry {
	var roles []*Entry
	for _, e := range g.Entries {
		if !e.IsRole() {
			continue
		}
		if g.DerivedStatus(e).Kind == StatusActive {
			roles = append(roles, e)
		}
	}
	sort.Slice(roles, func(i, j int) bool {
		if roles[i].Actor != roles[j].Actor {
			return roles[i].Actor < roles[j].Actor
		}
		return roles[i].ID < roles[j].ID
	})
	return roles
}

// Roles returns all kind: role decisions regardless of derived status.
// Used by list/filter paths that render all roles (including retired).
func (g *Graph) Roles() []*Entry {
	var roles []*Entry
	for _, e := range g.Entries {
		if e.IsRole() {
			roles = append(roles, e)
		}
	}
	return roles
}

// Actors returns all kind: actor signals regardless of chain status.
// Used by list/filter paths.
func (g *Graph) Actors() []*Entry {
	var actors []*Entry
	for _, e := range g.Entries {
		if e.IsActor() {
			actors = append(actors, e)
		}
	}
	return actors
}
