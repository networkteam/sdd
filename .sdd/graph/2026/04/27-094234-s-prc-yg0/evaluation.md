# Greenfield Bootstrap Evaluation — Viebrockhaus Chatbot POC

## Setup

- Project: external Viebrockhaus chatbot POC (path not recorded per policy — outside this graph)
- SDD bin: copied freshly-built `bin/sdd` to `~/.local/bin/sdd` to use under test
- Git: fresh repo; `sdd init --language de`
- Session: new Claude Code session in the project directory; ran `/sdd`
- Skill version under test: initial `/sdd-bootstrap` draft from commit `414be26` (2026-04-24)
- Participants in test: Christopher (local), Claude (assistant)

## Transcript excerpts

### `/sdd` entry — bootstrap offered (issue A)

Status output: `Local participant: Christopher` / `Language: de` / 0 entries / no WIP.

Agent response (too technical for a new user):

> Der Graph ist leer (0 Einträge), Sprache ist auf Deutsch konfiguriert. Für einen frischen Graphen ist /sdd-bootstrap der richtige Einstieg — der führt durch den Setup-Flow (Readiness-Sweep, Brownfield-Kontext, Actor-Capture, Golden-Circle-Seeding).
>
> Soll ich /sdd-bootstrap starten?

User commentary: *"The wording is too technical. A new user would be confused by readiness sweep, brownfield context, actor capture, and golden circle seeding. We must stay in the narrative here."*

### Bootstrap entry — orientation ask (issue D)

User: *"nur grob angeschaut bisher"*

Agent: *"Alles klar — ich schalte den Lehrmodus an, d.h. kurze Einordnung vor jedem Capture. Ich schaue mir die Repo-Landschaft kurz an, um zu entscheiden, ob wir Move 2 (Brownfield-Kontext) brauchen."*

`Greenfield-Projekt` and `Brownfield-Kontext` later used in user-facing narrative.

### Move 3 — actor capture, multiple questions per turn (issues B, C)

Agent — three questions in one turn, framework jargon throughout:

> Move 3 — Actors
>
> Kurz zur Einordnung: Actor-Signale erfassen, wer am Graphen teilnimmt — stabile Identität (Name, Hintergrund, Affiliation), die unabhängig von diesem konkreten Projekt gilt. Das ist wichtig, damit jeder spätere Eintrag konsistent derselben Person zugeordnet werden kann; der kanonische Name ist der, den alle Einträge im participants-Feld tragen.
>
> Fangen wir mit dir an: In der lokalen Config steht bereits Christopher als kanonischer Name. Passt das so, oder willst du was anderes?
>
> Und zwei weitere Fragen:
> - Unter welchen anderen Namen tauchst du sonst auf (Git-Commits, Chat, externe Handles)?
> - Etwas zu dir — Hintergrund, Affiliation, Expertise?

User commentary: *"Teaching is terrible. It's absolutely terrible. That's not at all what I expected because it uses very technical terms like 'kanonischer Name' and 'Identitätskette' and 'aliases'. What?"*

User principle: teaching mode should use plain words, then introduce SDD concepts gradually (graph, entries, types, kinds), without surfacing internals like canonical, aliases, write-once, identity chain.

### Vera & Florian capture — actor/role conflation (issues F, G, H, I)

Agent drafted Vera prose (issue H — `kanonisch:` surfaced):

> Vera (kanonisch: Vera) ist Projektmanagerin bei networkteam und Hauptansprechpartnerin für Kunden. In diesem Projekt führt sie die Kundenkommunikation mit Viebrockhaus.

Pre-flight blocked (issue F):

> [high] role-drift: The prose conflates this-graph role scope with identity content. 'In diesem Projekt führt sie die Kundenkommunikation mit Viebrockhaus' describes what Vera does inside this graph (project-specific responsibility), not who she is.

Second attempt also blocked (issue I):

> [high] role-identity-conflation: 'Projektmanagerin bei networkteam mit Schwerpunkt auf Kundenkommunikation' describes what Vera does *inside this graph*

