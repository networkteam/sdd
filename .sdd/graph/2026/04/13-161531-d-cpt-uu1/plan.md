# SDD distribution strategy — plan

Implementation shape for the conceptual distribution decision. Covers the stack, install UX, versioning, schema compat, and what's deferred. Flows from the agent-neutrality strategic directive and the research action's findings.

## Primary distribution stack

1. **GoReleaser** for release automation (build, package, tag, publish)
2. **GitHub Releases** as the canonical source of truth for binaries
3. **Custom Homebrew tap** (e.g., `networkteam/homebrew-sdd`) for `brew install` UX
4. **`binstaller`-generated `install.sh`** for non-brew users — derives the installer from the GoReleaser config (`binst init --source=goreleaser --file=.goreleaser.yml`, then `binst gen`), with embedded checksums and GitHub attestation verification chain-of-trust via [`actionutils/trusted-go-releaser`](https://github.com/actionutils/trusted-go-releaser). Installer is published as a release asset. Default install location is `~/.local/bin/` (XDG-compliant, user-scoped). Validated against binstaller's own self-installer on macOS — no Gatekeeper friction since `curl`-downloaded files to user directories are not quarantined.
5. **No Apple Developer Program enrollment at launch** — Homebrew handles Gatekeeper for brew users; curl-installed binaries to `~/.local/bin/` don't trigger Gatekeeper either (no `com.apple.quarantine` xattr is set by curl). No xattr workaround needed in practice.

## Install UX — two-stage, visible

```
# Stage 1: acquire binary
brew install networkteam/sdd/sdd     # most users

# or (basic) — downloads install.sh from latest release, runs it
curl -sL https://github.com/networkteam/sdd/releases/latest/download/install.sh | sh

# or (verified) — attestation chain-of-trust via gh CLI
curl -sL https://github.com/networkteam/sdd/releases/latest/download/install.sh | (
  tmpfile=$(mktemp); cat > "$tmpfile"
  gh attestation verify --repo=networkteam/sdd \
    --signer-workflow='actionutils/trusted-go-releaser/.github/workflows/trusted-release-workflow.yml' \
    "$tmpfile" && sh "$tmpfile"
  rm -f "$tmpfile"
)

# Stage 2: set up skills and local state
sdd install                          # extracts embedded skills to ~/.claude/skills/

# Stage 3: first real command
sdd status
```

**`sdd install` behavior:**
- Default: extracts skills to `~/.claude/skills/` (user scope; agent = Claude Code at MVP).
- `--scope project`: extracts to `./.claude/skills/` for per-project skill overrides.
- `--target <agent>`: future flag for multi-agent target selection (Claude, Cursor, etc.). Shape is designed in from day one; only Claude wired up at launch.
- Idempotent: re-running updates existing skills (compares version stamps, overwrites if binary is newer).
- Stamps each skill's frontmatter with the binary's version (`sdd-version: X.Y.Z`) for drift detection.

## Update story

Different acquisition paths have different update paths; both are covered without adding CLI code at MVP:

- **Homebrew users**: `brew upgrade sdd` — handled by the package manager, standard UX.
- **curl|bash users**: re-run the same installer command. binstaller's `install.sh` is idempotent — downloads the latest release, overwrites the existing binary. This is the pattern used by `starship`, `zoxide`, `uv` (pre-self-update), and others.
- **After updating the binary**: run `sdd install` again to refresh extracted skills. The version stamp check on subsequent CLI invocations detects skew and prints an advisory hint if skills weren't re-extracted.

**Documented explicitly in README** so the "how do I update?" question is never a mystery, especially for curl|bash users.

**Deferred to later**: a `sdd self upgrade` subcommand (analogous to `rustup self update` / `uv self update` / `mise self-update`). The shape is idiomatic and can be added additively whenever demand warrants. Not MVP because:
- Homebrew already solves it for the largest user cohort.
- Re-running the installer is a well-understood pattern for curl|bash users.
- Implementation cost (self-replace-on-disk, permission handling, rollback-on-failure) isn't justified at launch.

**Lightweight future touch (not MVP)**: `sdd version --check` could compare local binary version against the latest GitHub Release and print a nudge. Skip for MVP — brew users get this from `brew outdated`, and adding a network call on every invocation introduces privacy and surprise-behavior questions.

## Versioning mechanics

Three version stamps, each serving a distinct purpose:

1. **Binary version**: compiled-in via `-ldflags` (standard Go pattern). Used as the reference for all skew checks.
2. **Skill version stamp**: each installed skill's frontmatter carries `sdd-version: X.Y.Z`. Set by `sdd install`. Compared against binary version on CLI invocation — mismatch prints an advisory hint, never blocks.
3. **Graph metadata file** at `docs/framework/graph/.sdd-meta.json`:
   ```json
   {
     "graph_schema_version": 1,
     "created_with": "0.3.0",
     "last_touched_with": "0.5.2"
   }
   ```
   - Written on any mutating command; `last_touched_with` updated; `graph_schema_version` only changes on explicit `sdd migrate`.
   - Read on every command startup. Schema mismatch is blocking with clear error pointing at `sdd migrate`.
   - Also enables future capabilities: tracking which binary versions have touched a project's graph, detecting team-wide skew.

## Schema compatibility — Option A+D

**Strict main binary** (Option A): the binary supports exactly one `graph_schema_version`. Older-schema graphs trigger a clear error:

```
error: graph schema v1, this binary requires v2
  graph:  docs/framework/graph/.sdd-meta.json (last touched: sdd 0.4.1)
  binary: sdd 0.6.0

run `sdd migrate` to upgrade the graph in place, or install sdd 0.4.x to continue on v1.
```

**Bundled transitive migrations** (Option D): `sdd migrate` subcommand handles upgrades. Each migration is a pure transformation in `migrate/v1_to_v2.go`, `v2_to_v3.go`, etc. Chains auto-detect: from `graph_schema_version` in the metadata file up to the binary's target schema, run each step in order.

**Migration UX**: `sdd migrate` rewrites files in place, prints `N files changed, run 'git status' to review and commit.` Git becomes the undo mechanism — migration is not destructive as long as the graph is in a clean working tree.

**Migration code accumulation**: all historical migrations stay bundled in every binary. Users never need to find a historical sdd version to upgrade across multiple schema generations. Migration code is small (pure transformation) compared to runtime code. Deprecation policy (removing very old migrations) is a future concern — we'll define it once we have a supported-version policy.

**Why A+D over B or C**: multi-version reader/writer in the main binary (B or C) means every code path that touches entries has to know about schema variants. Complexity compounds as schemas accumulate. Migration-tool-only (D) keeps the hot path clean with a single conditional at startup.

## Claude Code plugin channel — deferred

Per the agent-neutrality directive, the Claude Code plugin marketplace is not a primary distribution path. It remains available as an optional secondary channel to be added later:

- **Shape 1**: submit a `marketplace.json` entry that points users to `brew install sdd` for the binary and uses the plugin's skill slot for discoverability only.
- **Shape 2**: plugin with a `SessionStart` hook that downloads the binary into `${CLAUDE_PLUGIN_DATA}` — more complex, adds single-vendor coupling.

Decision on which shape (if any) to ship in the plugin channel: deferred until the primary channels are stable. Either way, the plugin channel is marketing/discoverability layer, not a distribution dependency.

## Fork sequencing

Per `d-cpt-dlw`, the fork happens first; distribution infrastructure lives in the new sdd repo. Order:

1. Execute the fork (separate plan, per `d-cpt-dlw`).
2. In the new repo: scaffold `.goreleaser.yaml`, GitHub Actions release workflow (via `actionutils/trusted-go-releaser` for attestation chain-of-trust), Homebrew tap repo, binstaller configuration (`.config/binstaller.yml`), `sdd install` + `sdd migrate` subcommands.
3. Tag v0.1.0 and validate the end-to-end acquisition flow (fresh machine, `brew install sdd`, `sdd install`, `sdd status`).
4. Iterate based on real install UX.

## Alternatives considered and rejected

- **Claude Code plugin marketplace as primary**: violates the agent-neutrality directive; ties distribution to Anthropic's lifecycle; marketplace governance we don't control.
- **Embed skills in binary only, no conventional distribution**: leaves acquisition story unsolved ("how do users get the binary?"). Chicken-and-egg without a package manager layer.
- **Notarize via paid Apple Developer enrollment at launch**: cost + process overhead for an open-source single-maintainer project; most comparable tools don't bother, and Homebrew handles most users cleanly.
- **Multi-version graph reader/writer in main binary (Option C)**: complexity compounds with every schema change; migration-tool-only (Option D) achieves the same user outcome with much less code on the hot path.
- **Homebrew core formula submission at launch**: longer review process, no significant UX gain over a custom tap for a pre-1.0 tool. Revisit once we have user traction.

## Open sub-questions (deferred)

- **Skill format portability**: portable Markdown source + per-agent transforms at install time, or per-agent variants shipped in binary? Defer until multi-agent support is actually being built.
- **Multi-agent install UX**: auto-detect + prompt, explicit `--target`, TUI selector (like gsd)? Shape must allow extension; specifics wait.
- **Release cadence policy**: what triggers minor vs. patch bumps?
- **Migration deprecation policy**: how long do we keep older migrations in the binary?
- **Self-update subcommand**: add `sdd self upgrade` once demand warrants (see Update story section).

## Implementation tasks — high-level ordering

Not a formal task breakdown (that's spec-task-breakdown territory), but a rough sequence:

1. Scaffold: `.goreleaser.yaml`, release GitHub Action, tap repo bootstrap.
2. `sdd install` subcommand with `--scope` and (stubbed) `--target` flags.
3. Skill bundling via `go:embed`.
4. Graph metadata file (`graph_schema_version`, `last_touched_with`) + startup schema check.
5. Skill version stamp + advisory mismatch check on CLI invocation.
6. `sdd migrate` scaffold (no migrations yet, just the command shape and version-detection logic).
7. Binstaller setup: `.config/binstaller.yml` (initialized from the GoReleaser config), `install.sh` generated and published as a release asset.
8. README acquisition-path documentation (Homebrew + curl with optional `gh attestation verify` pattern).
9. v0.1.0 release validation end-to-end on a clean machine.

## MVP-vs-neutrality note

Per the agent-neutrality directive, the above is explicitly an MVP shape: Claude Code is the only wired-up target. The `--target <agent>` flag, `--scope` flag, skill-format abstractions, and non-marketplace distribution channels are all agent-neutral foundations; the MVP's Claude-only feature set sits on top. Extending to Cursor or another agent later should be additive, not a rewrite.
