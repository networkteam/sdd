# Grounding skip at the catch-up → dialogue handoff — diagnosis

## Failure timeline (this session)

1. `/sdd` started; catch-up briefing produced from pre-fetched summaries.
2. First substantive dialogue turn: the agent asserted graph connections (d-stg-3k0, d-stg-2wb, d-stg-beb, s-cpt-k8i) and framed the user's points as "fresh" — all from catch-up summaries and recall, no entries opened.
3. User: "do your homework and search ... before responding with claims." The agent then searched and read entries, and found most of the user's two discussion points were already captured (s-cpt-r57, d-cpt-313, s-cpt-5ox, s-cpt-bn1).
4. The agent then wrote "a year-old open gap" about s-cpt-5ox — dated 2026-04-10, ~7 weeks old, and the project itself began in April 2026. A second unverified claim, with the date visible in output it had just read.
5. Meta-critique surfaced the pattern. The agent's own account: the generative default produces a fluent, confident continuation; verification is a separate deliberate step that loses to the default unless a strong external trigger forces it. The two triggers that worked were both user pushes.

## Two skill-design contributors

1. **Catch-up no-lookup bleed.** `/sdd-catchup` instructs "sole input ... no other lookups, no `sdd show` follow-ups." Correctly scoped to composing the briefing, but with no reset at handback. The frame stays in context as the most recent, most absolute instruction when dialogue begins, so it suppresses grounding on the next turn rather than just during the briefing.
2. **`/sdd` search-discipline scoped to drafting.** `/sdd`'s "Surface candidates with `sdd search`" guidance fires "before drafting a new entry," not for dialogue claims. A conversational assertion fell in the gap between the two skills: catch-up said don't look; `/sdd` only says look when about to write an entry.

## Relationship to the graph

- A specific instance of the alignment/grounding skip at transitions (s-prc-7hw): the catch-up → dialogue handoff is exactly such a transition; this adds the concrete mechanism.
- Reasoned through the mentions-as-facts frame (s-prc-uq8), which reports the same pattern across projects and notes that a sharpened CLAUDE.md rule "proved insufficient" — hence the expectation that text guidance alone may not close this.
