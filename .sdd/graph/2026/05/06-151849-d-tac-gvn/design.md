# Type system extension to 7+7 — design

## 1. Type system change overview

Adding two new kinds to the SDD vocabulary:

- **`kind: annotation`** (signal) — purely structural carrier of typed edges (currently topic-membership). Excluded from catch-up narrative.
- **`kind: focus`** (decision) — intent declaration with involvement triples (target + optional actors + optional time scope). Dual lifecycle.

Plus a topic data model that lives on **every** entry kind (inline `topics:`), with annotation entries providing retroactive/bulk topic assignment.

The new contract entry (captured at plan-done time) supersedes `d-cpt-ygn` and codifies these rules permanently. The plan itself does not directly supersede a contract — cross-kind supersession isn't a graph primitive — instead it commits to producing the new contract as part of its done-signal deliverables.

## 2. `kind: annotation` schema

**Frontmatter:**

```yaml
type: signal
layer: cpt
kind: annotation
participants: [Christopher, Claude]
time: 2026-05-06T15:30:00+02:00
summary: "..."
topics:
  - label: catch-up-scaling
    members:
      - 20260505-215340-s-cpt-rwd
      - 20260505-215333-s-cpt-jq7
      - 20260504-100323-s-cpt-8tu
  - label: type-system/kinds
    members:
      - 20260423-203503-d-cpt-ygn
```

**Validators:**

- `topics:` must be a list of objects, each with `label` (string) and `members` (list of strings)
- Each member must resolve to an existing entry's full ID
- Each label validates per topic-path rules (component-wise `[\p{L}\p{N}\-]+`, no empty components)
- Multiple annotation entries with the same label are allowed; membership is the union (with per-entry first-seen casing wins)

**Body content:** annotations are structural; the body can be empty or carry brief metadata. They are excluded from catch-up narrative renderers (as-list, as-grouped) by virtue of `kind:` filter macros not naming them.

## 3. `kind: focus` schema

**Frontmatter (full example):**

```yaml
type: decision
layer: cpt
kind: focus
participants: [Christopher, Claude]
time: 2026-05-06T16:00:00+02:00
summary: "Drive catch-up infrastructure rework"
actors: [Christopher, Claude]              # default involvement set (optional)
when:                                       # default window (optional)
  from: 2026-05-06
  to: 2026-05-20
involvement:
  - target: 20260505-215340-s-cpt-rwd
    # inherits actors=[Christopher, Claude], when={from:2026-05-06,to:2026-05-20}
  - target: 20260505-215333-s-cpt-jq7
    actors: []                              # explicit unattributed (override)
  - target: 20260417-120309-s-tac-93s
    actors: [Claude]
    when:
      from: 2026-05-06
      to: 2026-05-13
```

**Validators:**

- `involvement:` is a list of objects; each has required `target` (resolves to existing ID); optional `actors:` (canonical-only list, may be `[]`); optional `when: {from?, to?}` with ISO dates and at least one of `from`/`to` if `when:` present
- Top-level `actors:` and `when:` accepted with same shape rules
- Resolution rule (read-side, not validated structurally — implemented in finders): per-involvement value if present (including explicit `[]`), else top-level value, else empty/null

**Lifecycle:**

- **Supersession:** new `kind: focus` entry with `supersedes: <prior-focus-id>`. Standard same-kind chain. Use when priorities shift mid-cycle.
- **Done-closure:** done signal with `closes: <focus-id>`. Use when a cycle finishes naturally (declared work was completed or work-set has resolved).

Both are valid; pre-flight does not argue which one applies when.

## 4. Topic data model

**Two layers:**

1. **Inline `topics:`** on any entry's frontmatter. Zero-ceremony at capture. Values are list of topic-label strings, `/`-joined paths.

   ```yaml
   topics:
     - catch-up-scaling
     - infrastructure/cli
   ```

2. **`kind: annotation`** entries for retroactive/bulk reorganization. Carries `topics: [{label, members}]` pointing at member entries.

**Path representation:**

