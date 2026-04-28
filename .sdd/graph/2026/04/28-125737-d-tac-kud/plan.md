# Plan: `sdd init` as fresh-clone readiness check

## Why this plan exists

Two related gaps surfaced together:

- **s-tac-z8o** — fresh clones have no automated setup path. `.sdd/config.local.yaml` is correctly gitignored, skill installation may or may not be present, and no current CLI behavior tells a contributor what to do.
- **s-tac-eo7** — `sdd init --scope project|global` chooses a skill install location but does not persist the choice. Subsequent runs and clones can't read the project's intent.

Solving them in isolation does not work: detecting "skills missing on a fresh clone" requires knowing the recorded scope, and persisting scope only pays off if some command actually consumes it. The unifying frame is to make `sdd init` the canonical readiness check — run it after cloning, and it sets up everything that's missing at the project's recorded scope.

A third concern is folded in: the version-skew infrastructure shipped under s-tac-3al / d-tac-r5g protects every mutation command via the dispatcher write gate, but `sdd init` itself is not wrapped. A contributor with an older binary cloning a project whose `minimum_version` is higher would have mutations blocked correctly, but `sdd init` would silently install older bundled skills over the project's expected newer set. Wrapping init with the same compatibility check closes this hole without new infrastructure. A new `--bump` flag complements the gate for the inverse direction: a project initialized with a dev build (no `minimum_version` recorded) can lock a floor on demand once a released binary is in use.

## Where the scope record lives

**Decision: project-committed `.sdd/config.yaml`.**

The project chooses where its skills should live, and that choice should travel with the repo so every clone behaves the same. A contributor who personally prefers global skills can install them at user scope on their own machine — they would simply not commit a different scope back. A per-contributor local override is a future concern, not part of this plan.

Alternative considered: `.sdd/config.local.yaml` (gitignored, per-contributor). Rejected because it requires every contributor to discover and set the scope independently, defeating the goal of a single `sdd init` command on a clone working without prompting whenever possible.

## What `sdd init` does on each starting state

- **Fresh greenfield (no `.sdd/`):** existing behavior — create `.sdd/`, prompt for language and participant, choose scope (now via Bubble Tea selector), install skills, write `meta.json` with `minimum_version`. Now also writes `skill_scope` to `config.yaml`.
- **Fresh clone (existing `.sdd/`, no `config.local.yaml`, no installed skills at recorded scope):** read scope from `config.yaml`, prompt for participant (interactive) or error with `--participant` guidance (non-interactive), install skills at the recorded scope. Pre-flight `minimum_version` check applies before any file is written.
- **Partial setup (one of: missing participant, missing skills at scope):** the same idempotent loop fixes whatever is missing, leaves the rest alone.
- **Fully set up:** no-op apart from refreshing skill content if drift is detected (current behavior preserved).

## Why scope override blocks

An explicit `--scope` flag that disagrees with the recorded value errors and points at `.sdd/config.yaml` for direct edit. Three reasons:

