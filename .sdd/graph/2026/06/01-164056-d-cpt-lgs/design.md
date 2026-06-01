# Tooling–graph coupling: decision + dev-build-freshness fix

## The question

Jonathan proposed a binding contract (s-cpt-t2n): (1) the decision graph must stay conflict-free, guaranteed by append-only immutability, and (2) conflict-prone tooling must be kept physically separate from it.

## Decision

Keep part 1 as a restatement of what the immutability contract (d-cpt-e1i) already guarantees — no new contract. Replace part 2's *physical separation* with *version discipline*.

### Why not physical separation

The coupling between tooling and graph is real and irreducible: a newer graph format needs a newer CLI to read/write it, and the CLI carries the installed skills. Splitting tooling into a separate repo or branch does not remove that dependency — it only adds ceremony and a synchronization burden. There is no clean split.

### Version discipline (the chosen mechanism)

1. `minimum_version` write gate — already shipped in `sdd init` (d-tac-r5g). A binary below the graph's `minimum_version` cannot write. This is "version-to-participate".
2. Backwards-compatible reading — a current binary must at least *read* any older graph it encounters. (Stronger than the strict single-schema stance sketched in the distribution plan d-cpt-uu1; this directive commits to read-compat.)
3. Standing practice — participants do not hand-edit installed tooling files (`.claude/skills/`, the binary). `sdd init` and the git hooks own them.

## The clobber, reframed (dev-DX, not a framework gap)

s-tac-ddi reported `sdd init` letting a stale binary overwrite newer committed skills. Root cause: in the sdd repo itself the binary is an uncommitted *dev* build with no tagged version → it stamps no `minimum_version` → the gate can't fire. Jonathan hit this because he didn't rebuild after pulling. External repos on a tagged release are already protected by the gate. So this is a dev-experience gap specific to developing sdd, not a hole in the framework.

## Fix: rebuild the dev binary on git events

Committed git hooks under `.githooks/`, selected by `core.hooksPath`:

- `post-merge` — merge pulls (incl. fast-forward)
- `post-rewrite` — rebases (`git pull --rebase`, our default sync path)
- `post-checkout` — branch switches (guarded on `$3 == 1` to skip file checkouts)

Each execs a shared `_build` that runs `devbox run build` (a new devbox script wrapping `go build -o bin/sdd ./cmd/sdd`). The Go build cache makes this near-free when nothing changed. `core.hooksPath` is per-clone config, so the devbox `init_hook` sets it on `direnv allow` — a fresh clone self-configures.

### Alternatives considered

- **devbox `init_hook` build** — fires on shell init / direnv reload, not on pull; misses the in-shell-pull case that caused the incident. Wrong granularity.
- **Lazy build-on-invoke wrapper** (a `sdd` shim that builds then execs) — guarantees freshness incl. local edits, but taxes every `sdd` call with build-cache-check latency and stalls on the first call after any change. Rejected for the per-call cost.

### Caveats

- Hook files need the executable bit (tracked by git).
- Hooks inherit the caller's PATH; from a GUI git client without the devbox env they no-op. Terminal/direnv usage (our workflow) is covered. `devbox run build` is used over bare `go build` for env robustness.
- Hooks cover git events only — local source edits still need a manual `devbox run build` (already the CLAUDE.md convention).

## Out of scope (left open)

- Branch-pinning main to stable tooling (s-prc-vm7) — a different mechanism for the same concern; this directive takes the auto-rebuild route instead.
- CLI evolution breaking entry-format compatibility (s-cpt-bn3) — answered in principle by the schema gate + read-compat, but the conformance/migration path remains its own thread.
- Mechanical detection of *semantic* conflicts between entries (s-cpt-ji9).
