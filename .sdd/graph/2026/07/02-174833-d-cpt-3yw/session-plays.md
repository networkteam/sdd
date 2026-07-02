# Session plays — four real sessions reconstructed as named moves

Evidence base for the workflow-engine architecture decision. Method: four Claude Code session transcripts from this repo (2026-06-12, 2026-06-25, 2026-06-28, 2026-07-02) were condensed to dialogue+tool-call event streams and independently reconstructed by cheap analyst sub-agents into "plays" — chronological episodes of named moves with triggers, nesting, outcomes, and friction. The goal: see through the interwoven surface, name the moves behind it, and test whether the proposed engine model maps to real sessions rather than theory.

## Session 1 — 2026-06-12: multi-agent skill render (implement-heavy)

Arc: **engage(d-tac-4ln)** as the spine enclosing the whole session.

- E1 engage on the render plan: chain read, 8 ACs briefed, code topology mapped
- E2 design dialogue nested inside: Go templates vs custom delimiters; user pushed back, scope bounded
- E3 capture of the design directive (d-tac-dtl) + WIP marker + branch
- E4 implement slice 1 (templates restructure, parity test) → E5 capture partial done (s-tac-fxz)
- E6 implement slice 2 (multi-agent config + init) → E7 capture done (s-tac-h39) + eval setup
- E8 user-driven Codex evaluation with nested sub-episodes: eval protocol briefing, first capture attempt (blocked by OpenAI param bug + sandbox), retry on Sonnet, additional gap captures; agent authored clean keeper entries from throwaway drafts
- E9 handoff note for next session

Friction worth engineering around: `sdd new` shell-quoting broke three times on apostrophes in JSON refs (dissolves under a structured write API); Codex over-reached a ref-kind (`refines` where `related` was right — caught in playback, i.e. by dialogue, not by a gate); gate-visible IDs must not leak into re-drive text.

## Session 2 — 2026-06-25: rule-system spec (design-heavy)

Arc: catch-up → **engage on a constellation** (six directives, not one entry) → long design dialogue → plan capture → artifact work → insight captures.

- E1 catch-up briefing
- E2 engage on the six-directive cluster: widening, chain reads, "one initiative, two halves" analysis
- E3 extended dialogue/decide with distinct phases: **explicitly requested interview** ("interview mode please") locking design answers one question at a time; playback consolidation; create-spec skill invoked *inside* the dialogue as supporting work; iterative spec grounding; two user scope corrections (rejected markdown-parsing for activation → explicit frontmatter; caught spec designing into retiring `sdd status` → re-scoped to `sdd view`)
- E4 capture of plan d-tac-eho with the spec as attachment; summary fidelity checked
- E5 artifact enhancement (hover glossary over entry IDs)
- E6 two insight captures sparked by E5 (reader-facing reference surfaces; skill bare-ID-dropping gap)

Key observations: the interview produced locked decisions and a capture needing no skim-confirm pass (empirical case for evidence-by-construction, s-cpt-qs2); nested tooling (create-spec) worked as host-side activity whose product became capture-attachment evidence; both scope corrections happened *inside* an open move — amendment, not re-drive.

## Session 3 — 2026-06-28: intent attribute (full arc: implement + evaluate)

Arc: catch-up → engage(d-tac-n9k) → six sequential implement slices → done + merge → evaluation spawning five captures.

- E1 catch-up; E2 engage: AC table built, **AC drift found** (AC 4 referenced retired `sdd list`) and resolved with the user before work started
- E3 WIP marker, worktree, slice sequence locked
- E4–E9 implement slices 1–6, each ending in a verified commit; one side-question (missed German vocabulary gloss) caught by user during landing and fixed pre-merge
- E10 done signal s-tac-o30 captured (closing plan + parent directive); E11 merge + WIP cleanup
- E12 evaluation with two lenses; user challenge ("why do we need hashes at all?") reframed a planned system into dissolution (d-cpt-4qi supersedes d-tac-44v); **five captures flowed out of the evaluation as reasoning resolved**; user's manual "is everything captured?" check surfaced two loose findings that became entries

