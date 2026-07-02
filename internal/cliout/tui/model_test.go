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
func newTestModel(view View, policy cliout.Policy, interrupt func()) (model, *cliout.Recorder) {
	_, consumer := cliout.NewLogPipe(policy.CaptureFloor())
	rec := cliout.NewRecorder(policy)
	return newModel(view, consumer, policy, rec, interrupt), rec
}

func TestModel_ObservesEveryEntryForReEmit(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}
	m, rec := newTestModel(View{Label: "indexing"}, policy, nil)

	nm, cmd := m.Update(logMsg(logEntry(slog.LevelInfo, "below keep")))
	m = nm.(model)
	nm, _ = m.Update(logMsg(logEntry(slog.LevelWarn, "kept warning")))
	_ = nm

	if cmd == nil {
		t.Error("expected the log stream to be re-issued (non-nil cmd)")
	}
	// Every entry feeds the recorder; only the Warn is at/above the keep level.
	got := rec.Flush()
	if len(got) != 1 || got[0].Message != "kept warning" {
		t.Errorf("recorder kept %v, want [kept warning]", got)
	}
}

func TestModel_LogDoneQuits(t *testing.T) {
	m, _ := newTestModel(View{Label: "indexing"}, cliout.Policy{Display: slog.LevelInfo}, nil)
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
	m, _ := newTestModel(View{Label: "indexing"}, cliout.Policy{Display: slog.LevelInfo}, func() { interrupted = true })

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
	m, _ := newTestModel(View{Label: "indexing", Progress: reporter}, policy, nil)

	nm, _ := m.Update(progressMsg(cliout.Progress{Done: 2, Total: 10, Unit: "entries"}))
	mm := nm.(model)
	if mm.lastProg.Done != 2 || mm.lastProg.Total != 10 {
		t.Errorf("lastProg = %+v, want Done=2 Total=10", mm.lastProg)
	}
}

func TestModel_ViewIsInlineFooter(t *testing.T) {
	reporter := cliout.NewReporter()
	policy := cliout.Policy{Display: slog.LevelInfo}
	m, _ := newTestModel(View{Label: "indexing", Progress: reporter}, policy, nil)

	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = nm.(model)
	nm, _ = m.Update(progressMsg(cliout.Progress{Done: 3, Total: 10, Unit: "chunks", Note: "embedding 2 entries · 5 chunks"}))
	m = nm.(model)

	view := m.View()
	if view.AltScreen {
		t.Error("inline footer must not use the alt-screen")
	}
	if !strings.Contains(view.Content, "indexing") {
		t.Errorf("footer missing label; content=%q", view.Content)
	}
	if !strings.Contains(view.Content, "3/10 chunks") {
		t.Errorf("footer missing count; content=%q", view.Content)
	}
	if !strings.Contains(view.Content, "embedding 2 entries · 5 chunks") {
		t.Errorf("footer missing note; content=%q", view.Content)
	}
	// One footer line — no scrolling log region embedded in the managed view.
	if strings.Count(view.Content, "\n") > 0 {
		t.Errorf("footer should be a single line; content=%q", view.Content)
	}
}
