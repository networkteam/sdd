# Workflow Engine v1 — the visible surfaces

Everything an author, agent, or shell can see or touch: entry kinds, the procedure definition schema, the Go registry exposed to YAML, the MCP tool surface, and session persistence. Distilled from the parity inventory and spec interview (s-tac-4yf), for the v1 plan (d-tac-tdh, step 3). Engine architecture per d-cpt-3yw. Reviewed 2026-07-02; all open questions resolved (§8).

## 1. Type-system revision: two new decision kinds

One revision supersedes the 7+7 contract (d-cpt-ni0) with a transitional enumeration, adding two decision kinds designed together. Only `procedure` ships in v1; `rule` is the fast-follow that joins the same serving mechanism (settled in s-tac-4yf, per d-tac-eho).

### `kind: procedure` — a playbook move as a graph entry

| Field | Meaning |
|---|---|
| `canonical` | Required. The procedure's stable identity (`capture`, `engage`, …). Write-once across chains, exactly like actor canonicals. Everything binds to the canonical, never the entry ID. |
| `params` | Typed inputs accepted at `start_procedure`. Per-procedure; see §2. |
| `state` | Report-writable variable store collected during the move. The only part of the store reports can touch — engine-written values (created IDs, findings, confirmations) are *not declared*; they enter the store per the Go contracts of the functions the spec names, and collisions with `state` are rejected at load. |
| `steps` | The state graph. See §2. |
| body | Markdown instruction units, one section per step (`## unit: <name>`). Go templates rendered against the store at serve time. |

Base procedures ship embedded in the binary — always loaded, correct independent of any graph (d-cpt-dym). A newer sdd version ships successor entries; a project customizes by superseding a chain head through normal capture discipline. **Fork rule: the project head wins.** When a shipped base successor and a project override compete, lint flags the fork as a grooming candidate; resolution is deliberate, merge-style — never automatic.

> **Decided — naming.** The kind is **`procedure`** — "process procedure decision" reads formally right on every rendered surface. The dialogue vocabulary stays **"playbook move" / "move"**: instructions and briefings speak of moves; frontmatter, tools, and the log say procedure. A running execution is a *procedure instance*.

> **Decided — layer.** Procedure entries are **pinned to the process layer**, like actor and role. A procedure defines how we work; the work it orchestrates varies, the definition doesn't.

### `kind: rule` — designed now, shipped fast-follow

Per d-tac-eho: required `enforcement` (`binding` | `advisory`) and an `activation` block (`when` for semantic discovery, `matches` for mechanical selection). In engine terms a rule is *an attachment that arrives from the graph instead of the spec*: binding rules add predicates at matching steps, advisory rules add instruction units. The step model is rule-ready from day one; v1 base procedures declare their checks in the spec.

## 2. Procedure definition schema

### Variable declarations

Every variable in `params` / `state`: `{type, optional?, desc}`. The `desc` seeds both the generated report schema and the "provide the following" instruction text — one declaration, both surfaces.

| Domain type | Meaning / built-in validation |
|---|---|
| `text` | Markdown or plain text |
| `bool` | — |
| `entry-id` | Full entry ID; resolution checkable for free |
| `ref` | `{id, kind, desc?}`, kind from the closed ref-kind set |
| `label` | Topic path (component rules enforced) |
| `participant` | Canonical name (active-actor checkable) |
| `entry-kind`, `layer`, `confidence`, `intent` | Closed enumerations from the type system |
| `attachment-handle` | Session-staged attachment reference (§5) |
| `preflight-findings` | Structured findings from the write gate |
| `list<T>` | List of any of the above |

Adding a domain type means adding it in Go, once, with its validation. No other types exist.

### Step fields

| Field | Meaning |
|---|---|
| `id` | Step name; default instruction unit is the body section of the same name |
| `collect` | State fields a report may write at this step. `field?` marks optional. Reports may batch fields for several steps — the cascade rule below makes one-shot drafting as fast as today |
| `inject` | Query ops the engine runs before serving the unit: `{fn, args?}`. Args are literals or Go templates rendered against the store with type-aware escaping (same mechanism as instruction units). Results enter the template context (dynamic injection, pattern per d-tac-k4l) |
| `render` | Instruction-unit override (body section name), when it differs from `id` |
| `chooser` | `gate` (default; auto-advance on guards) · `agent` · `user` |
| `options` | For agent/user choosers: `{choice, call?, collect?, to}`. `call` names a command run on selection; `collect` lets the answer carry revised fields |
| `guard` | Predicate expression that must hold before the step's op runs |
| `op` | Command executed at this step (gate steps only). Its reads/writes are its documented Go contract — no binding syntax in YAML |
| `transitions` | Ordered `{when, to}` list; final entry may be `{otherwise, to}` |