User decision: skip pre-flight to proceed.

User commentary: *"Maybe we drop the requirement and allow the actor and role to be conflated initially, with additional roles being captured. On the other hand, maybe this is a clear signal that our actor and role concept is too complex in reality. Because in practice both are really coupled together many times."*

User principle (issue G): when user volunteers role content alongside identity, skill should split and capture both — actor (identity) and role (project contribution) — not defer the role.

### Move 4 — Golden Circle (issues J, K, L, P)

Agent (issue J — Sinek terminology surfaced):

> Move 4 — Golden Circle
>
> Kurze Einordnung: Jetzt geht's um die Projekt-Form. Wir gehen WAS → WIE → WARUM (umgekehrte Sinek-Reihenfolge, weil's für's Erfassen einfacher ist).

Agent rendered aspiration framing (issue P — vocabulary-de metaphor leak):

> Aspirations sind der Sog, gegen den alle späteren Entscheidungen ausgerichtet werden — die Akzentwahl prägt später, wie Trade-offs fallen.

User commentary: *"'Sog' has quite some negative connotation in German. Because you will not pull towards it but into it and fall into it, and that sounds not nice. Maybe as a German word, we can use Bestreben or Zielsetzung."*

User dumped extensive workshop material; agent jumped to a 5-entry capture plan grouped by SDD concepts rather than the WAS/WIE/WARUM passes (issue K):

> Capture-Plan (5 Einträge):
> 1. Fakt, Prozess-Ebene — Kundenkontext...
> 2. Anspruch, strategisch (kind: aspiration) — WARUM...
> 3. Direktive, konzeptionell — Chatbot-Konzept + Phase-1-Scope + MUST/SHOULD/COULD/WON'T...
> 4. Direktive, strategisch — Vorgehen: Anbieter-Evaluation...
> 5. Lücke, taktisch — Als Nächstes...

User commentary: *"The capture plan is OK, but it could be better, because we were in the golden circle phase and now it's already grouped by concepts of SDD. The skill could have stayed in this mode for the golden circle and told that additional entries were captured because I provided quite some information already."*

On tension-surfacing (issue L), user had to ask: *"Was wird dagegen sprechen, beides aufzunehmen, sowohl den Besuchenden als auch den Business Case?"* Agent then proposed two separate aspiration entries — which is correct, but should have been offered proactively.

### Pre-flight findings — teaching mode missing (issue M)

When pre-flight fired multiple times (medium aspiration-tension, medium directive-aspiration-shape, medium type-mismatch on the gap-as-directive), the agent surfaced raw findings without any framing of what pre-flight is or why findings matter.

User principle: teaching mode should introduce pre-flight on first occurrence — *"entries get validated against quality rules; when findings come up, we read them, decide if they're right, and iterate or proceed."*

### Auto-summary language drift (issue O)

After captures landed, agent observation:

> Florians Actor-Zusammenfassung (automatisch generiert) ist auf Englisch gelandet — lass uns das bei Gelegenheit mit sdd summarize korrigieren, aber nicht jetzt.

The auto-generated summary ignored `Language: de`.

### Handoff

After captures, bootstrap exited cleanly with a /sdd catch-up narrative grouping by thread, ready/not-ready assessment ("Lint sauber, 10 Einträge"), and concrete next-step suggestions.

User commentary on the result:

> "I directly got pulled into the discussion of the evaluation plan, and now it's working quite well with the information we captured. The result of the bootstrap is very nice. But I didn't like the actual form, and I could do it because I had the knowledge and experience with sdd, but I doubt it would have been the right thing for a new user."

## Findings — full list

### A. `/sdd` entry to bootstrap — technical wording in offer message
**Observation**: /sdd's offer echoed internal move names ("Setup-Flow (Readiness-Sweep, Brownfield-Kontext, Actor-Capture, Golden-Circle-Seeding)").
**Root cause**: /sdd's Bootstrap mode entry contains a parenthetical listing the moves; the agent surfaced it verbatim.
**Severity**: medium — first user touchpoint, sets the wrong tone.

