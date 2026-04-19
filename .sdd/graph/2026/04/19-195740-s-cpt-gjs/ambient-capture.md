# Aspiration #6: Ambient capture — SDD emerges from the work

Extends the five-aspiration set captured in `s-cpt-wiv` + `s-cpt-hd8`.

## The aspiration

> SDD emerges from the work itself, not as a separate mode. Participants stay in their natural activity — chatting, coding, designing, running a meeting — and the graph grows from that motion. Tooling and interfaces minimize mode-switching; capture is ambient, not a parallel chore.

## Why a separate aspiration from #3

Related but distinct pulls:

- **#3** (overcome ceremony, bureaucracy, parallel artifacts): *artifact-facing*. No wikis, tickets, docs, specs as separate systems of record.
- **#6** (ambient capture): *activity-facing*. No dedicated SDD sessions, no "stop doing work, then do graph work" mode-switch.

Together they're the two halves of "minimal overhead": #3 catches artifact overhead, #6 catches activity overhead.

## Why a separate aspiration from #4

#4 (dialogue shapes decisions) specifies *what* the process is: multi-party dialogue produces decisions. It doesn't specify *where* dialogue happens — it could happen in dedicated SDD sessions or ambiently during normal work. #6 adds the *where*: dialogue happens inside existing work patterns, not in parallel to them.

## Distinguishing test decisions

**Decisions #6 catches but #3 doesn't:**

- **"CLI command requires `sdd dialogue start` before any capture is allowed"** — no artifact added, no ceremony per se, but forces an explicit mode-switch. #6 catches; #3 misses.
- **"Skill lets users say 'let's capture this' without specifying type / layer / refs"** — #6 rewards (reduces mode-switching); #3 neutral.
- **"Quarterly SDD planning offsite"** — #6 catches even the lightweight ritual form that wouldn't quite register as full ceremony for #3.

**Decisions #3 catches but #6 doesn't:**

- **"Publish a status dashboard read from the graph"** — #3 flags (parallel artifact); #6 neutral (ambient consumption doesn't force a mode-switch).

## Kōgen evidence

The story shows ambient capture throughout:

- Jun narrating his Sidama discovery while cleaning the Probat
- Morning bar conversations → captured decisions (not "bar conversation + separate capture session")
- Mara on her phone after closing the shop
- Priya working through technical choices in dialogue during normal implementation
- The analytics review agent generating signals from engagement data, not from a scheduled review

The closing summary — "no spec was written, no tickets were created or groomed, no sprint was planned, no standup happened, no status report was filed" — splits cleanly across #3 and #6:

- "No spec / no tickets / no status report" → #3 (artifacts)
- "No sprint planned / no standup" → #6 (activities)

## Aspiration test

- **Gradient**: "How much does this decision let users stay in their natural activity vs. force a mode-switch?" — yes, with degrees.
- **Not check-off-able**: SDD can always become more ambient — every new feature either pulls capture deeper into existing work or pushes it into a separate activity.
- **Catches unique failures**: explicit mode-switching tools, dedicated SDD sessions, ceremonial review activities.

## Constellation position

With #6 added:

- **#4** (dialogue shapes decisions) — *mechanism*: what the process is
- **#5** (autonomy) — *value*: why better local decisions matter
- **#1** (alignment across parallel work) — *outcome*: coherence despite parallel motion
- **#2** (accessible to non-technical) — *reach*: who gets to participate
- **#3** (overcome ceremony / artifacts) — *escape from artifact overhead*
- **#6** (ambient capture) — *escape from activity overhead*

#3 and #6 are paired — together they express the full "minimal overhead" pull. Keeping them separate (rather than merging into one "no overhead" aspiration) preserves the distinction between artifact and activity facets, which catch different failures in practice.

## Lifecycle

When the aspiration kind lands (via `d-cpt-omm` supersede, per `d-cpt-xqt`), #6 becomes a `kind: aspiration` entry alongside the other five. This signal is picked up as evidence for that plan.
