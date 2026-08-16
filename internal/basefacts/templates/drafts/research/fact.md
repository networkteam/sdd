# Research brief — the fact signal kind

## A. Semantics
- A1: "Discovered knowledge that informs but doesn't demand action by itself. Facts are stable reference material." (d-cpt-omm two-type-design attachment; table: "Drives work? Informs decisions" vs gap "Yes — primary driver").
- A2: Its question: "What do we know?" (overview.go — generated; do not restate).
- A3: A signal — the operative pre-flight form: "does the entry prescribe a commitment (decision) or explore material for dialogue (signal)?"
- A4: Fact and gap are separate entries even when one produced the other: "A fact may reveal a gap … The fact is stable knowledge. The gap is the tension that demands resolution."
- A5: NOT on the open-attention surface (OpenAttentionKinds = gap, question; "facts and insights are stable observational records, retired via directive close, not resolved"). NUANCE: a fact still derives {status: open} — it is simply not an attention item. Two distinct things; do not fuse.
- A6: Lifecycle: supersession by a corrected fact; or directive close ("no longer true / no longer relevant") with retirement rationale stating WHAT CHANGED (closing_decision.tmpl:24: stable kinds "have nothing intrinsic to resolve — they're retired, not fulfilled").
- A7: Dissolution: a fact may close a question by answering it — "the only sanctioned signal-closes-signal path" (SignalCloseRule; unblocked deliberately in s-tac-4j7).
- A8: Consumption side: other entries cite a fact as premise or empirical proof via grounded-in ("a fact taken as premise or cited as empirical proof"; "grounded-in absorbs empirical citation"). Engage moves: reference in decision · retire · correct (supersede).
- A9: Reference-knowledge role: facts are the pull layer — "reference knowledge you look up and cite", non-overlapping in AUTHORITY with step instructions and session serve (d-cpt-dtv; d-cpt-rh6).

## B. Make-up (Go names)
- FactFields{Index *FactIndex; Override string} — the only per-kind block a fact may carry.
- Index-only-on-fact (three enforcement points; "index is only valid on kind: fact").
- FactIndex{Title, Topic}: both required; title trimmed/non-empty/no control chars; topic parses AND must appear in the entry's topics ("index.topic %q must also appear in topics").
- Enrollment semantics: presence of the index object enrolls the fact; only open/active facts are index members; design authority d-cpt-qhn: "title must stand alone as an agent retrieval cue — enough to understand when pulling the fact will help, not merely a terse name. topic selects exactly one primary grouping". Rejected alternatives: boolean flag; auto-enrolling every fact ("only some facts need bootstrapping exposure").
- Override/OverrideClosed: only "closed" defined; fact-only; supersession refused on every write path — "its content renders from the running version's declarations, so a superseding copy would freeze stale truth; narrow through project rules instead". Recognized by declared property, never an ID list.
- NO structural requirement otherwise — no refs required, no layer pinned. One of the least-constrained kinds mechanically, "which is exactly why its craft has to do the work".
- Mechanics render: SignalKindValues(), OpenAttentionKinds() (fact absent — renderable as not-an-attention-kind), SignalCloseRule, OverrideClosed. Index rules are inline strings — extraction needed if rendered. FLAG for implementer.

