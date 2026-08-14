# Cold-start procedure authoring probe

An Opus agent with no SDD priming was given only the crumbs a real session serves (the two index lines: view grammar, type-system overview), read-only graph tools (search, show, view, registry, info), and the task of authoring an incident-review procedure entry — a dry run, nothing written. Its full report follows verbatim: navigation log, the drafted entry, and its critique of the served material.

---

# 1. NAVIGATION LOG

| # | Tool call | What I pulled | Cue that sent me there |
|---|---|---|---|
| 1 | `ToolSearch` `select:mcp__sdd__{search,show,view,registry,info}` | Schemas for the five read-only tools | Task instruction |
| 2 | `mcp__sdd__show` `[20260812-180000-s-prc-typ, 20260717-110000-s-prc-vwg]` (up 1, down 1) | The two crumb facts in full: the type-system overview and the view-layout grammar | The opening crumb named both and said "pull the relevant fact in full first" |
| 3 | `mcp__sdd__show` `[20260813-170000-s-prc-prd, 20260814-100000-s-prc-psr, 20260812-170000-s-prc-dnk]` (up 1) | Procedure-kind authoring fact, procedure **spec reference** fact, done-kind authoring fact | Type-system fact: "each kind has its own authoring fact … pull it before drafting that kind" — and it listed `s-prc-dnk` + `s-prc-prd` as refs. `s-prc-psr` appeared as a truncated ref ("+1 more refs truncated") and turned out to be the field-level spec reference. I pulled the **done** fact too because my procedure's third phase records a done. |
| 4 | `mcp__sdd__registry` (all classes) | Full live inventory: 46 predicates, 8 queries, 11 commands, each with `reads`/`writes` | `s-prc-psr`: "Pull the registry by class … whenever you write steps: what it lists is exactly what a spec can name." **Key discovery:** predicates are bound to *fixed field names* (`hasSynthesis` reads `synthesis`), so state naming is not free. |
| 5 | `mcp__sdd__search` `kind=procedure`, semantic | All 6 procedure entries in the graph (imp, eng, cap, cat, evl, bst) | I wanted a real example of the form; the procedure fact mentioned shipped vs project procedures, so I checked what exists |
| 6 | `mcp__sdd__show` `[20260703-200000-d-prc-evl]` | The evaluate procedure in full | Closest shape to my task: gathers evidence, judges, ends by recording a done with findings. **Discovery:** its frontmatter carries **no** `params`/`state`/`steps` — only id/type/kind/layer/confidence/canonical/status/time |
| 7 | `mcp__sdd__show` `[20260703-094500-d-prc-cap]` | The capture procedure in full | To confirm finding #6 wasn't specific to evaluate, and to learn capture's start inputs since my procedure must dispatch it. Confirmed: `show` serves no workflow frontmatter for any procedure, though downstream dones prove the fields exist ("the two field declarations, four collect lists"). Also gave me the ref-kind vocabulary and the template-placeholder idiom |
| 8 | `mcp__sdd__search` semantic: "dispatching a sub-move with parent, seeding a child procedure instance" | Found `s-tac-9l9` (parent→child grounding handoff carrying `widenReport`/`anchor`), `s-prc-tso` (the gap that *is* this exercise: port a workflow as the first custom project procedure using only what the graph serves) | No served material explained how a step starts another procedure — the spec fact's `call:` names commands only, and no registry command dispatches a procedure |
| 9 | `mcp__sdd__show` `[20260706-170000-d-prc-imp]` | The implementation procedure in full | Its summary says it "records the hash-citing closing done" — the exact pattern for my phase 3. Taught me the idiom: unit prose instructs the agent to *enter capture as a dispatched sub-move*, then report `doneEntry` — matching the registry's otherwise-unexplained `hasDoneEntry`/`doneEntryResolves` pair |
| 10 | `mcp__sdd__view` `active:as-counts` | Live topic vocabulary with counts and heat | Capture's assemble unit: "Labels are stable identifiers — reuse beats reinvention", and it injects exactly this. No incident/ops label exists; `reliability/discipline` (98 entries) is the nearest |

I did **not** call `mcp__sdd__info` (no session framing needed for a dry run) and made no gated call.

---

# 2. THE DRAFTED PROCEDURE ENTRY