### B. Teaching mode leaks framework vocabulary
**Observation**: framings used "kanonischer Name", "Aliase", "Identitäts-Kette", "write-once", "Actor-Signal", "Prozess-Ebene", "kind: actor" in user-facing text.
**Root cause**: teaching exemplar uses jargon; framework-concepts content loaded for judgment also flowed into surfaced prose without an explicit "vocabulary stays internal" rule. Teaching mode should build the user's mental model from plain words and introduce SDD concepts (graph, entries, types/kinds) gradually, not lecture mechanisms.
**Severity**: high — defeats the purpose of teaching mode.

### C. Multiple questions per turn
**Observation**: Move 3 asked three questions in one turn (canonical name, aliases, identity context).
**Root cause**: Move 3 sub-steps listed as three ordered items; agent read as "ask all in one turn" rather than "outline upcoming, ask one, wait."
**Severity**: high — overwhelming for human dialogue.

### D. "Greenfield/brownfield" in user-facing text
**Observation**: agent used "Greenfield-Projekt", "Brownfield-Kontext" in narrative.
**Root cause**: internal labels leaked into surfaced prose.
**Severity**: medium — jargon for non-technical users.

### F. Actor/role conflation at draft time
**Observation**: agent drafted Vera's actor signal with project-role content; pre-flight blocked. Pattern repeated on retry.
**Root cause**: skill doesn't apply identity/role boundary at draft time.
**Severity**: high — wastes pre-flight cycles, forces `--skip-preflight`.

### G. When user gives mixed identity + role content, skill should split and capture both
**Observation**: user volunteered Vera's project role alongside identity; agent drafted with both, got blocked, then deferred role capture instead of splitting.
**Root cause**: skill mentions splitting in principle but doesn't proactively capture both entries.
**Severity**: high — leaves volunteered material uncaptured.

### H. "Kanonisch:" surfaced in playback prose
**Observation**: playback showed "Vera (kanonisch: `Vera`) ist...".
**Root cause**: same as B.
**Severity**: medium — playback is a high-visibility surface.

### I. Pre-flight actor/role boundary too strict (or concept too coupled)
**Observation**: pre-flight rejected "Projektmanagerin bei networkteam mit Schwerpunkt auf Kundenkommunikation" as role content. But this is what Vera does at her external employer, not in this graph specifically. Boundary drawn tighter than practice supports.
**Root cause**: either rubric miscalibration (job-at-employer should be identity), or deeper concept tension — actor and role may be more coupled in practice than the framework allows.
**Severity**: high. Belongs in the parallel `s-prc-kve` thread and possibly a broader review.

### J. Sinek / Golden Circle terminology surfaced
**Observation**: agent said "umgekehrte Sinek-Reihenfolge" in user-facing text.
**Root cause**: skill text references Sinek's framework as the reasoning behind WHAT-first order.
**Severity**: medium — academic reference, not user-friendly.

### K. Rich user input collapsed out of Golden Circle structure
**Observation**: user dumped workshop material; agent proposed a 5-entry capture plan grouped by SDD concepts (Fakt/Anspruch/Direktive) rather than by WAS/WIE/WARUM passes.
**Root cause**: skill doesn't preserve pass-framing during batch capture. Partitioning itself worked; the loss was in cadence visibility.
**Severity**: medium.

### L. Skill should surface tensions and allow multiple aspirations proactively
**Observation**: user had to ask "why not capture both?" before the agent split user-focused + business-focused into two aspiration entries.
**Root cause**: skill doesn't proactively recognize when one answer spans multiple distinct pulls.
**Severity**: medium — the right outcome eventually, but required user push.

### M. Teaching mode missing pre-flight introduction
**Observation**: when pre-flight fired the first time, no explanation accompanied — just raw findings.
**Root cause**: skill teaching exemplar focuses on entry types; no framing for the validation system itself.
**Severity**: medium — without framing, blocking findings feel arbitrary.

