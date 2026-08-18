@AGENTS.md

# CLAUDE.md

Claude-Code-specific notes for the SDD framework. All agent-neutral guidance — project overview, setup, commands, architecture, structure, graph conventions, skill source of truth, git rules, and the no-parallel-memory rule — lives in [AGENTS.md](AGENTS.md), imported above. Add to `AGENTS.md` unless a note is genuinely Claude-Code-only.

## Communication

- Describe actions concretely — say what is actually being done ("writing the research summary to a file"), not metaphor shorthand like "banking" (Christopher, 2026-08-16).
- Use simple, clear, meaningful language in dialogue and in written deliverables: no jargon, no decorative metaphors ("rim decoration", "not a dashed dream"). SDD's own defined terms are fine (Christopher, 2026-08-18).
- Never use em dashes in outward-facing prose (website, docs, onboarding material) — many readers take them as the first sign of AI-generated text. Use commas, colons, periods, or parentheses instead (Christopher, 2026-08-18).

## Worktrees

`EnterWorktree` moves the session into the worktree, but the agent's Bash environment is a frozen snapshot from session start — `DEVBOX_PROJECT_ROOT`, `PATH`, and direnv all still point at the **base** checkout, and Claude Code has no supported way to auto-activate the worktree's env on entry (the `worktree` settings cover only `baseRef`/`bgIsolation`; direnv's hook doesn't re-fire in a non-interactive shell). So, from inside a worktree:

- Run devbox scripts as `devbox run -c "$PWD" <script>` from the worktree root — plain `devbox run` resolves to the base project root via the inherited `DEVBOX_PROJECT_ROOT` and operates on `main`. devbox prints `Running script … on <dir>`; check it names the worktree.
- `sdd` on `PATH` is the **base** build, so call `./bin/sdd` to exercise the worktree's binary (after `devbox run -c "$PWD" build`).
- `direnv allow <worktree>` only scopes an *interactive* terminal opened there (its prompt hook re-points the env); it does not change the agent's in-session runs.
