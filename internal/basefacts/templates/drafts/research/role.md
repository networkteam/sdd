# Research brief — the role decision kind

CHARTER: shares the actor/role failure record (20260722-141659-s-cpt-rza; 20260722-140955-s-tac-lgc): "roles not referencing the active actor head" was a consistent live miss. A parallel fact covers actor; this one carries the role side and the boundary.

## A. Semantics
- A role commits "actor X participates as Y" — binds one named participant to a contribution shape on this project (d-cpt-9dm verbatim).
- Its question: "How does an actor participate here?" (overview.go — generated; do not restate).
- Role records what a participant DOES here; actor records WHO they are (framework-concepts; s-prc-b6p).
- Role scopes ONE actor; universal rules are not roles: "roles constrain actor-scoped participation (who brings what, who does what), contracts commit to universal invariants" (d-cpt-9dm; role_capture.tmpl). Translate contract→guiding directive per overview.
- Specialization and preferred contribution shape, not raw capability (d-cpt-9dm; software specimen Ben/Jana — restate act-generally).
- Multiple concurrent roles per actor are permitted (d-cpt-9dm).
- A STANDING commitment, not this period's work: "Keep it to their stable part here, not this week's task" (d-prc-cap:158).
- Layer pinned process (d-cpt-9dm; d-cpt-7iy; construction.go finding "role decision should live at process layer").

Lifecycle — four exits:
1. Supersede — participation pattern shifts: scope expands/narrows, skills evolve, responsibilities change (d-cpt-9dm). Wins over cascade in derivation.
2. Cascade-retire — closing the bound actor chain's head derives-closed every role bound to any canonical in the chain's history; "no separate role-retirement entries needed" (framework-concepts; deriveRoleStatus → StatusCascadeClosedBy). THE distinguishing lifecycle property: status derived from another entry's chain.
3. Close by directive — permitted but "unusual because the actor cascade is a derivation rather than a prohibition" (d-cpt-enb verbatim).
4. Orphan (abnormal): canonical matching no chain → StatusCascadeOrphan, lint-flagged; "should never fire in normal operation".

Why cascade-by-canonical-history (arguable reasoning, from d-cpt-d34 plan attachment): simple canonical-match would wrongly retire roles on a typo correction "because the person didn't leave"; ref-chain-walk would make refs load-bearing for status; forbidding canonical mutation "too sharp". CONSEQUENCE: a role's actor value stays valid after the actor's name is corrected — never rewrite a role to track an identity fix.

