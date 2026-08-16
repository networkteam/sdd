# Research brief — the actor signal kind

CHARTER: actor/role capture is the recorded sharpest failure point (20260722-141659-s-cpt-rza; outer evaluation 20260722-140955-s-tac-lgc). The consistent misses, verbatim: "the canonical drafted from a full personal name instead of matching the configured participant, aliases not collected as part of the actor cluster, the canonical not introduced in the entry prose (which a quality check then flags), the actor drafted at the wrong layer, and roles not referencing the active actor head — each forcing a supersede or restart". This fact exists to prevent exactly these.

## A. Semantics
- A1: An actor records WHO a participant is — a first-class participant identity entry (actor_capture.tmpl; entry.go IsActor).
- A2: Its question: "Who is participating?" (overview.go — generated; do not restate).
- A3: Observational, fact-shaped — "this person exists as a participant, canonical C, aliases […]" (s-cpt-6ct; d-cpt-cye).
- A4: Participants are DECLARED, not inferred (s-cpt-ue6).
- A5: Unifies human and non-human participants — "Christopher and Claude are both actors" (s-cpt-ue6; live: s-prc-hav human, s-prc-s1o AI model family).
- A6: For a non-individual, prefer the STABLE identity over the momentary instance — a model family over a session, a team over its roster (actor_capture.tmpl "Identity scope"; ratified s-prc-sn7).
- A7: Lifecycle — identity evolves by superseding within the same chain: "superseded when any structurally-indexed identity fact shifts (canonical name change, alias added or removed, spelling correction)" (d-cpt-cye; s-cpt-6ct; both live exemplars carry supersedes).
- A8: Retirement — closure by a DIRECTIVE with rationale, not a done: "closed when the actor is no longer relevant to the graph (left the project, will not participate further)" (d-cpt-cye; playbook-engage:44).
- A9: Closing the chain head transitively derives-closed every role bound to any canonical in the chain's history — no separate role retirements (framework-concepts "Role-status cascade"; status.go deriveRoleStatus; d-cpt-7iy).
- A10: Actors carry the attribution surface: every entry's participants field lists canonical names; a person without an actor entry is not a participant the graph recognizes (d-cpt-7iy; s-cpt-jdy).
- A11: Grace mode — a graph with zero active actors captures freely; the first actor turns participant enforcement on (preflight_mechanical.go; SKILL:230). MECHANICS-side.
- A12: An actor entry can't be self-authored by the newcomer — an existing participant vouches them in (s-cpt-jdy). FLAG: current-mechanics consequence, likely surface not semantics — ruling wanted.

## B. Make-up (Go names)
- canonical: REQUIRED — "actor signal missing required canonical field" (construction.go:348; ActorFields{Canonical, Aliases}).
- aliases: optional, read-side only — "never used at capture time", never a second canonical.
- layer: pinned process — "actor signal should live at process layer (got %s)" (construction.go:351). F7: stated as pin in d-cpt-cye/contract, soft finding in code — state as a pin.
- Body prose carries mutable external context (job title, organization, focus areas) — "changes too often to warrant supersede ceremony, not queryable structurally" (d-cpt-cye).
- Alias hygiene enforced: empty alias, alias duplicating canonical, duplicate alias — findings (construction.go:354-366). Cross-actor shared alias → informational ambiguity flag (validateAliasAmbiguity).
- Chains: ActorChain{Entries, Head, CanonicalHistory}; ActiveActorHeads; head = not superseded; active = head not closed (actor.go).
- Write-once canonical ACROSS chains: "once used by any chain, it cannot appear in any other chain, even after the original chain is closed. Within a single supersession chain, canonicals can change across entries or repeat freely." Rationale: temporal stability of historical participant references (framework-concepts; validateActorInvariant; capture-time actor-canonical-reused with same-chain exemption).
- Participants relationship: entries reference participants by plain canonical name; capture-time matches ACTIVE canonicals; lint matches ANY chain history. Do NOT auto-ref the actor entry for routine participation — "participants already carries the identity reference" (d-cpt-979; SKILL:228).
- Role binding (far side): role.actor = the canonical, never the entry ID; capture requires matching the current head canonical of an active chain AND refs including that head's ID.