### Guard grammar — deliberately poor

```
expr := predicateName
      | expr "and" expr
      | expr "or" expr
      | "not" expr
      | "(" expr ")"
```

Boolean combinations of named Go predicates. Nothing else — no comparisons, no literals, no field access, no assignment anywhere in YAML. Logic that doesn't fit is either a new registry function (Go, unit-tested) or a chooser (judgment).

### Cascade rule

A report writes its fields; the engine re-evaluates guards; gate transitions cascade until something stops them — a failing predicate (its failure message becomes the instruction), a missing field, or a chooser that belongs to the agent or the user. If no transition fires, the step stays and names exactly what's missing.

> **Decided — where the logic lives.** All semantics live in named Go functions composed in YAML. Engine-written state enters the store only through ops and chooser `call`s, per their documented contracts — the spec neither declares nor maps it. Confirmation staleness (edit-after-confirm reopens playback) is an implementation detail inside the `confirmPlayback` / `playbackConfirmed` pair — invisible to the spec, unit-tested in Go.

## 3. Worked example: the capture procedure

The shared spine. Supersede, close, refine, short-loop, and augment are this procedure entered with different params.

```yaml
kind: procedure
canonical: capture
params:
  anchor:     {type: entry-id,       optional: true, desc: entry this capture is anchored on}
  supersedes: {type: entry-id,       optional: true, desc: chain head this capture replaces}
  closes:     {type: list<entry-id>, optional: true, desc: entries this capture resolves}
  kind:       {type: entry-kind,     optional: true, desc: pre-selected target kind}
state:
  body:        {type: text,          desc: entry description; self-describing first sentence}
  entryKind:   {type: entry-kind,    desc: signal or decision kind, from the distinguishing tests}
  layer:       {type: layer,         desc: strategic | conceptual | tactical | operational | process}
  refs:        {type: list<ref>,     desc: each {id, kind, desc?}; kind chosen from what the body asserts}
  topics:      {type: list<label>,   desc: reuse existing labels; new label starts a cluster}
  confidence:  {type: confidence,    desc: honest — high / medium / low}
  intent:      {type: intent,        optional: true, desc: required when entryKind is directive}
  attachments: {type: list<attachment-handle>, optional: true}
  widenReport: {type: text,          desc: searches run and entries inspected before drafting}
# no internal declarations — entryId, findings, the confirmation record enter the
# store via the Go contracts of newEntry / confirmPlayback / recordOverride
steps:
  - id: assemble
    collect: [body, entryKind, layer, refs, topics, confidence,
              intent?, attachments?, widenReport]
    inject:
      - {fn: viewLayout, args: {layout: "active:as-counts"}}   # existing labels, into the unit
    transitions:
      - when: hasBody and hasRefs and hasTopics and hasWidenReport
              and refsResolve and refKindsValid
              and participantsCanonical and intentPresentIfDirective
        to: playback

  - id: playback
    chooser: user
    options:
      - {choice: confirm, call: confirmPlayback, to: write}
      - {choice: adjust,  collect: [body?, refs?, topics?, confidence?, intent?],
         to: assemble}
      - {choice: abort,   to: end(abandoned)}

  - id: write
    guard: playbackConfirmed
    op: newEntry                               # runs pre-flight; writes entryId + findings
    transitions:
      - when: noHighFindings
        to: verifySummary
      - otherwise: reviseOrOverride

  - id: reviseOrOverride                       # pre-flight skip is user-only, by construction
    chooser: user
    render: findings
    options:
      - {choice: revise,   collect: [body?, refs?, topics?], to: assemble}
      - {choice: override, call: recordOverride, to: write}
      - {choice: abort,    to: end(abandoned)}

  - id: verifySummary
    chooser: agent
    inject:
      - {fn: generatedSummary}                 # reads entryId from the store
    options:
      - {choice: faithful, collect: [fidelityNote], to: end(completed)}
      - {choice: drifted,  collect: [correctedSummary],
         call: replaceSummary, to: end(completed)}
```

The body of the same entry carries `## unit: assemble`, `## unit: playback`, `## unit: findings`, `## unit: verifySummary` — the prose the agent actually receives, templated over the store, served full-text once per agent session and as a one-line reminder after.

## 4. Go registry exposed to YAML

