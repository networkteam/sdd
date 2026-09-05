package application

import (
	"context"

	"github.com/networkteam/sdd/pkg/application/types"
)

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
type ProjectID = types.ProjectID

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

// SessionAccessRequest carries what a continuation policy needs to answer
// whether Actor may continue the session Owner opened. The application has
// just loaded the session; the composition is not asked to load it again. It
// names no verb: continuing a dialogue is one act (d-cpt-yjc).
type SessionAccessRequest struct {
	Actor   Principal
	Owner   Principal
	Session SessionID
	Project ProjectID
}

// OwnerOnly is the shipped continuation policy: only the principal who opened
// a session may continue it. Compositions that share sessions replace it.
func OwnerOnly(_ context.Context, request SessionAccessRequest) error {
	if request.Actor.Subject == "" || request.Actor.Subject != request.Owner.Subject {
		return &ApplicationError{Code: ErrorSessionOwnership, Message: "session belongs to another principal"}
	}
	return nil
}

// AccessResolver is the single identity, project-access, session-continuation,
// and dependency authorization boundary. Implementations must resolve current
// authorization from ctx on every call; previously returned principals and
// runtimes are not proof of current access. Every call arrives inside a
// request: the application never calls the resolver from a connection
// lifecycle.
type AccessResolver interface {
	ResolvePrincipal(context.Context, RequestIdentity) (Principal, error)
	// ResolveParticipant names the graph participant the principal appears as
	// in the project — the name framing, authorship, and WIP markers carry. It
	// is asked once the project is known, because a person may appear under a
	// different name per project (s-cpt-ny6).
	ResolveParticipant(context.Context, Principal, ProjectID) (string, error)
	ListProjects(context.Context, Principal) (ProjectList, error)
	ResolveProject(context.Context, Principal, ProjectID, Access) (*ProjectRuntime, error)
	// ResolveDependency maps one dependency the project declares — a repo ID
	// from its committed configuration — to the runtime of the project that
	// carries it, or refuses. The declared string and the resolved project's
	// ID coincide only in the local composition. The application asks per
	// declared dependency, on every view over the horizon and on every
	// dependency-closure walk; a composition whose answer is costly caches it
	// itself, since only it knows when a mapping goes stale.
	ResolveDependency(context.Context, Principal, ProjectID, string) (*ProjectRuntime, error)
	// AuthorizeSession answers whether the actor may continue the session.
	// Membership in the session's home project is asked separately, so a
	// shared session never admits anyone into a project they cannot read.
	AuthorizeSession(context.Context, SessionAccessRequest) error
}
