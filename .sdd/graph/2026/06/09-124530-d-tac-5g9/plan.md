# `sdd index` / lazy-fill: interactive terminal output

Realizes the per-command terminal-experience implementation deferred by the terminal-experience architecture directive (d-cpt-mvb), for `sdd index` and `sdd search` lazy-fill.

## Problem

Both commands wire their progress callbacks to raw `fmt.Fprintf(os.Stderr, …)`:

- `sdd index` prints `  indexed <id> (N chunks)` / `  skipped <id>` per entry on stderr, then a summary on stdout.
- `sdd search` lazy-fill prints `  lazy-indexed <id> (N chunks)` per filled entry, right before the search results — burying them.

This is the naive logging d-cpt-mvb set out to replace. The handler is already CQRS-clean (logs via `slogutils.FromContext(ctx)`, reports progress via callbacks); only the CLI wiring is wrong.

## Boundary (CQRS)

Producers stay oblivious. `internal/handlers` and `internal/finders` log only through the context logger and never import the coordinator. The CLI surface (`cmd/sdd`) alone decides whether that logger drives a transient TUI (interactive terminal) or the plain leveled stderr handler (agents, pipes). This is the line d-cpt-mvb draws — terminal-UI code out of handlers and finders.

`sdd search --query` is the driving case: the work is *prelude handler (LazyFill) + pure finder (Search)*, and neither knows a TUI exists.

## Package layout

- `internal/cliout` — **bubble-tea-free** core: `logEntry`, `NewLogPipe`, the slog handler, `LogConsumer`, `Policy`, `Reporter`. Unit-testable without a program.
- `internal/cliout/tui` — imports bubble tea: the model (spinner + progress + viewport), the `recvCmd` adapter, `Interactive[T]` + the run loop.

Imported only by `cmd/sdd`.

## The log pipe

```go
type logEntry struct {
    Time    time.Time
    Level   slog.Level
    Message string
    Attrs   []slog.Attr   // snapshot; slog.Record must not be retained past Handle
}

func NewLogPipe(displayLevel slog.Leveler) (slog.Handler, *LogConsumer)
```

The handler holds `chan<- logEntry`; the consumer holds `<-chan logEntry` + a done channel. `NewLogPipe` creates the channel and injects each end — neither references the program, so there is no construction cycle.

`Handle` snapshots the record (level, time, message, attrs incl. accumulated `WithAttrs`), then sends non-blocking:

```go
select {
case h.ch <- e: // queued
default:         // full → drop; never block the producer
}
```

`WithAttrs` must accumulate (the codebase uses `…FromContext(ctx).With("command", …)`); returning the handler unchanged would silently drop the command tag.

## Decoupled consumer

```go
func (c *LogConsumer) Recv() (logEntry, bool) {  // ok=false when finished
    select {
    case e := <-c.ch: return e, true
    case <-c.done:    return logEntry{}, false
    }
}
```

bubble tea adapter (only in `internal/cliout/tui`):

```go
func recvCmd(c *cliout.LogConsumer) tea.Cmd {
    return func() tea.Msg {
        if e, ok := c.Recv(); ok { return logMsg(e) }
        return doneMsg{}
    }
}
```

## Two goroutines, one channel, one ring buffer

```
  Goroutine W (work)                       Goroutine T (bubble tea loop)
  ──────────────────                       ────────────────────────────
  LazyFill / Build
    logger.Info(...) ─┐
      Handle()        │   buffered chan (cap ~64)
        select{ch<-e; │   ┌────────┐
               default} ──┼──►│ □ □ □  │──► recvCmd → Update(logMsg):
    keep working  ◄───┘   └────────┘        ring.push(e)  // last ~256
                                            vp.SetContent(render(ring))
                                            re-issue recvCmd
                                          View(): spinner + bar + viewport → stderr (alt-screen)
```

- **Display ring buffer**: last ~256 entries, in the model. Bounded memory; bounded redraw (re-render on change/resize; `viewport` shows a screenful). Structured entries, styled at render time (level badge + message + `key=value`, charm/log layout, `palette.go` colors).
- **Non-blocking send** bounds *time coupling* (W never waits on T). **Ring buffer** bounds *space / redraw cost*. Different boundaries, both needed.
- **Absolute progress**: the `Reporter` sends `done/total` snapshots, so a dropped progress message self-corrects.

## Closing

The data channel is **never closed** (unknown producers could panic on send-to-closed). A separate `done` channel signals end-of-work; `Recv` / `recvCmd` select on both. The coordinator closes `done` after `work` returns → model quits → teardown.

## Model & teardown

One program, two producers (logs via `LogConsumer`, progress via `Reporter`) plus the spinner tick:

```go
func (m model) Init() tea.Cmd {
    return tea.Batch(recvCmd(m.logs), recvProgress(m.prog), m.spinner.Tick)
}
```

**Teardown = alt-screen** (verified against bubble tea v2): inline mode does *not* erase lines already in scrollback, and v2 has no inline-cleanup API. So the transient UI runs in **alt-screen on stderr** (`tea.WithAltScreen()` + `tea.WithOutput(os.Stderr)`); on quit the alt buffer drops and the prior screen restores intact — the UI vanishes, stdout untouched. Avoid `tea.Printf` / `tea.Println` (unmanaged, persistent).

**Launch only when there is work**: index knows `len(work)`, lazy-fill knows the missing count. Zero work → skip the program entirely (no alt-screen flash); take the plain path.

## Progress reporter (generic)

```go
type Reporter struct { /* chan progressMsg */ }
func (r *Reporter) SetTotal(n int)
func (r *Reporter) Add(n int)        // sends absolute snapshot
```

