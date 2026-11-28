package cliapp_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestTitleFlagHelpStatesTheTitleRule(t *testing.T) {
	for _, command := range []string{"new", "summarize"} {
		t.Run(command, func(t *testing.T) {
			f := newAppFixture(t)
			result := f.run(t, command, "--help")
			if result.err != nil {
				t.Fatalf("%s --help: %v\n%s", command, result.err, result.stderr)
			}
			help := result.stdout + result.stderr
			if !strings.Contains(help, "--title TITLE") || !strings.Contains(help, model.TitleRule) {
				t.Errorf("the --title help should name its value and state the title rule whole:\n%s", help)
			}
		})
	}
}
