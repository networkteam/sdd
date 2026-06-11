# Cross-agent skill & instruction portability — research record

Research grounding the Codex thin-port direction. Sources: Perplexity synthesis (mid-2026), official OpenAI Codex Skills docs, the Agent Skills standard (agentskills.io), Anthropic Claude Code memory/skills docs.

## The standard (agentskills.io)

Claude Code and OpenAI Codex both build on the open Agent Skills standard:

- A skill is a directory with `SKILL.md` (YAML frontmatter + markdown body) plus optional `scripts/`, `references/`, `assets/`.
- Required frontmatter: `name` (<=64 chars, lowercase alphanumeric + hyphens, no leading/trailing/consecutive hyphens, **must match the parent directory name**) and `description` (<=1024 chars).
- Optional standard fields: `license`, `compatibility` (e.g. "Designed for Claude Code"), `metadata` (arbitrary key-value map), `allowed-tools` (space-separated, experimental).
- Progressive disclosure: name/description (~100 tokens) loaded for all skills at startup; full `SKILL.md` (<5000 tokens recommended, keep <500 lines) on activation; `references/`/`scripts/` loaded on demand via relative paths (keep one level deep).
- `skills-ref validate ./skill` reference tool checks frontmatter/naming.

## Codex specifics (official OpenAI docs)

- Skills discovered in `.agents/skills/` — repo (CWD -> repo root), user (`$HOME/.agents/skills`), admin (`/etc/codex/skills`), system (bundled). Older `~/.codex/skills/` is legacy.
- **Codex follows symlinked skill folders** when scanning.
- Invocation: `/skills`, `$skillname`, or implicit by `description` match. `agents/openai.yaml` carries UI metadata + `allow_implicit_invocation`.
- Custom prompts deprecated in favor of skills.
- **No documented skill-invoking-skill mechanism** — skills are atomic; composition via plugins or prompt text.
- Initial skills list capped ~2% context / 8000 chars.

## Instruction files

- `AGENTS.md` is the cross-tool standard (Linux Foundation / AAIF; read by Codex, Cursor, Copilot, Gemini CLI, ...). Codex concatenates AGENTS.md root->down, nearer files override.
- Claude Code reads `CLAUDE.md`, not AGENTS.md natively; documented bridge is `@AGENTS.md` import inside CLAUDE.md (or a symlink on Unix). Import is the doc-aligned, cross-platform choice; symlink is weaker on Windows / some git workflows.

## What ports / what doesn't

Ports cleanly (all standard): SKILL.md + name/description, on-demand `references/` loading, progressive disclosure, `allowed-tools`. SDD-specifics get clean homes: `sdd-version`/`sdd-content-hash` -> `metadata:`; intended runtime -> `compatibility:`.

Does NOT port (Claude Code extensions outside the standard):
- Dynamic context injection (`!`cmd`` in frontmatter/body).
- Sub-skill orchestration via the Skill tool / `context: fork`.
- Slash-command entry point (`/sdd`) -> closest Codex analogue is a `$sdd` skill invocation.

## Thin port vs MCP gate (orthogonal)

- **Thin port** (this direction): render skills to `.agents/skills/`, instruct the agent to run the `sdd` CLI via shell (as Claude Code does). Cheap. Tests whether instruction-carried discipline survives on Codex.
- **MCP gate** (separate): the structural-enforcement server from the workflow experiment. Discipline-proof but heavy.

## Open questions for the eval (lower confidence)

- Does Codex reliably honor "when situation X, invoke skill Y" as an instructed substitute for native sub-skill invocation? (Untested — decides whether sub-skills are an issue at all.)
- Codex shell-permission/sandbox model for skill-instructed CLI calls (less crisply documented than Claude's `allowed-tools`).
- Whether Codex's on-demand `references/` loading matches Claude's behavior in practice.

## Sketch layout

- `AGENTS.md` canonical instructions; `CLAUDE.md` = `@AGENTS.md` + Claude-only notes.
- Per-agent render of one skill source into each agent's dir (`.claude/skills/`, `.agents/skills/`), driven by a committed list of supported agents.
