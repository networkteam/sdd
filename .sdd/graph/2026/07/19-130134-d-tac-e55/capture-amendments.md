# Capture-procedure amendments (proposal, for review)

Bootstrap dispatches ordinary `capture` sub-moves, but three of the kinds it needs
— actor, role, and (slice 2) focus — cannot flow through capture today: assemble
collects none of their kind-specific fields, no presence predicates exist for them,
the assemble gate demands refs+topics which actor/role/focus don't naturally have,
and playback renders none of them. These are the edits to `d-prc-cap` and the engine
that make those kinds capturable. The bootstrap entry above assumes they exist.

None of this is written into cap.md yet — it's proposal prose for your read. The
actual edits land in the wiring slices.

## 1. New capture state fields

Add to capture's `state:` (all optional — only the relevant ones are collected per kind):

```yaml
canonical:   {type: text, optional: true, desc: "an actor's canonical name — the stable identity used across the graph"}
aliases:     {type: list<text>, optional: true, desc: "other names the actor appears under — git, chat, external handles"}
roleActor:   {type: text, optional: true, desc: "for a role, the canonical of the actor this role binds to"}
# slice 2 (focus):
involvement: {type: list<involvement>, optional: true, desc: "the target(s) this focus advances, each with optional actors and time range"}
focusActors: {type: list<participant>, optional: true, desc: "the actors carrying this focus, when set at focus level"}
focusWhen:   {type: involvement-when, optional: true, desc: "the focus-level time range (from/to)"}
```

**`involvement` is a real structured type, not `list<text>`** (DECIDED). The engine
already has the exact precedent — `ref` is a structured type decoded from a JSON
object, validated by `refFromMap` (types.go), with a schema fragment advertising its
shape (schema.go). A new `involvement` type is that pattern applied once more, and it
is strictly cheaper than text: text would still need a separator convention, a
hand-rolled parser, and parse-error handling — the struct moves all of that into the
JSON schema, which *becomes* the format definition the agent is handed, so it can't
malform the value.

Concrete work (all mirrors `ref`):

- **types.go** — `TypeInvolvement BaseType = "involvement"` + baseTypes entry; an
  engine `Involvement` struct `{Target string; Actors []string; ActorsSet bool; When *When}`
  mirroring `model.Involvement`; a `case TypeInvolvement` in `validateBaseValue` calling
  an `involvementFromMap` helper (~40 lines, like `refFromMap`) — validates `target`
  with `model.ParseID`, `actors` as strings (preserving the ActorsSet nil-vs-explicit-empty
  distinction the model relies on), `when` via `model.FocusWhen.Validate` for the ISO
  dates. `focusWhen` reuses the same `when` sub-shape (`involvement-when`).
- **schema.go** — a `case TypeInvolvement` emitting
  `{type: object, properties: {target: <entry-id pattern>, actors: {array of string}, when: {from, to}}, required: [target]}`
  (~15 lines, copy the `ref` case). This is what tells the agent the object shape.
- **command path + playback** — needed regardless of representation (see §4, §6).

Net extra cost over `list<text>`: the validator + schema case (~55 lines), all
well-trodden. Isolated to slice 2 (focus) — slice 1 ships without any new engine type.

## 2. Kind-conditional assemble gate

Today the assemble gate hard-requires `hasRefs and hasTopics`. That structurally
blocks an actor signal (no natural refs) and a bare focus. Proposed: the refs/topics
requirement becomes conditional on kind.

- **actor / role**: require the identity fields instead of refs/topics.
  - actor: `hasBody and hasCanonical` (+ `aliasesWellFormed` if aliases present)
  - role: `hasBody and hasCanonical and roleActorResolves` (the bound actor must exist)
- **focus** (slice 2): `hasBody and hasInvolvement and involvementTargetsResolve`
- **everything else**: unchanged — `hasBody and hasRefs and hasTopics and …`

New presence/validation predicates to register (mirrors the existing `has<Field>` set):
`hasCanonical`, `hasRoleActor`, `roleActorResolves`, `hasInvolvement`,
`involvementTargetsResolve`, `aliasesWellFormed`.

Topics: actor/role/focus may still *carry* topics (optional), but topics are no
longer gate-required for them.

## 3. Kind-specific assemble guidance

The assemble unit's kind rubric currently omits actor, role, focus. Add short
guidance blocks, keyed on the selected kind, that tell the agent what each needs —
e.g. for actor: "introduce the canonical, the identity they bring from outside this
project (affiliation, background, expertise), and any aliases"; for role: "how this
person contributes *here* — authority, domain weight, authorship — bound to their
canonical"; for focus: "the target(s) it advances and who carries it."

