package query

import "github.com/networkteam/sdd/internal/model"

// SkillStatusQuery captures intent to read the install state of the
// embedded skill bundle against a target agent's skill directory.
type SkillStatusQuery struct {
	Target   model.AgentTarget
	Scope    model.Scope
	RepoRoot string // required for ScopeProject
	UserHome string // required for ScopeUser
}

// SkillStatusResult is a per-entry snapshot of the bundle compared to disk.
type SkillStatusResult struct {
	// InstallDir is the resolved absolute directory where skills install
	// for this target + scope combination.
	InstallDir string

	// Entries is one row per embedded skill file.
	Entries []SkillStatusEntry

	// Orphans is one row per installed file the bundle no longer carries,
	// excluding files sdd never wrote. Empty on a bundle that lost nothing.
	Orphans []SkillOrphanEntry
}

// SkillOrphanEntry carries an installed file with no embedded counterpart,
// classified by whether sdd may remove it.
type SkillOrphanEntry struct {
	Skill     string
	RelPath   string
	AbsPath   string
	Class     model.SkillOrphanClass
	Installed *model.SkillFile
}

// SkillStatusEntry carries the inputs a handler needs to decide whether (and
// how) to write the file: the embedded source, the parsed on-disk copy (nil
// if missing), and the computed status classification.
type SkillStatusEntry struct {
	Skill    string
	RelPath  string
	AbsPath  string
	Status   model.SkillInstallStatus
	Embedded model.SkillBundleEntry
	// Installed is the parsed on-disk copy. Nil when Status = Missing.
	Installed *model.SkillFile
}
