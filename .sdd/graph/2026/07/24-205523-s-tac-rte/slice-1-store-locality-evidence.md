# Slice 1 evidence — store locality and acknowledged offline migration

- Branch: `worktree-session-branch-binding`
- Commit: `4375b3a` (`feat: add acknowledged machine-global session relocation`)
- Scope: store locality portion of `20260722-112853-d-tac-ln1`

## Delivered

Sessions and staged blobs now live under the XDG state root keyed by stable repository identity, using the Git common directory for identity-less repositories so linked worktrees share one store. `sdd init` performs crash-safe, aggregate, move-if-absent relocation with local session identity rewriting, rooted path/category confinement, special-file rejection, non-clobber collision preservation, durable manifests/journals, cooperating-writer locks, and retry convergence. Runtime session discovery and recovery are shared across worktrees. `sdd serve` startup behavior is unchanged; pending/interrupted/abandoned state is exposed through standing notices.

## Explicit acceptance-criterion refinements

1. The plan's automatic in-tree relocation criterion is refined to an explicit offline acknowledgement when any supported durable session identity transition is pending: material in-tree state or a machine-global `local/<hash>` store transitioning to the repository-ID key. Interactive `sdd init` prompts with the real precondition—stop `sdd serve` processes and restart agent sessions—and `--migrate-sessions` is the non-interactive acknowledgement. Once acknowledged, move-if-absent semantics are unchanged.
2. Decline or non-interactive execution does not prevent the committed `repo_id` from existing. Instead, while the supported local/hash→repo-ID transition remains pending, runtime routing and newly created sessions stay on the current `local/<hash>` key until acknowledgement; the desired repo-ID key remains recorded, a durable transition marker keeps the move pending, and the standing notice re-prompts on the next `sdd init`.

## Explicit deferral

`20260724-145631-s-tac-vxs` remains open and deferred: changing an already established `repo_id` from A to B lacks a stable rendezvous for discovering A. Slice 1 adds no authority registry, availability blocking, permanent alias/coexistence layer, or generalized A→B rekey behavior. The narrow breadcrumb already produced by the supported local/hash transition remains plain recorded data only.

## Review and verification

Fourteen bounded implementation/review passes converged on a frozen diff. Independent architecture, feature-correctness, and adversarial data-integrity reviewers returned CLEAN on the final snapshot. Independent final gates passed: `go test ./...`, `go test -race ./local ./cmd/sdd`, `go vet ./...`, `golangci-lint run ./...` (0 issues), `git diff --check`, and `GOOS=windows GOARCH=amd64 go build ./local`.

No deviation beyond the two explicitly confirmed acknowledgement-trigger and decline-path refinements above.