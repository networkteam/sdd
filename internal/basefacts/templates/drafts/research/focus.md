# Research brief — the focus decision kind

## A. Semantics
- A1: Its question: "What are we attending to in this period, and who is engaged?" (overview.go — generated; do not restate).
- A2: An involvement declaration — "a decision that names what the project is advancing right now and who carries it" (d-prc-cap:161; focus_capture.tmpl: "an involvement declaration with dual lifecycle").
- A3: Exists to make the implicit roadmap graph-visible — current focus, involvement, time-boundedness previously "lived in heads" (s-cpt-8tu).
- A4: NOT an issue tracker: assignment and dates live in the focus entry, never on the target entry. Targets stay clean; the supersession chain of foci encodes evolution. Multi-actor default; single-owner assignment explicitly not adopted ("reality is angled rather than owned") (s-cpt-8tu).
- A5: DUAL LIFECYCLE — the defining property and why focus is a kind (not an activity): activity semantics require done-closure, but a focus is replaced when priorities shift (s-cpt-ke6; contract d-cpt-7iy: "focus dual lifecycle (supersede mid-cycle or close by done)").
- A6: Path 1 — supersession by a replacement focus when priorities shift mid-cycle (live: d-tac-hza supersedes d-tac-wqb "whose finish line was overtaken").
- A7: Path 2 — closing done when the cycle ended naturally (live: d-tac-0qn closed-by s-tac-3xk).
- A8: Path 3 — a directive retiring it.
- A9: Both paths valid; NO check argues which applies — deliberately (focus_capture.tmpl non-enforcement; the supersede-vs-done choice is where a checker oscillates).
- A10: PARALLEL FOCI ARE LEGITIMATE — ActiveFocuses returns a list; two eras of explicitly parallel foci in the live graph (d-tac-ha9 ∥ d-tac-mdt; d-tac-cxd ∥ d-tac-hza, recorded as related, not supersedes).
- A11: A new focus that does NOT replace an existing one says so and records the neighbor as related — otherwise the reader assumes displacement (d-tac-cxd ref desc).
- A12: Derived status recognises only closed and superseded. An elapsed when.to does NOT end a focus — CODE-DERIVED ONLY, no prose source. FLAG for ruling before stating.

## B. Make-up (Go names)
- Entry fields: FocusActors []string, FocusWhen *FocusWhen, Involvement []Involvement. Construction: FocusFields{Actors, When, Involvement}.
- Involvement triple: Target (required, full entry ID, must resolve), Actors (canonical-only, per-involvement override), ActorsSet (distinguishes unset from explicit empty), When (per-involvement override). FocusWhen{From, To} ISO dates, at least one required when present.
- B1: Must declare at least one involvement triple; each target must resolve — "focus decision must declare at least one involvement triple"; "involvement[%d].target: %s does not resolve" (validateFocus).
- B3/B4: Focus-level actors:/when: are defaults triples inherit unless they carry their own.
- B5: Resolution: per-involvement if set (incl. explicit empty), else focus-level, else nil (Entry.ResolveActors/ResolveWhen).
- B6: actors: [] explicit ≠ omitted — explicit empty = deliberately unattributed / PULL-AVAILABLE (in scope, awaiting pickup); omitted = inherits default (ActorsSet).
- B8: Actor names canonical-only, must match active actor canonicals — high finding focus-actor-drift.
- B9: Involvement targets create NO ref edge (RefsTo built from Refs only) — refs and involvement are separate channels. See F2.
- B10: Three resolution states deliberately smoke-tested (d-tac-0qn: bare / actors: [] / own actors + narrower when).
- Live shapes: from-only when is the norm; mdt has no when at all ("as time permits" in prose); only the bounded two-week push carries a to. 2-4 targets each. All 8 live foci are tactical. Target kinds seen: plan, directive, a signal.

