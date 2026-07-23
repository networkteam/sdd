package tui

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/networkteam/sdd/internal/cliout"
)

func logEntry(msg string, attrs ...slog.Attr) cliout.LogEntry {
	return cliout.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: msg, Attrs: attrs}
}

// newTestModel builds a model with a throwaway live consumer and no opening
// backlog (the hand-driven tests feed messages directly rather than running the
// command loop).
func newTestModel(view View, interrupt func()) model {
	return newModel(view, cliout.NewLogConsumer(64), nil, interrupt)
}

func TestModel_LogDoneQuits(t *testing.T) {
	m := newTestModel(View{Label: "indexing"}, nil)
	nm, cmd := m.Update(logDoneMsg{})
	mm := nm.(model)
	if !mm.done {
		t.Error("done flag not set on logDoneMsg")
	}
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
}

func TestModel_CtrlCInterruptsAndQuits(t *testing.T) {
	interrupted := false
	m := newTestModel(View{Label: "indexing"}, func() { interrupted = true })

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
	m := newTestModel(View{Label: "indexing", Progress: reporter}, nil)

	nm, _ := m.Update(progressMsg(cliout.Progress{Done: 2, Total: 10, Unit: "entries"}))
	mm := nm.(model)
	if mm.lastProg.Done != 2 || mm.lastProg.Total != 10 {
		t.Errorf("lastProg = %+v, want Done=2 Total=10", mm.lastProg)
	}
}

func TestModel_ViewIsInlineFooter(t *testing.T) {
	reporter := cliout.NewReporter()
	m := newTestModel(View{Label: "indexing", Progress: reporter}, nil)

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
	if strings.Count(view.Content, "\n") > 0 {
		t.Errorf("footer should be a single line; content=%q", view.Content)
	}
}

// The first-paint gate holds durable lines until a WindowSizeMsg plus the
// first-paint tick; only then do held lines flush and subsequent lines pass
// straight through.
func TestModel_FirstPaintGateHoldsThenFlushes(t *testing.T) {
	m := newTestModel(View{Label: "indexing"}, nil)

	nm, _ := m.Update(logMsg(logEntry("early")))
	m = nm.(model)
	if len(m.held) != 1 {
		t.Fatalf("line before paint should be held; held=%d", len(m.held))
	}
	if m.painted {
		t.Error("gate must not be open before the first-paint tick")
	}

	nm, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = nm.(model)
	if !m.gateArmed {
		t.Error("first WindowSizeMsg should arm the first-paint tick")
	}
	if cmd == nil {
		t.Error("first WindowSizeMsg should schedule the first-paint tick")
	}

	nm, cmd = m.Update(firstPaintMsg{})
	m = nm.(model)
	if !m.painted {
		t.Error("first-paint tick should open the gate")
	}
	if len(m.held) != 0 {
		t.Errorf("held lines should flush on first paint; held=%d", len(m.held))
	}
	if cmd == nil {
		t.Error("first paint with held lines should return a flush command")
	}

	nm, _ = m.Update(logMsg(logEntry("late")))
	m = nm.(model)
	if len(m.held) != 0 {
		t.Errorf("post-gate lines should pass through, not be held; held=%d", len(m.held))
	}
}