Three function classes, closed set, enumerable via the `registry` tool. Lint validates every name a spec references. Predicates are largely the existing mechanical pre-flight checks re-exposed — single path, no duplicated validation.

### Predicates (pure: store → bool)

| Name | Reads | True when |
|---|---|---|
| `hasBody` · `hasRefs` · `hasTopics` · `hasConfidence` · `hasKind` · `hasLayer` · `hasWidenReport` · `hasAnchor` · `hasTargets` · `hasGoal` | the named field | Field present and non-empty (one presence predicate per commonly collected field) |
| `refsResolve` | `refs`, `supersedes`, `closes` | Every referenced entry exists (capture-time invariant per d-cpt-uh0) |
| `refKindsValid` | `refs` | Every ref kind is from the closed set |
| `participantsCanonical` | `participants` | All names resolve to active actor canonicals (grace mode honored) |
| `topicsKnown` | `topics` | All labels already exist in the graph (guard for "flag new label in playback") |
| `intentPresentIfDirective` | `entryKind`, `intent` | Intent supplied whenever the target kind is directive |
| `playbackConfirmed` | engine-written store | Confirmation recorded and the confirmed state unchanged since |
| `noHighFindings` | `findings` | Last gate run produced no high-severity finding |

### Queries (finders: args + store → data, no side effects)

| Name | Args / reads | Returns |
|---|---|---|
| `sessionInfo` | — | Local participant, language, search modes (the `sdd info` header) |
| `viewLayout` | `args: {layout}` — the full `sdd view` pipeline syntax. May be a Go template over the store (`id("{{.anchor}}")`): store values render through type-aware escaping, and lint validates by rendering with typed dummies, then parsing | Rendered view pipeline result — static lanes (catch-up, framing) and store-parameterized lookups alike. `entryChains` stays for chain-tree output, which is `show`-shaped |
| `entryChains` | reads `anchor` / `targets`; `args: {up, down}` | Entries with upstream/downstream chains (engage inspect, explore injection) |
| `generatedSummary` | reads `entryId` | The stored summary for fidelity review |
| `registryList` | `args: {class?}` | Function docs — what spec authors consult |

### Commands (handlers: side effects; gate steps and chooser calls only)

| Name | Reads | Writes / effect |
|---|---|---|
| `newEntry` | the capture state fields | Creates the entry (pre-flight inside; attachments materialized from handles); writes `entryId`, `findings` |
| `replaceSummary` | `entryId`, `correctedSummary` | Writes the user-supplied summary |
| `confirmPlayback` | state snapshot | Records confirmation bound to current state (chooser call, user options only) |
| `recordOverride` | `findings`, relayed words | Records the user-only pre-flight skip, durably logged |
| `wipStart` · `wipDone` | marker fields | WIP marker lifecycle. Owned by procedures, never a free tool: the implementation procedure opens and closes markers (its instance spans sessions via resume); orphaned markers (lost local session, teammate leftover) close through groom's stale-marker walk. Further marker flows — reserving a longer arc early to signal cooperators, an explicit standalone close — are small dedicated procedures composed from these same commands, authorable graph-side without touching the binary |

> **Decided — contracts over bindings.** Each function's reads and writes are its documented Go contract, shown by `registryList` and enforced by tests. YAML never maps fields — it only names functions.

## 5. MCP tool surface — exact

Writes exist only as procedure transitions; reads are free and never gated (s-cpt-1dz). There is no `new` tool.

### The loop

| Tool | Input | Returns |
|---|---|---|
| `start_procedure` | `canonical`, `params?` | `{instance, step, instructions, report_schema, goal}` |
| `next` | `instance`, `report` (state fields *or* a chooser answer `{chooser, choice, userWords?, fields?}`) | Same shape as `start_procedure`, or `{pending_chooser}` with the rendered material and options, or `{completed, produced}` (e.g. the created entry ID + summary) |
| `abandon` | `instance`, `reason?` | Confirmation; logged as an abandonment transition. Never cleans up implicitly: an active WIP marker held by the instance is surfaced and left standing — resume later or close via groom |

`report_schema` is JSON Schema generated from the current step's `collect` list and variable declarations — named, typed, described fields; reports validate against it on arrival. Batched fields for later steps are accepted and cascade.

### Sessions & staging

| Tool | Input | Returns |
|---|---|---|
| `list_sessions` | — | Open sessions with self-derived descriptors: participant, anchor, open instances (procedure + step), last activity |
| `resume_session` | `session` | Rehydration briefing; step position and evidence persist, served-instruction memory resets (new agent consumer) |
| `stage_attachment` | `name`, `content` \| `path` | `{handle}` — session scratch, never a graph write; materialized by the write gate. Adopts d-tac-6zt |

