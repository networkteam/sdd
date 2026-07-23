# Design record: output-coordinator lifecycle + composable TUI architecture

Exploration record behind the coordinator-lifecycle directive. Two parallel Opus explorations (2026-07-23): a full architectural map of the implemented coordinator, and a source-level analysis of bubbletea v2 mechanics plus an architecture-pattern survey. Synthesis and dialogue conclusions at the end.

---

## Part 1 — Architectural map of the implemented coordinator

### Dependency versions (go.mod)

All Charm libs are the `charm.land` v2 GA line (vanity path, not `github.com/charmbracelet`):
- `charm.land/bubbletea/v2 v2.0.7`
- `charm.land/bubbles/v2 v2.1.0` (spinner, progress, textinput)
- `charm.land/lipgloss/v2 v2.0.3`
- `charm.land/glamour/v2 v2.0.0`
- renderer engine: `github.com/charmbracelet/ultraviolet` (indirect — screen diff/write, `uv.TerminalRenderer`)
- supporting: `colorprofile v0.4.3`, `x/term v0.2.2`, `x/exp/teatest/v2` (test only)

### internal/cliout/ — the bubbletea-free core

Package contract (logpipe.go:1-14): producers log only through the context logger and never import this package; the CLI installs the pipe handler when stderr is interactive.

- **tty.go** — `IsInteractive(f *os.File)` (tty.go:16): the single TTY gate (nil / NO_COLOR / term.IsTerminal).
- **logpipe.go** — slog → display bridge. `LogEntry` snapshot (:34); `NewLogPipe` (:47) builds `pipeHandler` + `LogConsumer` over a 64-slot buffered channel. `Handle` is **non-blocking drop-on-full** (:84-87). `Recv` prefers buffered entries over the done signal so the tail flushes (:144-155).
- **reporter.go** — absolute-count progress mailbox. `Progress{Done,Total,Unit,Note}` (:8), `Ratio()` clamped (:16). `Reporter` (:35): single-producer, capacity-1 channel, latest-wins `publish()` (:92). `SetTotal/SetUnit/SetNote/Add` (:56-88).
- **policy.go** — durable-vs-ephemeral policy + recorder. `Policy{Display, KeepAtOrAbove, FingersCrossed}` (:12); `CaptureFloor()` (:49); `ShowInDisplay` (:59). `Recorder` (:67) with `Observe` (:91), `MarkFailed` (:115), `Flush` (:123, dedup by seq). `WriteEntries` (:153) replays kept entries through the pre-swap durable logger.

### internal/cliout/tui/ — the bubbletea layer (only cmd/sdd imports it)

**interactive.go (126 lines)** — `View{Label, Progress, StreamLogs}` (:23); `programRunner`/`runReal` (:41-49, swappable for tests); `Interactive[T]` → `interactiveWith[T]` (:54-126), the lifecycle driver:

1. Capture `durable = slog.Default()` (:70); build pipe + recorder (:72-73).
2. **Swap global logger** `slog.SetDefault` to pipe handler (:75-76); `defer` restore (:77).
3. Derive `workCtx` with cancel; install pipe logger via `slogutils.WithLogger` (:79).
4. `newModel(...)` with `cancel` as interrupt (:82).
5. **Launch work goroutine (:89-97) before `run(m)` (:99)** — work runs, then `consumer.Close()` + `Progress.Close()`, sends to `outCh`.
6. `run(m)` blocks in the tea program; quits when log consumer closes.
7. `<-outCh` as happens-before barrier (:100); prefer final model's recorder (:104-106).
8. Teardown re-emit only when `!StreamLogs` (:111-115).
9. Error precedence: work error > run error > value (:118-125).

**model.go (186 lines)** — messages `logMsg/logDoneMsg/progressMsg/progressDoneMsg` (:15-20). `recvLog/recvProgress` (:73, :82) adapt tea-free consumers into `tea.Cmd`s. `Init()` batches recvLog + spinner.Tick + optional progress (:91). `Update()` (:99): WindowSizeMsg → width only (:101-106); ctrl+c → `m.interrupt()` + Quit (:108-114); `logMsg` → `rec.Observe`; **if `streamLogs && ShowInDisplay` → `tea.Printf`** (:117-124); `logDoneMsg` → Quit (:126-128, the only work-completion quit trigger); `progressDoneMsg` → no-op (:138). `View()` renders a single inline footer line (:161-179); no AltScreen anywhere.

