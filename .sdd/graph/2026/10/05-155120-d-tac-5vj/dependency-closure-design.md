# Shared dependency-closure walk: design record

Design dialogue, Christopher and Claude, 2026-10-04 to 2026-10-05, prompted by Christopher's review of
the experimental graph export (https://github.com/networkteam/sdd/pull/22). Code references are to the
PR #22 branch `feat/experimental-json-export` (2ccdd234) and `main` at the same time.

## Starting point

- The export walks the declared-dependency closure with
  `model.DependencyClosure(root string, direct []string, declared func(repoID string) ([]string, error))`
  in `internal/model/crossrepo_selection.go`; the CLI (`internal/cliapp/export.go`) passes a callback that
  freshens each reached repo's cache and reads its committed config through `ReadProjectConfigFS`.
- Serve walks its own closure in `Application.inDependencyClosure` (`pkg/application/workflow_project.go`),
  private to the application, so the export copied its logic.
- Review comments: the walk passes dependencies around as arguments instead of being a method on a type
  that owns them; repo IDs travel as `[]string` (typed separately, 20261005-154635-d-tac-1a7).

## The two walks compared

| | CLI / local MCP | serve (`pkg/application`) |
|---|---|---|
| a dependency is named by | a declared repo ID | a declared repo ID |
| resolved to | a connected-repo cache, the same answer whoever declares it | a `ProjectRuntime` via `ResolveDependency(ctx, principal, declaringProject, repoID)`: per principal, per declaring project, access-checked; repo ID and project ID coincide only in the local composition |
| declaration read from | the cached committed `.sdd/config.yaml` (`ReadProjectConfigFS`) | `runtime.options.Dependencies`, re-read from the materialized source view for reads |
| graph read from | `LoadGraph` on the cache directory | an acquired snapshot that must be released; entries copied out eagerly |
| closure keyed by | repo ID | project ID |
| unresolvable dependency | stays listed (the export marks it unavailable), nothing behind it reached | not part of the closure, nothing behind it reached |
| walk serves | which graphs to load and export | whether a session may work in a target project (early exit) |
| I/O | synchronous file reads | context-bound, access-checked per call |

Both split the same two things, what a repo declares and what it contains; they differ in how a declared
ID resolves. The traversal itself (breadth-first, each node once, cycle-safe, root excluded, early stop)
is identical.

## Decided shape

```go
// internal/model
// DependencyResolver answers one step of a declared-dependency walk: the
// dependencies k declares, as the keys they resolve to.
type DependencyResolver[K comparable] interface {
	Dependencies(ctx context.Context, k K) ([]K, error)
}

// DependencyClosure yields every key reached from root once, breadth-first,
// in first-reached order, never root itself; breaking out of the loop stops
// the walk.
func DependencyClosure[K comparable](ctx context.Context, root K, r DependencyResolver[K]) iter.Seq2[K, error]
```

- The context travels in the call chain; no resolver stores it.
- CLI resolver (finders side), keyed by `RepoID`: holds the local per-repo config and the read-only
  repo registry; answers the local repo from its config and a connected dependency from its cached
  committed config, read through the same reader the local composition uses for dependency caches
  (`ReadProjectConfigFS`, which honours a dependency's own `graph_dir`). A repo it cannot read returns no
  dependencies: it stays in the closure with nothing reached behind it.
- Serve resolver (`pkg/application`), keyed by `ProjectID`: built per walk; reads the current project's
  declarations (from its materialized source view for reads, as today), resolves each through
  `AccessResolver.ResolveDependency`, skips what does not resolve, and keeps the runtimes it resolved so
  the next step can read their declarations. `inDependencyClosure` becomes a loop over the walk that
  returns on the target and refuses after the walk ends; behaviour unchanged, the existing gate tests
  (`pkg/application/workflow_project_test.go`, `pkg/mcpapp/multiproject_test.go`) pass unchanged.
- `MultiGraph` stays the cross-graph read model over graphs only: local graph, direct dependencies as the
  bare-ID resolution scope (never widened to the closure), member graphs loaded lazily.
- Export: the finder computes the closure with the CLI resolver; `ExportQuery` loses `DependencyIDs`;
  cited-entry selection (`CitedAcross`) takes the closure as typed scope. The CLI refreshes caches
  between closure computations (compute, `EnsureReposFresh`, recompute while it reports a change), then
  assembles the graphs once.

Sketch of the serve resolver step:

```go
func (d *accessDeclarations) Dependencies(ctx context.Context, id ProjectID) ([]ProjectID, error) {
	current := d.runtimes[id]
	if d.readConfig && id != d.home {
		view, err := d.sourceView(current, "")
		if err != nil {
			return nil, err
		}
		current = view.runtime
	}
	var out []ProjectID
	for _, dep := range current.options.Dependencies {
		rt, err := d.access.ResolveDependency(ctx, d.principal, id, dep)
		if err != nil || rt == nil {
			continue // unresolvable: not part of the closure
		}
		d.runtimes[rt.options.Project.ID] = rt
		out = append(out, rt.options.Project.ID)
	}
	return out, nil
}
```

## Alternatives weighed

1. **Method on the per-repo config with a reader interface** (`PerRepoConfig.DependencyClosure(reader)`,
   the registry as reader): removes root and direct dependencies from the arguments but leaves a second
   path beside the cross-graph assembly and does nothing for serve. Superseded in the dialogue by the
   options below.
2. **`MultiGraph` owns the closure with a declaration stage**: each member's committed config loads on
   first touch, its graph only when entries are needed (`*Graph` itself cannot be lazy: its fields are
   read directly across the codebase). Attractive for the CLI, argument-free. Rejected because serve
   constructs a `MultiGraph` too (`pkg/application/application.go`, eager over direct dependencies) and
   its declarations resolve per principal through the access resolver: its member source would need a
   `Declaration` that always refuses, and running serve's walk through the assembly would make the model
   learn principals, contexts, project identities and snapshot release.
3. **A separate `MultiGraphMetadata` assembly** for repo-level metadata: closure computable without any
   graph. Rejected: a second assembly over the same caches that must agree with the first on the
   connected set, the loader and invalidation, for a need (metadata with no graph in play) no consumer has.
4. **Keep two walks** (the export's copy and serve's): rejected; an access gate and a reader would drift.
5. **Walk taking function arguments** (the PR #22 shape): rejected by the review.

The visitor idea (Christopher) is what unifies the walks; in Go its natural form is an iterator over a
resolver interface, with early exit by `break`. The walk is generic because the keys differ.

## Accepted tensions

- The shared part is small, roughly twenty lines of breadth-first walk; the generic interface earns its
  place through consistency between an access gate and a reader, not through saved code.
- The export still sits on internal finders, not on `pkg/application`, like the other CLI reads today;
  the structured read the application is to return stays the target.
- Each resolver states its own rule for unresolvable dependencies; the walk does not decide it.

## Delivery

A follow-up pull request to #22, after #22 merges with its review fixes, together with typed repo IDs
(20261005-154635-d-tac-1a7). Serve moves onto the walk in the same pull request, as its own commit.
