# Inline rendering rework — design note

Corrects the d-tac-5g9 implementation to match the captured terminal-experience
requirements (d-cpt-mvb). Closes the evaluation gap (s-tac-fh9).

## Decision
Render progress INLINE, not in an alt-screen. Two zones (bubble tea v2 inline mode):
- **Durable zone (scrollback):** leveled log lines emitted via `tea.Printf` /
  `tea.Println`, which print above the program and persist as normal terminal history.
- **Ephemeral footer (View()):** spinner + determinate progress bar + count. Redrawn
  in place; bubble tea clears it on quit. Never scrolls (small, stable), so the
  "inline can't erase scrollback" worry that drove the alt-screen choice does not apply.

No `WithAltScreen()`. Footer clears on completion; the styled summary / results then
print to stdout.

## Per-command behaviour (from d-cpt-mvb's matrix)
- `sdd index`: per-entry `indexed …` lines are DURABLE (scroll through via tea.Printf,
  leveled Info); footer = spinner + bar + `N/total entries`; on done the footer clears
  and the styled summary persists on stdout.
- `sdd search` lazy-fill: indexing is TRANSIENT — footer-only progress (spinner + bar +
  count), no durable per-entry lines; footer clears, ranked results render clean. Agent
  path unchanged: Warn floor, silent.

## What changes in the tui layer
- Drop alt-screen; drop the viewport + display ring buffer.
- Model log handling: instead of pushing entries into a viewport, emit each
  display-eligible entry as a `tea.Printf` command (durable) for index; for search,
  entries are not printed (transient) — still observed by the recorder for keep /
  fingers-crossed re-emit.
- View() returns only the footer (spinner + bar + count), no AltScreen.
- Durable-vs-transient logs is a View/policy flag the caller sets (index = durable,
  search = silent live logs).

## What stays (unchanged from the committed slices)
- cliout core: NewLogPipe / LogConsumer, Reporter, Policy + Recorder, WriteEntries, IsInteractive.
- CQRS boundary: handlers/finders log via the context logger only; OnBatchStart on BuildIndexCmd.
- Launch-when-there-is-work gate (Manifest.PendingCount); non-TTY/agent paths.
- Interactive[T] coordinator shell (logger swap/restore, work goroutine, teardown re-emit) —
  it just runs an inline program instead of an alt-screen one.

## Styling
Summary / success / warning lines go through the CLI palette (d-cpt-n0f), matching
sdd show / sdd stats. Addresses the unstyled-summary half of the gap.

## Tests
- Keep the cliout-core unit tests (unaffected).
- Rework the model tests: assert durable log lines emit as tea.Printf commands (index)
  vs suppressed (search), footer content, clear-on-done.
- Keep a teatest/v2 inline smoke (no alt-screen): logs land in output history, footer
  clears, program quits on done.

## Open for the implementer
- Exact wiring of "emit durable line" from the model loop: the log consumer's entries
  become tea.Printf commands in Update; confirm ordering vs the footer redraw.
- Confirm tea.Printf in inline mode interleaves cleanly with the spinner/bar redraw
  (research says yes; verify on the live TTY).
