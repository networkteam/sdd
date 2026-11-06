package model

import (
	"sort"
	"strings"
)

// ActorChain represents a supersession chain of kind: actor signals — one
// participant identity across its revisions.
type ActorChain struct {
	CanonicalChain
	// Head is the chain's current head — its oldest non-superseded actor
	// signal. Capture-time checks resolve against the head's canonical;
	// derivation-time checks walk CanonicalHistory.
	Head *Entry
}

// ActorChains groups all kind: actor signals into supersession chains.
// Each chain surfaces its head, full entry list, and canonical history.
// Pure computation — no I/O. Returned in deterministic order by head ID.
func (g *Graph) ActorChains() []*ActorChain {
	result := make([]*ActorChain, 0)
	for _, group := range g.chainGroups((*Entry).IsActor) {
		chain := &ActorChain{CanonicalChain: g.newCanonicalChain(group)}
		if len(chain.Heads) > 0 {
			chain.Head = chain.Heads[0]
		}
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

// AliasCanonical returns the canonical of the active actor declaring name as
// an alias. A name that is itself an active canonical, or that no active
// actor declares, reports false.
func (g *Graph) AliasCanonical(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	active := g.ActiveActorHeads()
	for _, a := range active {
		if a.Canonical == name {
			return "", false
		}
	}
	for _, a := range active {
		for _, alias := range a.Aliases {
			if strings.TrimSpace(alias) == name {
				return a.Canonical, true
			}
		}
	}
	return "", false
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
