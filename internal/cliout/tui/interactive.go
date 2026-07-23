// Package tui drives the transient interactive terminal view for long-running
// sdd commands. It imports bubble tea and depends on the bubble-tea-free core
// in internal/cliout; only cmd/sdd imports this package. Interactive is the
// reusable entry point: it constructs a cliout.Coordinator, hands it a starter
// that runs the bubble tea program, and lets the coordinator own the lifecycle
// (dormant → armed → live → done) so instant work never starts a program —
// realizing the per-command terminal experience the architecture directive
// (d-cpt-mvb) defers.
package tui

import (
	"context"
	"os"

	tea "charm.land/bubbletea/v2"

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
	// lines that scroll into terminal history — the "indexing logs persist"
	// case. When false the live view is footer-only and logs stay hidden,
	// surfaced only by the teardown re-emit on a warning/error — the "search
	// indexing is transient" case.
	StreamLogs bool
}

// programRunner runs the view's bubble tea program and returns the final
// model. Swappable in tests so the coordinator's starter contract is exercised
// without a real terminal.
type programRunner func(m tea.Model) (tea.Model, error)

func runReal(m tea.Model) (tea.Model, error) {
	// Output goes to stderr; stdout stays clean for the result. The view
	// renders inline (no alt-screen), so bubble tea clears only its footer on
	// quit while durable log lines remain in scrollback. WithoutSignalHandler:
	// ctrl+c is handled in Update so it can cancel the work context rather than
	// killing the process mid-teardown.
	return tea.NewProgram(m, tea.WithOutput(os.Stderr), tea.WithoutSignalHandler()).Run()
}

// starter adapts a View into a cliout.DisplayStarter: it builds the model with
// the coordinator's backlog and live consumer and runs the program.
type starter struct {
	view      View
	interrupt func()
	run       programRunner
}

func (s starter) Start(backlog []cliout.LogEntry, live *cliout.LogConsumer) ([]cliout.LogEntry, error) {
	m := newModel(s.view, live, backlog, s.interrupt)
	fm, err := s.run(m)
	var unpainted []cliout.LogEntry
	if final, ok := fm.(model); ok {
		unpainted = final.held // lines the gate never released
	}
	return unpainted, err
}

// Interactive runs work under a transient terminal view governed by policy,
// returning work's result. It is the single reusable coordinator entry for any
// long-running command — index, search lazy-fill, repo add, and future bulk
// operations. The caller has already established this is a TTY; off-TTY paths
// never reach here and keep plain slog on stderr.
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
	var result T

	coord := cliout.NewCoordinator(cliout.CoordinatorConfig{
		Policy:     policy,
		Stderr:     os.Stderr,
		StreamLogs: view.StreamLogs,
		Progress:   view.Progress,
	})
	coord.SetStarter(starter{view: view, interrupt: coord.Interrupt, run: run})

	err := coord.Run(ctx, func(ctx context.Context) error {
		v, e := work(ctx)
		result = v
		return e
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}