**styles.go (72 lines)** — TUI-local lipgloss styles + `renderEntry` (:44, used by tea.Printf) + `renderCount` (:59). Comment :13-17 documents deliberate duplication vs `internal/presenters` palette.

### Command wiring (all in cmd/sdd; shared policy `transientViewPolicy()` search.go:629-635 = Display Info, Keep Warn, FingersCrossed{Error, 50})

| Command | Site | View | Progress | StreamLogs | TTY gate |
|---|---|---|---|---|---|
| repo add | repo.go:78-79 | `{Label:"connecting"}` | none | true | `IsInteractive(stderr) && willClone` (repo.go:65-77) |
| index | search.go:309-310 | `{Label:"indexing", Progress}` | yes | true | `IsInteractive && (localPending>0 \|\| crossRepo)` (search.go:308) |
| search (vector) | search.go:592,601 | `{Label:"indexing", Progress}` | yes | false | `IsInteractive && (willFill \|\| crossRepo)` (search.go:511) |
| search (text-only cross-repo) | search.go:599,601 | `{Label:"fetching repos"}` | none | true | same |

Progress callbacks (CQRS command structs → Reporter): `OnPlanned`→SetTotal, `OnBatchStart`→SetNote, `OnEntryIndexed`→Add, `OnRepoStart`→SetNote("indexing "+id) — wired per call site (search.go:241-291, 512-585; repo.go:47-79). Handlers thread callbacks unchanged (handler_repo.go:459-523, :429-447); MCP passes nil.

### Exact failing call path (repo add clone)

repo.go:33 Action → `willClone` pre-check (repo.go:65-72) → `clitui.Interactive(...)` (repo.go:78) → logger swap + workCtx (interactive.go:75-79) → **work goroutine launched (:89) before run(m) (:99)** → `h.RepoAdd` (handler_repo.go:39) → `EnsureCloned` (manager.go:51) → **`logger.Info("cloning connected repo", …)` (manager.go:55-56)** → pipe → `logMsg` → `tea.Printf` (model.go:122) — potentially before the program's first frame. No readiness handshake exists.

### Other TUI surfaces

- Six hand-rolled bubbletea prompt models in cmd/sdd/main.go (graphDir :1134, participant :1192, language :1247, scope :1306, agents :1384, confirm :1533) — near-identical textinput wrappers, no shared base, bare `tea.NewProgram` (no stderr redirect, no signal-handler override), gated on `isTerminal(os.Stdin)`.
- `internal/presenters/`: `palette.go` (shared CLI palette), `feedback.go` `RenderResultLine` (:20, used post-teardown by repo add + index), `show_styled.go`, `stats.go`.
- **Two independent palettes** (presenters/palette.go vs tui/styles.go), deliberate but unshared.

### Stale comment

interactive.go:43-49 claims output is "alt-screen" and "AltScreen is declared by the model's View" — false: `View()` uses `tea.NewView` inline, zero `WithAltScreen`/`AltScreen` uses in the repo; model doc (model.go:22-23, :158-160) states inline/no-alt-screen. Real teardown is inline footer-clearing.

### Structural weaknesses (verbatim from the map)

1. **No readiness signal / implicit start-ordering contract** — work launches before first frame; nothing couples "footer rendered" to "work may log". Root of s-tac-bnw.
2. **Global mutable logger swap** — `slog.SetDefault` + defer restore; ordering vs teardown/Ctrl-C implicit; nested/concurrent coordinators would clobber.
3. **Quit tied to one channel only** — `logDoneMsg` alone quits; missed close hangs; too-fast close (fast path) = start+quit within a frame = DEC mode-query escape leak (s-tac-bg7). Mitigation is ad-hoc per-command gating.
4. **Per-command duplication of view assembly** — labels are bare strings at call sites; note callbacks hard-code "indexing"; the s-tac-jbm mislabel is structural.
5. **Drop-on-full log channel** — 64-slot channel silently discards under back-pressure.
6. **Ctrl-C error surfacing spread across layers** — humanized only by `errors.Is(err, context.Canceled)` at main.go:315-318; any wrap breaks it. No coordinator-owned sentinel.
7. **Stale alt-screen comment as load-bearing guidance.**
8. **Two divergent TUI stacks** — coordinator vs init prompts; different program options, different TTY gates, two palettes.

---

## Part 2 — bubbletea v2 mechanics (source-verified, v2.0.7) + upstream + patterns

### Why the scroll happens (root cause, exact)