```markdown
---
id: 20260814-143000-d-prc-inc
type: decision
kind: procedure
layer: process
confidence: medium
canonical: incident-review
class: move
topics:
    - reliability/discipline
    - incident/review
refs:
    - id: 20260703-200000-d-prc-evl
      kind: related
      desc: the sibling review move this one is deliberately not — evaluate judges work the project committed to, this reviews a failure nobody planned
params:
    anchorHint:
        type: text
        optional: true
        desc: the user's pointer to what broke — a service name, an error, an entry ID, or the incident in their own words
state:
    anchor:
        type: entry-id
        optional: true
        desc: the graph entry the incident centres on, when one exists — the done that shipped the change, the plan it realized, or the gap that predicted it
    widenReport:
        type: text
        desc: the evidence gathered and the graph searched — what you read outside the graph (logs, deploys, commits, dashboards) and what searches you ran inside it, each with what it surfaced
    inspectedIds:
        type: list<entry-id>
        desc: the entries this review examined in full and will reference — every ID the recorded review points at must appear here
    brief:
        type: text
        desc: the incident timeline — what happened in order, each moment tied to the evidence that shows it, with the gaps in the record named rather than smoothed over
    synthesis:
        type: text
        desc: the root-cause account — the mechanism that produced the failure, stated so a reader can see why it broke, with your confidence in it and what would falsify it
    doneEntry:
        type: entry-id
        desc: the ID of the done signal the sub-capture wrote — the durable record of this review
steps:
    - id: scope
      collect: [anchor?, widenReport, inspectedIds]
      inject: {fn: entryChains, args: {up: 2, down: 1}}
      transitions:
          - when: hasWidenReport and hasInspectedIds and inspectedIdsResolve
            to: reconstruct
    - id: reconstruct
      collect: [brief, synthesis]
      transitions:
          - when: hasBrief and hasSynthesis
            to: review
    - id: review
      chooser: user
      options:
          - {choice: confirm, to: record}
          - {choice: correct, collect: [brief?, synthesis?], to: reconstruct}
          - {choice: abandon, to: end(abandoned)}
    - id: record
      collect: [doneEntry]
      transitions:
          - when: hasDoneEntry and doneEntryResolves
            to: end(completed)
status: active
time: "2026-08-14 14:30:00"
---

# body

The incident-review procedure turns a production failure into a durable, agreed account: gather what actually happened from the host and the graph, reconstruct a timeline and a root-cause reading, put that account to the user to confirm or correct, and record the confirmed review as a done signal that closes nothing and references what it reviewed. It is deliberately not the evaluate move (20260703-200000-d-prc-evl): evaluate judges work the project committed to, against criteria the project set; an incident is a failure nobody planned, so there is no contract to judge it against and nothing for the review to complete. The account is the deliverable, and the user owns it — the agent drafts, the user rules. What the review *implies* — a fix to commit to, a fragility now visible, a question left open — is never folded into this record; each of those is its own entry pointing back at the review.

## unit: scope

Establish what is being reviewed and gather the ground truth before you narrate anything.{{if .anchorHint}} The user's pointer: "{{.anchorHint}}".{{end}}

Whatever the graph already holds near this incident, served in full:

{{.entryChains}}

**Gather outside the graph first — an incident's evidence is mostly not in it.** Read the actual record: application and error logs across the window, deploy and release history, the commits and merges that landed before the failure, monitoring or alert history, and any user or operator report of the symptom. This is host work; do it rather than reason about it. Where a source you would expect is unavailable, say so — a missing source is a fact about the incident, not a detail to omit.

**Then search the graph from several angles.** The change that shipped the broken behaviour, the plan or directive it realized, prior gaps describing the same symptom, and any earlier review of a similar failure — a repeat is the most valuable thing this move can find. Inspect promising results in full before they shape the account; titles are pointers, not facts.

Report `widenReport` with what you read and what each source showed, and `inspectedIds` with every entry you read in full and intend to reference. Report `anchor` when the incident genuinely centres on one entry, and leave it unset when it does not — an invented centre is worse than none. Every ID must resolve; the step holds until it does.

## unit: reconstruct

Write the account the user is going to rule on. Two fields, and they do different jobs — do not let the second leak into the first.

**`brief` — the timeline.** What happened, in order, each moment tied to the evidence that shows it: the deploy at a time, the first error, the first human notice, the mitigation, the recovery. Timestamps and sources belong beside the entries. Where the record is silent — no alert fired, no log covers the window — write the gap in as a gap. A timeline that reads as continuous when the evidence is not continuous is the single most misleading thing this move can produce.

**`synthesis` — the root cause.** The mechanism: what condition met what change to produce this failure, stated so a reader can see the causation rather than being asked to accept it. Separate what the evidence shows from what you infer, name your confidence plainly, and say what observation would falsify the reading. Stop at the mechanism — no remediation plan, no blame, no process verdict. Causes are about the system; if the honest account requires a choice ("we should switch to X"), that is a decision the user makes later in its own entry, not a clause in this one.

Present both to the user in their language as you report them — this step is where the agent's draft becomes reviewable, and the next step is the user's, not yours.

## unit: review

The account is the user's to rule on: they were there, they own the system, and a root cause they do not accept is not a root cause. Show it as written — the timeline and the cause verbatim, not a paraphrase — and ask them plainly.

Timeline:

{{.brief}}

Root cause:

{{.synthesis}}

Wait for an explicit answer; an affirmation in passing is not confirmation, and if it is unclear, ask. Relay their words verbatim with the choice.

- **confirm** — the account stands as shown, and the review is recorded.
- **correct** — the account is wrong or incomplete. Take their correction as authoritative over your reading, carry it into `brief` and `synthesis`, and the corrected account returns here for confirmation. Correcting is the expected path, not a failure of the draft.
- **abandon** — the wrong incident, or too early to account for it. Nothing is recorded.

## unit: record

The review outlives this session only as a graph entry. Record it as the dispatched capture sub-move, started with this instance as `parent` so the grounding you already gathered seeds the draft and you do not re-search: enter capture with `kind` done.

The entry:

- **It closes nothing, and that is deliberate.** A review is not the fix. Do not reach for `closes` to make the record feel finished, and do not close the gap that predicted the failure — the act that repairs the system closes that.
- **It references what it reviewed.** `refs` carries the entries from `inspectedIds`, each with the kind that says why it is there: `grounded-in` for evidence the account reasons from, `builds-on` for the shipped work the failure came out of, `related` only where nothing sharper fits. Every ref must be visible in the body's narrative, and every ID in the body must be a ref.
- **The body is the confirmed account** — the timeline and the root cause as the user confirmed them, plus what the reviewing itself was like: evidence that was missing, a source that disagreed with another, how far the reading is trusted. Cite the durable traces the account rests on — commit hashes, released versions, log locations. This review changed nothing in the system, so the record's own trace is the user's confirmation of the account: say in the body that they confirmed it.
- **Topics** name the shape of the work: `incident/review`, extended for the incident's own area where that helps (`incident/review/deploy`).

Anything the review surfaced that stands on its own is a separate capture, `surfaced-by` this done and anchored where its subject lives — the fragility now visible, the question the evidence could not settle, the fix worth committing to. Offer those to the user; do not smuggle them into the review record.

Then report `doneEntry` — the written done's ID. The run ends there; deciding what to do about the incident is its own work, with its own record.
```