Generic `done/total` (+ optional unit label for "42/120 entries"). Not indexing-specific — the index mapping (`OnBatchStart`/`OnEntryIndexed` → reporter) lives in the `cmd/sdd` bridge, keeping `cliout` reusable and the `command` package dependency-free.

## Policy (durable record vs ephemeral display)

```go
type Policy struct {
    Display        slog.Leveler    // live view level (chattier; it vanishes)
    KeepAtOrAbove  slog.Level      // re-emitted to the durable sink on teardown
    FingersCrossed *FingersCrossed
}
type FingersCrossed struct {
    Trigger slog.Level // e.g. Error
    Tail    int        // last N entries (ALL levels) flushed when triggered
}
```

- **Display** governs the live viewport (ephemeral).
- **KeepAtOrAbove** entries are re-emitted to the **durable sink** on teardown — the durable sink is the logger that was `slog.Default()` *before* `Interactive` swapped it (captured on entry, restored on exit). No new plumbing.
- **FingersCrossed** keeps its **own** ring of the last `Tail` entries at *all* levels (separate from the Display-filtered ring), so an error can dump context that was never shown.

Teardown order: quit program (alt-screen restored) → re-emit kept / fingers-crossed entries to stderr (normal screen) → caller prints the result to stdout.

## Two level floors

- **Agent / non-TTY**: configured floor — Warn default; explicit `sdd index` raises its floor to Info; `-v`/`-vv` raise further.
- **TTY display**: chattier (e.g. Info) regardless, since it is ephemeral. So a *user* watches lazy-fill lines scroll by and vanish; an *agent* running the same command sees nothing.

## API & flow

```go
func Interactive[T any](
    ctx context.Context, displayLevel slog.Leveler, view View,
    work func(ctx context.Context) (T, error),
) (T, error)

type View struct {
    Label    string    // "indexing", "searching"
    Progress *Reporter // nil → spinner only
}
```

`Interactive` builds the pipe, points ctx's logger **and** slog default at it, drives the program on this goroutine while `work` runs on another, closes `done` when work returns, tears down, restores the logger, returns the result. `work` receives the TUI-logger ctx.

**`sdd search --query`:**

```go
work := func(ctx context.Context) (*query.SearchResult, error) {
    if needsVector {
        if err := ih.LazyFill(ctx, &command.LazyFillIndexCmd{}); err != nil { return nil, err }
    }
    return finder.Search(ctx, sq)
}
if cliout.IsInteractive(os.Stderr) && willFill {           // launch only when there's work
    res, err = clitui.Interactive(ctx, slog.LevelInfo, clitui.View{Label: "searching"}, work)
} else {
    res, err = work(ctx)                                    // no program; logs on the leveled stderr handler
}
presenters.RenderSearch(os.Stdout, res, g)                  // result to clean stdout, after teardown
```

**`sdd index`** adds the reporter wiring:

```go
reporter := cliout.NewReporter()
buildCmd := &command.BuildIndexCmd{
    Force:          force,
    OnBatchStart:   func(ids []string) { reporter.SetTotal(total) },
    OnEntryIndexed: func(id string, n int) { reporter.Add(1) },
    OnEntrySkipped: func(id string) { skipped++ },
    OnComplete:     func(indexed, skipped int) { summary = … },
}
view := clitui.View{Label: "indexing", Progress: reporter}
_, err := clitui.Interactive(ctx, slog.LevelInfo, view,
    func(ctx context.Context) (struct{}, error) { return struct{}{}, h.Build(ctx, buildCmd) })
// after teardown: print summary to stdout
```

Same coordinator; index just wires `View.Progress`. The new `OnBatchStart(ids []string)` callback fires before each `EmbedDocuments` round-trip so the bar advances per batch, not in one end-of-run jump.

## Per-command behavior

|                | `sdd index` (explicit)                                              | `sdd search` lazy-fill (incidental)                  |
|----------------|---------------------------------------------------------------------|------------------------------------------------------|
| TTY            | spinner + determinate bar + scrolling log; vanishes; summary stdout | spinner + scrolling log; vanishes; results stdout    |
| Agent          | Info indexed lines + Debug-collapsed skips on stderr; summary stdout| silent at default (Debug reveals); results on stdout |
| Level floor    | Info                                                                | Warn                                                 |

## Reusability

`Interactive[T]` + `cliout` serve any long-running command — future bulk operations, the MCP server's batch work. This plan is the first consumer and sets the pattern.

## Settled decisions (former open questions)

- **Teardown**: alt-screen on stderr (inline can't erase scrollback in bubble tea v2).
- **Launch only when there's work** (skip the alt-screen flash on no-op runs).
- **Display ring**: last ~256 entries. **Fingers-crossed ring**: last ~50, all levels, separate.
- **Viewport height**: up to ~10 lines, capped to terminal height minus the header.
- **Durable sink**: the pre-swap `slog.Default()`.
- **Testing**: unit-test the tea-free core + the model's `Update`/`View` transitions; `teatest/v2` (experimental) only for a thin integration smoke.

## Alternatives rejected

- **Compose one handler with TUI + stderr branches** — the TTY/non-TTY fork picks exactly one.
- **Blocking channel send** — render speed would throttle indexing.
- **slog-only cleanup, no coordinator** — the goal is the designed experience, not just silence.
- **Pre-rendered joined log strings** — lose slog structure, break re-wrap on resize.
- **Closing the data channel** — send-on-closed panic with unknown producers.
- **Inline + empty final View for teardown** — does not erase scrollback in v2 (verified).
