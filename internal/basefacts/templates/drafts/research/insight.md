# Research brief — the insight signal kind

## A. Semantics

**What an insight is.** The one question it answers is **"What have we synthesized?"** — declared once in `internal/basefacts/overview.go:33` (`model.KindInsight: "What have we synthesized?"`), repeated in `internal/bundledskills/templates/sdd/references/framework-concepts.md.tmpl:24`.

Type: a **signal** — something noticed, not committed to (overview.md; `d-cpt-7iy` enumerates `insight` among the 7 signal kinds).

Framework-level characterizations, each with its source:
- **"observational record, not an actionable gap"** — `20260420-163845-s-tac-2bw` ("Insight signals appear in Open Signals … but carry no closure gate — they're observational records, not actionable gaps").
- **"stable observational record"** / **"stable-kind signal"** — `internal/model/graph.go:186-190` (OpenSignals doc), `framework-concepts.md.tmpl:148`, `playbook-groom.md.tmpl:21`, `closing_decision.tmpl:4,11,24`.
- **"insights are stable observations … not groom candidates"** — `sdd-groom/SKILL.md.tmpl:28`.
- **"connects what is already recorded without leaving anything owed"** — the insight-side test as stated *from the gap fact*, `internal/basefacts/templates/gap.md` ("Choosing gap at all").
- **dialogue material that may explore solution space without committing** — `signal_capture.tmpl:8`. Live specimen: `20260608-232836-s-cpt-smn` is an insight carrying *a candidate design*.

**Lifecycle — nothing owed after reading.**
- Not an attention kind: `model.openAttentionKinds = []Kind{KindGap, KindQuestion}` (`graph.go:194`), so an open insight never appears in `Graph.OpenSignals()`. It gets its own stream `Graph.RecentInsights(n)` (`graph.go:242`): *"Insights have no closure gate (they're retired via directive-close, not resolved), so they surface as their own stream rather than mixing into the actionable Open Signals view."*
- **It can close something**: an insight (or fact) may **dissolve a question** — `model.validateCloses` carve-out (`graph.go:1150-1155`), `model.SignalCloseRule` = *"only done-kind signals may close entries, or a fact/insight dissolving a question"*. Shipped by `20260617-170241-s-tac-4j7` closing gap `20260617-003401-s-tac-7nz`.
- **Retirement (documented path)**: supersede with a corrected insight, or a **directive closes it with rationale** — `framework-concepts.md.tmpl:137` (`| insight | corrected insight | directive: "noted, no action needed" |`), `playbook-groom.md.tmpl:21`, `closing_decision.tmpl:11,24` (stable kinds *"have nothing intrinsic to resolve — they're retired, not fulfilled"*).
- **Retirement (actual practice) is contested** — see F/C1: `20260608-004727-s-prc-4kh` (still **open**) records five of nine closed insights closed by *done* signals.
- **An insight can still be acted on without being "resolved"**: `addresses` is applicable to an open insight — `ref_applicability.go:87`, `ref-kinds.md.tmpl:12`, capture procedure body line 130. So "nothing owed" is about the *attention surface and closure gate*, not about no follow-up ever existing.

## B. Make-up — fields and enforced rules (Go names)

- Kind constant: `model.KindInsight` (`entry.go:74`); member of `model.signalKindOrder` / `model.SignalKindValues()`.
- **No per-kind block** in `model.EntryConstruction`. Insight uses only common fields.
- **No structural requirement of its own**: no analogue to `DoneAnchorRequirement`, `PlanAcceptanceHeading`, `DirectiveIntentRequirement`. An insight needs no ref, no section, no intent.
- **Cannot be index-enrolled**: `index`/`override` live on `FactFields` only.
- Enforced lifecycle rules touching insight: `validateCloses` fact/insight → question dissolution carve-out; every other non-done signal close rejected with `model.SignalCloseRule`. `model.SettledCloseRule`: a settled directive can never be closed.
- Read-side: `RefKindGroundedIn`/`TargetLiveSignal` note = *"a fact taken as premise or an insight reasoned from"*; `RefKindAddresses`/`TargetLiveSignal` = *"responding to an open gap, question, or insight"*.
- Pre-flight dispatch: `preflight.go:225-229` routes `KindFact || KindInsight` closing a question to `checkDissolution`.
- Wiring for the new fact: `InsightFactID` const, embedded `templates/insight.md`, mechanics renderer modelled on `gapMechanics()` (renders `SignalKindValues()`, `OpenAttentionKinds()` — **insight is absent from it: the mechanical anchor for "no closure gate"** — and `SignalCloseRule`), map entry in `authoringFactIDs`, `related` ref in overview.md.