### P. Vocabulary-de metaphor literalisms
**Observation**: agent rendered "perpetual pull" as "Sog" — negative connotation in German.
**Root cause**: vocabulary-de.md has term mappings but no metaphor guidance; agent translated literally from framework-concepts English idiom.
**Severity**: medium — jarring rendering. Fix idea: add a metaphor table with normalized English headers (`Source term / metaphor | Rendering | Do not translate as`) including bad-example entries (e.g. `perpetual pull | Bestreben, Ausrichtung, Leitmotiv | Sog (negative connotation)`).

## What worked — preserve

- **Workshop material partitioning**: when the user dumped extensive context, agent identified distinct entry candidates (fact / aspirations / conceptual directive / strategic directive / gap) and proposed them as a clear capture plan.
- **Tension-surfacing on aspirations**: pre-flight's low-finding correctly named the visitor-vs-business tension after both aspirations were captured.
- **Self-correction**: agent recognized pre-flight blocks as legitimate and proposed revised drafts (though F/G show drafts could have been better the first time).
- **Pragmatic refinement on user input**: when user clarified "screening is more practical, not categorical no-in-house", agent reformulated the directive accordingly rather than rigidly defending the first draft.
- **Catch-up handoff**: bootstrap exited via /sdd's catch-up playbook with a clean thread-grouped narrative and concrete next-step suggestions. Felt natural.
- **Result anchored real work**: after handoff, the user moved directly into discussing the evaluation plan grounded in the captured graph.

## Orthogonal concerns — separate signals warranted

### E. `sdd init` first-time tip
After `sdd init` on a fresh install, the CLI could hint: *"start a new Claude Code session and run /sdd to begin."* Lower friction for first-time adopters.
**Scope**: CLI feature request, not skill-level.

### I (orthogonal). Pre-flight actor/role boundary calibration
The rubric rejected natural identity descriptions touching on what someone does at their external employer. Either the rubric needs calibration, or actor/role is too coupled for practical use. Belongs in the parallel `s-prc-kve` thread.

### O. Auto-generated summary ignored configured language
Florian's actor summary (auto-generated) came out in English despite `Language: de`. The summary template either doesn't pull in the locale config or doesn't respect it.
**Scope**: LLM template / CLI bug.

## Improvement directions — non-binding

Surfaced from evaluation dialogue. Future sessions decide what to act on:

- **Bootstrap SKILL.md revisions**: user-facing language rule (vocabulary stays internal); one-question-at-a-time rule with outline-then-ask pattern; teaching exemplars rewritten in plain language; actor/role split at draft time when user mixes content; Golden Circle framing preserved during batch capture; tension-surfacing for multi-pull answers; pre-flight introduction in teaching mode.
- **/sdd SKILL.md**: simplify Bootstrap mode entry — drop the parenthetical move list, instruct to offer in plain terms.
- **vocabulary-de.md**: add metaphor guidance table with normalized English headers (e.g. `Source term / metaphor | Rendering | Do not translate as`) and bad-example entries; could extend retroactively to existing tables for header consistency.
- **Test alignment**: a brownfield run would exercise Move 2 (skipped this run) and might reveal more issues; alternatively, greenfield was already revealing enough that revising first and re-testing later may be the better path.
- **Deeper question on actor/role**: whether the framing holds up under real adoption — concrete-vs-intuited friction worth tracking across more runs.

## Next session options

- **Revise the bootstrap skill against the findings, then re-test** — same project (idempotent moves) or a new greenfield/brownfield project.
- **Run a brownfield test first** to surface Move 2 issues before committing to revisions.
- **Capture the orthogonal signals** (E, I-orthogonal, O) to track them outside the bootstrap thread.
- **Defer skill revision** until a second eval run accumulates more data.

Whichever path: closing `d-cpt-kv1` waits until at least one revision/re-test cycle confirms improvements address the high-severity findings (B, C, F, G, I).
