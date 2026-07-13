# Public API outer evaluation

## Scope

Outer validation only: use the implementation worktree's public root package and `mcpapp` as an external consumer. Judge whether an external composition can implement infrastructure and authorization ports without importing internal packages, mount the shared MCP application, and retain correct project and caller semantics. Inner verification of code quality, architecture, and test completeness was explicitly outside this run.

## Graph widening

The evaluation searched for the implementation plan by exact ID, public-API regressions and consumer-use surprises, earlier evaluations of the project-aware runtime, and external-module/port usability. No prior done or evaluation was recorded against the plan. The public API contract and subsystem-port amendment were read in full. Earlier outer evaluations established the method: genuine use is the evidence source, and concrete friction is recorded without duplicating inner code review.

## External consumer run

The nested `example.com/extendingsdd` module compiled independently and imported only `github.com/networkteam/sdd` and `github.com/networkteam/sdd/mcpapp`, not repository-internal packages.

The example was then run with isolated temporary state and mounted over Streamable HTTP. Observations:

- a valid bearer token initialized the MCP connection;
- an invalid bearer token was rejected with HTTP 401;
- the public `info` tool responded through the external composition;
- `start_session` opened a durable workflow session and served the normal SDD session orientation.

This validates the core composition promise: external code can supply authentication and infrastructure while SDD retains its application and MCP behavior.

## Participant-identity finding

The live session reported a blank local participant even though bearer authentication produced `TokenInfo.UserID = "example-user"` and the example's `ResolvePrincipal` returned `Principal{Subject: "example-user", Participant: "example-user"}`.

Tracing the public calls found inconsistent participant authorities:

1. MCP request metadata becomes protocol-neutral `RequestIdentity`.
2. `Application.resolve` calls `AccessResolver.ResolvePrincipal` and returns both the resolved principal and project runtime.
3. `Application.Info` discards the principal and returns `ProjectRuntimeOptions.Participant`.
4. Initial workflow creation uses that runtime participant for the engine session.
5. Durable session metadata stores `Principal.Participant`.
6. Resume replays the stored principal participant.
7. Entry and WIP defaults prefer `Principal.Participant` but silently fall back to `ProjectRuntimeOptions.Participant`.

The local CLI hides the split because its effective participant configuration is copied into both places. Its actual precedence is user-global configuration, then committed per-repository configuration, then machine-local per-repository configuration, with invocation flags above them.

Christopher clarified the hosted model: request identity resolves the hosted user; the user's project-specific participant mapping overrides an optional user-global canonical participant stored by the hosted server. Hosted projects have no repository-local participant configuration. The local equivalent is to resolve the effective participant configuration into the local principal.

Therefore participant identity has one legitimate authority: `Principal.Participant`, resolved for the current request. Any project-specific/global fallback belongs inside that resolver. `ProjectRuntime.Participant` is redundant even locally and unsafe as an independent fallback in a shared hosted runtime.

## Judgment

The public API is genuinely composable and the shared MCP application works in external use. It serves the intended integration shape, but not yet cleanly for hosted multi-user identity: initial framing, durable session identity, resume, and mutation defaults can disagree because participant authority is split. This is a release-significant hosted-use finding. The inner lens remains uncovered by this run.