---

# 3. CRITIQUE

## How useful was each piece, and did the trail hold?

**The crumb trail held for exactly two hops, then went quiet at the point of highest risk.**

- **Type-system overview (`s-prc-typ`) — genuinely excellent, and the best-designed piece in the chain.** It answered the question I actually had ("is a repeatable practice a procedure, an activity, or a plan?") with a discriminator sharp enough to settle it, and its closing line — *"each kind has its own authoring fact … pull it before drafting that kind"* — is the single most valuable sentence in the material, because it made the next hop non-optional rather than optional. Its refs carried me straight to both kind facts I needed. Note that I needed **two** kind facts (procedure for the entry, done for what the entry writes), which the overview supports well by listing per-kind facts as refs.
- **Procedure-kind fact (`s-prc-prd`) — strong on *why* and *when*, and its handoff worked.** "Every other entry is read, a procedure is also run" earned the kind for me immediately. Canonical-as-identity, the three classes, gate-vs-agent-chooser-vs-user-chooser ("a procedure that asks the user to confirm what a gate could check is ceremony") directly shaped my design: I gated the mechanical checks and spent my one user chooser on the confirmation that is genuinely the user's. Its pointer to the spec fact — *"reach for it the moment you move from choosing the kind to writing the workflow"* — is correctly placed.
- **Spec reference (`s-prc-psr`) — necessary but the thinnest link in the chain relative to what it must carry.** It gave me the three frontmatter fields, the declaration shape, the three step forms, the closed type set, and a skeleton. Without it I could not have written frontmatter at all. But it is roughly one screen of prose standing in for the entire authoring surface, and every gap below is in it.
- **Registry — the highest-value call I made, and the material undersold it.** `s-prc-psr` frames it as "the list of names a spec can name." It is much more than that: because each predicate declares its `reads`, the registry silently dictates **state field naming**. There is no `has(field)` — only `hasSynthesis`, `hasBrief`, `hasDoneEntry` over fixed names. So a gate is only expressible if my field is named what some shipped procedure already named it. That reshaped my design: my "timeline" had to become `brief` and my "root cause" `synthesis` to be gateable at all. The skeleton in `s-prc-psr` uses `synthesis` and so passes by coincidence, which actively conceals the constraint from anyone who reads only the skeleton.
- **View-grammar fact (`s-prc-vwg`) — served, but nearly irrelevant to this task.** I used it once, to compose `active:as-counts` for the topic vocabulary. It was in my opening crumb with equal billing to the type-system fact, which is a misallocation of a two-item crumb for an authoring task. It would have mattered had I needed a `viewLayout` inject.

