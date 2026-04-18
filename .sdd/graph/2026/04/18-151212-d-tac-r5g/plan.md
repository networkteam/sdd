# Plan: Unified `sdd init` with skill + schema management

## Context

Follow-on scope to the distribution conceptual decision (d-cpt-uu1). Realises the "skills ship embedded in the binary, extracted by a CLI command" part of that decision, but folds the planned separate `sdd install` subcommand into `sdd init` after dialogue concluded the two-command design created an onboarding trap.

d-cpt-uu1 remains intact as the conceptual distribution decision (immutability contract d-cpt-e1i). This plan is a tactical realization that changes the command surface while preserving the distribution intent (Homebrew + curl installer, embedded skills, three version stamps, schema strict-check with future migration chain).

## Design

### Command semantics

`sdd init` is idempotent — run whenever things change. Context-aware:

- **No `.sdd/`**: interactive project setup (existing d-tac-s2g flow), install skills, write meta.json.
- **`.sdd/` present, everything current**: no-op (no prompts, no writes).
- **Skill drift**: auto-refresh pristine files, prompt on modified ones.
- **Schema drift or `minimum_version` violation**: writes elsewhere refuse, pointing at init or future migrate.

Precedent: `terraform init` uses the same idempotent-refresh pattern.

### Pristine-hash mechanism

Each installed skill file carries two injected frontmatter fields:

- `sdd-version: 0.2.0` — binary version at install time
- `sdd-content-hash: <sha256>` — hash of the file with these two fields stripped and frontmatter canonicalized

On subsequent `sdd init`:

1. For each embedded skill, locate the installed counterpart on disk.
2. Read installed file, strip `sdd-version` + `sdd-content-hash`, canonicalize frontmatter, hash body.
3. Compare computed hash to stored `sdd-content-hash`.
4. Match → installed content is what we wrote → safe to overwrite silently.
5. Mismatch → user modified → prompt (`[y/N/diff/abort]`, default N).

Cost: one sha256 per skill file per init run. Negligible.

### `.sdd/meta.json`

Committed JSON:

```json
{
  "graph_schema_version": 1,
  "minimum_version": "0.2.0"
}
```

- **`graph_schema_version`** (required): structural schema version. Bumped only by migrations. Written on initial init.
- **`minimum_version`** (written on initial init, preserved thereafter): the oldest sdd binary permitted to write to this graph. Set once at graph creation time to the creator's binary version; preserved by subsequent inits; bumped deliberately by maintainer (hand-edit for MVP, future `sdd set-min-version`).

Dev builds (non-semver version strings) skip writing `minimum_version` on initial init. Leaves the field absent until a released-version init runs.

### Write gate

On write commands (`sdd new`, `sdd wip start|done`, `sdd summarize`):

1. Read `.sdd/meta.json`.
2. If binary version is dev → both gates pass (treated as matching anything).
3. `graph_schema_version` != binary's supported version → refuse with pointer to future `sdd migrate` or upgrade guidance.
4. `minimum_version` present and binary semver < it → refuse with upgrade guidance.
5. Both pass → command proceeds.

Read commands (`status`, `show`, `list`) do not gate — users always inspect their graph.

**Placement:** dispatcher-level guard in `cmd/sdd/` — single call site, not per-handler. Adheres to push-logic-down.

## CQRS decomposition (per d-cpt-ah1)

### Commands (`internal/command/`)

- `InstallSkillsCmd { Scope, Target AgentTarget, Force bool; OnSkillsInstalled func(installed, refreshed, skippedModified []string) }`
- `WriteSchemaMetaCmd { SchemaVersion int; MinimumVersion *string }` — nil `MinimumVersion` means "don't write/bump it"

Existing `InitProjectCmd` extended: orchestrates install + meta write through the handler.

### Queries (`internal/query/`)

- `SkillStatusQuery { Target AgentTarget, Scope Scope }` — per-skill install status
- `SchemaStatusQuery {}` — schema_version + minimum_version compatibility against binary constants

### Finders (`internal/finders/`)

- `SkillFinder.Status(ctx, q)` — reads installed skill frontmatter, compares with embedded bundle, returns per-skill status (Missing | Pristine | Modified | Current)
- `SchemaMetaFinder.Status(ctx, q)` — reads `.sdd/meta.json`, returns CompatibilityResult vs binary version constants

### Handlers (`internal/handlers/`)

- `InitHandler.Init` — existing setup → `SkillFinder.Status` → delegates refresh/install to `SkillHandler` → `WriteSchemaMetaCmd` via `SchemaHandler` (sets `minimum_version` only on initial creation, never on existing meta)
- `SkillHandler.Install` — writes skill files to target, injects `sdd-version` + `sdd-content-hash` frontmatter
- `SchemaHandler.WriteMeta` — writes `.sdd/meta.json`

