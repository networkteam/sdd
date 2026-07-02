# Workflow engine v1 — parity inventory and spec-interview record

Session record for steps 1 and 2 of `20260702-180130-d-tac-tdh`. Step 3 (capture the sliced v1 plan) starts from this document.

## Part 1 — Parity inventory (the plan's scope contract)

Buckets: **move** (declarative state graph run by the engine), **query** (data served without dialogue state), **host-side** (stays with the harness), **deferred/dropped**.

### Moves (8)

| Move | Notes |
|---|---|
| catch-up | Lane assembly as engine-side query ops (dynamic injection), briefing composition by agent, ends in a user-chooser |
| capture | The rich shared spine. Everything parameterizes it: supersede, close, refine, short-loop, augment are captures with pre-filled params. Steps: assemble (batched reports OK), playback (user-chooser), write gate (pre-flight inside; override exit user-only), summary verification |
| engage | Anchor → inspect → brief → offer moves; per-kind catalog becomes typed transitions into other moves (mostly parameterized capture) |
| groom | Scan step + per-candidate walk-through with user-choosers |
| evaluate | Two lenses; also the landing-gate junction inside implementation |
| implementation | Process shape only (scope check, AC-union checklist, close protocol); code work stays host work tracked via report-backs |
| interview | First spec-native move — no prose to freeze (per d-prc-e2y / s-cpt-qs2) |
| explore | Multi-target instruction-move: `targets: list<entry-id>` + optional goal; injects target chains, prescribes widen/inspect, agent uses free read tools; report cites what was read (grounding evidence). No fork in v1 |

### Folded into the capture move
Topics research (query steps), participants inference (session state), ref-kind selection + attachment assessment (step instruction units), pre-flight (hard gate; skip is a user-only chooser exit), summary verification (structural step).

### Shell (engine infrastructure, not moves)
- Alignment junctions (d-prc-1ob) at interaction start and mode transitions
- Goal-driving loop returns next goal every turn ("always suggest next steps" comes free)
- Two-tier knowledge text: base tier at session open; deeper units served just-in-time per step
- Served-instruction memory: per agent-session (not engine-session); full text first, one-line reminder after; resets on resume; pull-by-name always available
- Language rendering per configured locale; vocabulary references become data
- Session framing (aspirations, guiding directives, focus, participants) via dynamic injection

### Host-side (stays)
Worktree lifecycle (d-cpt-h4l), file/code tools, test runs, subagent spawning, free reflect/dialogue (deliberately un-policed per s-cpt-1dz).

### Deferred with revisit triggers
- **Bootstrap**: stays a skill through v1; port at the engine-only transition (its five-move sequence maps naturally to a move spec)
- **Sync reactions**: shell concern by nature (only interactive shells can walk a conflict merge); not in live use — past v1 entirely
- **Explore compression fork**: revisit if graph scale makes direct reads too heavy for host context; candidates then: host subagent or engine-side LLM runner; move spec stays dispatch-neutral either way

### Dropped / out of scope
- Meta-process reference text (the shell + moves are its replacement)
- CLI-workaround skill text: shell quoting conventions, heredoc patterns, JSON-flag guidance (the one-shot CLI constraint disappears; CLI itself stays for humans/scripts)
- Release skill (repo-local, never bundled)

## Part 2 — Interview decisions

### Q1 — Move-spec format
Moves are **graph entries**: a new decision kind (`move`), canonical-named like actors (`canonical: capture`; write-once across chains). Frontmatter carries the state machine; body sections carry per-step instruction units (heading-addressable, chunk-friendly). Base set embedded in the binary, always loaded. Revision = supersession: new sdd versions ship successor entries; projects override by superseding the chain head through normal dialogue + pre-flight. **Fork rule: project head wins**; lint flags base-moved-underneath forks as grooming candidates; resolution is deliberate, merge-style — never automatic. The `move` kind lands together with the `rule` kind as **one type-system revision**. d-cpt-dym holds: base correctness ships in the binary, independent of any project graph.