## What was underspecified — questions I could not answer from served material

Each of these is a place I guessed, and my draft could fail engine load on any of them:

1. **"How is a boolean guard written in YAML?"** `s-prc-psr` says a guard is "a boolean combination (`and`, `or`, `not`) of ability names" and then every example in the skeleton is a **single bare predicate**. Infix string (`when: hasBrief and hasSynthesis`) or structured (`when: {and: [hasBrief, hasSynthesis]}`)? I guessed infix. No served example shows a compound guard.
2. **"How does a step dispatch another procedure?"** This is the load-bearing hole. My procedure's whole third phase depends on it. `call:` names *commands*, and no registry command starts a procedure. I only resolved it by reading `d-prc-imp`, which does it in **unit prose** ("capture the closing done signal as the dispatched sub-move … enter capture with `kind` done"), with the `parent` argument supplied by the agent's own tool call. The facts never say that the dispatch is prose rather than structure — a reasonable author would hunt for a spec field that does not exist, or invent one.
3. **"Is `inject` one query or a list?"** `s-prc-psr` shows `{fn, args?}` singular. Can a step inject two? Capture's assemble renders `{{.viewLayout}}` from a layout arg and `{{.widenReport}}` from state, which is consistent with one inject — but does not settle it. I used one.
4. **"What are the arg names for each query?"** The registry documents `viewLayout` (`layout`, `maxBytes`) and `entryChains` ("Args up, down") in *prose*, not schema. I guessed `args: {up: 2, down: 1}`. `factIndex`, `procedureList`, `sessionInfo` document no args at all — I cannot tell if that means zero args or undocumented ones.
5. **"How is an injected result named in the template?"** I inferred `{{.entryChains}}` = the `entryChains` query, purely from reading capture and evaluate. Never stated.
6. **"Is `anchor?` the right optional-collect syntax, and does an optional field that stays empty break a resolve-predicate?"** The `?` suffix is documented. But `anchorsResolve` says "The anchor and every target resolve to existing graph entries" — while `involvementTargetsResolve` and `roleActorResolves` explicitly say "Absent … passes". The asymmetry looks meaningful, so I avoided `anchorsResolve` entirely and leaned on `inspectedIdsResolve`. That is a workaround for a documentation ambiguity, not a design choice.
7. **"What does an inject serve when the field it reads is empty?"** `entryChains` reads `anchor`/`targets`; my `anchor` is optional. Empty section, error, or omitted? Unknown — and my `## unit: scope` renders `{{.entryChains}}` unconditionally.
8. **"Can a chooser option route back to an earlier step, and does the target step re-serve its unit?"** My `correct` → `reconstruct` → `review` loop assumes yes. Capture's "This play-back recurs after every adjust" implies re-serving, but that is prose in a different procedure, not a stated engine semantic.
9. **"Do `status` and `time` belong in an authored file?"** The server instructions say status "is derived from the graph, never edited in place", yet every served entry displays `status:` and `time:` in frontmatter. I included them, matching the served form, with no way to know whether an author writes them or capture mints them.
10. **"Does a procedure capture require `refs` and `topics`?"** All six shipped procedures carry **neither**. But capture's assemble gate uses `hasRefs`/`hasTopics` and exempts only actor/role/focus. So either shipped procedures bypass capture as embedded entries (likely), or procedure is also exempt (unknown). I included both defensively — which means my entry may carry refs and topics that the form does not want.
11. **"Should the user confirmation use `confirmPlayback`/`playbackConfirmed`?"** Those exist and read exactly like what my `review` step does. Their docs are written in capture's vocabulary ("bound to the current state snapshot", "the recording chooser"), so I judged them capture-internal and did not use them. If they are general, my procedure is weaker than it should be for no reason.
12. **"What is the `class` default in the written file — omit or state?"** "empty means move." I stated `class: move`; I cannot see whether shipped move-class entries state it, because `show` hides it.

