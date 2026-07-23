package tui

import (
	"os"

	tea "charm.land/bubbletea/v2"
)

// surface selects the program-option policy for a bubble tea program. The two
// TUI surfaces differ deliberately: the coordinator renders its transient
// footer on stderr (stdout stays clean for the result) and handles ctrl+c in
// Update so it can cancel the work context rather than let the default signal
// handler kill teardown mid-flight; the init prompts render inline on the
// default output and read stdin with bubble tea's default signal handling.
type surface int

const (
	coordinatorSurface surface = iota
	promptSurface
)

// runProgram is the single owner of bubble tea program construction. Program-
// lifecycle policy (output target, signal handling) lives here so both TUI
// surfaces route through one place.
func runProgram(m tea.Model, s surface) (tea.Model, error) {
	var opts []tea.ProgramOption
	if s == coordinatorSurface {
		opts = append(opts, tea.WithOutput(os.Stderr), tea.WithoutSignalHandler())
	}
	return tea.NewProgram(m, opts...).Run()
}