### Q2 — Session state
Memory is runtime source of truth; persistence is an **append-only JSONL event log** — one line per transition report, state reconstructed by replay. Crash-safe by construction; version-stamped (sessions generally don't survive an sdd upgrade mid-flight — accepted). The log doubles as the session protocol: transition reports are the trajectory evidence (s-cpt-icg, s-cpt-qs2) and the forensic record for engine-vs-skill comparison during transition. Per-participant, gitignored. WIP markers stay separate (communication surface toward other participants, not engine state).

Sessions are **self-describing and listable**: descriptor from own state (participant, anchor, move+step, last activity). Tools: list-sessions, resume-session. Resume rehydrates: step position and evidence persist; served-instruction memory resets (new agent consumer). Lifecycle: close on shell close or last move completion; stale sessions sit listable until resumed or discarded.

### Q3 — MCP tool surface
- `start-move(canonical, params)` → instance handle + first instructions/goal. Params are per-move, declared in the spec.
- `next(instance, report)` → advance that instance; returns instructions + next goal or pending user-chooser.
- `abandon(instance)` — explicit discard, logged.
- Session pair: `list-sessions`, `resume-session`.
- **Free read tools**: search, view, show, info — never gated, usable inside or outside moves. Grounding never queues behind ceremony.
- **Writes exist only as move transitions** — no `new` tool (s-cpt-1dz: hard-gate the surface we own).
- Parallel moves: N open instances per session, serial dialogue interleaves them; sub-moves via parent link (engage spawns capture; parent waits on child-completed transition). One log per session, instance-tagged lines.
- Session-scoped **attachment staging**: `stage-attachment` → handle; drafts reference handles; the write gate materializes them. Staging writes session scratch, never the graph. Adopts d-tac-6zt (upload-token surface) and d-tac-d21 (read-side paged access) — both orphaned by the MCP-server plan supersession.

### Q4 — Evidence fields
Per-step report schemas ARE the evidence fields, declared concretely in each base move's spec. Reports may batch steps: a report writes state fields, the engine re-evaluates guards, auto-transitions cascade until a failing predicate, missing field, or user-chooser stops them — so the one-shot full-draft capture stays as fast as today. No generic evidence vocabulary in v1 (s-cpt-li1 stays open until dogfooding shows recurring shapes).

### Q5 — Rule-system sequencing (the d-tac-eho call)
**Spine first.** V1 ships instruction-unit serving with checks declared in move specs. The `rule` kind is the fast-follow: rules are attachments that arrive from the graph instead of the spec — binding rules add predicates at matching steps, advisory rules add instruction units — joining the same serving mechanism with semantic activation on top. The step model is rule-ready by design. `move` + `rule` kinds are designed now as one type-system revision.

### Q6 — Explore dispatch
Dissolved: no fork in v1. Explore is an instruction-move (see inventory); the engine's contribution is denser read output over time. Compression mission parked (see deferred items).

## Part 3 — Engine mechanics settled

- **Typed variable store per move instance.** Spec declares: `params` (set at start-move), `state` (report-writable, collected via `collect:` lists), `internal` (written only by ops and chooser calls — trust-bearing fields structurally out of the agent's reach). Domain types from a small closed set: text, entry-id, ref, label, participant, attachment-handle, bool, lists thereof. Field `desc` seeds both generated instructions and report schemas.
- **Closed Go registry, three function classes**: predicates (pure checks over the store: hasBody, refsResolve, refKindsValid, participantsCanonical — largely the existing mechanical pre-flight checks re-exposed; single path), queries (finders; results land in the store), commands (handlers; gate steps only). Registry is enumerable (`registry` query) for spec authors and lint.
- **Guards are boolean combinations of named predicates — nothing else.** No expression language, no assignment syntax in YAML. All logic in Go, composed by name. Chooser options carry `call:` (named Go function) + transition target; ops declare result bindings. Confirmation staleness (edit-after-confirm reopens playback) is an implementation detail inside the confirmPlayback/playbackConfirmed function pair.
- **Chooser protocol**: pending chooser validated (no early/late/double answers); user answers relayed by the agent as a distinct report type, logged verbatim as user-transitions. Honest relay is the cheap path; fabrication is explicit, durable, auditable (s-cpt-icg). Trust upgrades per shell: MCP elicitation routes choosers through host UI where supported (s-cpt-80v); the hosted webapp answers with a real click. Same spec, three trust levels.
- **Dynamic injection in the spine** (pattern per d-tac-k4l): steps run query ops engine-side; instruction units are Go templates (d-tac-2oj) rendered against the store. Needed by the session shell's framing before catch-up asks. Boundary: injection is for deterministic data lanes; judgment-driven grounding (widen) stays agent-executed with the report citing what was found.
- **Unit-testable by design**: store + predicates + transitions are pure Go over data; per-move table tests (given store + report → expect stop or cascade).
- **Authority rule** (from the activity, reaffirmed): once a move ships in the engine, its spec is authoritative and the corresponding skill prose freezes.

## Part 4 — Open items carried forward
- Multi-search read tool: captured as 20260702-204144-s-tac-j5a (probe list in, fused deduped results out; generalizes hybrid RRF).
- Generic evidence vocabulary: wait for dogfooding shapes (s-cpt-li1).
- Capture move's parameter surface (what an entering transition may pre-fill) — spec-design task in the plan.
- Denser show/search rendering as ongoing read-tools investment line.
