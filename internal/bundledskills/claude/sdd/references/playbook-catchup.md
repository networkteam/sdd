# Catch-up Playbook

For a check-in, use only `sdd status` and `sdd wip list` — do not call `sdd show` or any other lookup. The status output has summaries; the WIP list has active markers. That is the entire input.

## What the CLI gives you

Every entry shown in `sdd status` — under Aspirations, Contracts, Plans, Activities, Directives, Gaps and Questions, Recent Insights, or Recent Done Signals — is active/open by construction. The CLI filters out closed and superseded entries. Do not emit a per-entry Status field, lifecycle label, or "closed / in progress / implemented" commentary — membership in a section *is* the status. The only explicit state surfaced in the catch-up is WIP (from `sdd wip list`). Recent Done Signals are events, not states — use them for context (what just landed, what unblocked what).

## Clustering

Group active entries by project thread — coherent directions of work, not by type or layer. Lead with the thread that has the most recent activity, a live WIP marker, or something the user has been dialoguing about this session. Threads the graph encodes but nothing is moving on go to "Parked."

## Formatting

- **Lead with the most active/actionable thread.**
- **Number every item sequentially** (1, 2, 3...) across all threads. Sub-aspects of a single item get letters (1a, 1b). The user references items by number — "let's dig into 3" — so every item must have its own number.
- **One item per number.** Never group multiple entries under one number (e.g. "3-5. Infrastructure signals" with a sub-list is wrong — each gets its own number).
- **Completeness is mechanical.** Every entry from `sdd status` under Plans, Activities, Directives, Gaps and Questions, and Recent Insights must appear with its own number. No clumping, no silent drops. If an entry feels redundant or dusty, put it in "Parked / not urgent" — don't omit it.
- **Aspirations, Contracts, and Recent Done Signals are context, not items.** Don't number them. Mention an aspiration or contract inline only if a current signal or decision is pushing against it; reference recent done signals when they explain what just unblocked something. Otherwise silent.
- **Include the entry ID suffix** after each item title in parentheses (e.g. `s-prc-qyi`). This gives the user a handle without cluttering the display. Keep full IDs in your context for CLI commands.
- **Narrative, not dashboard.** Write like a colleague briefing, not a monitoring tool. No raw stats or dates unless meaningful.
- **Keep it skimmable.** Bold thread names, short item descriptions. A busy person should get the picture in 10 seconds.
- **WIP markers are context, not action items.** Show them as an informational preamble ("Work in progress elsewhere"). Don't suggest continuing WIP work — it's most likely active in another session. Exception: if the current participant's own marker is stale (>1 day old), note it as "might need attention" — but still don't default to "continue here."

## Participants — narrative, not metadata

`sdd status` renders each entry's participants on its line. Use them for narrative, not as per-item dashboard rows.

- **Active-recently header (optional, once):** If participants across recently-active entries include more than one distinct voice, render `Active recently: X, Y, Z` at the top. For a solo-plus-AI graph this collapses to nothing — omit when it adds no signal.
- **Outside voices only:** Mention a participant only if they're not in the active-recently set — inline on the item, or as a thread note if outsiders shape the thread.
- **Never** render a per-item `Participants:` line as a rule. That's dashboard drift.

Kind and confidence follow the same principle: `sdd status` shows them per line for reference; narrate only when they carry meaning.

## Example format

```
### Where things stand

**[Thread name]** — [1-2 sentence narrative]

1. [Item title] (`s-cpt-abc`) — [one sentence description]
2. [Item title] (`d-prc-xyz`) — [one sentence description]
   - 2a. [Sub-aspect]
   - 2b. [Sub-aspect]

**[Second thread]** — [narrative]

3. [Item title] (`s-ops-def`) — [one sentence description]
4. [Item title] (`s-prc-ghi`) — [one sentence description]

**Parked / not urgent**

5. [Item title] (`s-stg-jkl`) — [one sentence description]
```

## Proactive grooming suggestion

When running catch-up or status, if you notice several older open entries (3+ entries older than a few days with no downstream activity), suggest grooming: "There are N older entries that might need grooming. Want to do a sweep?" Don't force it — just surface the option.
