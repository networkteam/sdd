package tui

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/networkteam/sdd/internal/cliout"
)

// TestModel_StreamLogsDurable runs the real inline program: a streamed log line
// must land in the terminal output (durable, above the footer). We wait for it
// to render before signalling end-of-work — in teatest every message fires
// instantly, so closing first would quit before tea.Printf flushes (in a real
// terminal the seconds of embedding work leave ample time).
func TestModel_StreamLogsDurable(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}
	handler, consumer := cliout.NewLogPipe(policy.CaptureFloor())
	rec := cliout.NewRecorder(policy)

	m := newModel(View{Label: "indexing", StreamLogs: true}, consumer, policy, rec, func() {})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 24))

	slog.New(handler).Info("indexed", "entry", "20260101-aaa")

	// The streamed line reaches scrollback while the program is still running.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("20260101-aaa"))
	}, teatest.WithDuration(5*time.Second))

	consumer.Close()
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	if fm := tm.FinalModel(t).(model); !fm.done {
		t.Error("program did not quit on the done signal")
	}
}

// TestModel_TransientHidesLogs runs the inline program in transient mode: the
// footer label shows, but per-entry log lines are NOT streamed into output —
// the "search indexing is transient" behaviour. The program still quits on done.
func TestModel_TransientHidesLogs(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}
	handler, consumer := cliout.NewLogPipe(policy.CaptureFloor())
	rec := cliout.NewRecorder(policy)

	m := newModel(View{Label: "indexing", StreamLogs: false}, consumer, policy, rec, func() {})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 24))

	logger := slog.New(handler)
	logger.Info("indexed", "entry", "ZZZ-should-not-appear")
	consumer.Close()

	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	out, _ := io.ReadAll(tm.FinalOutput(t))
	got := string(out)
	if strings.Contains(got, "ZZZ-should-not-appear") {
		t.Errorf("transient mode leaked a per-entry log line into output; output=%q", got)
	}
	if !strings.Contains(got, "indexing") {
		t.Errorf("footer label missing; output=%q", got)
	}
}
