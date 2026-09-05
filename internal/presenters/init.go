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

// RenderInitPrune summarises the skill files removed for an agent dropped from
// supported_agents, and lists any user-modified files left untouched.
func RenderInitPrune(w io.Writer, result command.AgentPruneResult) {
	fmt.Fprintf(w, "pruned %s: %d file(s) removed from %s\n", result.Target, len(result.Removed), result.InstallDir)
	renderPreserved(w, result.KeptModified)
}

// RenderInitOrphans summarises the files removed from a still-rendered agent's
// install directory because the bundle no longer carries them, and names every
// user-modified copy left in place.
func RenderInitOrphans(w io.Writer, result command.AgentPruneResult) {
	fmt.Fprintf(w, "removed %d orphaned %s file(s) from %s (no longer part of sdd)\n", len(result.Removed), result.Target, result.InstallDir)
	for _, p := range result.Removed {
		fmt.Fprintf(w, "    - %s\n", p)
	}
	renderPreserved(w, result.KeptModified)
}

func renderPreserved(w io.Writer, keptModified []string) {
	if len(keptModified) == 0 {
		return
	}
	fmt.Fprintf(w, "  preserved: %d modified file(s) left untouched (pass --force to remove)\n", len(keptModified))
	for _, p := range keptModified {
		fmt.Fprintf(w, "    - %s\n", p)
	}
}
