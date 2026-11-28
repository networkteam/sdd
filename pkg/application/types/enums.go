package types

import "strings"

// EntryType is an entry's type.
type EntryType string

const (
	TypeSignal   EntryType = "signal"
	TypeDecision EntryType = "decision"
)

// Layer is an entry's layer.
type Layer string

const (
	LayerStrategic   Layer = "strategic"
	LayerConceptual  Layer = "conceptual"
	LayerTactical    Layer = "tactical"
	LayerOperational Layer = "operational"
	LayerProcess     Layer = "process"
)

// Kind is an entry's sub-type; which kinds a type allows is the model's
// concern.
type Kind string

const (
	KindGap        Kind = "gap"
	KindFact       Kind = "fact"
	KindQuestion   Kind = "question"
	KindInsight    Kind = "insight"
	KindDone       Kind = "done"
	KindActor      Kind = "actor"
	KindAnnotation Kind = "annotation"

	KindDirective  Kind = "directive"
	KindActivity   Kind = "activity"
	KindPlan       Kind = "plan"
	KindContract   Kind = "contract"
	KindAspiration Kind = "aspiration"
	KindRole       Kind = "role"
	KindFocus      Kind = "focus"
	KindProcedure  Kind = "procedure"
)

// StatusKind is an entry's derived lifecycle state.
type StatusKind string

const (
	// StatusNone is the state of an entry without lifecycle (a done signal),
	// named so it differs from an absent value.
	StatusNone            StatusKind = "none"
	StatusActive          StatusKind = "active"
	StatusSettled         StatusKind = "settled"
	StatusOpen            StatusKind = "open"
	StatusClosedBy        StatusKind = "closed-by"
	StatusSupersededBy    StatusKind = "superseded-by"
	StatusCascadeClosedBy StatusKind = "cascade-closed"
	StatusCascadeOrphan   StatusKind = "cascade-orphan"
)

// Status is an entry's derived lifecycle status.
type Status struct {
	Kind StatusKind
	// By is the live closing or superseding head for the compound kinds, as a
	// display ID.
	By string
	// SupersedePath is the origin→head trail of display IDs when superseded.
	SupersedePath []string
}

// Intent is a directive's stored posture.
type Intent string

const (
	IntentPending Intent = "pending"
	IntentGuiding Intent = "guiding"
	IntentSettled Intent = "settled"
)

// Confidence is an entry's stored confidence, kept as written; the
// constants are the values capture uses.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// RefKind is the relationship a stored reference describes.
type RefKind string

const (
	RefKindGroundedIn RefKind = "grounded-in"
	RefKindBuildsOn   RefKind = "builds-on"
	RefKindRefines    RefKind = "refines"
	RefKindAddresses  RefKind = "addresses"
	RefKindSurfaces   RefKind = "surfaces"
	RefKindSurfacedBy RefKind = "surfaced-by"
	RefKindDependsOn  RefKind = "depends-on"
	RefKindRequiredBy RefKind = "required-by"
	RefKindRelated    RefKind = "related"
	// RefKindUnknown is a legacy reference stored without a kind.
	RefKindUnknown RefKind = "unknown"
)

// LinkKind names a show edge as the citing entry sets it: a RefKind value,
// or one of the constants below.
type LinkKind string

const (
	LinkCloses     LinkKind = "closes"
	LinkSupersedes LinkKind = "supersedes"
	LinkRefs       LinkKind = "refs"
)

// Relation is one edge between a show tree node and its parent.
type Relation string

const (
	RelationRefs         Relation = "refs"
	RelationCloses       Relation = "closes"
	RelationSupersedes   Relation = "supersedes"
	RelationRefdBy       Relation = "refd-by"
	RelationClosedBy     Relation = "closed-by"
	RelationSupersededBy Relation = "superseded-by"
)

// ProcedureClass is a procedure's execution role.
type ProcedureClass string

const (
	ProcedureClassMove  ProcedureClass = "move"
	ProcedureClassShell ProcedureClass = "shell"
	ProcedureClassTask  ProcedureClass = "task"
)

// TopicPath is a hierarchical topic as its components.
type TopicPath []string

// String joins the components with "/".
func (p TopicPath) String() string {
	return strings.Join(p, "/")
}
