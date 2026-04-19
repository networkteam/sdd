# Adding `aspiration` as a decision kind

## The gap

The two-type redesign (d-cpt-omm) proposes five signal kinds (gap, fact, question, insight, done) and four decision kinds (directive, contract, plan, task). None of these map to perpetual aspirational direction — something the team commits to working toward, referenced by downstream work, but never checked off.

## Evidence from the Kōgen story

`d-stg-f3a` in the narrative ("explore a direct-to-consumer offering… sharing coffee discoveries carrying Jun's curation and story") is an aspiration in disguise. Nothing closes it. Every downstream decision references it. It carries medium confidence indefinitely — not because conviction is low, but because it's not the kind of thing confidence eventually rises into "done". SDD has no slot for this today; the pattern exists in practice but lives under the generic "strategic decision" label.

## Push vs pull — contract is not the answer

Contract is the nearest existing neighbor because both are perpetual (never closed until superseded). But they're semantically different:

- **Contract** = constraint. "Must / must not." Rule-shaped. Push against.
- **Aspiration** = attractor. "Aim toward." Direction-shaped. Pull toward.

A contract tells you what you can't do. An aspiration tells you what you're trying to become. Collapsing them would lose information — they guide different judgments.

## Constellation, not north star

A single north star collapses complex orientation into a scalar: every decision either heads toward it or doesn't. Aspirations-as-plural preserves the realistic shape of work: multiple pulls, sometimes in tension (growth vs. brand purity, speed vs. depth, reach vs. authenticity). A decision is evaluated as "which aspirations does this move us toward? Which away from? Where's the tension?" — not "is this aligned, yes or no?"

This also explains why detours can be coherent. A detour can still be pulled by some aspirations while departing from the most direct path to others. It's coherent if it's explicable against the constellation, even if it isn't the straightest line to any single star.

## Mutability and decay rate

Aspirations represent agreement among participants, so they're decision-shaped. Like any decision, they can be superseded when new dialogue and learning overturn earlier agreement. But larger-scope aspirations tend to decay more slowly than directives: they sit further from execution, so the stream of new learning reshapes them less often. Directives change as implementation reveals detail; aspirations shift when the underlying understanding of what we're doing shifts. This natural decay-rate difference is part of why separating them as a kind helps — it clarifies what should stay stable vs. what should churn.

## Naming

Candidates considered:

- **aspiration** (chosen) — dictionary-close to "work toward", no completion gate, pluralizes naturally, semantically narrow (nobody uses "aspiration" generically to mean "purpose of any decision"). Slight dreamy flavor, accepted as trade-off.
- **intent** — familiar from strategic/product/legal vocabulary ("strategic intent", "design intent"). Rejected due to overlap with generic "intent of any decision" usage — would create dialogue-level confusion ("does the intent of this directive align with our intents?"). CQRS collision (write-intent / read-intent) considered but judged minor since those terms are internal to the code, not user-facing.
- **aim** — simple, directional. Rejected as too modest for strategic-layer weight.
- **horizon** — spatial metaphor. Rejected because the star-map / geographical-metaphor family was explicitly set aside.

## Implications if adopted

- Pre-flight gains a new coherence check: "does this decision move toward any aspirations? Is there tension with any?"
- Catch-up can separate "landscape" (aspirations — always-on orientation) from "work" (active directives). Status stops treating aspiration-shaped strategic decisions as if they'll eventually close.
- `kind: aspiration` joins `kind: contract` as the second "never closes" kind, with a distinct semantic role — constraint vs attractor.
