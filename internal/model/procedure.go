package model

import (
	"sort"

	"gopkg.in/yaml.v3"
)

// ProcedureSpecRaw retains the machine part of a procedure entry's
// frontmatter — params, state, steps — as raw YAML nodes. The model does not
// interpret them: the type-system revision contract places structural
// validation at engine load time, so the engine decodes these nodes into its
// typed spec and reports spec errors there. Keeping the raw nodes on the
// entry lets FormatFrontmatter round-trip a procedure losslessly.
type ProcedureSpecRaw struct {
	Params yaml.Node
	State  yaml.Node
	Steps  yaml.Node
}

// ProcedureChain represents a supersession chain of kind: procedure
// decisions — one playbook move's identity across revisions, base-shipped
// and project-captured alike. Mirrors ActorChain: canonicals within a chain
// may change across entries; across chains each canonical is write-once
// (procedure canonicals are their own namespace, separate from actor
// canonicals — see validateProcedureInvariant).
type ProcedureChain struct {
	// Entries are all procedure decisions in the chain, oldest first.
	Entries []*Entry
	// Heads are the chain's non-superseded entries, oldest first. A linear
	// chain has exactly one.
	Heads []*Entry
	// LiveHeads are the Heads that are also not closed — the candidates for
	// execution-head resolution. More than one means the chain is forked: a
	// shipped base successor and a project override compete (or two project
	// overrides do). Lint flags forks as grooming candidates.
	LiveHeads []*Entry
	// Head is the resolved execution head — what a canonical resolves to
	// when the engine loads a move. Among live heads the project
	// (non-embedded) head wins over a shipped base successor; within each
	// class the newest wins, so resolution is total and deterministic even
	// on a fork. A chain with no live heads (retired) resolves to its
	// newest non-superseded entry so status can still render. This decides
	// execution only — resolving the fork in the graph stays a deliberate,
	// merge-style grooming step.
	Head *Entry
	// CanonicalHistory is the ordered list of distinct canonicals ever
	// carried by entries in this chain, oldest first.
	CanonicalHistory []string
	// canonicalSet is an internal lookup for membership checks.
	canonicalSet map[string]bool
}

// HasCanonical reports whether the chain has ever held canonical c.
func (c *ProcedureChain) HasCanonical(name string) bool {
	if c == nil {
		return false
	}
	return c.canonicalSet[name]
}

// Forked reports whether the chain has more than one live head — a shipped
// base successor and a project override (or two overrides) competing.
func (c *ProcedureChain) Forked() bool {
	return c != nil && len(c.LiveHeads) > 1
}

// ProcedureChains groups all kind: procedure decisions into supersession
// chains and resolves each chain's execution head under the project-head-
// wins fork rule. Pure computation — no I/O. Returned in deterministic
// order by resolved head ID.
func (g *Graph) ProcedureChains() []*ProcedureChain {
	superseded := g.supersededSet()
	closed := g.closedSet()

	result := make([]*ProcedureChain, 0)
	for _, group := range g.chainGroups((*Entry).IsProcedure) {
		chain := &ProcedureChain{
			Entries:      group,
			canonicalSet: make(map[string]bool),
		}
		for _, e := range group {
			if e.Canonical != "" && !chain.canonicalSet[e.Canonical] {
				chain.canonicalSet[e.Canonical] = true
				chain.CanonicalHistory = append(chain.CanonicalHistory, e.Canonical)
			}
		}
		for _, e := range group {
			if superseded[e.ID] {
				continue
			}
			chain.Heads = append(chain.Heads, e)
			if !closed[e.ID] {
				chain.LiveHeads = append(chain.LiveHeads, e)
			}
		}
		chain.Head = resolveProcedureHead(chain.LiveHeads, chain.Heads)
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

// resolveProcedureHead picks the execution head from a chain's head
// candidates: live heads when any exist (an active move), otherwise all
// non-superseded heads (a retired move still resolves for status
// rendering). Within the candidate set a project (non-embedded) entry wins
// over a base-shipped one, and the newest wins within each class.
func resolveProcedureHead(live, heads []*Entry) *Entry {
	pick := func(candidates []*Entry) *Entry {
		var best *Entry
		for _, e := range candidates {
			if e.Embedded {
				continue
			}
			if best == nil || e.Time.After(best.Time) {
				best = e
			}
		}
		if best != nil {
			return best
		}
		for _, e := range candidates {
			if best == nil || e.Time.After(best.Time) {
				best = e
			}
		}
		return best
	}
	if len(live) > 0 {
		return pick(live)
	}
	return pick(heads)
}

// ProcedureChainForCanonical returns the procedure chain that has ever held
// the given canonical. Returns nil when no chain matches. The write-once-
// across-chains invariant guarantees at most one match in a well-formed
// graph; callers that care about violations use ProcedureChainsForCanonical.
func (g *Graph) ProcedureChainForCanonical(canonical string) *ProcedureChain {
	if canonical == "" {
		return nil
	}
	for _, chain := range g.ProcedureChains() {
		if chain.HasCanonical(canonical) {
			return chain
		}
	}
	return nil
}

// ProcedureChainsForCanonical returns every procedure chain that has ever
// held the given canonical. In a well-formed graph this is zero or one;
// multiple results indicate a write-once invariant violation, surfaced by
// lint.
func (g *Graph) ProcedureChainsForCanonical(canonical string) []*ProcedureChain {
	if canonical == "" {
		return nil
	}
	var matches []*ProcedureChain
	for _, chain := range g.ProcedureChains() {
		if chain.HasCanonical(canonical) {
			matches = append(matches, chain)
		}
	}
	return matches
}

// ResolveProcedure resolves a procedure canonical to its chain's execution
// head under the project-head-wins fork rule. Returns nil when no chain has
// ever held the canonical. Callers that need liveness (the engine loading a
// move) check the head's derived status; a retired chain still resolves so
// status and show surfaces work.
func (g *Graph) ResolveProcedure(canonical string) *Entry {
	chain := g.ProcedureChainForCanonical(canonical)
	if chain == nil {
		return nil
	}
	return chain.Head
}

// Procedures returns all kind: procedure decisions regardless of chain
// status. Used by list/filter paths.
func (g *Graph) Procedures() []*Entry {
	var procedures []*Entry
	for _, e := range g.Entries {
		if e.IsProcedure() {
			procedures = append(procedures, e)
		}
	}
	return procedures
}