This is the split-at-draft discipline from the legacy skill: when the user describes
a person in mixed identity+contribution terms, the agent drafts TWO candidates (an
actor and a role) rather than one — but in bootstrap that split happens up in the
cluster synthesis, and each lands as its own capture here.

## 4. Playback rendering

The playback unit renders body/kind/layer/intent/confidence/refs/topics/branch/
supersedes/closes/attachments. Add conditional lines so the kind-specific fields
show in the verification contract:

```
{{if .canonical}}- canonical: {{.canonical}}{{if .aliases}} · aliases: {{range .aliases}}{{.}} {{end}}{{end}}{{end}}
{{if .roleActor}}- role of: {{.roleActor}}{{end}}
{{if .involvement}}- advances: {{range .involvement}}{{.}} {{end}}{{if .focusWhen}} · when: {{.focusWhen}}{{end}}{{end}}
```

## 5. Playback fidelity adaptation (the "recognition, not review" beat)

When capture is dispatched from bootstrap, the words in the draft were already
approved in bootstrap's cluster playback. The per-entry playback should read as
recognition, not a second full review.

**Mechanism: seeded `recognitionMode` flag** (DECIDED — explicit chosen over
parent-introspection). Bootstrap's `materialize` dispatch seeds a `recognitionMode`
bool onto the capture child. The playback unit branches on it: with it set, the ask
frames as *"these are the words you settled on a moment ago — confirming they go in
as-is"* rather than the full field-by-field contract. The invariant holds unchanged —
the body is still shown verbatim and an explicit confirm is still required; only the
framing around it softens. Requires: `recognitionMode: {type: bool, optional: true}`
in capture state, a `seed: {recognitionMode: recognitionMode}` on materialize's
`captureEntry` dispatch (bootstrap state carries a constant-true it seeds), and the
`{{if .recognitionMode}}…{{else}}…{{end}}` branch in the playback unit.

## 6. Command / write-path wiring (the mechanical half)

`application.EntryDraft` and `runWorkflowNewEntry` carry none of these fields;
`model.ValidateEntry` hard-fails without them. Add to `EntryDraft`: `Canonical`,
`Aliases`, `Actor` (role linkage), and (slice 2) `FocusActors`, `FocusWhen`,
`Involvement`; wire them through `CreateEntry` onto the `model.Entry`. The CLI-side
`NewEntryCmd` already carries all of these — this is copying its field set onto the
engine draft, not new domain logic.

## 7. Actionable gate errors (closes the s-prc-g0j wedge)

When `model.ValidateEntry` fails at the write gate, the failure must re-serve
naming the violated rule and the missing field, routing back to a step that can fix
it — not wedge the instance behind playback with an opaque "validation failed: 1
issue(s)". This is the second half of what g0j records.

---

## Resolved decisions

1. **`involvement` type** → real structured engine type (§1). The `ref` pattern; ~55
   lines net over text, and it removes the fragile string-format path entirely.

2. **Fidelity mechanism** → seeded `recognitionMode` flag (§5). Explicit over
   parent-introspection.

3. **Slice boundary** → two slices, cut so the new structured type is isolated:

   - **Slice 1 — actor + role, plus the shared gate infra.** New state fields
     `canonical`, `aliases`, `roleActor`, `recognitionMode` (§1); the kind-conditional
     assemble gate (§2) and its non-focus presence predicates (`hasCanonical`,
     `hasRoleActor`, `roleActorResolves`, `aliasesWellFormed`); kind-specific assemble
     guidance for actor/role (§3); playback rendering for the actor/role fields and the
     `recognitionMode` branch (§4, §5); the `EntryDraft`/`newEntry` wiring for
     `Canonical`/`Aliases`/`Actor` (§6); and actionable gate errors (§7). This is what
     makes the **bootstrap procedure loadable and its handoff dependency (an actor)
     reachable** — so the bootstrap entry, its presence predicates, and the readiness
     layout ride slice 1 too. Bootstrap ships and is evaluable on actor/role alone.
   - **Slice 2 — focus.** The new `involvement`/`involvement-when` engine type
     (types.go + schema.go, §1); `focusActors`/`focusWhen` state; the focus gate branch
     `hasInvolvement and involvementTargetsResolve` (§2) and those predicates; focus
     assemble guidance (§3) and playback (§4); the `FocusActors`/`FocusWhen`/`Involvement`
     `EntryDraft` wiring (§6). **Closes s-prc-g0j.** Focus is an optional bootstrap
     lens, so nothing in slice 1 depends on it.

   Rationale for the cut: slice 1 carries no new engine type (lower risk, unblocks
   bootstrap fast); slice 2 isolates the one genuinely new structured type with its own
   tests and lands the g0j closure independently.
