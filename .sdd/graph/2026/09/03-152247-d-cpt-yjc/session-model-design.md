# Session model: bare handles, composition-wide scaffolding, per-instance project targets

Design record from the 2026-09-03 dialogue. Types and functions name the code as it stands on `main` after 20260902-224832-s-tac-x8c.

## What each concept owns

**Project.** A graph, its declared dependencies as its read horizon, and the ports that read and write it. `ProjectRuntimeOptions` keeps `Project`, `DefaultBranch`, `Language`, `Dependencies`, `Graph`, `Targets`, `Branches`, `Recovery`, `Embedder`, `SearchIndex`, `LLM`, `Finalizers`, `ExcludeEmbeddedFromIndex`. Membership is the composition's answer through `AccessResolver.ResolveProject`, asked on every call. What a composition puts into `Embedder` and `LLM` is its own duty (d-cpt-q6n); nothing here is per project by SDD's design.

**Session.** One subject's dialogue with a home project. `SessionMetadata` records `Subject` and `Project` at creation; neither changes. The home project is the default read scope, the default write target, and the root of the dependency closure an instance may move within.

**Scaffolding.** Sessions and staged blobs. Composition-wide, keyed by session ID, namespaced by subject (`SessionRef{Subject, Session}`). Not project data, not durable record (d-cpt-u8o). The stores were already shaped this way: `SessionFilter{Subject, Project}` filters by project, `StagedBlobStore.StagedSessions` enumerates across projects, and the local composition hands the same store to the home runtime and every dependency runtime.

**Handle.** The session ID. Possession says "I may continue this dialogue" within the authenticated principal's scope (d-cpt-aen). The metadata says whose it is and where it points. The resolver says whether that principal still has access to where it points.

## Types

```go
type Clock interface{ Now() time.Time }

type Application struct {
	access   AccessResolver
	sessions SessionStore     // legacyEndStore wraps it here, no longer in NewProjectRuntime
	blobs    StagedBlobStore
	clock    Clock
}

type ApplicationOptions struct {
	Access      AccessResolver
	Sessions    SessionStore
	StagedBlobs StagedBlobStore
	Clock       Clock // default: the system clock
}

func NewApplication(options ApplicationOptions) (*Application, error)
```

`ProjectRuntimeOptions` loses `Sessions`, `StagedBlobs`, and `Now`. `SessionStore`, `StagedBlobStore`, `SessionMetadata`, and `SessionRef` keep their shapes except for the bounded enumeration below. What changes is who is handed the stores.

```go
// SessionAccessRequest carries the facts the continuation policy needs. The
// application has just loaded the session; the composition is not asked to
// load it again. Owner is a Principal so both sides grow together.
type SessionAccessRequest struct {
	Actor   Principal // who is asking
	Owner   Principal // who opened the session
	Session SessionID
	Project ProjectID // the home project
}

type AccessResolver interface {
	ResolvePrincipal(context.Context, RequestIdentity) (Principal, error)
	ResolveParticipant(context.Context, Principal, ProjectID) (string, error)
	ListProjects(context.Context, Principal) (ProjectList, error)
	ResolveProject(context.Context, Principal, ProjectID, Access) (*ProjectRuntime, error)
	ResolveDependency(context.Context, Principal, ProjectID, string) (*ProjectRuntime, error)
	// AuthorizeSession answers whether the actor may continue the session. The
	// core does not own how a principal is authorized; SDD ships OwnerOnly.
	AuthorizeSession(context.Context, SessionAccessRequest) error
}

// OwnerOnly is the shipped default: Actor.Subject == Owner.Subject.
```

No verb on the request. Continuing a dialogue is one act (resume, advance, park, abandon, handle-carried reads). A distinction between viewing and driving joins the request as a field when a composition needs it, as `RecoveryAccessRequest` carries `Verb`.

Alternative rejected: `AuthorizeSession(ctx, actor, sessionID)` with the composition loading the session itself. The application has just loaded it, every handle-carried read would load it twice, and the shipped default would need the session store wired into the resolver.

## The session door

Every session-addressed operation resolves through one path, beside the project-addressed `resolve`:

```go
func (a *Application) resolveSession(ctx context.Context, identity RequestIdentity, id SessionID) (Principal, *ProjectRuntime, StoredSession, error) {
	principal, err := a.resolvePrincipal(ctx, identity)
	// 1. Keyed read, no policy. ErrSessionNotFound reads as "unknown session".
	stored, err := a.sessions.Load(ctx, id)
	// 2. The composition's continuation policy.
	err = a.access.AuthorizeSession(ctx, SessionAccessRequest{
		Actor: principal, Owner: Principal{Subject: stored.Metadata.Subject},
		Session: id, Project: stored.Metadata.Project,
	})
	// 3. Membership in the home project, current on every call.
	runtime, err := a.access.ResolveProject(ctx, principal, stored.Metadata.Project, AccessRead)
	// 4. What LoadWorkflow does today: validateStoredSession, refuse Ended, stampAttachment.
	return principal, runtime, stored, nil
}
```