- `tea.Printf/Println` are Cmds returning `printLineMessage` (renderer.go:59-92), handled synchronously: `p.renderer.insertAbove(msg.messageBody)` (tea.go:861-862).
- `insertAbove` (cursed_renderer.go:707-763): `w, h := s.cellbuf.Width(), s.cellbuf.Height()`; `down := h - y - 1`; emits `ansi.CursorDown(down)` = `ESC[<n>B` (:716-723). Assumes `h` = footer frame height — but `h` is the cellbuf height.
- The cellbuf is created at **full terminal size** (`newCursedRenderer` → `uv.NewScreenBuffer(w,h)`, cursed_renderer.go:46, size from `term.GetSize` in Run, tea.go:1045-1066) and is **only resized to the view's height inside `flush()`** (`frameHeight := content.Height()`; `s.cellbuf.Resize(...)`, cursed_renderer.go:276, 295-306). `render()` only stashes the view (:579-584); `resize()` does not touch cellbuf (:618-631).
- Flushes run on a ticker goroutine at 1s/fps (~16.6ms at default 60fps) started by `startRenderer` (tea.go:1393-1422, flush at :1417-1418). The first Printf can traverse Init→handleCommands→Send→eventLoop→insertAbove well inside that window. **No happens-before edge between first flush and first insertAbove; WindowSizeMsg does not imply a flush.** Higher FPS (max 120) shrinks but never closes the window.
- No readiness hook exists: renderer interface has no callback, flush is unexported, startup msgs (ColorProfileMsg, WindowSizeMsg, EnvMsg) are sent via `go p.Send` before any flush necessarily ran. `WithWindowSize` only seeds p.width/height — does not pre-resize cellbuf. Sending an explicit WindowSizeMsg calls resize, which skips cellbuf. All confirmed non-starters.

### Escape leak on fast quit (s-tac-bg7 mechanics)

`shouldQuerySynchronizedOutput(environ)` (tea.go:960-986) is true for most modern terminals; Run then buffers `RequestModeSynchronizedOutput + RequestModeUnicodeCore` (DEC 2026/2027 DECRQM, tea.go:1109-1115), flushed on the ticker. Terminal DECRPM replies arrive async on stdin, normally consumed as `ModeReportMsg` (tea.go:786-798). On fast quit `shutdown` (tea.go:1241-1264) closes the input reader with **no drain step** — pending replies land on the shell prompt. No `WithoutCapabilityQueries` option and no drain exist in v2.0.7.

### Ctrl-C path

sdd uses `WithoutSignalHandler`; model maps ctrl+c KeyPressMsg → cancel workCtx + Quit. Work returns `context.Canceled` → `Interactive` returns it → tamed only by exact `errors.Is` at main.go:315 ("cancelled.", exit 130). Thin contract: any wrapped error dumps raw.

### Upstream status

- **#1627** "[v2] Terminal Escape Sequence Leak in Short-Lived Programs" — exactly the DEC 2026/2027 leak; proposed WithoutCapabilityQueries / drain; **nothing shipped in v2.0.7**. https://github.com/charmbracelet/bubbletea/issues/1627
- **#1666** "Add tea.PrintlnRaw to bypass insertAbove rendering" — confirms insertAbove cursor-arithmetic fragility; the specific first-render race is **not tracked upstream**. https://github.com/charmbracelet/bubbletea/issues/1666
- Context: #1384, discussion #1482 (scrollback printing fragility); older #297, #1004 (frame-height/window coupling).
- Conclusion: maintainers acknowledge the family; no option, no drain, no readiness hook as of v2.0.7. The one-line upstream fix would be syncing cellbuf height in `render()`/`resize()` or clamping `down` in `insertAbove` to view content height.

### Architecture patterns surveyed

- **Docker buildx `util/progress`/`progressui`**: one long-lived Printer/Display owns the terminal; callers push typed status events over a channel; DisplayMode (auto/tty/plain/quiet) selected by TTY detection; non-tty collapses to ordered plain lines. Strongest reference; maps ~1:1 onto sdd's Reporter/LogConsumer mailboxes.
- **gh CLI `iostreams`**: central IO façade owns the single progress indicator; commands call Start/StopProgressIndicator; TTY detection in one place. Model for "commands declare intent, one owner renders".
- **Writer+mutex+refresh (mpb, schollz/progressbar, charm log)**: coordinator owns the writer behind a mutex; footer via erase/write/redraw. Full control of frame height — but hand-owned ANSI discipline, no bubbletea input handling or teatest harness.
- **Program-per-phase**: worst shape here — every start/stop re-incurs both lifecycle edges.
- **"Delay output until ready" idiom**: buffer in model, gate on first WindowSizeMsg — community-standard but must be hardened with a frame-margin tick since WindowSizeMsg ≠ flush.

