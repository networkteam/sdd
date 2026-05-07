# `sdd init` readiness check — completion findings

## Per-AC outcomes

### AC 1 — `skill_scope` persisted in `.sdd/config.yaml`
Added `SkillScope model.Scope` field to `model.Config`. `FormatConfig` writes the value under a commented block on fresh init; `MergeConfig` overlays it; `SetYAMLField` upserts it on existing configs that predate the field (the upgrade path). Round-trip verified by handler tests.

### AC 2 — `--scope` contradicting recorded value blocks
`resolveSkillScope` reads the recorded value and rejects an explicit `--scope` flag that disagrees, naming the conflict and pointing at `.sdd/config.yaml` for manual edit. `ScopeExplicit` on `InitCmd` separates "operator typed `--scope`" from "default fallback" so a default never contradicts.

### AC 3 — scope selector when no value recorded
New `scopePromptModel` (Bubble Tea) presents two options with project default-highlighted per `d-tac-07q`. Non-interactive contexts route through the aggregated error from AC 5 instead.

### AC 4 — idempotent re-install at recorded scope
Already covered by AC 1+3 wiring: the handler reads the recorded scope from `.sdd/config.yaml`, threads it through `InstallSkillsCmd.Scope`, and the install pass classifies missing files as `SkillStatusMissing` and writes them. Smoke-tested by deleting `.claude/skills/` and re-running `sdd init` non-interactively; 14 skills land at the recorded project path. The participant detection-and-prompt path was already in place; the refactor preserves it under the TTY branch.

### AC 5 — aggregated non-interactive error
Non-interactive contexts now report all missing pieces in a single message: `--language LOCALE`, `--scope project|user`, `--participant NAME`. The slice-2 isolated scope error folded into this aggregate so a CI run sees one actionable message rather than one error per re-invocation.

### AC 6 — write-gate on `sdd init`
`initCmd()`'s action is now wrapped by `withWriteGate`. Smoke-tested with a synthetic `v0.4.0` binary against a graph pinned to `minimum_version: v0.5.0`; refused before any skill files were written. Fresh installs (no `.sdd/`) and dev builds remain exempt — the gate already handles those branches.

### AC 7 — `--bump`
New `--bump` flag on `sdd init`. New `BumpMinimumVersionCmd` + `handler_bump_minimum_version.go`. New `model.ShouldBumpMinimumVersion` helper keeps the version-comparison logic pure-domain. Behavior:

- Binary higher than recorded (or no recorded value) → write the binary version.
- Equal → no-op with an "(unchanged)" stderr line.
- Dev build → fail before any other side effect with the plan's verbatim wording: "cannot bump from a dev build, use a released sdd binary".

CLI fast-fails the dev case before invoking the handler; the handler also enforces it for direct API consumers.

### AC 8 — participant-missing warning on other commands
`warnIfParticipantMissing` runs in the root command's `Before` hook for every non-init command. One-line stderr nudge naming `sdd init` as the fix; suppressed for `sdd init` itself and silently dropped outside an SDD-instrumented repo. **Deviation:** the AC's `--quiet` / structured-output suppression branch is deferred since no such flag exists in the CLI today; the suppression call site is the right place to attach those cases when the flags land. Dialogued before implementation.

### AC 9 — README Quickstart
Step 1 split into two named cases. "Cloned an SDD-instrumented repo?" leads (the keystroke-free contributor path now that scope persists), "Instrumenting a new project?" follows with the prompt-driven setup. Added explicit notes for re-running, `--bump`, and non-interactive flag invocation so the new readiness-check surface is discoverable from the entry doc.

### `d-tac-07q` (augmenting directive)
Project scope is the default-highlighted option in `scopePromptModel`. The `scopeOption` slice orders project before user and the cursor starts at index 0, so the keystroke-free path installs into the repo-local `.claude/skills/` tree.

## Dialogued deviations

1. **`skill_scope` value names.** AC 1 reads `skill_scope: project|global`. Persisted as `user`/`project` instead, matching the existing `model.Scope` type (`ScopeUser`, `ScopeProject`). Renaming the type to `ScopeGlobal` would have touched every call site without changing behavior; "global" reads as an informal synonym for the user-global path. Confirmed before starting.
2. **AC 8 `--quiet` clause.** No `--quiet` flag and no structured-output mode exist in the CLI today. Warning suppressed for `sdd init` only; the broader suppression hook is queued behind the `--quiet` flag itself. Confirmed before starting.

## CQRS shape note

Slice 1 added `InstallDir` to `SkillInstallResult` so the CLI's `OnSkillsInstalled` callback uses the handler's resolved scope rather than recomputing from a closure-captured `--scope` flag value. The change was forced by the new behavior: once the handler began overriding the operator's `cmd.Scope` with a recorded value from `.sdd/config.yaml`, the old closure would have rendered the wrong directory in the install summary. The fix moves the install-dir derivation into the handler and lets presenters consume what the handler actually did, not what it was asked to do — a small case in favor of the broader `s-tac-m09` follow-up on read-side dependency hygiene.

## Tests added

- `TestInit_PersistsSkillScopeOnFreshInit`
- `TestInit_UpgradeUpsertsSkillScope`
- `TestInit_RecordedScopeWinsOverFlagDefault`
- `TestInit_ContradictingExplicitScopeErrors`
- `TestInit_MatchingExplicitScopeIsNoop`
- `TestInit_BumpRaisesMinimumVersion`
- `TestInit_BumpEqualIsNoop`
- `TestInit_BumpDevBuildErrors`
- `TestShouldBumpMinimumVersion` (table-driven across 8 cases)

All existing tests in `internal/handlers`, `internal/model`, and `cmd/sdd` continue to pass; lint clean.

## Follow-up candidates

- A process gap signal for the `--quiet` / structured-output suppression branch so the AC 8 deviation doesn't get lost when someone reviews AC 8 retrospectively.
- The CQRS shape note above could fold into the existing `s-tac-m09` follow-up on read-side dependency hygiene rather than living as its own entry.

## Commit log

```
b674a8d README: lead Quickstart with "run sdd init" for cloned SDD repos
7e31a14 init: warn other commands when local participant is missing
022d4c7 init: write-gate the action and add --bump for minimum_version
09842d7 init: aggregate non-interactive missing-flag errors into one message
4e4d9c1 init: scope selector + non-interactive error when no scope recorded
d503dd0 init: persist skill_scope, reject contradicting --scope
```