Key observations: captures embed in evaluation rather than following it (handle seeding); the completeness check is a structural feature waiting to exist (close-gate checklist); slices map to a goal loop with commit+test evidence — all deterministic-checkable.

## Session 4 — 2026-07-02: enforcement spike (the session preceding this decision)

Arc: catch-up → engage(spike d-tac-tjw) → implement (server build) → evaluate (three automated postures + one human-driven run) → dialogue convergence → four captures.

- Scope was reframed twice by the user *inside* engage ("think that through": toy goals → real playbook moves; REPL → live agent eval) — frame errors no gate could catch, corrected by dialogue
- Three automated runs: compliant, lazy, skip-everything. Structural sequencing held where instructions had failed; but the skip-prone worker **fabricated a well-shaped grounding report** (the gate checked shape, not work; the re-drive text had leaked a valid ID)
- The human-driven run exposed the deeper miss: the goal loop ran ground → draft → capture **around the user** — no confirmation turn existed
- Convergence dialogue produced: evidence-as-incentive (s-cpt-icg), dialogue-presence-as-structure (s-cpt-p6f), the open evidence-form question (s-cpt-li1); four entries captured back-to-back

## Cross-session findings

1. **Engage is the spine, not an episode.** One engage per session stayed open for hours; everything nested inside it; its working set grew (AC status, drift notes, slices, findings). As a long-lived handle this continuity becomes structural — including cross-session pickup.
2. **Dialogue is the space between handles, not a handle.** Free reasoning converges *into* captures; no goal sequence was ever followed in the dialogue stretches. The deliberate exception: the interview — dialogue in move shape, invoked when alignment matters.
3. **Captures are short-lived handles opened from anywhere, often flushed in batches** at convergence points (4 at the end of session 4; 5 inside session 3's evaluation). Seeding from another handle's evidence is the normal case.
4. **Mid-move re-scoping is first-class.** The most consequential turns were user corrections inside open moves. The engine needs `amend` alongside advance/re-drive/abandon — and these frame corrections are precisely what no gate can catch: the floor is structural, the frame is conversational.
5. **The completeness check exists as a manual habit** ("is everything captured?") and maps directly to a close-gate checklist derived from engine-held state.
6. **A whole friction class dissolves under a structured write path** (shell-quoting of JSON refs in `sdd new`).

## The rewritten play — session 3 under the engine

```
sdd_session_start()
  ← orientation, move/query menu, WIP, ultralight delta

h1: catch-up as QUERY — engine serves lanes + guidance; agent composes; no report-back

── free dialogue: "implement d-tac-n9k, use a worktree" ──

h2 = start(engage, anchor: d-tac-n9k)            ← the spine, stays open
  ← goal: ground [engine serves anchor + chain pre-fetched]
  → report(evidence: AC table + drift finding "AC4 references retired sdd list")
  ← goal: align — USER state; drift resolved in dialogue; reply is the exit
  h2 working set: anchor, amended AC contract, chosen path

h3 = start(implement, seeded from h2)
  ← goal: setup → WIP marker via engine write path
  ← goal: slice → report(commit, test output, ACs touched)
                  [state checks: commit exists, tests reported] → next slice
  …six iterations; partial dones spawn capture handles seeded from h3 evidence
  ← goal: landing → per-AC evidence table → h3 closed

── free dialogue: "help me evaluate this change" ──

h5 = start(evaluate, lens: d-tac-n9k + done signal)
  ← goals: inner lens, outer lens — findings accumulate in working set
  [user challenge on hashes happens in free dialogue; enters h5 via amend]
  → converged findings spawn capture handles carrying finding + trajectory
  ← goal: completeness — engine flags findings without spawned captures (2)
  → h5 closed

session close: open-handle checklist, ledger, carry-overs
```

Every episode of the real session has a home; nothing had to be forced. The corrections this mapping forced on the architecture (catch-up demoted to query, `amend` added, chooser-typed junctions replacing a three-verdict corridor, session shells per posture) are folded into the decision's design note.
