package tui

import (
	"log/slog"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/networkteam/sdd/internal/cliout"
)

// TestModel_TeatestSmoke runs the real bubble tea program end to end through a
// simulated terminal: Init's commands fire, the log/progress adapters drain the
// live pipe over the event loop, and the program quits on the done signal. This
// confirms the wiring the hand-driven tests bypass (they call Update directly).
func TestModel_TeatestSmoke(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}
	handler, consumer := cliout.NewLogPipe(policy.CaptureFloor())
	reporter := cliout.NewReporter()
	rec := cliout.NewRecorder(policy)

	m := newModel(View{Label: "indexing", Progress: reporter}, consumer, policy, rec, func() {})

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Drive the live pipe from the test goroutine while the program runs.
	logger := slog.New(handler)
	reporter.SetTotal(2)
	logger.Info("indexed", "entry", "d-tac-5g9")
	reporter.Add(1)
	logger.Warn("slow embed")
	reporter.Add(1)

	// End-of-work: the view drains its tail and quits.
	consumer.Close()
	reporter.Close()

	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	fm := tm.FinalModel(t).(model)
	if !fm.done {
		t.Error("program did not quit on the done signal")
	}
	// The recorder, fed over the real event loop, kept the Warn for re-emit.
	if got := len(fm.rec.Flush()); got != 1 {
		t.Errorf("recorder kept %d entries, want 1 (the Warn) — adapters did not drain over the loop", got)
	}
}
