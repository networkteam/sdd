# SDD distribution tooling research — synthesis

Consolidated findings from a seven-angle research pass on distribution tooling for SDD. Each angle's raw findings are attached alongside this file (`research-01-*.md` through `research-07-*.md`).

## Convergent findings

### 1. Conventional distribution is the durable bet (angles 2, 6)

Every Go CLI that matters — `zoxide`, `starship`, `direnv`, `gh`, `uv` — converged on the same stack: **GoReleaser → GitHub Releases → Homebrew tap**, with curl|bash as a convenience layer. Angle 6's strategic analysis argues explicitly against Claude Code plugin marketplace as the primary channel: CLIs outlive the AI products they integrate with, Anthropic controls the marketplace, and other agents (Cursor, Copilot CLI, Gemini CLI) are already shipping their own "skills" concepts — so the likely convergence is at the skill-package level, not at any single marketplace.

### 2. Two-stage install is the idiom (angle 5)

`rustup`, `uv`, `gh`, `mise` all separate binary acquisition from asset/extension setup. The clean UX:

```
brew install sdd        # gets the binary
sdd install             # drops skills into ~/.claude/skills/, sets up state
sdd status              # first real command
```

Research explicitly warns against silent "first-run mutates home directory" — make the second stage visible via a noun-verb subcommand.

### 3. Bundle skills inside the binary (angle 7)

The most robust answer to version skew is embedding skills as compiled assets and extracting on `sdd install`. Runtime version checks are a backup; semver contracts alone fail in practice (Terraform, Helm prove this repeatedly). Bundling eliminates "binary updated, skills stale" structurally. Downside — prompt edits require binary releases — is acceptable given expected release cadence.

### 4. Notarization is not a wall for Homebrew-delivered binaries (angle 3)

Apple Developer Program enrollment is paid; no free notarization path exists. But the Rust CLI ecosystem (uv, ripgrep, bat, zoxide) overwhelmingly ships unsigned or ad-hoc signed. Homebrew manages the quarantine attribute on install, so `brew install` sidesteps Gatekeeper. For direct downloads, curl|bash installers commonly run `xattr -d com.apple.quarantine` after download. At launch: skip notarization, lean on Homebrew plus a documented workaround if needed. **Session-validated update**: the xattr workaround is not actually needed for curl-installed binaries placed in user-scoped directories like `~/.local/bin/`. Gatekeeper's quarantine attribute is set by GUI applications on file download, not by curl. Verified by running binstaller's own generated install.sh on macOS — no Gatekeeper friction.

### 5. No prior art for "Go CLI + Claude Code skills" combo (angle 1)

SDD is pioneering this specific shape. The closest precedent (`everything-claude-code`) ships plugin content + a custom `./install.sh` — similar spirit to the two-stage model. This means we can define the pattern rather than inherit one.

### 6. Claude Code plugin ecosystem is real but niche (angle 4)

Small enough to feel curated. SessionStart hooks are used in the wild but no clear public example of downloading platform-specific binaries into `${CLAUDE_PLUGIN_DATA}`. The marketplace is a reasonable secondary channel for discovery, not a primary bet. Still rapidly evolving.

## Design conclusions

From the convergent findings and the session dialogue:

- **Primary channels**: GoReleaser → GitHub Releases → custom Homebrew tap + `binstaller`-generated `install.sh` (identified during session dialogue as the companion to GoReleaser; generates an installer from the GoReleaser config with embedded checksums and GitHub attestation verification chain-of-trust via `actionutils/trusted-go-releaser`).
- **Skill delivery**: Embedded in binary via Go's `embed` package; `sdd install` extracts to target agent's skills directory.
- **Versioning**: ldflags-stamped binary version, per-skill `sdd-version` frontmatter stamp, graph metadata file (`graph_schema_version`, `last_touched_with`).
- **Schema compat**: Option A+D — strict binary + bundled transitive migrations; schema mismatch triggers a clear error pointing at `sdd migrate`.
- **Notarization**: Skipped at launch. Homebrew handles Gatekeeper for brew users; curl-installed binaries to `~/.local/bin/` don't trigger Gatekeeper either (session-verified).
- **Claude Code plugin channel**: Deferred as optional secondary, not primary (reinforced by the agent-neutrality directive).
- **Install scope**: Homebrew forces global binary install. Default `sdd install` target is user scope (`~/.claude/skills/`); `--scope project` is an escape hatch. Global + transitive migrations keeps worktree use smooth.
- **Sequencing**: Fork to dedicated SDD repo first (per d-cpt-dlw), then set up distribution infra in the new repo.

## Decision inputs resolved

- **Can we ship via Claude Code plugin?** Yes — plugins support `bin/` directory, skills, hooks, `SessionStart` hook for binary download into `${CLAUDE_PLUGIN_DATA}`. But it's single-vendor and still evolving. Not the primary path.
- **Is notarization a blocker?** No. Homebrew manages quarantine for brew users; curl-installed binaries to user-scoped paths don't trigger Gatekeeper (session-verified).
- **How do comparable tools bootstrap?** Two-stage: package manager installs binary, `tool install` / `tool setup` / `tool use` does the second stage. Visible, never silent first-run mutation.
- **How do we keep skills and binary in sync?** Bundle in binary + version stamps + graph metadata for schema versioning. Advisory skew check on CLI invocation; blocking schema-version mismatch with migration path.
- **Does GoReleaser ship a curl|bash installer?** No — not first-class. Hand-written `install.sh` (as used by starship/uv/zoxide) is one option, but the session identified `binstaller` (github.com/binary-install/binstaller) as the purpose-built companion: derives an `install.sh` from the GoReleaser config, embeds checksums, supports GitHub attestation chain-of-trust. Chosen over hand-writing.

## Open questions remaining for implementation

- **Skill format portability**: portable Markdown source with per-agent transforms at install time, vs. per-agent variants shipped in binary. Defer until multi-agent support is actually being built.
- **Multi-agent install UX**: auto-detect + prompt, explicit `--target <agent>`, or TUI selector (like gsd). Shape must allow extension; specifics can wait.
- **Homebrew tap repo naming**: follow Homebrew convention (e.g., `networkteam/homebrew-sdd`).
- **Release cadence policy**: what triggers minor vs. patch bumps? Needs articulation once releases are live.
- **Migration deprecation policy**: how long do we keep older schema migrations in the binary? Probably: keep all until we have a supported-version policy.

## Research method note

All seven angles used Perplexity research (`perplexity_research` tool). Each returned citations to primary sources — official docs, GitHub repositories, relevant issues. Cross-angle corroboration was strong: claims about the Go CLI stack (GoReleaser + Homebrew + curl|bash) appeared independently in angles 2, 5, and 6, which strengthens confidence in that direction beyond any single source.
