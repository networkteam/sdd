package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func key(code rune) tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: code}) }

// typeChar simulates typing a printable character (Text set so textinput
// inserts it), as opposed to key which drives navigation/action keys.
func typeChar(c rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: c, Text: string(c)})
}

func TestTextPrompt_EnterConfirmsTrimmedValue(t *testing.T) {
	m := newTextPromptModel(TextPrompt{Label: "Participant name", Default: "alice", Width: 60})
	nm, _ := m.Update(key(tea.KeyEnter))
	fm := nm.(textPromptModel)
	if !fm.done {
		t.Fatal("enter should mark the prompt done")
	}
	// Empty input falls back to the default.
	if v, ok := fm.value(); !ok || v != "alice" {
		t.Errorf("value = %q ok=%v, want alice/true", v, ok)
	}
}

func TestTextPrompt_EscCancels(t *testing.T) {
	m := newTextPromptModel(TextPrompt{Label: "Graph language", Default: "en", Width: 20})
	nm, cmd := m.Update(key(tea.KeyEscape))
	fm := nm.(textPromptModel)
	if fm.done {
		t.Error("esc must not mark the prompt done")
	}
	if _, ok := fm.value(); ok {
		t.Error("cancelled prompt must not yield a value")
	}
	if cmd == nil {
		t.Fatal("esc should quit")
	}
}

func TestTextPrompt_View(t *testing.T) {
	m := newTextPromptModel(TextPrompt{Label: "Graph directory (relative to repo root)", Default: ".sdd/graph", Width: 60})
	got := m.View().Content
	if !strings.HasPrefix(got, "Graph directory (relative to repo root) [.sdd/graph]: ") {
		t.Errorf("view = %q", got)
	}
}

func TestConfirm_YesAndDefaultNo(t *testing.T) {
	// Enter with no typed char confirms as "no" (empty → false).
	m := newConfirmPromptModel(ConfirmPrompt{Prompt: "Overwrite?"})
	nm, _ := m.Update(key(tea.KeyEnter))
	if fm := nm.(confirmPromptModel); !fm.done || fm.result() {
		t.Errorf("empty enter should be done=true result=false; done=%v result=%v", fm.done, fm.result())
	}

	// Typing y then enter confirms true.
	m2 := newConfirmPromptModel(ConfirmPrompt{Prompt: "Overwrite?"})
	nm2, _ := m2.Update(typeChar('y'))
	nm2, _ = nm2.(confirmPromptModel).Update(key(tea.KeyEnter))
	if fm := nm2.(confirmPromptModel); !fm.done || !fm.result() {
		t.Errorf("y+enter should confirm true; done=%v result=%v", fm.done, fm.result())
	}

	// Esc cancels: not done, result false.
	m3 := newConfirmPromptModel(ConfirmPrompt{Prompt: "Overwrite?"})
	nm3, _ := m3.Update(key(tea.KeyEscape))
	if fm := nm3.(confirmPromptModel); fm.done || fm.result() {
		t.Errorf("esc should leave done=false result=false; done=%v result=%v", fm.done, fm.result())
	}
}

func TestConfirm_View(t *testing.T) {
	m := newConfirmPromptModel(ConfirmPrompt{Prompt: "Overwrite user-edited x?"})
	// The affordance sits on its own line so a long prompt cannot push it off
	// the terminal edge, where bubble tea truncates rather than wraps.
	if got := m.View().Content; !strings.HasPrefix(got, "Overwrite user-edited x?\n[y/N]: ") {
		t.Errorf("view = %q", got)
	}
}

