# Terminal-experience architecture — requirements

Captured from planning dialogue (Christopher, Claude) so the implementation plan starts grounded rather than re-deriving this conversation. Refines the output policy (d-cpt-rkj) and builds on the three-renderer split (d-cpt-5f4).

## Two consumer modes — never conflate them

The sdd CLI serves two fundamentally different consumers, and the terminal experience must keep them distinct:

- **Interactive TTY humans** — get the full experience: transient progress, styled feedback, leveled logging integrated into one coherent terminal UI.
- **Non-TTY consumers (agents, pipes, scripts)** — need none of the TUI. The TUI is a human affordance. They get clean, deterministic structured results on stdout and structured observability logs on stderr. No animation, no decoration, no cursor control.

Selection is by TTY detection plus `--format` — this extends d-cpt-5f4's audience-to-renderer split from output rendering to the whole progress/feedback/logging surface. NO_COLOR honored.

## Requirements

1. **Transient progress messages** — distinct from log lines. TTY path: spinner / status line. Non-TTY path: no-op, or a single plain stderr line. Examples: "fetching repo X…", "indexing N entries…".
2. **Leveled observability logging** — slog with levels (the verbose / extra-verbose tiers already exist). Human path: routed through the coordinator that owns the terminal so it never corrupts TUI redraws (the concrete failure being a naive stderr logger interleaved with charmbracelet components). Agent path: structured slog on stderr, stdout stays clean.
3. **Styled feedback components** — reusable error / success / warning renderers for the TTY path; plain or structured equivalents for non-TTY.
4. **Per-command UX design** — for each command, name what is shown transiently vs persisted, and the parallel non-TTY behaviour.

## Mechanism (CQRS-clean)

- No terminal-UI code in handlers or finders.
- Command and query structs carry optional progress/feedback **reporter callbacks**, generalizing the existing result-callback pattern (e.g. OnNewEntry).
- Handlers/finders call the reporter as they work; the **CLI layer** wires it to a TTY renderer or a plain/structured non-TTY renderer.
- A single output **coordinator** owns the terminal on the TTY path so progress, logs, and feedback don't fight over redraws.

## Rejected

- Naive stderr logger alongside charm components — corrupts redraws. This is the reason requirement 2 exists.

## Per-command UX matrix (template — fill in the plan)

| Command | TTY transient | TTY persisted | Non-TTY / agent |
|---|---|---|---|
| sdd new | preflight progress | entry id + summary | structured result, slog stderr |
| sdd search | indexing progress | ranked results | results to stdout, slog stderr |
| … | | | |

## Open questions for the plan

- Coordinator design: pause/redraw around log emission vs a dedicated log region.
- Which commands warrant progress UI vs stay silent.
- How `--format json` and non-TTY fully suppress progress and decoration.
- Whether feedback components live in `presenters/` or a new `ui/` package.
