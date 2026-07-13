# Public API inner evaluation

## Scope

Inner verification of the implementation governed by 20260712-234500-d-tac-x1m, the public application contract 20260712-104530-d-tac-1mb, the subsystem-port directive 20260712-194427-d-tac-wjq, and the repository architecture and fail-loud rules. The outer lens is separately covered by 20260713-160800-s-tac-luv.

## Verification run

- `go test ./...`: passed across the root module.
- `go test ./...` in `examples/extendingsdd`: passed.
- `go vet ./...`: passed.
- `golangci-lint run ./...`: 0 issues.
- `git diff --check 941ec4a^..HEAD`: passed.
- `go test -race . ./mcpapp`: passed.
- Twenty repetitions of focused durable-session, transition, and multi-project tests: passed.
- Ten repetitions of the real HTTP identity/runtime tests: failed four times because asynchronous disconnect watchers continued using TempDir-backed session storage during cleanup.
- An isolated external-module probe forced the second rename of a two-file filesystem graph mutation to fail. `Apply` returned `MutationUnknown`, but the first file was already visible, disproving atomic batch apply.

## Judgment

The implementation is a strong partial result, not sound enough to release or close the active plan. It establishes a genuine protocol-neutral application, public subsystem ports, local adapters, shared MCP composition, durable session primitives, multi-project authorization, an external example, and broad ordinary test coverage. Several normative guarantees nevertheless remain unmet.

## Confirmed findings

1. **Canonical graph apply is not atomic.** `FilesystemGraphStore.Apply` renames writes and performs deletes sequentially. Mid-batch failure exposes partial state, and reconciliation cannot complete or roll back the batch once the revision changes.
2. **The public root package is over-concentrated.** Twenty-three production files totaling 4,501 lines combine facade, application runtime, ports, local adapters, storage, search, workflows, and writes. A clean direction is a tiny root facade over nested `application`, `ports`, and `local` packages, with `mcpapp` depending on the application surface and `sddtest` on ports; nested packages must not import the root facade.
3. **Hosted server shutdown is not drainable.** Context cancellation should trigger graceful shutdown, stop new work, finish connection watcher leave-session handling with a bounded context, and provide a completion barrier before stores are torn down. The repeated HTTP test demonstrates the current race.
4. **New durable/workflow paths violate fail-loud policy.** `ApplyPrepared` discards staged-retention release failure after intent persistence fails; workflow advance, abandon, and park discard shell-serving errors.
5. **Participant authority remains split.** This is already captured in 20260713-163153-s-cpt-94l and should not be duplicated.
6. **Completion evidence is incomplete.** The real public-runtime identity test does not reach mutation authorization or changed-user rejection; no temporary external-module release smoke without a local replace or published consumable version is evidenced; and the required per-slice commit-citing done signals are absent.

## Package dependency observation

The present DAG is acyclic but lopsided: `mcpapp` imports root plus `internal/engine` and `internal/query`; `sddtest` imports root; the root imports ten internal subsystems. Passing tests prove behavior, not maintainable dependency boundaries.
