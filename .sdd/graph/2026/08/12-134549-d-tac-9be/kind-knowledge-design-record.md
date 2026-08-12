# Kind-knowledge layer — design record

Working session between Christopher and Claude, 2026-08-11/12, engaging the type-system-knowledge focus
(20260811-231119-d-tac-uea) with the question "what is the next move". The dialogue reframed the target,
settled the write-model and composition design, and produced the plan that supersedes the full-type-capture
plan (20260726-193730-d-tac-zrh). This record keeps the reasoning, the alternatives rejected, and the
source-code evidence out of the plan body.

## 1. Where the line actually stood

Read of the four focus targets established:

- The pull mechanism ships (base facts embedded, merged on load, index derived from active facts), but
  **no type-system fact exists**. Live evidence: this session's own orientation served exactly one fact
  (the view-grammar fact, `20260717-110000-s-prc-vwg`). Since the index is derived from active facts
  carrying `index` metadata (20260719-133529-d-cpt-qhn), a type-system overview fact would have appeared.
- `d-tac-zrh` had **zero landed slices** since 26 July. Corroborated by search: no done signal references it.
- `d-cpt-x7a` carried `intent: pending` with no plan under it. Its only downstream, `s-tac-r7h`, is the
  defect it classified as wrong-under-any-architecture (the guide not being shown closure edges) — not
  fact work.
- `view wip` returned no active WIP markers: nothing in flight.
- **Contradiction found:** `zrh` AC1 commits one unindexed detail fact per kind; `x7a`, which refines it,
  commits two facts per kind. Implementing `zrh` literally builds the wrong pool.

## 2. The target — mechanism versus outcome

The focus states an *order* (facts, then consumers, then calibration). `zrh` states *deliverables*.
`x7a` states a *structure*. None stated an outcome, so "are we done?" resolved to "do the fact entries
exist?" — mechanism mistaken for result. The nearest existing end-condition, bootstrap parity
(20260726-195300-d-tac-fim), measures whether the engine's setup flow matches the legacy skill, not
whether entries come out better.

Christopher's formulation, adopted as the plan's opener: **the agent should correct itself and the user
toward capturing knowledge according to the framework.** Not "drafts valid entries" — an agent that only
complies cannot push back in dialogue. This has a direct content consequence: the authoring fact must
carry the *why* behind a kind, in reasoning form, because a required-field checklist would pass every
mechanical check and still miss the target.

Two baselines already exist in the graph and were not recognised as such:

- `20260811-183929-s-tac-fu8` — the live writing guide returned five, four, then two findings on
  *identical input* across three runs. That variance is an instrument reading.
- `20260722-141659-s-cpt-rza` — drafting agents recovering through supersedes, restarts and cascading
  overrides across two live bootstrap runs.

## 3. The authoring job is consolidation, not composition from scratch

Claude's initial framing — thirty hand-written prose entries carrying corpus-narrowing risk — was
**wrong** and corrected by Christopher. The knowledge already exists, distributed:

