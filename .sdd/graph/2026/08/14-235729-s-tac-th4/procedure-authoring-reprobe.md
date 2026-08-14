# Cold-start procedure authoring probe — re-run after the improvements pass

A fresh Opus agent with no SDD priming was given only the crumbs a real session serves (the two index lines: view grammar, type-system overview), read-only graph tools (search, show, view, registry, info), and the task of authoring an incident-review procedure entry — a dry run, nothing written. Same conditions as the original probe attached to 20260814-134331-s-tac-9kn; run against the binary carrying the improvements pass. Its full report follows verbatim, then the loader verification the original run never had.

---

# 1. NAVIGATION LOG

| # | Tool call | What I pulled | Cue that sent me there |
|---|---|---|---|
| 1 | `ToolSearch` | Schemas for the 5 permitted tools | Task instruction |
| 2 | `show` `s-prc-vwg`, `s-prc-typ` | View-layout grammar; type system (kinds, layers, competing-kind tests) | The two facts named in my starting knowledge |
| 3 | `show` `s-prc-prd`, `s-prc-psr` | Procedure-kind authoring fact; procedure-spec writing reference (params/state/steps, gateable field vocabulary, variable types, worked example) | `s-prc-typ` closes with "each kind has its own authoring fact — pull it before drafting"; `s-prc-prd` was its named ref, and `s-prc-psr` was `s-prc-prd`'s named ref plus a truncated ref on `s-prc-typ` |
| 4 | `registry(predicate)`, `registry(query)` | Every guard name with what it reads; `entryChains`, `viewLayout`, `procedureList`, etc. with their args | `s-prc-psr` §Abilities: "request it by class whenever you write steps: what it lists is exactly what a spec can name" |
| 5 | `registry(command)`, `search(kind=procedure)` | Commands (`newEntry`, `wipStart`, `confirmPlayback`…); one procedure hit (`engage`) | Same cue; search to find a real procedure entry to model frontmatter on |
| 6 | `show` `d-prc-eng`, `view active:kind(procedure):as-list` | Engage's full frontmatter + unit style; the 10 active procedures | Needed a real on-disk procedure shape — the spec fact shows only a bare `procedureSpec`, never a whole entry. The `kind` search filter under-returned (1 of 10), so I fell back to a view layout composed from `s-prc-vwg` |
| 7 | `show` `d-prc-cap`, `search` entry-construction | Capture's full param/state surface (my dispatch target's seedable names) + its assemble unit, which is the de-facto per-field authoring guide | `s-prc-psr` §Dispatching: seeds map "child field: parent field", so I needed capture's declared field names |
| 8 | `show` `d-prc-evl`, `view active:as-counts` | Evaluate — nearest kin (evidence → judgment → user junction → dispatched done capture); live topic labels with counts | Evaluate was the closest existing shape to an incident review; capture's assemble unit says topics must reuse existing labels |
| 9 | `info` | Participant `Christopher`, search modes `vector,text` | Check whether `participants` was expected |

# 2. THE DRAFTED PROCEDURE ENTRY

