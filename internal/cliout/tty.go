package cliout

import (
	"os"

	"github.com/charmbracelet/x/term"
)

// IsInteractive reports whether f is an interactive terminal suitable for a
// transient TUI. It returns false for non-terminals (pipes, files, /dev/null,
// agent stdio) and when NO_COLOR is set — both take the plain leveled path,
// so agents and piped consumers never see an alt-screen program. term.IsTerminal
// is used rather than a file-mode check because special devices like /dev/null
// are character devices but not terminals, and bubble tea opens /dev/tty
// directly and fails in non-interactive contexts.
func IsInteractive(f *os.File) bool {
	if f == nil {
		return false
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	return term.IsTerminal(f.Fd())
}