## C. Craft claims
1. The structure carries who/what/when; the BODY carries why now (d-prc-cap:166).
2. The body carries the narrative of the period: what it is about, why these entries were selected, what is in scope (design attachment).
3. State the sequencing/dependency shape among targets — what ships first, what unblocks what, what runs parallel (all live specimens do).
4. State what would close it — the completion condition in the project's own terms (ha9: "Closes when both targets reach their done signals… The goal is functional: … not just present").
5. Name what was deliberately left out and why, so omission doesn't read as oversight (mdt; gk7).
6. A focus is gated on involvement, not refs or topics — refs/topics optional (d-prc-cap:161).
7. In practice refs carry grounding/evidence, involvement carries the work — disjoint sets in every specimen.
8. Target picking: targets are the entries actually being driven. Corpus range 2-4 — OBSERVATION ONLY, no rule (flag).
9. Pull-available is a first-class statement: declares a target in scope and open for pickup — use it rather than dropping the target or guessing an owner.
10. Period honesty (dates): external coordination constraint → capture; internal manufactured cadence → don't (s-cpt-8tu — PROPOSAL-side only, never ruled; FLAG).
11. Period honesty (practice): from-only when = honest "started now, no fixed end"; omit when entirely when pace is undetermined, saying so in prose.
12. A per-triple when is for a target with a genuinely different window, not restating the focus window.
13. Actors default up when the same across targets; named per triple only where they differ.
14. A focus that supersedes another says what changed — which finish line was overtaken and why (hza; mdt).
15. Retirement carries rationale — cross-kind, point at the directive fact's rule.

## D. Reverse side (focus_capture.tmpl — low-severity only, never blocking)
- D1: Closing with no target completions is legitimate — a cycle can end without completing its targets; the closing record says so.
- D2: A replacement sharing zero targets = wholesale priority shift — name it in the body so it reads as a shift, not an oversight.
- D3: All-pull-available is either deliberately open-for-pickup or incomplete — if deliberate, say which.
- D4/D5: Nothing judges closure-rationale completeness, supersession scope, commitment sufficiency, or layer — focus is fluid by nature.
- D6: Mechanical layer owns missing involvement, dangling targets, malformed when, non-canonical actors.

## E. Discriminators
- vs directive: directive.md carries it from its side ("Does the commitment hold only for the present period, naming who attends to what? A focus."). Complement, don't mirror. Live counter-specimen: d-stg-ous "the strategic focus is adopting SDD…" is a GUIDING DIRECTIVE because it is standing direction with no fixed completion — standing direction over an area ⇒ directive; bounded present attention with named carriers ⇒ focus.
- vs plan: a plan enumerates what must be true when done; a focus has no ACs — it names which existing commitments are being driven now. A focus POINTS AT plans; it does not contain their outcomes.
- vs activity: activity is work to be completed (done-closure); a focus is "a standing declaration of current focus that gets replaced when priorities change" — exactly why the kind was minted (design attachment, s-cpt-ke6).
- vs role: bootstrap's "is that your usual focus, or just this week's work?" — standing contribution ⇒ role; current attention ⇒ focus.
- Non-software instantiation: "Through November the shop is attending to the barn-frame commission: Mara is driving the joinery layout, the timber-sourcing decision is open for whoever picks it up, and the client walkthrough runs from the 20th." — the plan for the frame is a TARGET, not this entry; the standing pegged-joints rule is a guiding directive that keeps holding after November.

## F. Contradictions / rulings
- F1 LAYER: framework-concepts says layer-flexible, use the layer matching the cadence; capture procedure says "typically tactical"; corpus 8/8 tactical. RULING NEEDED on which line the fact carries.
- F2 Involvement targets in refs: proposal said refs include all targets; implementation did not — a focus is invisible in its targets' downstream unless a ref is added separately. Consequence unstated anywhere. RULING NEEDED (should authors double-list?).
- F4 when.to elapsed: code-derived only — an expired focus stays active. FLAG before stating.
- F5 Date-capture discriminator (C10) exists only in the closed proposal. FLAG: adopt or drop.
- F6 Target count: observation only. FLAG.
- Incidental (not fact material): sdd show does not render actors/when/involvement on a focus — separate capture candidate.

## G. Overview coverage — point, don't restate
Signal/decision split; the kind question verbatim (generated); cross-kind tests it runs; retirement split (the fact adds only the DUAL lifecycle specifics and that no check arbitrates); layer list (only the cadence question, subject to F1); directive.md's focus test from the directive side — complement it.
