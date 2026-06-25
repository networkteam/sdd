# Catch-up open-loops lane — design note

## The two questions catch-up answers

A returning reader needs two different things from a catch-up:

1. **Converging** — what is the project's center of gravity? What is everyone building on? → served well by **heat** (Σ over incoming refs, decayed by ref age).
2. **Carrying** — what have *I* committed to that I haven't acted on yet? → the open loops.

When the catch-up moved from agent-synthesized narrative to mechanical heat ranking (a cost decision — synthesis was eating roughly a third of session context at 400+ entries), heat quietly became the *only* selection signal. "Central" stood in for "important", and it answers only question 1.

## Why heat is structurally blind to "carrying"

Heat sums *incoming* refs. A freshly captured entry has none — not because it's unimportant, but because nothing has had time to point at it. So heat is a **lagging** measure (what the graph has already digested) being asked to do a **leading** job (what I just committed to). The divergence is sharpest at the worst moment: right after you capture a plan, when it's most alive in your head, it's at its least visible.

Empirical instance (downstream project): a just-captured plan was absent from "active and hot" at check-in; ~40 min later, once one other entry referenced it, it jumped from heat 0.000 to 2nd place. One inbound ref flipped it from invisible to prominent — visibility was governed by inbound-ref count, not relevance.

## One axis, two ends — the hand-off

Heat and "open-loop-ness" are not two metrics; they are the **same axis at opposite ends**:

- Born **cold**: no incoming refs → pure open loop.
- Migrates **hot**: as the graph acts on it (refs accrue), it warms.

So the single inbound ref that lifts an entry into the **hot** lane is the *exact event* that should demote it out of the **open-loop** lane. **The lanes hand off on the first ref.** Acted-on ⇒ leaves your open loops, enters the project's center.

## Rejected: newest-N still-open (recency as the signal)

Recency was the first candidate ("show my newest N still-open captures"). It's a *proxy* for unacted-on, and the proxy has two failures:

- A plan captured today that *already* got picked up (refs accrued) is no longer an open loop, but newest-N keeps showing it.
- A plan from months ago that *nobody ever acted on* is a real languishing loop, but newest-N dropped it long ago.

The real signal is the **absence of incoming refs**, not recency.

## Chosen: slow-decayed inverse-heat ("coldness")

```
coldness(e) = decay(age_of_entry) / (1 + in_degree(e))
```

- Reuses the existing decay vocabulary (`exp-Nd`), but applied to the **entry's own age** instead of the ref's age — the only new idea.
- `in_degree = 0`, brand-new → max coldness.
- Each incoming ref divides it down → the hand-off to hot.
- Age fades it → ancient orphans drop off (SDD is not a todo list; it should be slow).

Raw `in_degree`, not decayed heat — age is already supplying the slowness, so the "does an old ref still count" sub-knob is dropped.

### Self-pacing with a horizon

Show top-N by coldness:

- Newest unacted entries occupy the top slots (age decay hasn't touched them).
- Older parked loops rank below the cap, hidden.
- When you **act** on a newer one, its coldness drops, it leaves the top-N, and the next-coldest (an older parked loop) rises into the freed slot. → Clearing fresh loops gradually reveals older ones, a few at a time, never all at once.
- An entry old enough that age decay has bottomed out stays near-zero coldness and won't climb back even when everything newer clears. → Genuinely ancient loops fade and stay faded.

The lane nags gently and forgets eventually.

## Open knobs (for the build plan)

- **Half-life.** Heat uses `exp-14d` ("hot lately"). Coldness is slower — start around `exp-30d`–`exp-60d` and tune. A default, not a fork.
- **Which kinds count.** Plans, activities, gaps, directives are the action-carriers; freshly captured facts/insights may be observational noise. To decide.
- **Whose loops.** Just mine, or everyone's? Showing collaborators' fresh commitments serves cross-participant coherence but mixes "what I'm carrying" with "what landed near me."

## Presentation (settled in dialogue)

The open-loop lane is a **source guarantee**, not a rendered block:

- A fresh entry that belongs to a live arc is **woven into that arc**, marked as new — threaded via its own **upstream** refs (which a fresh entry almost always has, even at heat 0). `sdd view`'s `expand(refs)` already renders those upstream IDs / kinds / desc as sub-lines.
- A genuinely **disconnected** fresh entry (no upstream either) is not an orphan to bin — it signals a **new line of work** and earns its own short thread.
- No separate flat "Fresh" block — that would recreate the kind-grouped flatness the briefing exists to avoid and scatter one storyline across two lists.

## Deferred: the third "delta" lane

"Since I last looked" (a seen-watermark) turned out to be a *distinct third lane*, not the open-loop lane:

- It models **novelty** — what changed while I was away (others' new captures, closures, unblocks) — where seeing-it-once-is-enough is *correct*.
- The open-loop lane models **carrying** — persists until *resolved*, not until *seen*; an open loop shouldn't vanish because I glanced at it.
- The watermark needs session-tracking state SDD doesn't keep today, and a subtlety (a fresh plan should keep showing every session until acted-on, not just the first catch-up after capture). Deferred as additive, later work.

## Next

Build follows as a `plan` decision with ACs: the `coldness` rank algorithm in `sdd view`, the second catch-up lane wired into the fetch plus sub-skill threading guidance, and the half-life default chosen.
