@AGENTS.md

# CLAUDE.md

Claude-Code-specific notes for the SDD framework. All agent-neutral guidance — project overview, setup, commands, architecture, structure, graph conventions, skill source of truth, git rules, and the no-parallel-memory rule — lives in [AGENTS.md](AGENTS.md), imported above. Add to `AGENTS.md` unless a note is genuinely Claude-Code-only.

## Worktrees

Always run `direnv allow <path>` on a new worktree under `.claude/worktrees/` (safe — it's this repo's own `.envrc`) so a shell opened there loads the devbox env.
