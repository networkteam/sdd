# Evaluation — interactive terminal output (d-tac-5g9), live TTY

## What was evaluated
The on-branch `internal/cliout` + `tui` implementation driving `sdd index`
and `sdd search` lazy-fill, run on a real terminal (iTerm, Ollama local
embedder). `sdd index` exercised both warm (no work) and partial (manifest
trimmed to force ~40 re-embeds); `sdd search --query` exercised vector
lazy-fill. Behaviour captured from a 22s screen recording of the search run.

## Findings

### 1. Full-screen alt-screen takeover (the core problem)
Both commands clear the terminal to an alt-screen for the duration. This
conflicts with the captured requirements (d-cpt-mvb):
- Requirement 1: transient progress should be a **spinner / status line**,
  not a framed full-screen view.
- Per-command matrix: `sdd search` indexing is **transient**, only ranked
  results **persist**.

### 2. Search is the worst case — frame timeline
- 0s: prompt, `sdd search --query "improve CLI surface"`.
- ~2–12s: alt-screen clears to a near-empty black void; only a small
  `⠿ searching` spinner top-left. No progress signal at all while the query
  embeds + the lazy-fill runs.
- ~16s: a burst of `INFO indexed entry=… chunks=2` lines appears at once
  (first embedding batch returned).
- ~19s: drops back to the normal screen; ranked results render. Total ~19s.

### 3. Root cause
The plan resolved the directive's open coordinator-design question
("pause/redraw around log emission vs a dedicated log region") toward a
**dedicated log region** (a viewport), and used **alt-screen** so that
region could erase cleanly on teardown. That choice is what produces the
takeover, and it stretched requirement 1's "spinner / status line" into a
whole framed TUI.

### 4. Unstyled output (secondary)
- The warm-index summary `indexed 0 entries (660 skipped) in 46ms` prints
  with no palette styling.
- The background-sync warning renders indented and unstyled
  (`  ▲ sync: …`) — a separate, pre-existing surface.
Both should align to the CLI palette (d-cpt-n0f) the way `sdd show` /
`sdd stats` now do.
