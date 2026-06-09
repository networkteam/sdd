package tui

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/networkteam/sdd/internal/cliout"
)

func logEntry(level slog.Level, msg string, attrs ...slog.Attr) cliout.LogEntry {
	return cliout.LogEntry{Time: time.Now(), Level: level, Message: msg, Attrs: attrs}
}

// newTestModel builds a model with a throwaway consumer (the hand-driven tests
// feed messages directly rather than running the command loop).
func newTestModel(policy cliout.Policy, interrupt func()) (model, *cliout.Recorder) {
	_, consumer := cliout.NewLogPipe(policy.CaptureFloor())
	rec := cliout.NewRecorder(policy)
	return newModel(View{Label: "indexing"}, consumer, policy, rec, interrupt), rec
}

func TestModel_DisplayEntryPushedToRingAndRecorder(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}
	m, rec := newTestModel(policy, nil)

	nm, cmd := m.Update(logMsg(logEntry(slog.LevelWarn, "slow embed")))
	mm := nm.(model)

	if len(mm.ring) != 1 || mm.ring[0].Message != "slow embed" {
		t.Errorf("ring = %v, want one entry 'slow embed'", mm.ring)
	}
	if cmd == nil {
		t.Error("expected the log stream to be re-issued (non-nil cmd)")
	}
	// Warn is at/above the keep threshold → retained for re-emit.
	if got := len(rec.Flush()); got != 1 {
		t.Errorf("recorder kept %d entries, want 1", got)
	}
}

func TestModel_BelowDisplayLevelSkipsRingButRecorded(t *testing.T) {
	policy := cliout.Policy{
		Display:        slog.LevelInfo,
		KeepAtOrAbove:  slog.LevelError,
		FingersCrossed: &cliout.FingersCrossed{Trigger: slog.LevelError, Tail: 5},
	}
	m, rec := newTestModel(policy, nil)

	// Debug is below the Info display floor: not shown, but still observed so
	// fingers-crossed can flush it on a later failure.
	nm, _ := m.Update(logMsg(logEntry(slog.LevelDebug, "chunked")))
	mm := nm.(model)
	if len(mm.ring) != 0 {
		t.Errorf("ring = %v, want empty (below display level)", mm.ring)
	}
	rec.MarkFailed()
	if got := len(rec.Flush()); got != 1 {
		t.Errorf("recorder flush = %d, want 1 (tail flushed on failure)", got)
	}
}

func TestModel_LogDoneQuits(t *testing.T) {
	m, _ := newTestModel(cliout.Policy{Display: slog.LevelInfo}, nil)
	nm, cmd := m.Update(logDoneMsg{})
	mm := nm.(model)
	if !mm.done {
		t.Error("done flag not set on logDoneMsg")
	}
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("logDoneMsg should yield tea.Quit")
	}
}

func TestModel_CtrlCInterruptsAndQuits(t *testing.T) {
	interrupted := false
	m, _ := newTestModel(cliout.Policy{Display: slog.LevelInfo}, func() { interrupted = true })

	key := tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
	if key.String() != "ctrl+c" {
		t.Fatalf("constructed key = %q, want ctrl+c", key.String())
	}
	_, cmd := m.Update(key)

	if !interrupted {
		t.Error("ctrl+c should invoke the interrupt callback")
	}
	if cmd == nil {
		t.Fatal("ctrl+c should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c should yield tea.Quit")
	}
}

func TestModel_ProgressUpdatesLatest(t *testing.T) {
	reporter := cliout.NewReporter()
	policy := cliout.Policy{Display: slog.LevelInfo}
	_, consumer := cliout.NewLogPipe(policy.CaptureFloor())
	rec := cliout.NewRecorder(policy)
	m := newModel(View{Label: "indexing", Progress: reporter}, consumer, policy, rec, nil)

	nm, _ := m.Update(progressMsg(cliout.Progress{Done: 2, Total: 10, Unit: "entries"}))
	mm := nm.(model)
	if mm.lastProg.Done != 2 || mm.lastProg.Total != 10 {
		t.Errorf("lastProg = %+v, want Done=2 Total=10", mm.lastProg)
	}
}

func TestModel_ViewRendersLabelAndEntries(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo}
	m, _ := newTestModel(policy, nil)

	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = nm.(model)
	nm, _ = m.Update(logMsg(logEntry(slog.LevelInfo, "indexed", slog.String("entry", "d-tac-5g9"))))
	m = nm.(model)

	view := m.View()
	if !view.AltScreen {
		t.Error("transient view must run in the alt-screen")
	}
	if !strings.Contains(view.Content, "indexing") {
		t.Errorf("view missing label; content=%q", view.Content)
	}
	if !strings.Contains(view.Content, "indexed") {
		t.Errorf("view missing log message; content=%q", view.Content)
	}
}

func TestModel_ResizeCapsViewportHeight(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo}
	m, _ := newTestModel(policy, nil)
	// Tall terminal: viewport caps at maxViewportLines, not the full height.
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 200})
	mm := nm.(model)
	if h := mm.viewport.Height(); h != maxViewportLines {
		t.Errorf("viewport height = %d, want capped at %d", h, maxViewportLines)
	}
}