```markdown
---
id: 20260814-193000-d-prc-inc          # engine-assigned at write
type: decision
kind: procedure
layer: process
confidence: medium
canonical: incident-review
class: move
topics:
    - engine/base-procedures
    - reliability/discipline
refs:
    - id: 20260703-200000-d-prc-evl
      kind: related
      desc: the kindred backward-looking move — evaluate judges committed work, this reconstructs an unplanned event
    - id: 20260703-094500-d-prc-cap
      kind: depends-on
      desc: the capture procedure this move dispatches to record the confirmed review; it writes nothing itself
    - id: 20260813-170000-s-prc-prd
      kind: grounded-in
      desc: the procedure kind's authoring fact — the basis for making this a runnable entry rather than prose guidance
status: active                          # derived
time: "2026-08-14 19:30:00"             # engine-assigned at write
params:
    incidentHint: {type: text, optional: true, desc: "the user's pointer to the incident in their words — what broke, when, or where they noticed it"}
state:
    anchor: {type: entry-id, optional: true, desc: the graph entry the incident centres on, when one already exists}
    targets: {type: list<entry-id>, optional: true, desc: entries implicated alongside the anchor — the commitment that shipped it, neighbouring signals}
    goal: {type: text, desc: "what this review must answer, in the user's terms — settle it with them before gathering"}
    widenReport: {type: text, desc: "every source consulted and what it surfaced: graph searches, logs, commits, alerts, human accounts — each with what it showed and what it did not"}
    inspectedIds: {type: list<entry-id>, desc: every graph entry read in full that the review will lean on}
    brief: {type: text, desc: "the account of what happened — sequence, blast radius, detection and recovery, each claim tied to the evidence that carries it"}
    synthesis: {type: text, desc: "the reading of why it happened, stated so its confidence and its limits are visible — name what remains unexplained rather than closing the story"}
    confidence: {type: confidence, desc: "honest grade on the causal account: high / medium / low"}
    doneEntry: {type: entry-id, desc: the recorded review the dispatched capture hands back
steps:
    - id: frame
      inject:
          - {fn: viewLayout, args: {layout: 'done:n(15):name("Recently completed work")', maxBytes: 2500}}
      collect: ["anchor?", "targets?", goal]
      transitions:
          - when: hasGoal
            to: gather
    - id: gather
      inject:
          - {fn: entryChains, args: {up: 2, down: 2}}
      collect: [widenReport, inspectedIds]
      transitions:
          - when: hasWidenReport and hasInspectedIds and inspectedIdsResolve
            to: reconstruct
    - id: reconstruct
      collect: [brief, synthesis, confidence]
      transitions:
          - when: hasBrief and hasSynthesis and hasConfidence
            to: confirm
    - id: confirm
      chooser: user
      render: account
      options:
          - choice: confirm
            dispatch:
                procedure: capture
                seed:
                    widenReport: widenReport
                    anchor: anchor
            to: record
          - {choice: correct, collect: ["brief?", "synthesis?", "confidence?"], to: reconstruct}
          - {choice: abandon, to: end(abandoned)}
    - id: record
      collect: [doneEntry]
      transitions:
          - when: hasDoneEntry and doneEntryResolves
            to: end(completed)
---

# body

The incident-review procedure turns a production incident into a durable, user-confirmed account: frame what the review must answer, gather evidence from the graph and from the running system, reconstruct what happened and why with each claim tied to its source, put that account to the user to confirm or correct, and record the confirmed review through a dispatched capture. It is the backward-looking counterpart to evaluate (20260703-200000-d-prc-evl) — where evaluation judges work the project committed to, this reconstructs an event nobody planned, so its output is an account with stated confidence rather than a verdict against acceptance criteria. Like every move, it writes nothing itself: the confirmed review lands as a done signal through the capture procedure (20260703-094500-d-prc-cap), dispatched with this run's grounding already seeded. Encoding it as a procedure rather than prose guidance (20260813-170000-s-prc-prd) is deliberate — the two things incident reviews reliably skip are grounding the account in evidence and getting the affected human to correct it, and both are held here by structure: the gather step cannot advance without a widen report and resolved inspected entries, and the account reaches the graph only through a user chooser that stops the run until the user answers.

The confidence grade is collected as its own field rather than folded into the prose, because the honest answer to "why did this happen" is often *partly* — a review that reads as certain when it is not is worse than one that names its gaps, and the graded field is what forces that admission before the user ever sees the account.

## unit: frame

Establish what this review covers and what it must answer.{{if .incidentHint}} The user's pointer: "{{.incidentHint}}".{{end}}

Recently completed work, for locating the incident against what shipped near it:

{{.viewLayout}}

Report `goal` — what the user needs out of this review, in their terms. Ask them; do not infer it from the incident's severity. A review aimed at "why did the alert fire so late" gathers different evidence than one aimed at "can this recur", and the goal chosen here selects what you go looking for.

Report `anchor` when a graph entry already centres this incident — the signal someone captured while it was happening, or the commitment whose work introduced it — and `targets` for entries implicated alongside it. Both are optional and often absent: a fresh incident usually has no entry yet, and that is normal, not a blocker. Do not mint a placeholder anchor to fill the field, and do not resolve an anchor from recency — "the last thing we shipped" breaks in a shared graph. If the user gestures at something, search from several angles and settle the centre with them.

## unit: gather

What the graph holds around the incident, served in full — read it; summaries elsewhere are pointers:

{{.entryChains}}

Now gather the rest of the evidence. This is host work, not a graph exercise: go to the logs, the metrics, the deploy history, the commits in the window, the alert timeline, and the people who were there. Search the graph from several angles too — the failing component, the goal phrase, terms from the chains above — for prior incidents with this shape, directives that were supposed to prevent it, and open gaps that predicted it.

Record in `widenReport` every source you consulted and what each one showed — including the ones that showed nothing. A source that was expected to carry evidence and did not is itself a finding, and it is the detail that a later reader cannot reconstruct.

Report `inspectedIds` for every graph entry you read in full that the account will lean on. Report them honestly: this is the set that carries the review's grounding, and a downstream capture inherits it as its own inspection record.

Resist writing the account here. Gathering that stops the moment a plausible story appears is how the first plausible story becomes the recorded cause.

## unit: reconstruct

Build the account from what you gathered.

Report `brief` — what happened: the sequence with times, what was affected and for whom, how it was detected, and how it was recovered. Every claim carries its source from `widenReport`. Where the evidence runs out, say so in the account rather than smoothing the gap with an inference; a reader must be able to tell reconstruction from record.

Report `synthesis` — why it happened. Go past the proximate trigger to the conditions that let the trigger matter, and name what the incident revealed about how the system is built or operated. State it so its limits are visible: what you are confident about, what is inference, what stayed unexplained. If a contributing cause is contested between sources, present both readings rather than picking the tidier one.

Report `confidence` on the causal account: high — the evidence pins it; medium — a well-supported reading with gaps; low — a hypothesis worth recording as one. Grade the *why*, not the timeline; the sequence is often certain while the cause is not.

Do not write remediation here. What to do about it is a decision the user makes with this account in hand, captured on its own afterwards — folding a fix into the review makes the account unreviewable, because rejecting the fix now means rejecting the history too.

## unit: account

The reconstructed incident review, as it stands:

**What happened**

{{.brief}}

**Why**

{{.synthesis}}

Confidence in the causal account: **{{.confidence}}**{{if .anchor}} · centred on {{.anchor}}{{end}}

## unit: confirm

Put the account above to the user — whoever carried the incident knows things no log holds, and this is the step where that lands. Present it in their language, but show the account itself as written rather than paraphrasing it: they are correcting these words, and the recorded review will carry them.

Ask directly whether the sequence is right, whether the causal reading matches what they lived, and whether the confidence grade is honest. Silence is not confirmation, and neither is a nod at the summary — wait for an explicit answer and relay their words verbatim with it.

- **confirm** — the account stands as written. The run moves to recording it, with this run's grounding seeded into that capture.
- **correct** — carry their correction back into `brief`, `synthesis`, or `confidence`, and the account is rebuilt and shown again. Expect this to happen more than once; a review corrected twice is working, not failing. Report only the fields that changed.
- **abandon** — the review should not exist: it was mis-framed, or the incident turned out to be something else. Nothing is recorded.

Never reach for confirm on their behalf because the account looks complete. Whether this history is true is theirs to say, not yours.

## unit: record

The account is confirmed and now needs to outlive this session — unrecorded, it evaporates when this dialogue ends.

Start the capture procedure as a sub-move of this run, with this instance as `parent`. The answered dispatch already seeded your `widenReport` and `anchor`, so you draft without re-grounding. Draft it as:

- `entryKind` **done** — a review is a past act, and this records that it was carried out.
- `layer` **operational** unless the synthesis reaches further: a review whose causal reading is really about how the system is shaped is conceptual, and one about how the team works is process.
- Body carrying the confirmed account — the sequence, the causal reading, and the stated confidence, with the first sentence standing alone as a summary of what the incident was and what the review concluded.
- `refs`: `grounded-in` this procedure (20260814-193000-d-prc-inc); `surfaced-by` or `builds-on` the work implicated by the synthesis; the anchor entry when one exists.
- `closes` the incident's own signal entry when this review resolves it — the account *is* the resolution.

Then report `doneEntry` — the entry ID the capture hands back. That ID is the only thing that crosses back from the child run, and this step holds until it resolves.

If the synthesis pointed at something that must change, that is a separate capture the user decides on next, anchored where its subject lives and `surfaced-by` this review. Do not fold it into the review entry.
```