### Model (`internal/model/`)

- `AgentTarget` — string type, Claude hardcoded at MVP
- `Scope` — `User` | `Project` enum
- `SkillBundle` — embedded skill manifest (name, relative paths, content bytes)
- `SkillInstallStatus` — `Missing` | `Pristine` | `Modified` | `Current`
- `SchemaMeta { GraphSchemaVersion int; MinimumVersion *string }`
- Pure functions:
  - `CanonicalizeFrontmatter(fm map[string]any) []byte` — deterministic serialization for hashing
  - `ComputeSkillHash(fileContent []byte) string` — strips `sdd-version` + `sdd-content-hash`, canonicalizes, hashes
  - `ComputeSkillStatus(embedded SkillBundleEntry, installed *SkillFile) SkillInstallStatus`
  - `CheckCompatibility(meta SchemaMeta, binaryVersion string, binarySchemaVersion int) CompatibilityResult`
  - `IsDevVersion(v string) bool`

### Presenters (`internal/presenters/`)

- `InitPresenter` — init summary (what was installed/refreshed/skipped/prompted)
- `SchemaErrorPresenter` — schema/minimum_version drift errors

### Cross-cutting: write-gate dispatcher guard

In `cmd/sdd/`, before executing any write command, invoke `SchemaStatusQuery` via the finder. If incompatible, exit with presenter-formatted error. Single call site.

## Alternatives considered

- **Two-command (`sdd init` + `sdd install`)** — rejected: onboarding trap (new user `sdd init`'s, opens Claude, `/sdd` finds nothing). Added vocabulary without user benefit.
- **Drop `sdd init` entirely, lazy-init on first `sdd new`** — rejected: loses the interactive graph-dir prompt flow from d-tac-s2g; conflates "first write" with "tool setup."
- **Always-prompt on refresh** — rejected: too much friction for the 99% pristine case.
- **Always-auto-refresh** — rejected: silently overwrites user modifications.
- **`last_init_version` write gate (bumped on every init)** — rejected: locks out collaborators on every release regardless of whether compatibility actually broke. Creates upgrade-treadmill friction.
- **Auto-bumping `minimum_version` on every init** — rejected: same problem as `last_init_version`.
- **`minimum_version` absent-by-default** — rejected in favor of "set once on creation": having the floor present from day 0 catches accidental older-binary writes without requiring maintainer action upfront.

## Deferred scope

Captured in this plan as explicit out-of-scope; revived via follow-on decisions when triggered:

- **`sdd migrate` subcommand + migration chain** — triggered when the first v2 schema change is proposed. Graph is implicitly v1, no migration pressure.
- **`sdd set-min-version` command** — hand-edit sufficient for MVP given the rarity of deliberate floor bumps. Add when the first real bump happens and hand-editing proves awkward.
- **`--target <agent>` abstraction (registry pattern, per-agent path templates)** — revisit when a second agent (Codex, etc.) actually needs wiring.

## Open implementation questions

- **Dev-build version rule** — non-semver version strings (e.g., `dev`, commit hashes) treated as matching anything for gating purposes. Initial init with a dev build skips writing `minimum_version` (field remains absent until a released-version init runs). `IsDevVersion(v string) bool` helper encapsulates the detection.
- **Frontmatter canonicalization** — JSON-style canonicalization (sort keys, UTF-8, LF line endings, no trailing whitespace) for hashing. Codified in `CanonicalizeFrontmatter` with unit tests covering edge cases (nested maps, arrays, numeric precision).
- **Modified-skill prompt UX** — `[y/N/diff/abort]`, default `N` (safe). Minor polish at implementation time.
- **Embed path layout** — `//go:embed skills/*/**` vs `//go:embed .claude/skills/*/**`. Design detail — ideally `skills/` inside the module so the embed path doesn't depend on Claude-specific directory structure.

## Sequencing

1. `model/` additions: `SchemaMeta`, `SkillInstallStatus`, `AgentTarget`, pure functions (`CanonicalizeFrontmatter`, `ComputeSkillHash`, `ComputeSkillStatus`, `CheckCompatibility`, `IsDevVersion`)
2. Embed infrastructure: `//go:embed` directive and `SkillBundle` assembly
3. Skill frontmatter injection + hash computation glue
4. Finders: `SkillFinder.Status`, `SchemaMetaFinder.Status`
5. Handlers: `SchemaHandler.WriteMeta`, `SkillHandler.Install`
6. `InitHandler` extension (orchestration)
7. Dispatcher-level write-gate guard in `cmd/sdd/`
8. Presenters + CLI wiring
9. README.md, `docs/`, `.claude/skills/` content updates (unified `sdd init` language)
10. Integration tests covering the scenario matrix (fresh init, repeat init, post-upgrade refresh, modified skill, schema drift, minimum_version gate, dev-build handling)
