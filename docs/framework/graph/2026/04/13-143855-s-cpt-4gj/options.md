# SDD distribution tooling: options breakdown

## The gap

- Go CLI works fine in-repo (`./framework/bin/sdd`) but there's no acquisition path for external users
- Skills (`/sdd`, `/sdd-catchup`, `/sdd-groom`, `/sdd-explore`) live in the repo's `.claude/skills/`, not portable
- No version story, no update story, no binary release flow
- Blocks external evaluation — the fork decision gives SDD a repo, but the repo needs a ship-ready artifact

## Option 1: Claude Code plugin

- Package CLI + skills as a Claude Code plugin
- Native fit — users install via Claude Code's own mechanisms, skills discovered automatically
- Unknowns: does the plugin system accept/ship platform-specific binaries? How are downloads handled? Code signing for macOS?
- Research needed on the plugin spec and whether binaries are a supported pattern

## Option 2: Embed skills inside the sdd binary

- Single-artifact distribution: the sdd binary carries its own skills
- `sdd install` stamps skills into a project's `.claude/skills/` (or `~/.claude/skills/`)
- Benefits: one thing to version, easy self-update via `sdd upgrade`, skills guaranteed to match the CLI version
- Concerns: does Claude Code's skill-discovery find skills placed by an arbitrary tool? Global vs project scope decisions?
- Question: where does the user get sdd in the first place? Still needs a bootstrap step

## Option 3: curl | bash installer

- Simple: one-line install from a GitHub release
- Works for the binary, but skill distribution still needs solving (maybe bundled into the same script)
- No built-in update path unless we add one (`sdd upgrade` or re-run the installer)
- Familiar pattern; lowest barrier to entry
- Doesn't address the Claude Code plugin ergonomics question

## Cross-cutting concerns (regardless of option)

- GitHub releases flow (tag → build → artifact)
- macOS + Linux builds minimum; Windows later
- macOS code signing / notarization (Gatekeeper otherwise blocks the binary on first run)
- Version policy: semver? what triggers a bump?
- Release infrastructure lives in the new sdd repo (post-fork)

## Decision inputs to gather

- Claude Code plugin spec: does it support binaries, what's the install UX?
- Examples of similar tools (Go CLIs + AI tooling integrations) and how they distribute
- User journey: what's the minimum number of steps for an external evaluator to get from "I heard about SDD" to "I'm capturing my first signal"?

## Tension

- Easy-first (option 3) vs best-integrated (option 1) vs most self-contained (option 2)
- Research should resolve enough of the plugin unknowns to pick meaningfully
