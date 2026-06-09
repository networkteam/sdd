// Package tui drives the transient interactive terminal view for long-running
// sdd commands. It imports bubble tea and depends on the bubble-tea-free core
// in internal/cliout; only cmd/sdd imports this package. Interactive is the
// reusable entry point: it installs the log pipe for the duration of an
// operation, runs the work on its own goroutine while a bubble tea program
// renders progress and a scrolling log tail, and tears the view down on
// completion so only the result reaches stdout — realizing the per-command
// terminal experience the architecture directive (d-cpt-mvb) defers.
package tui

import (
	"context"
	"log/slog"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/cliout"
)

// View configures the transient display for one operation.
type View struct {
	// Label is the operation name shown beside the spinner, e.g. "indexing".
	Label string
	// Progress, when set, drives a determinate bar; nil shows the spinner
	// alone. The caller advances it from the work goroutine via absolute
	// counts (SetTotal/Add).
	Progress *cliout.Reporter
	// StreamLogs, when true, emits display-eligible log entries as durable
	// lines above the footer (they scroll into terminal history) — the
	// "indexing logs persist" case. When false the live view is footer-only
	// and logs stay hidden, surfaced only by the teardown re-emit on a
	// warning/error — the "search indexing is transient" case.
	StreamLogs bool
}

// programRunner runs the view's bubble tea program and returns the final
// model. Swappable in tests so the coordinator's logic (logger swap/restore,
// work goroutine, durable re-emit) is exercised without a real terminal.
type programRunner func(m tea.Model) (tea.Model, error)

func runReal(m tea.Model) (tea.Model, error) {
	// Output goes to stderr (alt-screen); stdout stays clean for the result.
	// AltScreen is declared by the model's View so teardown restores
	// scrollback. WithoutSignalHandler: ctrl+c is handled in Update so it can
	// cancel the work context rather than killing the process mid-teardown.
	return tea.NewProgram(m, tea.WithOutput(os.Stderr), tea.WithoutSignalHandler()).Run()
}

// Interactive runs work under a transient terminal view governed by policy,
// returning work's result. It is the single reusable coordinator for any
// long-running command — index, search lazy-fill, and future bulk operations.
func Interactive[T any](ctx context.Context, policy cliout.Policy, view View, work func(context.Context) (T, error)) (T, error) {
	return interactiveWith(ctx, policy, view, work, runReal)
}

func interactiveWith[T any](
	ctx context.Context,
	policy cliout.Policy,
	view View,
	work func(context.Context) (T, error),
	run programRunner,
) (T, error) {
	var zero T

	// The durable sink is whatever logger was installed before this call —
	// the leveled stderr handler from main.go. Captured here, restored on
	// exit, and used for the policy re-emit on teardown.
	durable := slog.Default()

	handler, consumer := cliout.NewLogPipe(policy.CaptureFloor())
	rec := cliout.NewRecorder(policy)

	logger := slog.New(handler)
	slog.SetDefault(logger)
	defer slog.SetDefault(durable)

	workCtx, cancel := context.WithCancel(slogutils.WithLogger(ctx, logger))
	defer cancel()

	m := newModel(view, consumer, policy, rec, cancel)

	type workOutcome struct {
		val T
		err error
	}
	outCh := make(chan workOutcome, 1)
	go func() {
		v, err := work(workCtx)
		// Signal end-of-work so the view drains its tail and quits.
		consumer.Close()
		if view.Progress != nil {
			view.Progress.Close()
		}
		outCh <- workOutcome{val: v, err: err}
	}()

	finalModel, runErr := run(m)
	out := <-outCh // happens-before: work's writes are visible past this read

	// The final model carries the recorder the view fed; prefer it so the
	// fully-observed state is read after the program goroutine has settled.
	if fm, ok := finalModel.(model); ok && fm.rec != nil {
		rec = fm.rec
	}
	// Footer-only (transient) views never showed their logs live, so surface
	// the kept / fingers-crossed entries on teardown. Streaming views already
	// persisted their display-eligible lines via tea.Printf — re-emitting would
	// duplicate them, so the durable record is left as the live stream wrote it.
	if !view.StreamLogs {
		if out.err != nil {
			rec.MarkFailed()
		}
		cliout.WriteEntries(ctx, durable, rec.Flush())
	}

	switch {
	case out.err != nil:
		return zero, out.err
	case runErr != nil:
		return zero, runErr
	default:
		return out.val, nil
	}
}
