package cliout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/networkteam/slogutils"
)

// ErrUserCancelled is the sentinel a coordinator returns when the user
// interrupts the work (ctrl+c cancels the work context). The top-level CLI
// handler maps it to a calm "cancelled." message and exit 130 — no raw
// "context canceled" string ever reaches the user.
var ErrUserCancelled = errors.New("cancelled by user")

// armDebounce is how long the coordinator waits, after the first
// display-worthy event, before starting a bubble tea program. Work that
// completes inside the window flushes plainly and never starts a program, so
// instant operations produce no footer flash and no terminal escape leak.
const armDebounce = 70 * time.Millisecond

// lifecycleState is the coordinator's explicit display state.
//
//	dormant → armed → live → done
type lifecycleState int

const (
	stateDormant lifecycleState = iota
	stateArmed
	stateLive
	stateDone
)

// DisplayStarter runs the live terminal program. The coordinator owns the
// decision to start it (armed debounce expiry) and calls Start once, on the
// driver goroutine, so it blocks there until the program exits. backlog holds
// the display lines buffered before the program existed (its opening backlog);
// live delivers subsequent lines, and its Close signals end-of-work so the
// program quits. Implemented by internal/cliout/tui — this interface keeps
// cliout free of any bubble tea import.
type DisplayStarter interface {
	Start(backlog []LogEntry, live *LogConsumer) error
}

// Coordinator owns the terminal-output lifecycle for one long-running command.
// It is the slog.Handler installed on the work context (once, via
// slogutils.WithLogger — no global logger swap), routing each record by state:
// plain to stderr while dormant, buffered while armed, and into the running
// program while live. The Recorder observes every record for the teardown
// re-emit, independent of what the display does.
type Coordinator struct {
	policy     Policy
	stderr     io.Writer
	streamLogs bool
	debounce   time.Duration
	starter    DisplayStarter

	mu      sync.Mutex
	state   lifecycleState
	rec     *Recorder
	backlog []LogEntry
	timer   *time.Timer
	live    *LogConsumer

	cancel context.CancelFunc

	startCh   chan struct{} // fires when armed→live: the driver starts the program
	doneCh    chan struct{} // fires when work finishes without a program
	startOnce sync.Once
	doneOnce  sync.Once
}

// CoordinatorConfig configures a Coordinator. Debounce defaults to armDebounce
// when zero; Stderr defaults to a no-op writer when nil.
type CoordinatorConfig struct {
	Policy     Policy
	Stderr     io.Writer
	StreamLogs bool
	Debounce   time.Duration
	// Progress, when set, arms the coordinator on its first published event so
	// the footer appears for progress-only work (no display-eligible log yet).
	Progress *Reporter
}

