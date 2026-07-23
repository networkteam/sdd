package tui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// ErrPromptCancelled is returned when the user aborts a prompt (ctrl+c / esc)
// instead of confirming a value.
var ErrPromptCancelled = errors.New("prompt cancelled")

// TextPrompt configures a single-line text-input prompt. The rendered line is
// "<Label> [<Default>]: <input>"; empty input returns Default.
type TextPrompt struct {
	Label   string
	Default string
	Width   int
}

type textPromptModel struct {
	cfg   TextPrompt
	input textinput.Model
	done  bool
}

func newTextPromptModel(cfg TextPrompt) textPromptModel {
	ti := textinput.New()
	ti.Placeholder = cfg.Default
	ti.Focus()
	ti.SetWidth(cfg.Width)
	return textPromptModel{cfg: cfg, input: ti}
}

func (m textPromptModel) Init() tea.Cmd { return textinput.Blink }

func (m textPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m textPromptModel) View() tea.View {
	return tea.NewView(fmt.Sprintf("%s [%s]: %s", m.cfg.Label, m.cfg.Default, m.input.View()))
}

func (m textPromptModel) value() (string, bool) {
	if !m.done {
		return "", false
	}
	v := strings.TrimSpace(m.input.Value())
	if v == "" {
		v = m.cfg.Default
	}
	return v, true
}

// RunTextPrompt runs the text prompt on a TTY, returning the entered value
// (Default on empty input) or ErrPromptCancelled if the user aborts.
func RunTextPrompt(cfg TextPrompt) (string, error) {
	fm, err := runProgram(newTextPromptModel(cfg), promptSurface)
	if err != nil {
		return "", err
	}
	v, ok := fm.(textPromptModel).value()
	if !ok {
		return "", ErrPromptCancelled
	}
	return v, nil
}

// ConfirmPrompt configures a single-char [y/N] confirmation.
type ConfirmPrompt struct {
	Prompt string
}

type confirmPromptModel struct {
	cfg   ConfirmPrompt
	input textinput.Model
	done  bool
}

func newConfirmPromptModel(cfg ConfirmPrompt) confirmPromptModel {
	ti := textinput.New()
	ti.Placeholder = "N"
	ti.CharLimit = 1
	ti.SetWidth(3)
	ti.Focus()
	return confirmPromptModel{cfg: cfg, input: ti}
}

func (m confirmPromptModel) Init() tea.Cmd { return textinput.Blink }

func (m confirmPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m confirmPromptModel) View() tea.View {
	return tea.NewView(fmt.Sprintf("%s [y/N]: %s", m.cfg.Prompt, m.input.View()))
}

func (m confirmPromptModel) result() bool {
	if !m.done {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(m.input.Value()))
	return v == "y" || v == "yes"
}

// RunConfirm runs the confirmation prompt on a TTY. Aborting (ctrl+c / esc) or
// empty input yields false without an error — the safe "leave it alone" side.
func RunConfirm(cfg ConfirmPrompt) (bool, error) {
	fm, err := runProgram(newConfirmPromptModel(cfg), promptSurface)
	if err != nil {
		return false, err
	}
	return fm.(confirmPromptModel).result(), nil
}

// SelectOption is one row of a single- or multi-select prompt. Value is the
// domain value the option stands for, returned when the option is chosen.
type SelectOption[T any] struct {
	Label string
	Hint  string
	Value T
}

// SelectPrompt configures a cursor-navigated single-select. Header is printed
// on its own line above the options; Cursor is the initial selection.
type SelectPrompt[T any] struct {
	Header  string
	Options []SelectOption[T]
	Cursor  int
}

type selectModel[T any] struct {
	header  string
	options []SelectOption[T]
	cursor  int
	done    bool
}

func newSelectModel[T any](cfg SelectPrompt[T]) selectModel[T] {
	return selectModel[T]{header: cfg.Header, options: cfg.Options, cursor: cfg.Cursor}
}

func (m selectModel[T]) Init() tea.Cmd { return nil }

func (m selectModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m selectModel[T]) View() tea.View {
	var b strings.Builder
	b.WriteString(m.header + "\n")
	for i, opt := range m.options {
		marker := "  "
		if i == m.cursor {
			marker = "› "
		}
		fmt.Fprintf(&b, "%s%s — %s\n", marker, opt.Label, opt.Hint)
	}
	return tea.NewView(b.String())
}

// RunSelect runs the single-select prompt on a TTY, returning the chosen
// option's value or ErrPromptCancelled if the user aborts.
func RunSelect[T any](cfg SelectPrompt[T]) (T, error) {
	var zero T
	fm, err := runProgram(newSelectModel(cfg), promptSurface)
	if err != nil {
		return zero, err
	}
	final := fm.(selectModel[T])
	if !final.done {
		return zero, ErrPromptCancelled
	}
	return final.options[final.cursor].Value, nil
}

// MultiSelectOption is one row of a multi-select prompt, carrying the domain
// value it stands for and its initial checked state.
type MultiSelectOption[T any] struct {
	Label    string
	Hint     string
	Value    T
	Selected bool
}

// MultiSelectPrompt configures a cursor-navigated multi-select toggled with
// space; enter confirms only once at least one option is selected.
type MultiSelectPrompt[T any] struct {
	Header  string
	Options []MultiSelectOption[T]
}

type multiSelectModel[T any] struct {
	header  string
	options []MultiSelectOption[T]
	cursor  int
	done    bool
}

func newMultiSelectModel[T any](cfg MultiSelectPrompt[T]) multiSelectModel[T] {
	opts := make([]MultiSelectOption[T], len(cfg.Options))
	copy(opts, cfg.Options)
	return multiSelectModel[T]{header: cfg.Header, options: opts}
}

func (m multiSelectModel[T]) Init() tea.Cmd { return nil }

func (m multiSelectModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			for _, o := range m.options {
				if o.Selected {
					m.done = true
					return m, tea.Quit
				}
			}
			// No selection yet — ignore enter until at least one is chosen.
		case " ", "space":
			m.options[m.cursor].Selected = !m.options[m.cursor].Selected
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m multiSelectModel[T]) View() tea.View {
	var b strings.Builder
	b.WriteString(m.header + "\n")
	for i, opt := range m.options {
		cursor := "  "
		if i == m.cursor {
			cursor = "› "
		}
		check := "[ ]"
		if opt.Selected {
			check = "[x]"
		}
		fmt.Fprintf(&b, "%s%s %s — %s\n", cursor, check, opt.Label, opt.Hint)
	}
	return tea.NewView(b.String())
}

// selectedValues collects the values of the checked options in option order.
func (m multiSelectModel[T]) selectedValues() []T {
	var chosen []T
	for _, o := range m.options {
		if o.Selected {
			chosen = append(chosen, o.Value)
		}
	}
	return chosen
}

// RunMultiSelect runs the multi-select prompt on a TTY, returning the selected
// options' values in option order, or ErrPromptCancelled if aborted.
func RunMultiSelect[T any](cfg MultiSelectPrompt[T]) ([]T, error) {
	fm, err := runProgram(newMultiSelectModel(cfg), promptSurface)
	if err != nil {
		return nil, err
	}
	final := fm.(multiSelectModel[T])
	if !final.done {
		return nil, ErrPromptCancelled
	}
	return final.selectedValues(), nil
}
