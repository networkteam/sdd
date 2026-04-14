package command

// LintFixCmd captures intent to apply mechanical fixes to graph entries.
// The handler loads the graph, applies all available fixers, writes patched
// files, and commits. OnFixed is invoked per entry with the fixes applied.
type LintFixCmd struct {
	// OnFixed is invoked for each entry that was fixed, with the entry ID
	// and a list of human-readable fix descriptions.
	OnFixed func(id string, fixes []string)
}
