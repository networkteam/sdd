package tui

import (
	"bytes"
	"io"
	"log/slog"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/networkteam/sdd/internal/cliout"
)

// TestModel_StreamLogsDurable runs the real inline program: a durable line
// handed in as the opening backlog must reach terminal output above the footer,
// but only after the first-paint gate — then the program quits on end-of-work.
func TestModel_StreamLogsDurable(t *testing.T) {
	consumer := cliout.NewLogConsumer(64)
	backlog := []cliout.LogEntry{logEntry("indexed", slog.String("entry", "20260101-aaa"))}

	m := newModel(View{Label: "indexing", StreamLogs: true}, consumer, backlog, func() {})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 24))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("20260101-aaa"))
	}, teatest.WithDuration(5*time.Second))

	consumer.Close()
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	if fm := tm.FinalModel(t).(model); !fm.done {
		t.Error("program did not quit on the done signal")
	}
}

// TestModel_NoFullHeightCursorDownBeforeFirstFrame is the s-tac-bnw regression
// guard: a log line present at program start must not trigger a full-height
// cursor-down escape (ESC[<n>B at terminal-height scale) before the renderer
// has flushed its first frame. The first-paint gate holds the line until after
// a frame tick, by which point the cellbuf is sized to the footer, so any
// insert-above cursor movement stays small.
func TestModel_NoFullHeightCursorDownBeforeFirstFrame(t *testing.T) {
	const height = 24
	consumer := cliout.NewLogConsumer(64)
	backlog := []cliout.LogEntry{logEntry("cloning connected repo")}

	m := newModel(View{Label: "connecting", StreamLogs: true}, consumer, backlog, func() {})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, height))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("cloning connected repo"))
	}, teatest.WithDuration(5*time.Second))

	consumer.Close()
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	out, _ := io.ReadAll(tm.FinalOutput(t))
	if n := maxCursorDown(out); n >= 10 {
		t.Errorf("emitted a full-height cursor-down ESC[%dB (terminal height %d) — first-paint gate did not hold the line", n, height)
	}
}

var cursorDownRe = regexp.MustCompile(`\x1b\[(\d+)B`)

// maxCursorDown returns the largest n across all ESC[<n>B (cursor-down) escapes
// in b, or 0 when there are none.
func maxCursorDown(b []byte) int {
	max := 0
	for _, m := range cursorDownRe.FindAllSubmatch(b, -1) {
		if n, err := strconv.Atoi(string(m[1])); err == nil && n > max {
			max = n
		}
	}
	return max
}