# 3. CRITIQUE

## Usefulness of the served material, hop by hop

**The trail held, and it held cleanly — that is the strongest finding here.** Four hops, each one signposted by the previous, no dead ends and no guessing at what to pull next:

- `s-prc-typ` → **high value.** Settled type/kind/layer in one read and closed with an explicit instruction ("each kind has its own authoring fact — pull it before drafting"), with the pointer sitting right there in `refs`.
- `s-prc-prd` → **high value.** Told me what the kind buys (structure the agent can't drift from), that `canonical` is the identity, what `class` means, and — crucially — the "ask where the choice is real" test, which is what shaped the confirm step into a user chooser and kept the gates mechanical. Ended with an explicit "reach for the spec reference the moment you move from choosing the kind to writing the workflow."
- `s-prc-psr` → **exceptional.** The single most load-bearing document. The gateable-field-name vocabulary is the standout: it converts the hardest silent failure mode (a guard that loads fine and then never advances) into a lookup table, and it says so in those words. The worked example is explicitly "valid by construction, shipped from the same source the engine's tests load" — which is what let me trust its shape rather than reverse-engineer it.
- `registry` → **exceptional, and correctly positioned.** `s-prc-psr` doesn't list abilities, it tells you to ask — right call, and the registry answered with `reads` sets and arg names. I verified all nine guards and both queries in my spec against it. Zero guessing on ability names.
- `s-prc-vwg` → **served its purpose but I nearly didn't need it.** Only used to compose the `viewLayout` inject and my fallback `view` call. Note the machine-generated hint on the `view` response already points at it — good discovery affordance.

**The real gap in the trail: nothing pointed me at a whole procedure entry.** `s-prc-psr` shows a bare `procedureSpec` fragment; it never shows the surrounding frontmatter. I only learned the entry-level shape (`kind: procedure`, `layer: process`, `canonical` at top level, `class` as a sibling of `params`) by going and reading `d-prc-eng` on my own initiative. An agent that took the served material at its word would produce a workflow with no idea how it attaches to an entry. **`d-prc-cap`'s assemble unit turned out to be the de-facto authoring guide for every non-workflow field** — refs kinds, topics, confidence, the standalone-first-sentence rule — and nothing in the procedure trail points at it. I found it only because I went hunting for my dispatch target's field names.

Also worth flagging: `search(kind=procedure)` returned **1 of 10** active procedures. The `view` layout returned all 10. If I had trusted search, I'd have had one example instead of three.

## Underspecified — where I guessed

1. **Does `anchorsResolve` pass when `anchor` is absent?** Its doc says "The anchor and every target resolve to existing graph entries" — no absence clause. But `roleActorResolves` and `involvementTargetsResolve` *explicitly* say "Absent X passes." That asymmetry reads as deliberate, so I **dropped `anchorsResolve` from my `frame` gate entirely** rather than risk a permanently-stalled step. Cost: a bogus anchor ID goes unvalidated until capture rejects it. I guessed, and I guessed defensively.
2. **Param vs. state for seeded fields — the served rule contradicts the shipped code.** `s-prc-psr` states flatly: "A field that arrives from a dispatching parent rather than the caller is state, not a param — seeding writes state only." But `d-prc-eng` seeds `anchor` into `capture`, and `capture` declares `anchor` as a **param**. Either the rule is wrong, the shipped procedure is wrong, or seeding silently falls back. I followed the shipped precedent over the documented rule. This is the single most likely thing in my spec to be wrong.
3. **Are `topics` required on a procedure?** All three procedures I read (`engage`, `capture`, `evaluate`) carry **none**. But `capture`'s assemble unit exempts only actor, role, and focus from topics — procedure isn't on that exemption list, and `draftValidates` is opaque about which rules apply per kind. I included topics, reasoning that the shipped ones are embedded/exempt while a project entry is not. Guess.
4. **Which topics?** No incident/ops/postmortem label exists in the landscape. I picked `engine/base-procedures` (wrong-ish — this isn't a base procedure) and `reliability/discipline` (adjacent at best). The served guidance says "a new label deliberately starts a cluster" but gives no test for when starting one is warranted. Guess.
5. **Can a `dispatch` block name a `procedure` on an option that also has `collect`?** `engage` uses `dispatch` with a `seed` but **no `procedure:` key at all**; `evaluate` uses `procedure: capture`. Both are shipped and valid. What the omitted form means (any procedure? deferred to prose?) is never explained.
6. **Does `render:` accept only non-step unit names, or any unit?** Both live uses (`guideFindings`, `findings`) name non-step units. I assumed non-step-named units are the intended form and wrote `## unit: account` accordingly — but nothing says a step-named unit couldn't be rendered elsewhere.
7. **Is `class: move` written explicitly equivalent to omitting it?** "Empty means move" is stated; whether the explicit literal is accepted at the same place isn't. `engage` and `evaluate` both omit it. I wrote it explicitly for legibility. Small risk.
8. **What layer should the *recorded review* be?** I told the runner "operational unless the synthesis reaches further." That's my invention — the served material gives layer definitions but no guidance for classifying a retrospective record.
9. **How is the entry actually submitted?** I produced a file-shaped entry with `id`/`status`/`time`, but the real path is reporting `body` + `entryKind` + `canonical` + `class` + `procedureSpec` (as *bare YAML*, per capture's field desc) through capture's assemble step. The relationship between "the entry as it lands on disk" and "the fields you report" is never stated in one place — I inferred it from capture's state declarations.
10. **`maxBytes` on `viewLayout`** — I used 2500. No guidance on sizing exists; the worked example uses 2000 with no rationale.
11. **Confidence on the entry itself** — I used `medium`, copying all three shipped procedures. No guidance on grading a *procedure's* confidence (confidence in what? that the workflow is right?).

## Missing entirely

- **A whole valid procedure entry as a reference.** The single biggest hole. The spec fact shows a workflow fragment; nothing shows the entry it lives in.
- **Any guidance on step granularity.** Nothing says whether an incident review should be three steps or eight, or what makes a step a step rather than a paragraph in a unit. I patterned mine on `evaluate` because it was there, not because a principle told me to.
- **Unit prose craft.** `s-prc-prd` says units are "written to the person in the situation, never documentation about the procedure" — one sentence, and it's the only guidance for the majority of the artifact by volume. What makes a good unit, how long, whether to use headers or bullets: absent. I imitated `capture` and `evaluate`.
- **Template syntax.** Units "may use template placeholders." That it's Go `text/template` — `{{if .x}}`, `{{range}}`, `{{.field.subfield}}` — I learned only by reading shipped bodies. An agent that didn't go read one would guess at `${}` or `{}`.
- **How a project procedure differs from a shipped one in practice.** `s-prc-prd` says project procedures "live in the graph beside them" and may depend on the project's environment, but nothing about whether they need topics/refs that shipped ones skip.
- **Ref-kind guidance for a procedure entry.** The closed set lives in capture's assemble unit; nothing helps you choose kinds when the target is another procedure.

## Verdict

**Partly — an agent with no SDD priming can produce a *structurally* loadable procedure from the served material alone, but not a well-formed graph entry, and only if it independently thinks to go read a shipped procedure.**

The workflow half is genuinely well-served. The gateable-name vocabulary plus an askable registry with `reads` sets plus a worked example that is valid by construction is a strong combination — I wrote nine guards and two injects with zero name-guessing, and I'd expect that half to load. Credit where due: the material anticipated the exact failure mode (guard names a field nothing declares) and pre-empted it.

The entry half is not served by this trail. Everything outside `params`/`state`/`steps` — the frontmatter envelope, refs, topics, unit prose craft, template syntax — I got from reading `capture` and `engage` on my own initiative, which the trail never told me to do. A more literal agent stops after `s-prc-psr` and submits a workflow with no entry around it. And my three most consequential decisions (guessing #1, #2, #3) each came down to trusting shipped code over served prose.

**Top 3 improvements:**

1. **Show one complete procedure entry in `s-prc-psr`, or point at one.** Replace or supplement the fragment with a full entry — frontmatter envelope, refs, topics, body, units — or add a line: "read `20260703-200000-d-prc-evl` in full for a complete worked entry." One sentence closes the largest gap in the probe.
2. **Fix the param-vs-state contradiction, and give `anchorsResolve` an absence clause.** These are the two places where served material and shipped behaviour disagree or go silent, and both sit on the path of any procedure that dispatches or takes an optional anchor. The registry docs should state absence semantics uniformly — every resolver either says "absent passes" or says it doesn't.
3. **Point the procedure trail at capture's assemble guidance for the non-workflow fields.** `s-prc-prd` ends by pointing at `s-prc-psr` for the workflow; neither points anywhere for refs, topics, confidence, or the standalone-first-sentence rule, all of which are already written down in `d-prc-cap`'s assemble unit. A closing pointer — "for the entry's own fields, the capture procedure's assemble guidance is authoritative" — would complete the trail. (Bonus, cheap: `search(kind=procedure)` returning 1 of 10 active procedures looks like a real retrieval bug worth a signal of its own.)

---

# 4. LOADER VERIFICATION (run by the session after the probe returned)

The drafted spec was reconstructed verbatim and run through the engine's actual loader (`model.ParseEntry` + `engine.LoadSpec` against the builtin registry with the two shell queries stubbed):

- **Run 1, as written:** YAML parse error — the `doneEntry` declaration is missing its closing `}` (line-numbered diagnostic).
- **Run 2, brace fixed:** two YAML errors — unquoted flow-mapping `desc` strings containing commas (`anchor`, `targets`), each with a line number.
- **Run 3, quotes fixed:** **loads clean.** All nine guards, both injects, the `render: account` unit, the dispatch block, optional collects, and the correction loop are valid on first try.

Every failure was character-level YAML quoting — the class of error the original probe never even reached, and the class that the structured procedure-spec reporting value (shipped the same day) removes at the transport boundary entirely.
