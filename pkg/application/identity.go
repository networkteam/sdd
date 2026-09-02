package application

import "context"

// RequestIdentity is current-request authentication material supplied by a
// transport composition. SDD treats Subject as opaque and does not interpret
// application-specific roles.
type RequestIdentity struct {
	Subject    string
	Scopes     []string
	Attributes map[string]any
}

// Principal is the stable identity resolved from a current request. It is
// binding and audit data, never cached authorization. The participant the
// subject appears as is resolved separately, per project (s-cpt-ny6).
type Principal struct {
	Subject string
}

// ProjectID is a composition's stable project identity.
type ProjectID string

// ProjectRef is the only project identity exposed in project-scoped results.
type ProjectRef struct {
	ID          ProjectID
	DisplayName string
}

// Access is the permission required for the current operation.
type Access string

const (
	AccessRead  Access = "read"
	AccessWrite Access = "write"
)

// ProjectState describes whether a listed project can be used immediately.
type ProjectState string

const (
	ProjectReady          ProjectState = "ready"
	ProjectActionRequired ProjectState = "action_required"
	ProjectUnavailable    ProjectState = "unavailable"
)

type ProjectAction struct {
	ID          string
	DisplayName string
	State       ProjectState
	ActionURL   string
	Reason      string
}

type ProjectSummary struct {
	ProjectRef
	SourceID string
	CanRead  bool
	CanWrite bool
	State    ProjectState
}

type ProjectList struct {
	Actions  []ProjectAction
	Projects []ProjectSummary
}

// AccessResolver is the single identity, project-access, and dependency
// authorization boundary. Implementations must resolve current authorization
// from ctx on every call; previously returned principals and runtimes are not
// proof of current access. Every call arrives inside a request: the
// application never calls the resolver from a connection lifecycle.
type AccessResolver interface {
	ResolvePrincipal(context.Context, RequestIdentity) (Principal, error)
	// ResolveParticipant names the graph participant the principal appears as
	// in the project — the name framing, authorship, and WIP markers carry. It
	// is asked once the project is known, because a person may appear under a
	// different name per project (s-cpt-ny6).
	ResolveParticipant(context.Context, Principal, ProjectID) (string, error)
	ListProjects(context.Context, Principal) (ProjectList, error)
	ResolveProject(context.Context, Principal, ProjectID, Access) (*ProjectRuntime, error)
	ResolveDependency(context.Context, Principal, ProjectID, string) (*ProjectRuntime, error)
}