1. **Skill installs are visible side effects.** A silent rewrite would change the project's record and write skill files into a new directory in one go — a non-obvious migration disguised as setup.
2. **The committed config is the contract.** A flag that overrode it on every contributor's machine would erode the "the project records its choice" guarantee.
3. **Direct file edit is the right mental model.** If you want to change scope intentionally, edit `.sdd/config.yaml`, commit the change, and have all contributors run `sdd init` against the new value. A guided "switch scope" command is a separate concern with its own design space (cleaning up the old scope's installation, prompting confirmation across contributors, etc.) — explicitly out of scope here.

## Why a Bubble Tea selector

When no `skill_scope` is recorded yet (greenfield, or legacy clone that initialized before this plan), the contributor needs to pick. A Bubble Tea selector with project/global options and the default highlighted is consistent with the language prompt pattern already in `sdd init` and gives clear keyboard-driven UX. Non-interactive contexts (CI, scripted setup) error with `--scope` flag guidance instead — same fallback pattern as the other prompts.

## `--bump` rationale and edge cases

`sdd init --bump` writes `minimum_version` to the running binary's version. Behavior:

- **Recorded value absent:** writes the binary's version. (Resolves the dogfood / external-project case where init was first run with a dev build.)
- **Recorded value < binary version:** writes the binary's version, raising the floor.
- **Recorded value == binary version:** no-op. Idempotent.
- **Recorded value > binary version:** unreachable in practice — the write gate (AC6) blocks `sdd init` before this code runs.
- **Dev builds:** errors with "cannot bump from a dev build, use a released sdd binary". Consistent with the existing rule that dev builds don't write `minimum_version` on initial init.

The flag deliberately accepts no semver argument: the running binary's version is the only legal value, and the write gate prevents lockout. Naming (`--bump` vs `--bump-min-version` vs `--bump-minimum-version`) is left for implementation; `--bump` is preferred for terseness if disambiguation isn't needed.

## Warning on participant-missing for other commands

A single one-line stderr warning when a non-init command runs without a configured local participant. Naming `sdd init` as the fix gives the user one canonical path. Suppressed for `sdd init` itself (it's already running) and for structured / `--quiet` output modes (avoids polluting machine-readable output).

Open question for implementation: should the warning fire on every command, or only on commands where participant matters (`new`, `wip start`)? AC scopes it to "all non-init commands" for simplicity; tightening is a no-regret refinement during build if noise is observed.

## Out of scope (deliberately deferred)

- **Auto-detecting setup needs in legacy projects without a recorded scope.** Projects that initialized before this plan landed have no `skill_scope` field. `sdd init` will treat them as "no scope recorded yet" and elicit the choice via the selector — same as a fresh greenfield. No retroactive migration tool.
- **Detecting skill-installation state from arbitrary `sdd` commands.** The CLI does not check whether `.claude/skills/sdd/` exists on every invocation. The only structural enforcement of skill presence is via `sdd init` running.
- **Auto-discovering "is SDD set up at all" on a clone before the user runs `sdd init`.** A contributor who never runs `sdd init` won't be helped by this plan. The README Quickstart change is the only mitigation.
- **Skill-version drift warnings between project-installed skills and the binary's bundle.** Existing `sdd-version` + `sdd-content-hash` stamps in skill frontmatter could ground a "your installed skills are stale" check, but this plan does not add that surface.
- **Multi-agent (Claude vs Codex etc.) installation.** See the next section.

## Multi-agent rationale (deferred to a separate gap)

Scope and agent-target share the surface (`sdd init`, recorded config) but have different semantics:

- **Scope** is a directory-location concern, project-wide by nature. Same answer for every contributor — that's why it goes in committed `.sdd/config.yaml`.
- **Agent target** is per-contributor by nature. If one contributor uses Claude and another uses Codex on the same project, they need different skill bundles installed locally. A single project-committed target doesn't fit; per-contributor settings do.

Two prerequisites also block a concrete design today: only the Claude bundle ships (`internal/bundledskills/claude/` is the only target tree), and cross-agent skill compatibility — whether the same source survives a Codex translation, or each agent needs an authored variant — is an open research question we haven't touched. Folding multi-agent into this plan would design a multi-contributor multi-agent model on top of a single-agent codebase with the second agent's bundle as a prerequisite that doesn't exist.

A separate conceptual gap signal captures the data-model question so the concern lives in the graph and can be picked up when a second agent skill bundle is closer to real.

## Relationship to s-prc-epa

`s-prc-epa` flagged that `sdd init`'s post-install commit can sweep up unrelated staged changes. That's a separate concern — about what gets included in the auto-commit, not about the readiness-check behavior. This plan does not close `s-prc-epa`; it should be addressed independently, possibly alongside this implementation if both surface in the same touch on `sdd init`.

## Connection to existing version infrastructure

`s-tac-3al` shipped:

- `.sdd/meta.json` with `graph_schema_version` and write-once `minimum_version`
- `cmd/sdd/writegate.go::withWriteGate` wrapping mutation commands
- Dev-build bypass to keep local development unblocked

This plan does not change any of that. It extends the gate's coverage to `sdd init` itself (since init writes skill files, and that write should respect the same minimum-version invariant) and adds `--bump` as the explicit lever for raising the floor when desired.