// NewCoordinator builds a dormant coordinator. Call SetStarter before Run.
func NewCoordinator(cfg CoordinatorConfig) *Coordinator {
	debounce := cfg.Debounce
	if debounce == 0 {
		debounce = armDebounce
	}
	stderr := cfg.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	c := &Coordinator{
		policy:     cfg.Policy,
		stderr:     stderr,
		streamLogs: cfg.StreamLogs,
		debounce:   debounce,
		rec:        NewRecorder(cfg.Policy),
		startCh:    make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
	if cfg.Progress != nil {
		cfg.Progress.Notify(c.Arm)
	}
	return c
}

// SetStarter injects the live-program starter (provided by internal/cliout/tui).
func (c *Coordinator) SetStarter(s DisplayStarter) { c.starter = s }

// Interrupt cancels the work context. The live program's ctrl+c path calls it;
// the resulting context.Canceled from work is translated to ErrUserCancelled.
func (c *Coordinator) Interrupt() {
	if c.cancel != nil {
		c.cancel()
	}
}

// Run installs the coordinator as the context logger, launches work on its own
// goroutine, and drives the lifecycle: it starts the program if the armed
// debounce expires, or returns without one if work finishes first. On teardown
// it re-emits kept / fingers-crossed entries to the durable sink for
// footer-only views, exactly as before. A context.Canceled from work surfaces
// as ErrUserCancelled.
func (c *Coordinator) Run(ctx context.Context, work func(context.Context) error) error {
	durable := slogutils.FromContext(ctx)

	logger := slog.New(newPipeHandler(c.policy.CaptureFloor(), c.handle))
	workCtx, cancel := context.WithCancel(slogutils.WithLogger(ctx, logger))
	defer cancel()
	c.cancel = cancel

	workErr := make(chan error, 1)
	go func() {
		err := work(workCtx)
		workErr <- err
		c.finishWork()
	}()

	var runErr error
	select {
	case <-c.startCh:
		c.mu.Lock()
		backlog, live := c.backlog, c.live
		c.mu.Unlock()
		runErr = c.starter.Start(backlog, live)
	case <-c.doneCh:
		// Work finished before a program was needed (dormant or armed within
		// the debounce window); nothing was rendered by a program.
	}

	err := <-workErr

	// Footer-only (transient) views never showed their logs live, so surface
	// the kept / fingers-crossed entries on teardown. Streaming views already
	// persisted their display-eligible lines, so re-emitting would duplicate.
	if !c.streamLogs {
		if err != nil {
			c.rec.MarkFailed()
		}
		WriteEntries(ctx, durable, c.rec.Flush())
	}

	switch {
	case errors.Is(err, context.Canceled):
		return ErrUserCancelled
	case err != nil:
		return err
	default:
		return runErr
	}
}

// handle routes one snapshotted record through the lifecycle. It runs on the
// work goroutine (slog is synchronous), so state transitions are guarded by mu.
func (c *Coordinator) handle(e LogEntry) {
	c.mu.Lock()
	c.rec.Observe(e)

	// A display-eligible record is display-worthy (it arms the coordinator);
	// it is also surfaceable — rendered as a durable line — only for streaming
	// views. Footer-only views arm (to show progress chrome) but never print
	// their log lines except via the teardown re-emit.
	displayWorthy := c.policy.ShowInDisplay(e.Level)
	surfaceable := c.streamLogs && displayWorthy

	switch c.state {
	case stateDormant:
		if surfaceable {
			c.renderPlain(e)
		}
		if displayWorthy {
			c.arm()
		}
		c.mu.Unlock()
	case stateArmed:
		if surfaceable {
			c.backlog = append(c.backlog, e)
		}
		c.mu.Unlock()
	case stateLive:
		live := c.live
		c.mu.Unlock()
		if surfaceable {
			live.Offer(e)
		}
	default: // stateDone
		c.mu.Unlock()
	}
}

// Arm is exported for the progress mailbox to arm the coordinator on the first
// progress event even when no display-eligible log has arrived yet.
func (c *Coordinator) Arm() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == stateDormant {
		c.arm()
	}
}

// arm transitions dormant→armed and starts the debounce timer. Caller holds mu.
func (c *Coordinator) arm() {
	c.state = stateArmed
	c.timer = time.AfterFunc(c.debounce, c.debounceFired)
}

// debounceFired transitions armed→live and signals the driver to start the
// program. A no-op if work already finished (state left armed).
func (c *Coordinator) debounceFired() {
	c.mu.Lock()
	if c.state != stateArmed {
		c.mu.Unlock()
		return
	}
	c.state = stateLive
	c.live = NewLogConsumer(logChanCap)
	c.mu.Unlock()
	c.startOnce.Do(func() { close(c.startCh) })
}

// finishWork resolves the lifecycle when work returns. Runs on the work
// goroutine after the result is sent, so the driver's <-workErr sees a fully
// observed recorder.
func (c *Coordinator) finishWork() {
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
	}
	switch c.state {
	case stateDormant, stateArmed:
		// No program will start: flush any buffered surfaceable lines plainly.
		backlog := c.backlog
		c.backlog = nil
		c.state = stateDone
		c.mu.Unlock()
		for _, e := range backlog {
			c.renderPlain(e)
		}
		c.doneOnce.Do(func() { close(c.doneCh) })
	case stateLive:
		live := c.live
		c.state = stateDone
		c.mu.Unlock()
		live.Close() // the program drains its tail and quits
	default: // stateDone
		c.mu.Unlock()
	}
}

// renderPlain writes one entry as a styled line to stderr — the dormant /
// armed-flush display path, no program involved.
func (c *Coordinator) renderPlain(e LogEntry) {
	fmt.Fprintln(c.stderr, RenderEntry(e))
}