func TestSelect_NavigateAndConfirm(t *testing.T) {
	cfg := SelectPrompt[string]{
		Header:  "Where?",
		Options: []SelectOption[string]{{Label: "project", Hint: "a", Value: "p"}, {Label: "user", Hint: "b", Value: "u"}},
	}
	m := newSelectModel(cfg)
	if m.cursor != 0 {
		t.Fatalf("cursor should start at 0, got %d", m.cursor)
	}
	// Down moves to user; up past top clamps; enter confirms.
	nm, _ := m.Update(key('j'))
	m = nm.(selectModel[string])
	if m.cursor != 1 {
		t.Errorf("j should move cursor to 1, got %d", m.cursor)
	}
	nm, _ = m.Update(key(tea.KeyUp))
	m = nm.(selectModel[string])
	if m.cursor != 0 {
		t.Errorf("up should move back to 0, got %d", m.cursor)
	}
	nm, _ = m.Update(key(tea.KeyUp))
	m = nm.(selectModel[string])
	if m.cursor != 0 {
		t.Errorf("up at top should clamp at 0, got %d", m.cursor)
	}
	nm, cmd := m.Update(key(tea.KeyEnter))
	m = nm.(selectModel[string])
	if !m.done || cmd == nil {
		t.Error("enter should confirm and quit")
	}
	if got := m.options[m.cursor].Value; got != "p" {
		t.Errorf("confirmed value = %q, want p", got)
	}
}

func TestSelect_View(t *testing.T) {
	m := newSelectModel(SelectPrompt[string]{
		Header:  "Where should skills be installed?",
		Options: []SelectOption[string]{{Label: "project", Hint: "repo"}, {Label: "user", Hint: "home"}},
	})
	got := m.View().Content
	want := "Where should skills be installed?\n› project — repo\n  user — home\n"
	if got != want {
		t.Errorf("view = %q, want %q", got, want)
	}
}

func TestMultiSelect_ToggleAndConfirm(t *testing.T) {
	cfg := MultiSelectPrompt[string]{
		Header: "Which agents?",
		Options: []MultiSelectOption[string]{
			{Label: "claude", Hint: "cc", Value: "claude", Selected: true},
			{Label: "codex", Hint: "cx", Value: "codex"},
		},
	}
	m := newMultiSelectModel(cfg)
	nm, _ := m.Update(key('j')) // move to codex
	m = nm.(multiSelectModel[string])
	nm, _ = m.Update(key(tea.KeySpace)) // toggle codex on
	m = nm.(multiSelectModel[string])
	nm, cmd := m.Update(key(tea.KeyEnter))
	m = nm.(multiSelectModel[string])
	if !m.done || cmd == nil {
		t.Fatal("enter with a selection should confirm and quit")
	}
	// Values come back in option order regardless of toggle order.
	if got := m.selectedValues(); !slices.Equal(got, []string{"claude", "codex"}) {
		t.Errorf("selected values = %v, want [claude codex]", got)
	}
}

func TestMultiSelect_RequiresSelection(t *testing.T) {
	cfg := MultiSelectPrompt[string]{
		Header:  "Which agents?",
		Options: []MultiSelectOption[string]{{Label: "claude", Value: "claude", Selected: true}, {Label: "codex", Value: "codex"}},
	}
	m := newMultiSelectModel(cfg)
	nm, _ := m.Update(key(tea.KeySpace)) // deselect the only selected option
	m = nm.(multiSelectModel[string])
	nm, _ = m.Update(key(tea.KeyEnter)) // enter with nothing selected
	m = nm.(multiSelectModel[string])
	if m.done {
		t.Error("enter with no selection must not confirm")
	}
}

func TestMultiSelect_View(t *testing.T) {
	m := newMultiSelectModel(MultiSelectPrompt[string]{
		Header: "Which agents?",
		Options: []MultiSelectOption[string]{
			{Label: "claude", Hint: "cc", Selected: true},
			{Label: "codex", Hint: "cx"},
		},
	})
	got := m.View().Content
	want := "Which agents?\n› [x] claude — cc\n  [ ] codex — cx\n"
	if got != want {
		t.Errorf("view = %q, want %q", got, want)
	}
}

// newMultiSelectModel must copy the caller's options so a returned model never
// mutates the prompt config's slice.
func TestMultiSelect_CopiesOptions(t *testing.T) {
	opts := []MultiSelectOption[string]{{Label: "claude", Selected: true}, {Label: "codex"}}
	cfg := MultiSelectPrompt[string]{Header: "x", Options: opts}
	m := newMultiSelectModel(cfg)
	m.options[1].Selected = true
	if opts[1].Selected {
		t.Error("mutating the model must not touch the caller's slice")
	}
}