- captured graph entries about the type system;
- `internal/bundledskills/templates/sdd/references/` (the skill's on-demand reference tier);
- the engine's own embedded texts: base-procedure entry bodies and their served instruction units, plus
  the shell entry;
- the 22 kind-specific pre-flight templates (`actor_capture`, `role_capture`, `focus_capture`,
  `aspiration_capture`, `annotation_capture`, `signal_capture`, `closing_done`, `closing_decision`,
  `dissolution`, `settled_justification`, `supersedes`, `contracts`, …), which hold the knowledge **in
  reverse**, as rejections.

Since those sources were written as framework-general guidance rather than lifted from software specimens,
the narrowing mechanism identified by `20260811-172858-s-prc-6b0` applies far less to them than to
specimen-derived discriminators. That weakens — but does not remove — the argument for measuring before
investing.

### Method, as settled

1. **Positive first.** Construct each kind's knowledge from the positive side.
2. **Rubric as audit, never as source.** Then run the pre-flight rubric against the result to find what is
   missing. Rationale: a check used as a *source* silently becomes the definition; a check used as an
   *audit* can only reveal gaps. Reversal is where invention creeps in unnoticed, because each reversal
   feels like a restatement.
3. **Christopher reviews every kind** before it lands. Soundness is his call.
4. **Non-software instantiation.** Each kind's wording is instantiated in a named non-software act — coffee
   roastery, timber construction, child care — and kept only if it still discriminates there. Software
   development is one supported field, not the reference field.

## 4. Composition layer

### Current state (explored in source, file:line evidence)

- Base **procedures** are literal markdown embedded via `//go:embed entries`
  (`internal/baseprocedures/baseprocedures.go:21`), ten files named by full entry ID.
- Base **facts** are *not* markdown. They are Go constants assembled by
  `basefacts.build(id, frontmatter, body)` (`internal/basefacts/basefacts.go:50-64`), which concatenates
  `"---\n" + frontmatter + "---\n\n" + body` and pushes the result through the same `model.ParseEntry`
  used for on-disk entries. Package doc states the split deliberately (`basefacts.go:8-13`).
- Exactly **two** base facts exist: the view-grammar fact (body **fully generated**,
  `basefacts.go:93-95` → `viewlayout.Markdown`, renderer `internal/viewlayout/reference.go:243-267`) and
  the working-principles fact (hand-written Go raw string, `internal/basefacts/principles.go:29-47`).
- **Generation is already established practice**, at load time rather than via codegen: there are zero
  `//go:generate` directives, no Makefile, no `tools/` dir and no devbox codegen script. Names come from
  live registries (`internal/finders/view_vocabulary.go:11-20`); prose comes from metadata tables
  (`viewlayout/reference.go:38-130`); a name with no metadata renders a loud inline
  `ERROR: missing reference metadata` (`reference.go:284-286`); two tests enforce completeness
  (`internal/finders/view_reference_examples_test.go:32-42`, `cmd/sdd/view_test.go:50-52`). Nothing is
  checked in — it recomputes on every graph load, so the entry text cannot drift from the code.
- Entry bodies are **already Go templates with injected query results** at serve time:
  `## unit: <name>` sections parsed by `parseUnits` (`internal/engine/spec.go:574-611`) and rendered by
  `Session.renderUnit` (`internal/engine/instance.go:537-582`).

### Decisions

- **Typed frontmatter, not YAML string constants.** The current shape re-spells the `index` block as text
  even though `model.FactIndex` is a strict typed value with its own validation.
- **Per-kind prose in embedded template files**, not Go raw strings: reviewable diffs, no Go escaping.
- **Full-entry templates, validated on render.** Two options were weighed:
  - *(A) Emit the whole entry as a template and parse it through the write model.* You see the whole entry
    at once; prose authors work in entry-shaped files. Cost: stringly-typed field names.
  - *(B) Compose programmatically and set the body from a rendered template.* Compile-checked frontmatter.
    Cost: the entry is never visible in one place.
  Chosen: **A with B's floor** — author whole entries as templates, validate every rendered entry through
  the write model, and add a table test that renders and validates all base entries so a typo fails the
  build rather than surfacing at a user's graph load. Precedent for the loudness already exists in
  `viewlayout`'s missing-metadata error.
- **Named placeholders for generated blocks.** A kind's prose template carries an explicit placeholder
  (e.g. `{{ mechanics }}`) filled at render from the Go declaration. Regeneration can only touch that spot
  and never disturbs the prose. This replaces string concatenation as the seam.

### Why the mechanics must be *carried*, not pointed at

Claude first advised "point at the declaration, don't restate it", by analogy with the code-comment and
knowledge-homes discipline. That advice **does not cross the binary boundary**: a base fact ships to a
project that has none of this repository's Go source, so the reader cannot reach the authority. Pointing
there produces a stranded entry — the unrepairable failure named by `20260810-121912-d-cpt-7zr`.
Generation is the only shape that carries the mechanics *and* keeps them in sync. The rule holds inside
this repo, where a reader can open the file.

## 5. The entry-construction model

### Shape, as settled with Christopher

One entry-construction type: frontmatter fields common to every entry, plus a set of **nil-able per-kind
field groups of which at most one may be set**, which must agree with the declared type and kind. Shared
sub-structs where kinds genuinely share a group, so `canonical` (actor, procedure) is not defined twice.
One construction boundary used by CLI writes, engine capture and base-entry assembly alike.

Chosen over an interface with per-kind implementations: one nil-able pointer per kind on a single struct,
so reflection over the struct's fields yields the set of kinds and a new kind cannot be silently forgotten.

### Construct freely, fail on validate

The tension named and resolved: **total-by-construction and represents-all-history cannot both be
properties of one type.** History contains shapes current validation rejects — the ~115 kindless signals
are the standing example, and a kindless signal has no kind block to convert into. Either the model
tolerates that (losing the can't-build-a-bad-one guarantee) or conversion fails (leaving two
representations again).

Settled: **the model is permissive in structure; validity is a checked property.** Zero-set is
representable, so translation never fails — a missing or mismatched block yields an absent block plus a
finding. One rule set, two consequences: on the write path findings block; on the read path the same
findings are the health data `GraphFinder` already reports.

### Relationship to the read model

Christopher's refinement, sharper than Claude's first framing: the domain model is a **projection** used
where domain logic runs, not the read model itself. The raw parsed form stays the storage representation;
translation to the domain model happens where needed and **may be lossy**, because nothing durable depends
on the projection round-tripping.

Read-side demands that make this mandatory:

- `GraphFinder` must load every parseable historical entry and report structural drift as health data,
  never blocking load (`zrh` committed this explicitly: no compatibility read mode, no new load blocker).
- View ranking, catch-up lanes, search indexing, filters and presenters treat entries uniformly.
  Polymorphism there would mean type switches everywhere.
- `show` is the one read consumer that genuinely wants the structured per-kind shape, and it switches per
  kind by nature.

Consequence accepted: base-entry safety comes from the validation call at render time rather than from
types — which is the loud build failure already wanted.

**Rejected:** building a read→write conversion for general use. Nothing needs it today; if a revise flow
later does, it is a lossy upgrade with explicit failure.

**Drift risk and its fix:** two homes for kind rules (write structs, read validators) would drift, so the
read side stops owning rules — its per-kind warnings are produced by running the write model's rules over
the parsed entry.

## 6. Schema extraction — reflection, codegen, or declarations

What consumes the per-kind schema: the generated mechanics block in each fact; the engine's JSON-Schema
generator; validation itself; possibly CLI flag surfaces.

- **Reflection over struct tags** — no build step, idiomatic types, compile-time safety. Cost: tags become
  a mini-language, and cross-field invariants (exactly-one-block-set; `index.topic` must appear in
  `topics`; actor and procedure pinned to the process layer) are real logic that gets ugly in tags.
- **Schema-first codegen** — one artifact, trivially rendered, reusable by non-Go consumers. Cost: this
  repo has *zero* codegen practice today; every field addition round-trips through the schema; generated
  code cannot be hand-edited.
- **Declarations carrying their own descriptions (chosen)** — copy what already works here twice:
  `engine.VarDecl` carries a `desc` that feeds both the generated report schema and the served instruction
  text (`internal/engine/spec.go:50-64`), and `viewlayout` pairs a live registry with a prose table plus a
  completeness test. Prose lives with the field; no reflection gymnastics; no new build step.

Reflection keeps one narrow role: deriving *which* kind blocks exist, as the completeness check.
Codegen is rejected **for now**, with the flip condition named: a non-Go consumer needing the schema
directly rather than over an API (revisit when the hosted webapp gets real, not before).

## 7. What is and is not generatable today

- **Generatable now.** Kind names and the kind→type mapping are iterable data
  (`signalKinds`/`decisionKinds`, `internal/model/entry.go:91-111`). Procedure mechanics are fully
  data-shaped: `engine.Spec`, `VarDecl`, `Step`, choosers (`internal/engine/spec.go:20-207`); report and
  answer schemas already generate from them (`internal/engine/schema.go:19-115`); and
  `publicProcedureSignature` (`application/application.go:522-542`) already renders a human-readable form.
- **Not generatable today.** Per-kind required/optional fields and structural invariants exist only as
  imperative control flow — `if !e.IsActor() { return }` followed by field checks — spread across
  `internal/model/graph.go:1178-1560`, mirrored in `internal/command/new_entry.go:120-175` and
  `application/write_api.go:37-66`, with per-kind meaning surviving only in struct comments. There is no
  kind→fields table anywhere. `validateKind` even re-spells the kind lists as prose string constants
  (`graph.go:1179-1180`).

Claude called that missing table "the validator table"; it is the same thing as Christopher's
entry-construction model. Building the model *is* extracting the table, which is why the original
sequencing question — ship `procedure` generated now or wait — dissolves: with the model in scope, every
kind's mechanics become generatable.

The requirements the model must express are enumerated by the active 7+8 contract
(`20260702-222259-d-cpt-7iy`): plan AC section; done `closes`/`refs`; actor `canonical` at process layer;
role `actor`; annotation `refs`+`topics` with member subsets; focus `involvement` with resolving targets
and `when` shape; directive `intent`; procedure `canonical`, process layer, `class`.

## 8. Duplication inventory

Kind knowledge is hand-copied in at least six places today, so this work removes a live drift problem
rather than a hypothetical one:

1. `internal/model/entry.go:69-111` — the enumeration itself (the source of truth).
2. `internal/model/graph.go:1179-1180` — kind lists re-spelled as prose string constants.
3. `internal/engine/schema.go:188-192` — all fifteen kind strings hard-coded, while the adjacent ref-kind
   case at `:154-158` correctly derives from `model.RefKindValues()`. Layer/confidence/intent enums are
   likewise literal (`:193-201`).
4. `internal/baseprocedures/entries/20260703-094500-d-prc-cap.md:129,133,143,147` — the distinguishing
   tests for 7 signal + 5 decision kinds, all 9 ref kinds, index rules, all 3 intents.
5. `internal/baseprocedures/entries/20260704-100000-d-prc-dlg.md:51` — both full kind lists and all five
   layers, in the shell serve.
6. `internal/bundledskills/templates/sdd/SKILL.md.tmpl:178` — the legacy skill's copy.

**Reconciliation, not deletion.** Christopher's requirement: nothing may be lost. Every copy is
inventoried, every unique claim it carries is either mapped into a fact or raised as a contradiction for
his ruling, and only then removed.

## 9. Discrimination must be reachable before the kind is chosen

The overview fact has to carry how the types and kinds play together and how to discriminate the right
one. By the time an agent is inside a kind lane, the choice is already made — so this knowledge cannot
live only in the lane. The shell's index line is a pointer and the capture procedure's first step must
create the pull, per the teaser contract (`20260730-110228-d-cpt-rh6`): a teaser must carry why and when
to reach for the fact and must never read as complete. Site 4 above is exactly this text, hand-carried
today — which makes the discrimination requirement and the de-duplication one piece of work.

## 10. Lifecycle — type-system facts are override-closed

Claude first framed generated facts as breaking immutability. That was **wrong**: generated facts
recompute on every graph load and are never stored, so there is no bytes-changing-under-a-fixed-ID
problem. The only real exposure was a **project override freezing a stale copy** of a generated block.

Christopher's position, adopted: type-system facts describing the framework foundation are **not
overridable at all**, because they are coupled to Go internals — validation, declarations — that a project
cannot override anyway. Narrowing arrives through the `rule` kind when it ships
(`20260622-084244-d-tac-eho`), which was conceived exactly as the extension point that makes constraints
*more narrow*, never opens base ones up.

- Claude's intermediate proposal — project conventions landing "additively on the same topic" — was
  **dropped as vague**. Addition of facts is not an important aspect now.
- This diverges from `20260707-140544-d-cpt-dtv`, which committed base facts as per-project overridable
  via a superseding fact. The plan records the divergence for code-coupled facts rather than claiming
  consistency.
- Procedure versioning with proper supersede chains for override conflict detection is a real and
  different need, and explicitly **not** part of this plan.

## 11. Slicing, and why `done` first

1. Entry-construction model and composition layer, plus the overview fact and the **`done`** authoring
   fact end to end, with the form reviewed by Christopher. `done` first because it has the most banked
   material already (`20260811-160518-s-cpt-igk`, `20260811-174502-s-cpt-ayb`, the one-delivery-event
   insight), it is semantics-heavy, and its discriminators have already been argued through — so
   consolidation there is fast and well-evidenced.
2. **`procedure`** early, because it is the mechanical extreme and forces the carry-don't-point rule and
   the generated-block seam before the form hardens across fourteen more kinds.
3. The consolidation sweep over the remaining kinds, one at a time with per-kind review.
4. Reconciliation of the six duplication sites.

The weakened form of the "measure before investing" argument survives as: do one kind end to end before
the sweep, to fix the form, so fifteen entries do not need re-editing after the form settles on the
fourteenth.

## 12. Why `zrh` is superseded rather than layered on

With the construction model, complete kind coverage and improved assembly in scope, this plan absorbs most
of `zrh` — its shared draft-construction boundary, per-kind requirements, per-kind diagnostics and lane
guidance. `zrh` has zero landed slices and its AC1 already contradicts the directive refining it. Leaving
it active alongside this plan and `x7a` would leave three overlapping commitments describing one body of
work. Superseding removes the duplicate, gives `x7a`'s pending intent a home, and leaves the checking
facts as a clean follow-on. Christopher's call: "This plan should then supersede the existing one to
prevent duplicates."

**Carried forward from `zrh`:** annotation capture; the multi-target supersede fix
(`20260731-083311-s-tac-4vx`, whose natural home `zrh` already named); kind-relevant-only diagnostics
(`20260722-141852-s-tac-dhn`); carried instruction units; the ideal-path table test; the live-head `show`
contract; presentation coverage across `show`, `view` and catch-up; and the freeze on legacy `/sdd` skill
prose.

**Dropped from `zrh`:** its one-fact-per-kind AC1, superseded in substance by the two-facts shape; and the
per-kind required-field duplication its wording tolerated across procedure expressions, CLI command
validation and the application draft mirror.

## 13. Explicitly out of scope

- The **checking** fact per kind (`x7a`'s second half) — lower value than the authoring facts, since
  checking material is calibration nuance over a stated semantics.
- Per-kind calibration sweeps (`20260811-141038-d-tac-rvu`), which `x7a` already re-sequenced behind the
  facts, plus the audience-test step `s-prc-6b0` adds to that method.
- Procedure entry versioning and override conflict detection.
- The `rule` kind itself.
- Bootstrap parity (`20260726-195300-d-tac-fim`), which waits on this line.

## 14. Completion check

One re-run of the specimen harness already banked in `20260811-190704-s-tac-r7h`, over the specimen already
banked, compared against the recorded baselines in §2. This is **not** calibration — it is a smoke test
that the form serves the target: an agent correcting itself and the user toward the framework.

For that re-run to be able to move at all, the writing guide must actually read something new — see §15.4.

## 15. Review round (independent code check and plan review)

### 15.1 The write surfaces diverge in behaviour, not only in code

Verified in source: the intent-on-directive and class-on-procedure rules exist **only** in the CLI
command's validation (`internal/command/new_entry.go:147-159` — intent required on directive, intent
invalid off directive, class invalid off procedure, class value set). The engine/application write path
builds the entry and calls `model.ValidateEntry(entry, snapshot.graph)` (`application/write_api.go:200`)
with no intent or class checks of its own.

So the two write surfaces do not enforce the same rules today. Unifying them at one construction boundary
closes a **live rule divergence**, which is a stronger justification than removing duplicated code — a
directive captured through the engine is not held to the requirement the CLI enforces.

### 15.2 Reconciliation sites — the engine schema generator

`internal/engine/schema.go:188-192` hard-codes all fifteen kind strings while the adjacent ref-kind case
(`:154-158`) correctly derives from `model.RefKindValues()`; layer, confidence and intent enums are literal
too (`:193-201`). Already listed as site 3 in §8 and confirmed as a reconciliation target: it is the
clearest case of a copy sitting beside a correct derivation, so the fix pattern is already present in the
same file.

### 15.3 Marking a fact as type-system must be a declared property

The override-closed rule (§10) needs the write gates to recognise *which* facts are type-system facts.
A hard-coded ID list is rejected: it would drift the moment a fact is added, and it repeats exactly the
mistake the derived fact index already fixed (`20260719-133529-d-cpt-qhn` derives enrollment from
metadata on the fact rather than from IDs in the shell). The marking must therefore be a declared property
resolved through ordinary graph resolution, in keeping with that contract.

### 15.4 The completion check needs the framework floor in scope

As first drafted, the plan deferred every checking-side change, and the specimen harness exercises the
**writing guide** — so nothing in scope would have changed what the guide reads, and the re-run could not
have moved against the baselines. The check would have measured nothing.

Resolved by pulling one small consumer into scope: the guide's prompt renders the **overview fact** as the
framework floor beneath every judgment, which is exactly what `20260811-184407-d-cpt-x7a` commits as the
first of its two prompt elements. The per-kind *checking* facts and the calibration sweeps stay out. This
also puts the plan back in step with the focus's committed order — facts, then consumers — rather than
stopping at the facts.

The alternative considered and rejected: restate the completion check as merely banking a before-baseline
for the checking follow-on. That would leave the drafting-side target checked only by per-fact user review,
with no instrument reading at all.

### 15.5 The per-kind lane pointer was dropped and is restored

The superseded plan's second acceptance criterion gave **every capture lane** a served pointer to its
kind's fact. The first draft of this plan carried only the overview pointer at the capture procedure's
first step, which loses the pull at the moment the drafter is actually composing that kind. Restored: the
overview teaser at the first step *and* a per-kind teaser in each assembly lane, both under the same
teaser discipline (`20260730-110228-d-cpt-rh6`).

### 15.6 Overview reach — index-and-teaser versus always-on priming

Open call recorded rather than settled. The overview fact is committed as indexed plus teased. The
alternative is enrolling it in the shell's opening serve **in full**, using the mechanism that already
ships: a shell-declared topic selector over active facts rendering selected bodies before the process core
(`20260810-134906-d-cpt-a2a`, delivered for the working-principles fact). Enrolling is cheap — a topic on
the fact and a selector on the shell, no new serve surface.

The constraint is that directive's own guard: a priming fact is paid for in context at **every** session
open — the dilution cost at its maximum — so the set stays small by discipline and "a fact that primes is
written tersest of all". Always-on is therefore conditional on the overview earning it by staying terse.
If it cannot, index-and-teaser stands. Either way the call is recorded, since it interacts with the
shell-index clause of `20260707-140544-d-cpt-dtv` the same way the principles fact already does.

### 15.7 Figures replaced by their general form

Two figures were removed from the plan body per the entry-craft criterion's rule that evidence enters in
general form: the guide's five/four/two finding counts became "unstable findings across repeated runs on
identical input" (the record stays in `20260811-183929-s-tac-fu8`), and "at least six places" became the
shape of the duplication — the write path, the served procedure bodies, the engine's schema surface — with
the site-by-site inventory kept here in §8.

Likewise, the per-kind structural requirements are now cited to the contract that enumerates them
(`20260702-222259-d-cpt-7iy`) rather than restated inline in an acceptance criterion, since an inline copy
would drift exactly the way the code copies did.