## What was missing entirely

- **A worked example of a procedure spec — anywhere.** This is the biggest failure, and it is structural, not editorial: **`show` does not serve `params`/`state`/`steps` for procedure entries.** I read three procedures in full and saw zero lines of workflow frontmatter, while their downstream dones openly discuss "the two field declarations, four collect lists" and "`supersedes` declared `list<entry-id>`" — so the fields exist and the read tool omits them. The one thing an author must write is the one thing the graph will not show. The 12-line skeleton in `s-prc-psr` is therefore the *entire* corpus available, and it contains no compound guard, no `inject`, no `call`, no `render`, no `default`, no gate `op`, and no multi-step routing.
- **The `has*`-predicate naming constraint, stated as a constraint.** Nothing warns that state names are effectively a closed vocabulary inherited from shipped procedures. An author naming fields naturally (`timeline`, `rootCause`, `evidence`) would write a spec with no expressible gates and only discover it at engine load — and the error would be "unknown ability `hasTimeline`", which points at the guard rather than at the real problem (the field name).
- **How a project procedure becomes loadable at all.** `s-prc-prd` says project procedures "live in its graph beside" shipped ones, and `s-prc-psr` says "Start small, load it, run it." Nothing says what makes a captured procedure entry *discoverable* as a move — whether the head of a canonical chain is picked up automatically, whether anything must be registered, or how to load-test a spec without running it against a real session.
- **Guidance on writing an instruction unit.** `s-prc-prd` says what a unit *is* (authoritative guidance, to the person in the situation, never documentation) — good and useful — but nothing on the mechanics: which placeholders resolve, that `{{range}}`/`{{if}}` work, that struct fields are reachable (`{{.index.title}}`, `{{.when.from}}`), or that findings are `{{.severity}}`/`{{.Severity}}` (capture's units use **both** casings — `{{.severity}}` for guide findings, `{{.Severity}}` for pre-flight findings, which looks like a latent bug and is certainly a trap).
- **A `params`-vs-`state` decision rule.** The definitions ("params seed the store once at start", "state are working fields") do not resolve the real case: `anchor` and `widenReport` arrive from a *dispatching parent*, not from the caller. I declared them state because capture and evaluate visibly treat them that way — inferred from examples, never stated. `s-tac-75b` records that capture's lifecycle fields were **moved** from params to state, so this is a distinction the project itself got wrong once.
- **Any incident/operations vocabulary.** The topic census has 68 labels and nothing for incidents. Minting `incident/review` is defensible under "a new label deliberately starts a cluster", but no served material helps an author judge when a new family is warranted versus stretching `reliability/discipline`.

## What the existing procedure entries taught me that the reference material had not

Reading `d-prc-evl`, `d-prc-cap`, and `d-prc-imp` was worth more than the spec fact, and everything below came only from them:

1. **Sub-procedure dispatch is prose, not structure.** `d-prc-imp`'s `## unit: record` — "capture the closing done signal as the dispatched sub-move … enter capture with `kind` done and `closes` naming the fulfilled commitments" — then "Report `doneEntry`". That is the entire mechanism, and it explains why `hasDoneEntry` and `doneEntryResolves` exist in the registry with no fact mentioning them. My `record` step is a direct copy of this shape.
2. **The body opens with an un-headed preamble whose first sentence is a standalone summary.** All three do it. No fact says a procedure body has anything but `## unit:` sections; the summary that appears in `search` results comes from that preamble.
3. **Units address the running agent in second person and route the junction options by name**, listing each choice with what it means and what it collects — `d-prc-imp`'s six-option `setup` unit is the model. The facts say units are "written to the person in the situation" but never show that the option list is spelled out in prose *and* declared in frontmatter, in both places.
4. **"Relay the user's answer verbatim" is a standing convention** at every user chooser, in all three procedures. Not in any fact I was served.
5. **Conditional inheritance of parent grounding is a house idiom:** `{{if .widenReport}}Grounding was inherited from the dispatching move… extend it only where…{{else}}Search the graph…{{end}}`, near-identical in capture, evaluate, and implementation. I did not replicate it (I kept `scope` single-branch to stay small), but I would not have known it was the convention.
6. **`confidence: medium` is the house norm for procedures** — all six carry it. And procedures carry **no topics and no refs**, which contradicts what I inferred from capture's own assemble gate.
7. **Units point at facts rather than restating them** — capture's assemble says "pull the type-system fact `20260812-180000-s-prc-typ` before choosing." `s-prc-prd` states this rule ("a pointer inside a unit says why and when to go read a fact"); seeing it executed is what made it writable.

## Verdict

**No — an agent with no SDD priming cannot reliably produce a loadable procedure from the served material alone.** It can get the *judgment* right: the crumb trail is genuinely good at kind selection, class, canonical discipline, gate-versus-chooser economy, and what a done must and must not carry. My entry's *shape* — three phases, one user chooser on the confirmation, a done that closes nothing and refs what it reviewed, findings split into their own entries — came almost entirely from served facts, and I am confident in it.

But the **syntax** is guesswork at a dozen points, and at least three of my guesses (compound-guard form, `inject` arg names, optional-field/resolve-predicate interaction) are coin flips that would fail at engine load. Worse, the one guess I could not have made at all — how to dispatch the capture sub-move that makes phase 3 possible — I resolved only by going off-trail to read a shipped procedure body. An agent that trusted the crumb trail and stopped where it stopped would have written a procedure that cannot record anything. That is precisely the failure mode `s-prc-tso` set this exercise up to detect: *"if that succeeds, the knowledge system works; if not, the gap persists."* The knowledge system got me to a well-shaped procedure and left me guessing at whether it loads.

**Top 3 improvements:**

1. **Serve procedure workflow frontmatter through `show`.** This is one change and it fixes more than any amount of new prose. Six worked specs already exist in the graph, covering compound guards, multi-inject, choosers with `call`, `render`, `default`, and gate `op` — and the read tool hides all of it while cheerfully serving the bodies. Every underspecification above would be answerable by reading one real spec. (If procedures must render redacted for some reason, then `s-prc-psr` needs a second, *fully annotated* example spec inlined — 60 lines, not 12 — exercising every field it documents.)
2. **Make the `has*`-predicate/field-name coupling explicit in `s-prc-psr`, and give it a rule.** State plainly: "a gate can only test a field a predicate names — consult the registry's `reads` before choosing a field name, and reuse the established names (`brief`, `synthesis`, `plan`, `contract`, `widenReport`, `inspectedIds`, `doneEntry`, `anchor`) rather than inventing." Then either add a generic `hasField(name)` predicate or say outright that the state vocabulary is closed. Right now the constraint is discoverable only by cross-referencing 46 predicate `reads` lists, and the skeleton's `synthesis` hides it by luck.
3. **Document sub-procedure dispatch as a first-class part of the procedure model.** Add a "Dispatching another procedure" section to `s-prc-psr`: the dispatch happens in *unit prose*, the agent starts the child with this instance as `parent`, `anchor` and `widenReport` flow parent→child automatically, the child's product comes back as a reported state field, and the parent gates on it (`hasDoneEntry` + `doneEntryResolves`). Include the `task` class's role here, since `s-prc-prd` defines `task` as "a delegate a move dispatches with resolved inputs" and then never shows a dispatch.

*Runner-up, and cheap:* replace the view-grammar fact in an authoring session's opening crumb with the spec-reference fact. For this task, `s-prc-vwg` bought one `as-counts` call while `s-prc-psr` — the fact I actually could not work without — was reachable only through a **truncated** ref line ("+1 more refs truncated") on the type-system fact. The most load-bearing document in the chain was one display-truncation away from being invisible.
