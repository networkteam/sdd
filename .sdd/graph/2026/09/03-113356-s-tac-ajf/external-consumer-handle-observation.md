# Observation: the session handle should be the session ID alone (2026-09-02)

Reported by the first external consumer of `pkg/mcpapp` and `pkg/application`, a multi-project composition, after evaluating its integration against networkteam/sdd `main` following the capability rebuild (20260902-224832-s-tac-x8c). A design observation, not a bug report.

## What the code does

- `pkg/mcpapp/handle.go`: `composeHandle(project, id)` returns `<project>:<session-id>` whenever the project is non-empty; `splitHandle` splits at the last `:s_` and falls back to the server's pinned `Options.Project` when there is no prefix.
- Every tool result composes `Session` from the session's project and the session ID (`pkg/mcpapp/tools.go`: the serve and resume results and `AbandonResult`). The pinned project is never empty locally: `cmd/sdd/serve.go` passes the repository ID as the project. So a local `sdd serve` hands out handles such as `github.com/<org>/<repo>:s_…`. That effect was not stated in `d-tac-1z6`.
- The same results already return `Project` as its own field next to `Session`. The project inside the handle is redundant on the way out.
- The application never sees a handle. `ResumeWorkflow(ctx, identity, project, request)` takes the project ID and the session ID separately; so does every other application method.
- Local sessions do not live in the project directory. They sit under the machine-global XDG state root, keyed by repository, and the local composition serves exactly one project.
- Since the rebuild, session IDs carry 128 random bits. The external consumer's store keys sessions by session ID alone and records the project on the session.

## Why the composite has no upside

- For the agent: it never parses the handle. It receives it from `start_session` and echoes it back, and every result already carries `Project`. What it gets is a longer opaque token with a colon in it, carried through every call. Long opaque strings are where agents make transcription mistakes.
- For the person: the handle matters in one moment, copying it into a second client. People name sessions by label, never by handle. The prefix is noise there, and the consumer's listing already shows the project name next to the handle.
- For local users: a visible regression. On 0.17.0 every local handle is a bare `s_…`. The next release would prefix every handle a local `sdd serve` returns, in the open-threads orientation and the skill's wording, for a user with exactly one project.
- The only benefit is internal: the server knows the project before it touches a store, so it needs no lookup and one code path serves local and multi-project compositions alike.

## The world wanted

The handle a client carries is the session ID. Nothing else is encoded in it. The project a session belongs to is a fact about the session, resolved from the session: a keyed lookup in a multi-project composition, the served project in the local one. `start_session` keeps its optional `project` argument and keeps returning `Project` beside `Session`. Every other tool takes the bare handle and the server derives the project before it resolves access.

## Consistency check

- Every door that needs a project either takes it (`start_session`) or holds a session from which it is derived (`next`, `resume_session`, `abandon`, `park`, the free reads).
- A session ID is globally unique by construction (128 random bits) and in the consumer's store; no per-project scoping of IDs is lost.
- Nothing is derived from the connection, so the capability rule of `20260828-165352-d-cpt-aen` is untouched.

## Decision hygiene

- `d-tac-1z6` is closed; the change wants a decision in networkteam/sdd recording why the composite was wrong: it duplicated `Project`, it changed local handles without saying so, and the project of a session is derivable from the session.
- Vocabulary: name `pkg/mcpapp` (the MCP server) and `pkg/application` (the SDD application). The word "wrapper" blurs which layer owns what.
