# Planning entries as roadmap — architecture sketch

## Problem framing

`sdd status` today surfaces every open entry as equally live, but in practice only a slice is being actively driven at any moment. The implicit roadmap — what's in focus, who's engaged, what's time-bound by external coordination — lives in heads. Each catch-up rebuilds context from scratch, violating the alignment aspiration (d-stg-qlt). The trap to avoid: per-entry mutable assignment fields, sprint windows, manufactured cadences — all of which conflict with the immutability contract (d-cpt-e1i) and the no-parallel-artifacts aspiration (d-stg-3k0).

## Architecture

### Planning entries

- `kind: activity` decisions at the appropriate layer:
  - **process** for daily/weekly plans
  - **strategic** or **conceptual** for roadmap entries
- One active planning entry per cadence (one active daily plan, one active weekly plan, one active roadmap)
- Refresh via supersession; closure is supersede-by-next-plan, not done-by-completion

### Involvement triples

Body of the planning entry contains `(target-entry, [actor-canonical, ...], optional-date-range)` triples.

- **Multi-actor by default** — same target may have several involved actors representing different angles (brand, technical, content). Prose names angles when relevant.
- **Date range optional**, scoped to the (target, actors) commitment. Not all triples need dates.
- **Refs field includes all triple targets** so traversal, lint, and search work unchanged.

## Worked example: morning coffee round from Scene 3 of the story

*(IDs from the Kōgen story in docs/story.md, used here to ground the example.)*

After the day-three prototype evaluation, the Kōgen team's morning round identifies three things to address: the layout conflict, the business model tension, and the share-flow gap. Captured as a daily plan:

    ---
    type: decision
    kind: activity
    layer: process
    participants: [Mara, Jun, Priya]
    date_range: 2026-04-08
    refs:
      - 20260407-213000-s-cpt-q2r
      - 20260408-065000-s-cpt-w7m
      - 20260406-211500-s-cpt-r4w
      - 20260407-194500-s-tac-v6k
    supersedes: 20260407-...-d-prc-...  # yesterday's plan
    ---

    # Daily focus — 2026-04-08

    Resolve three signals from the prototype evaluation: layout conflict (parallel branches, side-by-side review), business model tension (resolved this morning as subscription-with-sharing-built-in), and share recipient dead-end (straight to main).

    ## Involvement

    | Target                                   | Actors             | Window                  |
    |------------------------------------------|--------------------|-------------------------|
    | 20260407-213000-s-cpt-q2r (layout)       | Mara, Jun, Priya   | 2026-04-08 to 2026-04-10|
    | 20260408-065000-s-cpt-w7m (content)      | Jun                | tonight                 |
    | 20260406-211500-s-cpt-r4w (business)     | Mara               | resolved this morning   |
    | 20260407-194500-s-tac-v6k (share fix)    | Priya              | today                   |

The next day's plan supersedes this one. The chain across the week encodes how engagement evolved.

## Catch-up rendering — mock

    ### Where things stand

    Active planning: 2026-04-08 daily plan (Mara, Jun, Priya)

    **In focus** (4 items)

    1. Layout conflict (`s-cpt-q2r`) — Mara, Jun, Priya · 2026-04-08 to 2026-04-10 — branch experiment in flight
    2. Discovery content (`s-cpt-w7m`) — Jun · tonight
    3. Business model tension (`s-cpt-r4w`) — Mara · resolved this morning
    4. Share dead-end (`s-tac-v6k`) — Priya · today

    **Warm but unmentioned** (2 items — needs driving or parking)

    5. Customer email update flow (`d-tac-c9w`) — last touched 3 days ago
    6. Magic link auth concern (`s-tac-f2a`) — recent ref from share-flow review

    **Parked** (8 items — expand to view)

## Triple storage format — two options

**Option A — Frontmatter:**

    involvements:
      - target: 20260407-213000-s-cpt-q2r
        actors: [Mara, Jun, Priya]
        date_range: 2026-04-08/2026-04-10
      - target: 20260408-065000-s-cpt-w7m
        actors: [Jun]

Pro: structured, queryable without parsing prose.
Con: separates from descriptive prose; risk of frontmatter/body drift.

**Option B — Body markdown table:**

    ## Involvement

    | Target                    | Actors             | Window                  |
    |---------------------------|--------------------|-------------------------|
    | 20260407-213000-s-cpt-q2r | Mara, Jun, Priya   | 2026-04-08 to 2026-04-10|
    | 20260408-065000-s-cpt-w7m | Jun                | —                       |

Pro: readable, lives next to descriptive prose, parses cleanly with a tooling convention.
Con: parsing convention to define and enforce.

Both are viable. Choice deferred to implementation; the architecture is format-agnostic.

## Open sub-dimensions

1. **Heat decay function** — how recent must a ref be to count as warm. Linear over N days? Decay tied to supersession of containing plan?
2. **Triple storage format** — frontmatter list vs body table (above).
3. **Routing as separate concern** — s-cpt-5jk remains open. Routing asks "who *should* weigh in" on a decision; involvement asks "who *is* engaged" during a planning cadence. Different mechanisms.
4. **Notification on involvement change** — vision-level. Out of scope for first-pass shape.

## Closure check

- **Supersedes s-cpt-cca** (focus/priority layer gap). cca named the need with three candidate mechanisms (focus directive vs focus activity vs priority frontmatter); this proposal absorbs the question and proposes the planning-activity-with-involvement-triples shape with structural reasoning distinguishing it from issue-tracker semantics.
- **Adjacent: s-cpt-5jk** (participant profiles for routing) remains open as sibling concern.
