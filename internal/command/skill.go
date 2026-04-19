package command

import "github.com/networkteam/sdd/internal/model"

// InstallSkillsCmd captures intent to extract the embedded skill bundle onto
// a target agent's skill directory. Idempotent: files already matching the
// embedded bundle are skipped, pristine files are refreshed silently, and
// user-modified files are routed through PromptOverwrite.
type InstallSkillsCmd struct {
	Target        model.AgentTarget
	Scope         model.Scope
	RepoRoot      string
	UserHome      string
	BinaryVersion string

	// Force unconditionally overwrites user-modified files without
	// consulting PromptOverwrite. Intended for non-interactive runs where
	// the operator has already decided to discard local edits.
	Force bool

	// PromptOverwrite is invoked for each file whose on-disk copy has been
	// user-edited. Return true to overwrite the file with the embedded
	// version, false to leave it in place. If nil and Force is false,
	// modified files are preserved without prompting (treated as a "no"
	// response).
	PromptOverwrite func(absPath string) (overwrite bool, err error)

	// OnInstalled fires once after all files have been processed, carrying
	// a per-category summary for presenter consumption.
	OnInstalled func(result SkillInstallResult)
}

// SkillInstallResult categorises each bundle entry's outcome after the
// handler runs. AbsPath values are returned (not relative) so presenters can
// surface them directly to the user.
type SkillInstallResult struct {
	Installed       []string // entry was missing, now written
	Refreshed       []string // entry was pristine (user hadn't edited), now rewritten with fresh stamps
	Overwritten     []string // entry was modified, prompt returned true
	SkippedModified []string // entry was modified, prompt returned false (or no prompt supplied)
	Current         []string // entry already matched the embedded bundle — no write
}