### Free reads

| Tool | Wraps | Notes |
|---|---|---|
| `search` | `sdd search` | Terms, query, filters, limit. Future: multi-probe fused dedup (s-tac-j5a) |
| `view` | `sdd view` | Layout pipeline string in, rendered sections out |
| `show` | `sdd show` | `ids[]`, `up`, `down` |
| `read_attachment` | new accessor | Paged attachment content; agents never derive paths. Adopts d-tac-d21 |
| `info` | `sdd info` | Session framing header |
| `registry` | `registryList` | Function docs per class — for humans superseding base procedures |

## 6. Session persistence

Memory is runtime source of truth. Persistence is an append-only JSONL event log per session — one line per transition report; state is a fold over the lines; recovery is replay. The log doubles as the session protocol: transition reports are the trajectory evidence (s-cpt-icg, s-cpt-qs2) and the forensic record for the engine-vs-skill comparison during transition. Per-participant, gitignored, version-stamped (a session generally does not survive an sdd upgrade mid-flight — accepted).

```json
{"v":1, "ts":"2026-07-02T20:41:00Z", "session":"s_9f2", "seq":17,
 "instance":"i_3", "event":"report",
 "data":{"step":"assemble", "fields":["body","refs","topics"]}}

{"v":1, "ts":"…", "session":"s_9f2", "seq":18, "instance":"i_3",
 "event":"chooser_answer",
 "data":{"chooser":"playback", "choice":"confirm", "userWords":"capture it"}}
```

- **event**: `started` · `report` · `chooser_answer` · `op_result` · `served` · `transition` · `completed` · `abandoned`
- **instance**: one handle per running procedure; a session interleaves N instances serially, sub-procedures carry a parent link
- **lifecycle**: close on shell close or last-move completion; stale sessions stay listable until resumed or discarded. Long-horizon procedures (implementation with a WIP marker) live as resumable instances across agent sessions — the marker outlives any single session by design; the session store is local, so orphaned markers fall to groom

## 7. Trust properties, in one place

- **Reports can only write `state`.** Trust-bearing values (confirmations, findings, created IDs) are engine-written — they enter the store solely through ops and chooser calls, per their Go contracts, and are never declarable or writable from a report.
- **Choosers can't be gamed on sequence.** A pending chooser is validated — no early, late, or double answers; edits after confirmation reopen playback.
- **The pre-flight override is a user-only chooser exit.** The agent can propose; only the user's relayed answer transitions.
- **Relay is auditable.** On the MCP path the agent carries the user's words; fabrication is possible but must be explicit, verbatim, and lands in the append-only log. Honest relay is the cheap path.
- **Trust upgrades per shell, same spec.** MCP elicitation routes choosers through the host UI where supported (s-cpt-80v); the hosted webapp answers with a real click.

## 8. Resolved questions & deferred

> **Decided — review pass, 2026-07-02.** ① Procedure entries pinned to the **process layer** (§1). ② **Agent choosers are advisory**: the pick is logged with whatever evidence fields the option collects (`verifySummary.faithful` carries a one-line fidelity note); binding escalation per step arrives via rules only if dogfooding demands it. ③ The remaining procedures (engage, groom, evaluate, implementation, interview, explore) are **designed per slice against the §2 schema** — spec + table tests as the mechanics gate — under a plan-level parity goal: a usable SDD flow comparable to today's skills, with the inventory defining "comparable" and side-by-side dogfooding measuring it. ④ `report_schema` is **JSON Schema generated from the variable declarations** — the same `desc` that feeds instructions feeds the schema.

> **Deferred with triggers.** Bootstrap (port at engine-only transition) · sync reactions (shell concern, not in live use) · explore compression fork (revisit at graph scale; spec stays dispatch-neutral) · `rule` kind shipping (fast-follow after the spine) · generic evidence vocabulary (s-cpt-li1, after dogfooding) · multi-probe search (s-tac-j5a).

---

Provenance: engine directive d-cpt-3yw · planning activity d-tac-tdh · session record s-tac-4yf (inventory + interview attachment) · rule plan d-tac-eho · Go runtime verdict d-cpt-8k5 · single-source contract d-cpt-chi · template standard d-tac-2oj · dogfooding boundary d-cpt-dym · enforcement ceiling s-cpt-1dz.
