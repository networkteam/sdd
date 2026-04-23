package model

// StatusKind is the lifecycle state of an entry derived from graph relationships.
type StatusKind string

const (
	StatusNone            StatusKind = ""               // done signals — terminal facts with no lifecycle state
	StatusActive          StatusKind = "active"         // decision (directive, plan, contract) not closed or superseded
	StatusOpen            StatusKind = "open"           // signal not closed or superseded
	StatusClosedBy        StatusKind = "closed-by"      // closed by another entry (By carries the full ID)
	StatusSupersededBy    StatusKind = "superseded-by"  // superseded by another entry
	StatusCascadeClosedBy StatusKind = "cascade-closed" // role cascade: actor chain head is closed (By = head actor ID)
	StatusCascadeOrphan   StatusKind = "cascade-orphan" // role references a canonical with no matching chain
)

// Status is the computed lifecycle status for an entry. By is populated only
// for compound states (ClosedBy, SupersededBy) and holds the full entry ID of
// the first closer/superseder.
type Status struct {
	Kind StatusKind
	By   string
}

// DerivedStatus returns the computed lifecycle status for an entry, derived
// from graph relationships. Superseded is checked before closed so a
// superseded-then-closed entry (rare) surfaces as superseded. When multiple
// entries close or supersede the target, the first one (by graph insertion
// order) is reported.
//
// Role decisions additionally derive status via the actor-chain cascade
// (see ActorChain / RoleStatus in actor.go). A role whose bound actor chain
// has a closed head surfaces as StatusCascadeClosedBy. A role whose Actor
// does not match any chain's canonical history surfaces as
// StatusCascadeOrphan — an abnormal state flagged by lint.
func (g *Graph) DerivedStatus(e *Entry) Status {
	if ids := g.SupersededBy[e.ID]; len(ids) > 0 {
		return Status{Kind: StatusSupersededBy, By: ids[0]}
	}
	if ids := g.ClosedBy[e.ID]; len(ids) > 0 {
		return Status{Kind: StatusClosedBy, By: ids[0]}
	}
	switch e.Type {
	case TypeSignal:
		// Done signals are terminal — facts of execution with no lifecycle state.
		// If something does close a done signal (the rare "corrective done" case),
		// the ClosedBy check above fires first.
		if e.Kind == KindDone {
			return Status{Kind: StatusNone}
		}
		return Status{Kind: StatusOpen}
	case TypeDecision:
		if e.IsRole() {
			return g.deriveRoleStatus(e)
		}
		return Status{Kind: StatusActive}
	}
	return Status{Kind: StatusNone}
}

// deriveRoleStatus applies the role cascade: if the actor chain bound via
// Actor is unresolved, return cascade-orphan; if its head is closed, return
// cascade-closed-by that closer; otherwise active.
func (g *Graph) deriveRoleStatus(role *Entry) Status {
	chain := g.ChainForCanonical(role.Actor)
	if chain == nil || chain.Head == nil {
		return Status{Kind: StatusCascadeOrphan}
	}
	if ids := g.ClosedBy[chain.Head.ID]; len(ids) > 0 {
		return Status{Kind: StatusCascadeClosedBy, By: ids[0]}
	}
	return Status{Kind: StatusActive}
}