## B. Make-up (Go names)
- RoleFields{Actor string} — "names the canonical of the actor-identity chain the role binds to" (construction.go:63). Frontmatter key `actor`.
- Missing/blank → "role decision missing required actor field" (blocking + read warning).
- Wrong layer → "role decision should live at process layer (got %s)".
- Binding is to the CANONICAL, never an entry ID — "stays stable across actor supersessions" (d-cpt-9dm; d-cpt-7iy).
- Resolution: Graph.ResolveRoleChain → ChainForCanonical over full CanonicalHistory.
- Capture-time stricter than derivation-time: role-canonical-mismatch (must match current head canonical of an ACTIVE chain) + role-refs-missing-head (refs must include the head entry's ID), both high/blocking, no grace mode. Engine gate looser: roleActorResolves ("capture the actor first, then bind the role to its canonical").
- No declared rule constant exists for role — inline strings; mechanics rendering needs extraction (implementation note).

## C. Craft claims
1. Capture the actor first, then the role — "It must already resolve to a known actor … the role binds to its canonical, never mints a new name" (d-prc-cap:157).
2. State the standing contribution, not the current task — "authority, domain weight, authorship, the decisions they hold" (d-prc-cap:158).
3. Name the scope of authority and what is delegated — live exemplars: "holding strategic and conceptual ownership while delegating tactical and operational work" (d-prc-hfs); "strategic and conceptual direction flows through Christopher" (d-prc-j28).
4. Name decision rights explicitly (hfs: "authoring and approving strategic and conceptual direction … reviewing all proposed graph entries").
5. Make the shape routable — "clearly enough that another participant can reason about when to route work or dialogue to this actor" (role_capture.tmpl §1).
6. Introduce the bound actor canonical in the prose — "so the role reads coherently without the frontmatter visible" (§3; both exemplars open on the canonical).
7. Specialization over capability (d-cpt-9dm).
8. A single-period pattern is not yet a role — "the trap is capturing 'did feature X this week' as a role"; ask "is that your usual focus, or just this week's work?" (bootstrap skill:133; d-prc-bst:200).
9. Split mixed answers at draft time — two entries (actor + role); "deferring role is explicit, never implicit; don't silently drop volunteered role content" (bootstrap:255-265).
10. No structured taxonomy — no reviewer/capturer vocabulary; minimum surface (d-cpt-9dm).
11. Don't cite graph-mechanics rules in role prose (SKILL:183; s-prc-b6p).
12. A role has no natural topics; the gate does not ask (d-prc-cap:155). Refs contested — F1.

## D. Reverse side (role_capture.tmpl)
- (high) A role names ONE participant's shape; team-wide discipline prose is a standing constraint (guiding directive), not a role.
- (medium) Enough shape to route against — "Christopher contributes to the project" is a role in form only.
- (low) Reads coherently alone: canonical in prose.
- (no-finding) Contribution scope clear, actor linkage named, per-actor scope.
- Structural matters (canonical mismatch, head ref, orphan, layer) are mechanical, not prose judgments.

## E. Discriminators
- vs actor: OVERVIEW OWNS "outside vs here" + single-period line — point. The fact ADDS: binding direction/dependency order (actor exists first); derived-status consequence (retiring the person retires the roles — no own closing entry); several roles at once; the mixed-answer two-entry drafting move.
- vs a directive about ways of working: a role scopes one named participant; a guiding directive binds everyone and anyone's violation is observable. Test: if the sentence stays true with a different name in it, it is a guideline, not a role (translated from role-vs-contract sources per overview's contract closure).
- vs focus: focus is period-bounded attention with involvement; role is the standing part with no period.
- Non-software instantiations (composer-supplied, flagged): Roastery — actor "Ana, twenty years green-coffee buying, trained cupper (Trieste)"; role "Ana holds sourcing and the roast profiles: selects and approves every green lot, sets profile changes, purchases over the standing limit go through her; day-to-day roasting runs without her." Timber — "Jonas carries layout and joinery decisions on site; cut-list changes are his call, material substitutions come to him before order." Child care — role "Marta holds the settling-in period for new children, first contact for the family's first six weeks" vs focus "Marta covers the toddler group this month".

## F. Contradictions / rulings needed
- F1 REFS: capture procedure's role lane says "No natural refs or topics"; pre-flight REQUIRES refs to include the active actor head's entry ID (high, blocking); d-cpt-9dm + framework-concepts state the head-ref requirement; the bootstrap failures name the miss. RULING NEEDED: is the head ref part of role make-up (fact states it), or should the pre-flight check go?
- F2 Two write surfaces enforce different strictness (engine: chain exists; CLI: active head + head ref) — the known divergence d-tac-9be closes; don't encode as semantics.
- F3 Expertise boundary: d-cpt-9dm (superseded in substance) put domain expertise in role; actor rubric puts it in actor. Later resolution direction (user-attested): background expertise = actor; expertise as the weight carried here = role. State explicitly — it is the recorded confusion.
- F4 Open validator bug (s-tac-tom) — don't encode current-code behavior as settled semantics.
- F5 "Default confidence medium" (d-cpt-9dm, framework-concepts) — not enforced; same shape as CLI-legacy default (d-cpt-1dk). FLAG. The REASONING (role is an evolving commitment; actor an observational fact) is genuine semantics and can be kept without the default.
- F6 Role transfer: UNSOURCED — mechanically a transfer must be a new role bound to the new actor plus retirement of the old, but nothing states it. FLAG.

## G. Overview coverage — point, don't restate
Signal/decision split; kind question; actor-vs-role in full; retirement split (the fact adds the cascade as a third path + directive-close-unusual note); contract-takes-no-new-entries (use, don't restate); layer list (state only the process pin and why); the per-kind fact pointer.
