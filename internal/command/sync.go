package command

// SyncPullCmd captures intent to pull the shared graph from the upstream
// branch using a merge (never a rebase), so background sync never rewrites
// the shared history. The handler refuses on a dirty working tree and defers
// to the user. On a successful pull it invokes OnPulled with git's own output
// (the "Already up to date." line or the merge summary).
type SyncPullCmd struct {
	// OnPulled is invoked after a successful merge pull with git's combined
	// stdout/stderr — the text the user wants to see verbatim.
	OnPulled func(output string)
}