## C. Craft claims (one line each, with source)

1. **An insight reasons over material the graph already holds; its refs are load-bearing, not decoration.** — `d-cpt-7zr`; specimens `s-cpt-h5c`, `s-cpt-86s`, `s-cpt-igk`, `s-cpt-qyo`, `s-cpt-sne`.
2. **The connection is the substance** — what the entry adds is the relation among the referenced entries; restating their content is duplication debt. — `d-cpt-7zr`; `SKILL.md.tmpl:182` (body↔edge consistency).
3. **An insight that corrects or bounds a prior claim names exactly which part it touches and which stands.** — `20260816-165718-s-cpt-sne`; `20260811-175406-s-cpt-b6g`.
4. **An insight may carry a candidate direction or weighed alternatives; it stops at commitment.** — `signal_capture.tmpl:8,17`; specimens `s-cpt-smn`, `s-cpt-fc0`.
5. **Warrant to capture at all: it must change what someone does next.** — `d-cpt-7zr` Purpose.
6. **An insight may be deliberately banked — recorded now, deliberately not written into guidance or acted on.** — `s-cpt-b6g`; `s-cpt-qyo`.
7. **Confidence is proportionate to the evidence behind the synthesis, not to the author's conviction.** — `signal_capture.tmpl:10`; medium specimens `s-cpt-uq8`, `s-cpt-smn`, `s-cpt-86s`, `s-cpt-j2w`; high specimens `s-cpt-qyo`, `s-cpt-b6g`, `s-cpt-sne`.
8. **Layer names the depth of the synthesis, not its importance.** — overview layer list; `signal_capture.tmpl:9`; specimen spread `s-stg-ob9` / `s-cpt-*` / `s-prc-fc0`.
9. **One thing, minimal, standing alone, exact words, shaped for the pull.** — `d-cpt-7zr` (cross-kind; point, don't restate).
10. **Where a synthesis fuses two things, split and cross-reference.** — `d-cpt-7zr` conflation axis; live example: symptom-gap `s-prc-l75` + cause-insight `s-cpt-j2w` as separate entries.
11. **When the work that produced the insight is itself recorded, the relation is `surfaces`/`surfaced-by`, not `addresses`.** — `ref_applicability.go`; specimens `s-prc-hx9 → s-cpt-j2w`, `s-tac-k06 → s-cpt-sne`.
12. **The first sentence must stand alone** — insight overview surfaces render dated first-sentence lists.

## D. Reverse-side claims from rejections, stated positively

- **An insight that dissolves a question names the question in its own narrative** — restate the framing or say which unknown it settles. Source: `dissolution.tmpl:8,13-15`.
- **The dissolution test is presence of dialogue-captured context, not correctness.** Source: `dissolution.tmpl:4,9`.
- **An insight closes a question and nothing else** — any other close target is mechanically rejected or requires explicit rationale. Source: `unusual_close.tmpl:9,13,17,20`; `validateCloses`.
- **An insight states understanding; the moment it prescribes a commitment with owner and horizon it is a decision.** Source: `signal_capture.tmpl:8,16-18`.
- **Retiring an insight requires saying what changed in the world or the thinking** — it has nothing intrinsic to resolve, so it is retired, not fulfilled. Source: `closing_decision.tmpl:11,24`; `framework-concepts.md.tmpl:148`.
- **A corrected understanding replaces its predecessor by supersession, not by editing or by a fresh unrelated entry.** Source: retirement table; overview; specimen `20260609-001150-s-cpt-ov7`.
- **Layer must match the synthesis's scope; confidence must match its evidence.** Source: `signal_capture.tmpl:9,10`.

## E. Discriminators (act-general + non-software instantiation)

| # | Discriminator | Non-software instantiation | In overview? |
|---|---|---|---|
| E1 | **vs fact — one observation vs a relation among several.** A fact reports a state checkable by looking at one thing; an insight asserts a connection across things already observed/recorded — the connection is what the entry adds. Test: strip the connection — if a standalone observation survives intact, it was a fact. | Roastery: fact = "the January Guji lot cups 86 and costs €7.20/kg"; insight = "every lot we scored above 85 this year came through a direct relationship — the sourcing route, not the origin, tracks cup score." | **No.** |
| E2 | **vs gap — nothing owed vs action demanded.** A gap sets actual against expected and stays on the attention surface; an insight leaves the attention surface untouched. | Timber: gap = "spec calls for C24, delivered stock is C16"; insight = "every rejected delivery came from the merchant with the fastest quoted turnaround." | Partly, from the gap side (gap.md). Duplication risk — state the insight-side test without re-teaching the gap. |
| E3 | **vs question — knowledge reached vs unknown named.** A question is resolvable by new knowledge; an insight *is* the knowledge — and may dissolve a question (the only close it may make). | Child care: question = "why do afternoon handovers run long?"; insight = "the long handovers are exactly those where a parent arrives before tidy-up ends — the overlap, not the conversation, is the cost." | Partly (retirement line only). |
| E4 | **vs directive — understood vs committed.** Test: does anything become expected of anyone because this entry exists? | Roastery: insight = "the two profiles that held through summer both dropped charge temperature"; directive = "we roast summer lots at the lower charge temperature." | Yes at type level — point. |
| E5 | **vs done — act performed vs understanding reached.** A done records the world changed; an insight records the reading changed. The same session commonly produces both, as two entries. | Child care: done = "held the handover-timing review"; insight = what the review revealed. | Partly; note open question C4. |

## F. Contradictions (verbatim) — NEEDS USER RULING

**C1 — how an insight retires (three-way, open).** framework-concepts: directive-close ("noted, no action needed") or corrected-insight supersede. Practice per `20260608-004727-s-prc-4kh` (open): five of nine closed insights were closed by done signals. Rule, validator, behavior disagree. → The fact cannot state a retirement path without a ruling.

**C2 — "nothing owed" vs "still addressable".** No closure gate + off the attention surface, yet `addresses` explicitly applies to open insights. Reconcilable as "no closure gate ≠ never acted on" but that reconciliation is recorded nowhere. → State only if ruled.

**C3 — the confusion history (resolved).** `20260420-163845-s-tac-2bw` (insights wrongly in Open Signals) closed by `20260420-221159-s-tac-izi` (allow-list separation, survives as `openAttentionKinds`). Citable as why the kind sits off the attention surface.

**C4 — which kind carries an evaluation finding (open, `s-prc-10m` attachment).** fact vs insight vs done for evaluation findings is unruled. → Don't resolve by fiat.

**C5 — pre-flight declines kind policing** (`signal_capture.tmpl:21`) → the pre-flight corpus holds almost no insight-vs-neighbor knowledge; positive side comes from specimens and neighbor facts.

**Unsourced-but-needed:** there is no banked insight-kind discriminator entry (unlike done and gap). Claims C3–C12 beyond items 1–2, 4–7 are generalized from specimens, not a recorded ruling.

## G. What overview.md already covers — point, don't restate

Signal/decision split ("the first test, and the strongest"); the loop and immutability; kind-picked-by-question incl. "insight — What have we synthesized?"; the competing-kind tests it runs (aspiration/directive, plan/activity, intent, standing constraints, actor/role, done, annotation — NO insight test); the retirement split incl. "a fact or insight may close a question by answering it" and "same-kind supersession replaces"; the layer table; the per-kind fact pointer. Also owned elsewhere: entry-craft criterion (`d-cpt-7zr`), gap.md's insight clause, kind-default ruling (`d-cpt-1dk`).
