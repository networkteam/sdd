# SDD positioning — research synthesis

## Finalized opener

> # SDD
> **Keep humans and AI agents aligned across parallel work.**
>
> SDD records your project's reasoning as an immutable decision graph — signals (what you noticed), decisions (what you committed to), actions (what you did). At any moment, anyone (human or agent) can see what's in flight, which decisions are active, and what's still open. Built for developers and teams shipping with AI agents.
>
> **Why SDD**
>
> **Without SDD:** insights and decisions from each agent session get lost or hard to discover once the session ends.
>
> **Most other systems:** scatter state across separate static artifacts — docs, plans, phases, roadmaps, trackers, specs, tickets — that duplicate across layers, go stale, and resist new insight.
>
> **With SDD:** an append-only graph where new insight → new entry (`refs`, `supersedes`, `closes`). State is *derived from the graph*, never maintained in parallel docs. Tracked in Git, so humans and agents work against the same graph — each session runs independently, and signals, decisions, and actions are captured by the work itself.

## Research takeaways

1. **Category & audience.** "Agentic software teams and solo developers" as umbrella — both solo and collaborative contexts without org-chart framing. Avoid "PM replacement" framing, which creates resistance and dilutes the promise.
2. **Differentiator is live alignment + current-state visibility**, not retrospective tracing. Dogfooding insight: multi-threaded work is productive *only if* context is preserved. "Keep aligned" is the stronger selling point than "trace back."
3. **Two distinct failure modes in the status quo** (not one):
   - *Without any system:* insights and decisions from agent sessions get lost once the session ends.
   - *With traditional systems:* separate static artifacts (docs, plans, phases, roadmaps, trackers, specs, tickets) duplicate state, go stale, resist new insight.

   SDD's mechanism — append-only graph with derived state — answers both.
4. **Language: concrete over conceptual.** "Keep humans and agents aligned across parallel work" beats "framework for traversing decision graphs."
5. **README anatomy.** First 50 words: what / who / why different. Then: visual → quick start → one concrete workflow → concepts → comparison → docs. Concepts come *after* outcomes.
6. **Demo format.** Short GIF in README (core loop), longer video (60–90s) on landing page later. Scenario: "parallel feature work without losing context."
7. **Landing page** is a separate surface, not a priority now. Order: README → demo → landing page.

## Key wording decisions

- **"Keep humans and AI agents aligned across parallel work"** over "decision graph for agentic software teams..." (outcome-first beats category-first for the tagline; live alignment is the primary selling point, retrospective tracing is a bonus).
- **Three-label contrast** (`Without SDD` / `Most other systems` / `With SDD`) over a single bullet list — distinguishes *no structure* (insight loss) from *rigid structure* (staleness, resistance to new insight) as two separate failure modes that SDD answers.
- **"scatter state across separate static artifacts"** over "docs duplicate state" — broadens the category from doc-specific to all parallel-artifact systems (docs, plans, phases, roadmaps, trackers, specs, tickets).
- **"each session runs independently"** over "agents run autonomously" — honest about current reality; full agent autonomy is the vision, not today's claim. Independence implies parallel work, tying back to the tagline.
- **"captured by the work itself"** over "captured as they go" — makes the equivalence explicit: doing the work *is* the capture mechanism, not a separate documentation step.

## Tagline variants explored

- **A (chosen, refined):** "Keep humans and AI agents aligned across parallel work." Outcome-first. Live-state visibility over retrospective tracing.
- B — "A decision graph for agentic software teams and solo developers." Category-first, SEO-friendly but lower emotional pull.
- C — "Three agent sessions, one feature, zero memory of why. SDD fixes that." Pain-first, memorable but polarizing; better for launch posts than README.

## Out of scope for this signal

- The specific README restructure (moving concepts below session examples, adding GIF, adding comparison section) — to be captured as a tactical plan decision with acceptance criteria.
- Landing page design and launch plan — separate downstream signals/decisions.
- Demo scenario and GIF production — downstream.