### Mechanisms ranked (verbatim ranking)

1. **Lazy/debounced program start** — coordinator dormant; first real work event (after ~50-80ms debounce) starts the program; pre-start lines go plainly to stderr; instant ops never start a program. Kills scroll race for the fast case, the 2026/2027 leak, and the footer flash in one move. No fork.
2. **Buffer Printf-bound lines in the model; flush after first-paint proxy** — hold display-eligible lines; on first WindowSizeMsg schedule `tea.Tick(2-3× frame interval)` → flush buffered via tea.Printf, then pass-through. Probabilistic-with-margin rather than hard happens-before; ~15 lines in the model; no fork. Best effort/safety ratio.
3. **WithFPS(120)** — cheap stacked mitigation only; never alone.
4. **Escape leak: avoid the query** — lazy start (query never sent for instant ops); env-manipulation via WithEnvironment rejected as hacky; post-Run stdin drain racy.
5. **Fork/patch bubbletea** — the only hard fix of the root cause (sync cellbuf in resize/render, or clamp down in insertAbove). Rejected by default: replace-directive fork, rebase on every upstream release, breaks `go install pkg@version`. File upstream instead; keep defense-in-depth locally.

Testing: `teatest/v2` (already a dep) can drive the model with fixed WithWindowSize and assert no large `ESC[<n>B` is emitted before the first frame — makes the race regressible.

---

## Part 3 — Dialogue synthesis (agreed design)

**Lifecycle: dormant → armed → live → done, coordinator-owned.**
- The coordinator implements `slog.Handler` and routes records **by lifecycle state**: dormant = render display-eligible lines plainly to stderr, recorder as usual; armed (debounce ~60-80ms after first display-worthy event) = buffer, flush plainly if work finishes inside the window, else start the program and hand the buffer over as the model's opening backlog; live = model buffers until first-paint gate (WindowSizeMsg + one frame tick), then tea.Printf and pass-through; done = fingers-crossed re-emit for non-streamed views as today.
- Handler installed once via `slogutils.WithLogger` on the work ctx — **no `slog.SetDefault` swap**, no global mutable state, no wrong-handler window. The routing moves, not the installation.
- The coordinator owns the start decision: per-command `willClone`/`localPending`-style gates disappear; instant operations never start a program (kills the s-tac-bg7 fast-path leak and the flash structurally).
- Ctrl-C becomes a coordinator-owned cancelled-by-user sentinel error, not a raw `context canceled` caught by exact match in main.

**Rejected alternatives (with reasons):**
- *Log lines as model/View state*: inverts durability — frame content is transient, capped, lost on quit or beyond the viewport; terminal scrollback is the correct infinite durable store; frames taller than the terminal mis-render. The model is a **staging area, not the destination**: transient state renders in the View; durable lines pass through the model into scrollback.
- *Writer+mutex, no bubbletea*: defensible fork-in-the-road, rejected in favor of hardening the current shape (keeps bubbletea input handling, teatest, and the existing adapter).
- *Fork bubbletea*: rejected by default (maintenance surface); file the one-line cellbuf fix upstream referencing #1666/#1627; local buffering stays as defense-in-depth regardless.
- *WithWindowSize / explicit WindowSizeMsg / FPS alone*: verified non-starters (see Part 2).

**Component layer.** Commands declare views composed from reusable components (spinner+label, determinate bar, phase indicator, log stream). Work reports **typed phase events** (connecting → syncing → indexing) through the reporter mailbox; footer labels derive from the reported phase, never from a bare string at the call site — fixes s-tac-jbm by construction and realizes d-cpt-dgk's promised "reusable styled feedback" + "per-command UX design". Logs and progress stay two deliberate channels: slog records = durable narrative; reporter = transient state. Never route progress through log records; never derive durable output from footer state.

**One TUI foundation.** Shared program runner (options, TTY gating, output target) for coordinator + init prompts; one palette shared with presenters.

**Accepted trade-off:** armed-state buffering delays a display-eligible line by up to ~80ms — imperceptible on stderr; the price of never starting a program for instant work.

**Also:** correct the stale alt-screen comment (interactive.go:43-49); teatest regression asserting no full-height cursor-down before first frame.