The store is read before the membership check. Acceptable because the ID carries 128 random bits and the caller learns nothing until both checks pass; the composition's store has no authorization duty.

Session-addressed methods drop `project`: `LoadWorkflow`, `ResumeWorkflow`, `AbandonSession`, and the recovery operations that name a session. Project-addressed methods keep it: `OpenWorkflow`/`StartWorkflow`, `Info`, `Projects`, the reads, `ListSessions`. `WorkflowSession` reaches the stores through `w.app`, not through its runtime; the intent/apply/outcome path in `transition.go` uses `a.sessions` and `a.blobs` with the target runtime's `Graph`.

## Authorization: three questions, three owners

1. Who is asking: `ResolvePrincipal`.
2. May this principal continue this session: `AuthorizeSession`. Local and the example composition use `OwnerOnly`. Sharing a session, or an operator continuing another's dialogue, is a composition's rule. Membership in the home project is still asked separately; a shared session never admits someone into a project they are not a member of.
3. May this principal read project P, or write this mutation to target T: `ResolveProject` with read access at the session door and for every read in another project; `ResolveProject` with write access plus `Targets.Acquire` at the write gate, per target.

Not built, named as a seam: a policy about a prepared mutation (target plus batch), for per-kind write scopes or review before a strategic decision lands. Review before landing is a landing disposition, not a denial, and belongs with the logical-lineage gap (s-cpt-e8x).

SDD's own invariants, never policy: the home project never changes; an ended session is never continued; a handle is required; every write names an explicit target (d-cpt-65i).

## Instances target projects

An instance's target is `MutationTarget{Project, Branch}`, resolved by the application, unknown to the engine (as `workflowBranchFields` is today).

**Where the project lives.** An application-owned fact recorded in the session ledger as a typed event keyed by instance, the instance-level counterpart of the session branch binding (d-tac-ln1). Specs declare nothing; the engine stays unaware; host neutrality (d-cpt-476) is untouched. Rejected: a `project` state field declared in every spec (cross-cutting concern in every procedure, forgotten by a graph-local one), and a host-owned slot on `engine.Instance` (the engine carrying an attribute it cannot interpret).

**Sources, in order.**
1. An optional top-level `project` on `start_procedure`, beside `canonical`, `params`, `parent`, parallel to `start_session`'s. Recorded in the same CAS append as the engine's start event.
2. The anchor, once it resolves to a foreign `<repo-id>:<entry-id>`. Recorded when known, which may be after the first step.
3. The dispatching parent's project at dispatch. Derived.
4. The home project. Derived.

Resolution: the instance's latest recorded target, else its parent's, else home. `next` takes no project; retargeting a running instance is not a case. An instance's project becoming known after start is wanted: a session opens at home, orients, and starts a move elsewhere.

```go
// Branch precedence stays what d-tac-ln1 fixed; the project joins it.
func (w *WorkflowSession) effectiveTarget(store *engine.Store, instance string) (MutationTarget, bool) {
	project := w.instanceProject(instance) // recorded → parent → w.project
	for _, field := range workflowBranchFields {
		if branch, _ := workflowStoreString(store, field); branch != "" {
			return MutationTarget{Project: project, Branch: branch}, false
		}
	}
	if w.branch != "" && project == w.project {
		return MutationTarget{Project: project, Branch: w.branch}, true
	}
	return MutationTarget{Project: project}, false // zero branch: the target project's configured default
}
```

**Two grants, kept distinct.**
- *Closure, a graph fact.* The target lies in the home project's declared dependency closure, transitively, computed from the configurations the composition can reach. True for every principal; this is what keeps reference direction correct. A dependency the composition cannot reach is not a valid target.
- *Membership, a principal fact.* `ResolveProject(principal, B, read)` succeeds; write access is asked again at the write gate.

`ResolveDependency(principal, A, B)` grants reading B inside A's view. `ResolveProject(principal, B)` grants being in B. Different questions; a composition may allow the first and refuse the second. The two resolver methods are not merged. Consequence: a dependency a principal works in must be resolvable as a full project. Locally that runtime is built on demand from the dependency's cache: graph from the cache, its declared dependencies from its cached config, its own index namespace, no `Targets` until the ephemeral clone (d-cpt-n8z) exists. The horizon-member runtime that composes A's view stays as it is.