## C. Craft claims
1. An entry earns its place only by improving a later situation; the test is never well-formedness (d-cpt-7zr).
2. One thing · minimal · stands alone · exact words · shaped for the pull · meaning that stays true (d-cpt-7zr — point, don't restate).
3. "A figure belongs in the body when it is the claim itself" — for a fact this frequently resolves the OTHER way than for done/gap: the measurement often IS the claim (d-cpt-7zr; measurement-shaped fact examples in the founding design).
4. Residue discriminator: "The discriminator is work, not durability. Counts, durations, line numbers and sizes belong in a body only when they are part of the meaning" (s-cpt-ecl).
5. Evidence scales its home: small observation inline as dated evidence; a record (tables, sweeps, transcripts) in an attachment, body keeps the verdict.
6. A fact points at its source, it does not copy it — "'uses Go and Devbox — see the README for setup', not the full command list inlined. The docs stay the record; the graph points at them" (d-prc-bst; bootstrap skill "Facts point at docs, don't replace them"). Act-general: the source document stays the record; the fact points.
7. Captured facts reflect VERIFIED claims, not echoed phrasing; ungroundable claims are surfaced for dialogue rather than echoed as fact (d-prc-vlu).
8. Confidence proportionate to evidence; high requires strong evidence.
9. First sentence stands alone.
10. Every entry ID in the body is an edge; every edge visible in the narrative.
11. Borrow another entry's words only to convey the relationship, never its content.
12. Topics reuse existing vocabulary.
13. An index title conveys WHEN pulling the fact will help, not merely its name (d-cpt-qhn; teaser discipline d-cpt-rh6).
14. A fact written to be served at session open is "written tersest of all" — paid for in context at every open (d-cpt-a2a).
15. Content in another entry that "asserts what the world is actually like — a claim with its own pull, no delivery of its own" belongs in its own fact — facts get EXTRACTED from dones and dialogues (s-cpt-ayb).
16. A person's stable identity belongs in the actor signal's prose, not a fact entry.

## D. Reverse side
- D1: A fact states what is; prescribing what must be done by when by whom is a decision in signal's clothes.
- D2: Naming possible remedies as dialogue material stays a signal; committing to one makes it a decision.
- D3: Layer matches scope.
- D4: A dissolving fact's narrative makes the connection traceable — restating the question, its framing, or the unknown it resolves (dissolution.tmpl worked example: question "preferred shipping provider?" closed by "DHL ships for €4.50-5.80" with no provider-choice framing = finding).
- D5: A dissolving fact is judged on presence of dialogue context, never accuracy.
- D6: Retiring a fact by directive requires stating what changed.
- D7: closes-instead-of-supersedes needs its rationale in prose ("Not superseding — no updated measurement available; just retiring the stale value").
- D8: Artifact-durability does NOT apply to a fact — scoped to done.
- D10: Pre-flight declines prose-level type policing — the authoring fact is the ONLY place a wrong-kind fact gets caught.

## E. Discriminators (gap.md runs E1/E2 partly from the gap side — carry the fact side without verbatim reuse)
- E1 vs gap: a fact informs; a gap demands. Child care: "the group has 22 children registered for the autumn term" (fact) vs "the licence allows 20 and 22 are registered" (gap).
- E2 vs question: a fact asserts what is known; a question marks an unknown. Roastery: "the drum roaster holds 12 kg per batch" (fact) vs "what batch size should we standardise on?" (question). The fact is the kind that can retire the question by answering it.
- E3 vs insight — THE FACT'S OWN TEST (overview does NOT run it): checked against the world, or reasoned out of the record? Fact = discovered knowledge (research findings, measurements); insight = synthesized knowledge (patterns, connections). Timber: "C24 stock has been unavailable from both regional suppliers since March" (fact — checkable against the suppliers) vs "supply constraints, not design preference, have been driving our joint detailing all year" (insight — reasoned across the record).
- E4 vs done: a claim about the world vs a record of an act. Roastery: "re-labelled the 250g bags and reprinted the batch" (done) vs "the new label stock absorbs ink unevenly below 8°C" (fact — true whether or not anyone acted). Structural: a done requires an anchor and evidence; a fact requires neither.
- E5 vs decision: overview owns the type split.
- E6 vs actor: a participant's stable identity belongs in actor prose. "Jun trained in Oslo and holds the cupping expertise" → actor, not fact.

## F. Contradictions / rulings
- F1 Close matrix: the model warns on fact-closes-fact while pre-flight teaches how to write it well (unusual_close low example). RULING NEEDED before the fact states the close rule.
- F2 Kind default is legacy (d-cpt-1dk) — no default language.
- F3 WHAT EARNS A FACT is unsettled — open question s-cpt-sxn: "Where is the line that makes a fact capture useful … versus inflation of the graph with claims that don't carry their weight? … None of these is yet a settled line." Any "when a fact earns its place" paragraph is UNRULED — flag, do not blend.
- F4 Stable-kind closure practice contested one kind over (s-prc-4kh, insight census) — no fact census exists.
- F5 Which kind carries an evaluation finding (fact vs insight vs done) is open (s-prc-10m) — do not silently settle.
- F7 VOCABULARY COLLISION: "fact" the kind vs "fact" the English word ("done signals are terminal facts of execution", "proposal into fact"). The fact authoring fact must never use "fact" in the loose sense.
- F8 Living external sources have no re-sync story (open question s-cpt-jnr) — facts from sources that move (a price list, a regulation) go stale silently. Do not imply a mechanism.

## H. Index/override/base-fact recommendations
- H1: Index enrollment is generic, not framework-internal — carry it: craft prose for title-as-retrieval-cue (judgment), shape rules in mechanics. Non-software: a roastery's "how we run a cupping" reference fact enrolls so an agent finds it before guessing.
- H2: override: closed is framework-internal — mechanics-line at most; RULING: carry as one enumerated mechanics value or omit? The user-facing half worth keeping: "some shipped facts refuse supersession; narrow through project rules instead".
- H3: Base-fact population: generalizable — a fact is the home for reference knowledge you look up and cite; a project may override shipped process knowledge by superseding the fact. Framework-only (flag): embedded-in-binary, merged-on-load, no-participants. The priming craft claim generalizes: "a fact that primes is written tersest of all".
- H5 Unsourced (flag): provenance-required-on-fact-entries (no source; the gap's names-its-source is a GAP rule); fact-specific confidence reading; preferred layer; whether a superseding fact must state what changed.

## G. Overview coverage — point, don't restate
Signal/decision split; kind questions; competing-kind tests it runs; the retirement split incl. dissolution line; layers; the per-kind fact promise. NOT in overview (the fact fact's own ground): fact-vs-insight/gap/question/done, the index, the override property, provenance/verification/confidence craft.