- Internal: `[]string` (e.g., `["catch-up-scaling"]` or `["infrastructure", "cli"]`)
- I/O: `/`-joined (`catch-up-scaling`, `infrastructure/cli`)
- Component validation: `[\p{L}\p{N}\-]+` Unicode-aware; supports `Onboarding/Bootstrap`, German `Übersicht`, etc.
- Empty components rejected (no `//`, leading/trailing `/`)

**Comparison:**

- Case-insensitive on each path component (`Catch-Up` matches `catch-up`)
- First-seen casing per component preserved for display
- Collision: first-seen wins

**Filter behavior:**

- `topic(UX)` matches `UX`, `UX/CLI`, `UX/Onboarding/Bootstrap`
- `topic(UX/CLI)` matches `UX/CLI`, `UX/CLI/Status`
- Prefix-match on path components, not raw strings (so `topic(UX)` does not match `UXTesting`)

**No aliases in v1.** Topic drift is a future problem; if it surfaces, address with annotation-superseding-annotation patterns.

## 5. CLI capture surface

JSON-in-repeated-flags decouples CLI vocabulary from frontmatter storage shape:

```bash
# Focus capture
sdd new d cpt --kind focus \
  --participants Christopher,Claude \
  --actors Christopher,Claude \
  --when '{"from":"2026-05-06","to":"2026-05-20"}' \
  --involvement '{"target":"20260505-215340-s-cpt-rwd"}' \
  --involvement '{"target":"20260505-215333-s-cpt-jq7","actors":[]}' \
  --involvement '{"target":"20260417-120309-s-tac-93s","actors":["Claude"],"when":{"from":"2026-05-06","to":"2026-05-13"}}' \
  "$DESC"

# Annotation capture
sdd new s cpt --kind annotation \
  --topic '{"label":"catch-up-scaling","members":["20260505-215340-s-cpt-rwd","20260505-215333-s-cpt-jq7"]}' \
  --topic '{"label":"type-system/kinds","members":["20260423-203503-d-cpt-ygn"]}' \
  "$DESC"

# Inline topics on any entry
sdd new s cpt --kind insight \
  --topics catch-up-scaling,infrastructure/cli \
  "$DESC"
```

**Quoting convention:** outer single quotes for the JSON value (preserves `$`, backticks, etc.), inner double quotes for JSON keys/strings. Documented in `cli-reference.md`.

**Error reporting:** JSON parse failures cite line/col. Shape failures cite field paths (e.g., `involvement[2].actors[0]: 'Chris' is not a canonical name`). Mirrors existing pre-flight finding clarity.

## 6. Pre-flight rules

**High-severity (blocking) — mechanical shape only:**

- Frontmatter validation: `involvement:` list shape, target resolution, canonical actors, when format
- Top-level `actors:`/`when:` shape if present
- Standard graph rules: language match, refs/closes/supersedes resolution

**Low-severity (informational, non-blocking, dialogue prompts):**

- Focus closed via done signal but no targets in its involvement carry done signals → potentially abandoned vs completed
- Focus superseded by another focus that shares zero targets with the predecessor → wholesale priority shift (might be intentional cleanup)
- Focus with all involvement unattributed (entirely pull-available) → may not be a meaningful focus declaration

**Out of pre-flight scope entirely:**

- Whether closure rationale is "complete"
- Whether supersession scope is appropriate
- Whether involvement triples represent enough commitment
- Anything semantic about lifecycle decisions

This stance aligns with `s-prc-vvd`'s observation: pre-flight oscillates on supersession-shape rubrics. Keeping focus pre-flight to mechanical-shape-only avoids reproducing that pattern.

**Findings prompt language:** observational, not corrective. "this focus closes with no target completions" — not "missing rationale".

## 7. Display rendering

