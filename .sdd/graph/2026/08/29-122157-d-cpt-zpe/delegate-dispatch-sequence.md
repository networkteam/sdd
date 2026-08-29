# Delegate dispatch — call sequence

How a host agent dispatches a delegate that runs a task-class procedure in a
forked child session, and how the result returns inside the engine. The child
session and instance exist before any sub-agent runs; the sub-agent's prompt
carries the child handles and never the parent's.

```mermaid
sequenceDiagram
    participant OA as Outer agent<br/>(conversation, holds s_parent)
    participant SA as Sub-agent<br/>(fresh context)
    participant MCP as MCP surface<br/>(sdd serve)
    participant EN as Engine

    Note over OA,EN: 1 — Dispatch: parent move reaches a step offering delegated research
    OA->>MCP: next(s_parent, i_parent, {choice: research, goal})
    MCP->>EN: advance i_parent
    activate EN
    Note right of EN: forks child session s_child<br/>(inherits branch binding + project,<br/>non-interactive, empty read set)<br/>creates instance i_child (task procedure)<br/>lineage: s_child → (s_parent, i_parent)
    EN-->>MCP: serve: dispatch instructions
    deactivate EN
    MCP-->>OA: dispatch text + handles (s_child, i_child)<br/>instructions: launch fresh-context sub-agent,<br/>prompt carries ONLY s_child, i_child, the goal<br/>— never s_parent

    Note over OA,SA: 2 — Launch: the host harness builds the sub-agent prompt<br/>the outer agent composes it per the served instructions
    OA->>SA: spawn(prompt = dispatch text + s_child + i_child + goal)

    Note over SA,EN: 3 — The delegate works only via MCP, only in s_child
    SA->>MCP: resume_session(s_child)
    MCP-->>SA: serve: current step of i_child + report schema
    loop research per served steps
        SA->>MCP: search / show (reads land in s_child read ledger)
        MCP-->>SA: entries in full
        SA->>MCP: next(s_child, i_child, report)
        MCP-->>SA: serve: next step instructions
    end

    Note over SA,EN: 4 — Result returns inside the engine, not through prose
    SA->>MCP: next(s_child, i_child, final report)
    MCP->>EN: final transition of i_child
    activate EN
    Note right of EN: delivers declared values into<br/>declared fields of i_parent<br/>(parent step does not move)<br/>concludes s_child
    EN-->>MCP: serve: completed
    deactivate EN
    MCP-->>SA: done
    SA-->>OA: host-side return (text = summary, not a source)

    Note over OA,EN: 5 — Parent continues with delivered values in i_parent state
    OA->>MCP: next(s_parent, i_parent, {})
    MCP-->>OA: serve shows delivered values<br/>instructions require pulling entries in full (show)<br/>before engaging — the child's reads never<br/>counted for s_parent
```

Boundary notes:

- The engine mints `s_child`/`i_child` at dispatch; the sub-agent never creates
  or discovers anything. It can only act on what its prompt carried.
- The engine cannot check the sub-agent's prompt (guest-surface ceiling). The
  served dispatch instructions are what keeps `s_parent` out of it; fresh
  context is the only supported launch mode.
- The delegate's whole working path is MCP: no `sdd` CLI, no grep over graph
  files. Per-host configuration is only the launch side (sub-agent definition,
  tool allowlist granting the MCP tools).
- The parent's evidence gate is untouched: reads in `s_child` satisfy nothing
  in `s_parent`, so a later capture in the parent must `show` the entries the
  delegate surfaced.