## C. Craft claims
1. The canonical is "the stable name this person is known by across the graph … Pick the form they'd recognize as theirs" (d-prc-cap:150).
2. Reuse a matching canonical rather than reconfirming known identity — resolve active actor heads and aliases first (d-tac-fim).
3. Recorded failure: canonical drafted from a full personal name instead of matching the name the project already uses to attribute work (s-cpt-rza). Act-general: match the attribution name in use; don't invent a more formal variant.
4. The canonical is the stable handle across supersessions — everything binds to it, not the entry ID (d-cpt-cye, d-cpt-9dm).
5. Introduce the canonical in the prose, one sentence is enough: "Christopher, canonical `Christopher` in graph participants, is CEO of networkteam with…" (actor_capture.tmpl point 1).
6. Prose covers stable identity: affiliation, background, domain expertise — "who they are independent of the work here" (s-prc-b6p; d-prc-cap:152).
7. Legible to future readers without external context (actor_capture.tmpl).
8. Missing external context is acceptable; conflating this-graph role scope into actor prose is the finding (actor_capture.tmpl point 2).
9. Terse-to-uninformative prose ("actor: Christopher") is a real defect (calibration low).
10. Aliases are the other names they appear under — the name a person signs work with, goes by in conversation, uses on external correspondence. List each once (d-prc-cap:151, act-generalized from git/chat).
11. When aliases are present, the prose says WHERE each variant appears, so readers resolve them (actor_capture.tmpl point 3; all three live exemplars do this).
12. Collect aliases as part of the same identity capture — not collecting them was a named live failure (s-cpt-rza; bootstrap skill).
13. Supersede for any structurally-indexed identity change (d-cpt-cye); same canonical across a supersede is legitimate (same-chain exemption).
14. Capture the actor BEFORE the role and anything binding to it — "the role binds to its canonical, never mints a new name" (d-prc-cap:157).
15. On a sparse graph, warm the project frame before asking a person to describe themselves (d-prc-bst:156).

## D. Reverse side (actor_capture.tmpl)
- D1 (high): the body describes who the participant IS, not what they do inside this project — "Christopher reviews PRs and owns migrations" belongs in a role.
- D2 (medium): the canonical is named in the prose, not only frontmatter — "an identity record with no readable identity" is the failure (exactly the live miss).
- D3 (low): bare naming is a defect; prose grounds the identity.
- D4 (no-finding): identity grounded, canonical named, this-graph role scope kept out.
- D5: legitimate actor content: external organizational roles, career background, identity scope (family vs session, collective vs individual).
- D6: structure is mechanical (missing canonical, cross-chain reuse, layer) — semantics vs mechanics split.

## E. Discriminators
- vs role: OVERVIEW OWNS "outside vs here" + "This week's task is neither" — point, don't restate. The fact ADDS: frame-relativity (a position held in another frame — CEO elsewhere, member of Y — is identity HERE; actor_capture.tmpl via s-prc-sn7); the MIXED-TERMS rule ("Users describe people in mixed terms — that's normal. Draft TWO entries before playback… Default is capture-both; deferring role is explicit, never implicit. Don't silently drop volunteered role content" — bootstrap skill:255-265, engine d-prc-cap:152); binding direction (actor first); cascade consequence (retiring the person retires the roles).
- vs a plain participants mention: listing a name records that someone took part; the actor entry makes the name a recognized identity. A guest quoted in one meeting's minutes is a participants mention; a person attributed across the record from now on gets an actor. Routine participation never warrants a ref to the actor entry (d-cpt-979).
- vs fact (a claim about a person): UNSOURCED — flag for ruling. Nearest anchor: actor is a declared identity with a binding handle; an observation about a person carries no handle.
- Non-software instantiations: village choir — actor: "Marta, canonical `Marta`; trained mezzo, twelve years city opera chorus, teaches music at the Gymnasium"; role: "Marta selects the repertoire for the winter concert." Bakery: actor "Yusuf, master baker trained in Gaziantep"; role "runs the sourdough line and signs off every new recipe."

## F. Contradictions / rulings needed
- F1 Topics: capture procedure says "no natural refs or topics"; all three live exemplars carry `collaboration/identity`. Pick a line.
- F2 Canonical form: no rule prefers short forms — hav uses `Christopher` (short), 5zj uses `Jonathan Philipp` (full). The only sourced rule: match what the project already uses.
- F3 Aliases in participants: early directive accepted aliases; current contract + code are canonical-only. Use canonical-only.
- F4 Validator quirks (s-tac-tom open) largely historical — don't encode.
- F5 Expertise on actor vs role: reconcilable reading (UNSOURCED synthesis, flag): expertise brought from outside = identity; expertise as the weight carried here = role.
- F6 External context optional (rubric) vs always asked (bootstrap interview) — choose a posture.
- Per-kind "default confidence high" (d-cpt-cye, framework-concepts) — not enforced anywhere; same shape as the kind default ruled CLI-legacy (d-cpt-1dk). FLAG for ruling.
- "An actor has no natural refs" — true of a first capture, false of supersedes. Phrase the ref posture carefully.

## G. Overview coverage — point, don't restate
Signal/decision split; kind question; actor-vs-role outside/here test + this-week's-task line; retirement split; layer list; per-kind fact pointer. The fact's own ground: frame-relativity, mixed-terms split, canonical as write-once binding handle, alias hygiene + where-variants-appear, introduce-canonical-in-prose, supersede-within-chain vs directive-closure-with-cascade, participants-mention discriminator.