**Entry-line topic rendering** (in `sdd list`, `sdd status`, consumed by `sdd view`'s `as-list`):

```
20260423-203503-d-cpt-ygn conceptual contract decision [confidence: high] (Christopher, Claude) {status: active} <UX/CLI, Onboarding/Bootstrap> Summary text...
```

Topics render as `<label1, label2>` between `{status: ...}` and the summary. Ordering: inline `topics:` first (declared at capture), then annotation-derived (added later), deduplicated.

**`sdd show <id>` derived topic field:**

The existing depth-0 rendering gains a `Topics:` line listing the entry's topics, marked as derived (computed from inline + annotation membership). Stored attrs vs derived attrs convention applies — `Topics:` is derived (curly-brace style or labeled "derived").

**`sdd list --topic <label>`** filters by topic via the `topic(L)` filter primitive (shared with `sdd view`). Case-insensitive prefix-match on path components.

## 8. Routing vs involvement boundary

| Question | Answered by | Source structure | Cadence |
|---|---|---|---|
| Who's driving this thread *right now*? | involvement | focus entry's `involvement.actors` | weekly-ish |
| Who *should weigh in* when this kind of work happens? | routing | `kind: role` decisions | rarely changes |

Conflating bloats focus entries with role-shaped data, forces role lifecycles through weekly cadences, dilutes both signals.

`s-cpt-5jk`'s broader scope (profile filtering, profile inference, profile evolution) carries forward as a separate question. This plan only settles the boundary.

## 9. CQRS scaffolding

```
internal/
├── command/
│   ├── new_focus.go                    # NewFocusCmd struct
│   └── new_annotation.go               # NewAnnotationCmd struct
├── query/
│   └── (existing query files extended for new kinds)
├── handlers/
│   ├── new_focus.go                    # NewFocusHandler
│   └── new_annotation.go               # NewAnnotationHandler
├── finders/
│   └── topic_filter.go                 # topic(L) filter primitive (shared with sdd view)
├── model/
│   ├── focus.go                        # Focus type, Involvement struct, When struct, resolution helpers
│   ├── annotation.go                   # Annotation type, TopicCluster struct
│   └── topic_path.go                   # TopicPath ([]string), parsing, comparison, validation
├── presenters/
│   └── (existing entry-line presenter extended with topic field)
└── llm/
    └── (pre-flight templates extended for focus and annotation rubrics)
```

Capture commands flow through handlers; reads (topic filter, focus state derivation) flow through finders. Topic filter primitive lives in `internal/finders/topic_filter.go` and is consumed by both `sdd list --topic` (this plan) and `sdd view`'s `topic(L)` macro (Plan 2's shared internals).

## 10. Migration semantics

**Forward-only.** Existing entries are not retroactively annotated.

If users want to topic-tag historical entries, two paths:

1. Supersede the historical entry with a new entry adding inline `topics:` (heavy — only justifiable for active entries)
2. Capture a `kind: annotation` entry pointing at the historical IDs as members (lightweight; the canonical path)

The new contract entry's narrative documents this scope choice.

## 11. New contract entry (plan deliverable)

Captured as part of the plan's done-signal delivery. Shape:

```yaml
type: decision
layer: cpt
kind: contract
participants: [Christopher, Claude]
supersedes: 20260423-203503-d-cpt-ygn
time: <plan-done time>
summary: "..."
```

**Body covers:**

- 7+7 type system (carried forward 6+6 from `d-cpt-ygn` plus annotation and focus)
- Annotation kind structural requirements (frontmatter shape, narrative-exclusion contract)
- Focus kind structural requirements (involvement shape, dual lifecycle, top-level defaults)
- Topic canonicalization rules (path representation, validation, comparison)
- Carried-forward rules from `d-cpt-ygn`: write-once canonical, plan-acceptance-criteria, done-closure-target, canonical-only participants, role-status cascade

## 12. Open implementation questions

These are settled inside plan implementation, not before:

- **Topic-label collision behavior under concurrent capture** — if two sessions independently capture entries introducing the same topic with different casing, which wins? Likely first-merged-via-git wins; document the behavior. Not a blocking design question.
- **Focus entry layer** — `cpt` (conceptual) seems right (focus is approach/shape, not strategy or tactics). But could be argued either way for some focuses. Plan implementation picks based on the typical use case observed during smoke testing.
- **Pre-flight low-severity finding wording** — exact phrasing decided during template design.

## 13. Out-of-scope items

- **Topic alias mechanism** — deferred; if drift becomes a real problem, address with annotation patterns
- **Topic-tree rendering** — Plan 2 reserves `source(topics)` + `as-tree`; out of scope here
- **Retroactive annotation backfill** — forward-only; users do this manually as needed
- **Focus involvement-state visualization** — Plan 2 owns the rendering algorithm; this plan provides the data model