**What being in B means.** The instance's operations resolve through B's runtime: reads and their horizon (B plus the parts of B's declared dependencies the composition can reach), writes and their authority, `ResolveParticipant(principal, B)` for the name on entries it writes, B's config, actors, and rules. `workflowGraphs.targets` already keys snapshots by `MutationTarget`; `CreateEntry` already builds its validation snapshot from the target runtime's dependencies. `MutationTarget.Validate` compares against the instance's project.

What stays with the home project: the session, its ledger and staged blobs, the shell's framing, the branch binding, and any instance running at home concurrently.

**Branch in B.** The session binding is a fact about the home checkout and applies only there. In B: the instance's own branch field if a procedure set one, else B's configured default. No third source.

**Reference direction.** Resolve-or-block runs on the instance's snapshot. An entry captured in B may reference A only if B declares A; a session whose home is A grants nothing in B. A reference B cannot resolve, including one into a second-level dependency the composition has not cached, blocks the capture at the write gate as an unauthorized dependency does today. Whether working in A should require reachable access to all of A's dependencies, or degrade per reference, is an open question captured separately.

**Free reads** (`search`, `view`, `show`, `read_attachment`, `info`) take an optional `project`, defaulting to the home project with the session's branch (today's `readScope`), validated against the closure and membership. Not derived from a running instance: several moves may run at once, and the never-ambient rule of d-cpt-65i applies to reads as to writes. The served step text of an instance in B names B.

**Evidence.** The read log already records foreign IDs in `<repo-id>:` form, so reads in B ground a capture in B, and a B entry read from home counts the same. Nothing new recorded.

**Serves.** The instance's project is intent-bearing state and projects into the step serve and the resume serve (d-cpt-0tm). Session metadata keeps only the home project.

## Collection

Garbage collection of ended sessions and their staged blobs becomes an operator act on the composition's stores: no identity, no project. `sdd serve` startup locally, a scheduled job elsewhere; who may trigger it is the composition's to gate.

The current `List(ctx, filter)` returns every matching session with its full event log, and `StagedSessions` returns every staging area; neither scales. Enumeration becomes bounded and resumable, storage-neutral:

```go
type SessionFilter struct {
	Subject     string
	Project     ProjectID
	EndedBefore *time.Time // candidate selection from metadata, no event decoding
	After       SessionID  // cursor: IDs are time-prefixed and unique, so ID order is a cursor every store honors
	Limit       int
}
// StagedSessions(ctx, after SessionRef, limit int) takes the same shape.

type CollectSessionsCmd struct {
	Retention time.Duration
	Limit     int
	After     SessionID
}
type CollectSessionsResult struct {
	RemovedSessions []SessionID
	RemovedStaged   []SessionRef
	DrainedIntents  int
	Skipped         []SessionID
	Next            SessionID // empty when the store is exhausted
}
```

A pass processes one page and returns where it stopped; convergence is the caller repeating until `Next` is empty. Sessions a pass keeps (claimed by an in-flight declaration, or unreadable) do not starve later pages because the cursor moves past them. The store decides how it pages. Inside the application a page may be walked as an `iter.Seq2`, but the port stays page-based: the application never holds a storage cursor open across its own appends and deletes.

## Alternatives weighed for the handle

- **Locator port** (`LocateSession(ctx, principal, id) (ProjectID, error)`), consulted only when no project is pinned. Rejected: a second source for a fact `SessionMetadata.Project` already records, kept consistent only by a check after load.
- **Probe every accessible project's store.** Rejected: cost grows with the number of projects, which may be dozens, and a miss is N failures to tell apart from a real miss.
- **Bare authorization func in the options.** Rejected: splits the authorization boundary `AccessResolver` is declared to be.
- **Composite handle only in multi-project compositions.** Rejected: two handle shapes, and the multi-project handle stays redundant.

## Consequences

- `pkg/mcpapp` deletes `handle.go`, `projectFor`, and the project compare in `attachedSession`; `Session` in every result is the bare ID; `Options.Project` goes (a sole project is inferred, several are listed); `start_procedure` gains `project`; the read tools gain `project`.
- `sdd sessions abandon` takes the bare ID.
- Recovery requests drop the redundant project.
- Local composition: hands the two stores and the clock to `NewApplication`; the XDG root keyed by repository stays; answers `ResolveProject` for a reachable dependency from its cache.
- Local writes in a dependency need the ephemeral clone as `Targets` (d-cpt-n8z). Until then a write fails at acquire naming the target.
- Surfaces named by the handle or the new arguments change in one delivery: tool argument descriptions, server instructions, tool-contract snapshot, CLI listing parser, composition example, conformance suites, external consumer wiring.
