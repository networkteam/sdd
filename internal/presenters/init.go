package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/command"
)

// RenderInitSkills summarises a skill install pass for the user. Counts are
// always printed; individual paths appear only for categories that warrant
// per-path attention (overwritten, skipped-modified).
func RenderInitSkills(w io.Writer, installDir string, result command.SkillInstallResult) {
	total := len(result.Installed) + len(result.Refreshed) + len(result.Overwritten) +
		len(result.SkippedModified) + len(result.Current)
	changed := len(result.Installed) + len(result.Refreshed) + len(result.Overwritten)

	fmt.Fprintf(w, "skills: %d file(s) at %s\n", total, installDir)
	if len(result.Installed) > 0 {
		fmt.Fprintf(w, "  installed: %d\n", len(result.Installed))
	}
	if len(result.Refreshed) > 0 {
		fmt.Fprintf(w, "  refreshed: %d\n", len(result.Refreshed))
	}
	if len(result.Overwritten) > 0 {
		fmt.Fprintf(w, "  overwritten: %d (modified files the user approved)\n", len(result.Overwritten))
		for _, p := range result.Overwritten {
			fmt.Fprintf(w, "    - %s\n", p)
		}
	}
	if len(result.SkippedModified) > 0 {
		fmt.Fprintf(w, "  preserved: %d modified file(s) left untouched\n", len(result.SkippedModified))
		for _, p := range result.SkippedModified {
			fmt.Fprintf(w, "    - %s\n", p)
		}
	}
	if changed == 0 && len(result.SkippedModified) == 0 {
		fmt.Fprintf(w, "  up to date\n")
	}
}
