package model

import "strings"

// SyncState describes the overall state of background sync detection.
type SyncState string

const (
	// SyncStateUpToDate: no divergence between local and remote.
	SyncStateUpToDate SyncState = "up-to-date"
	// SyncStateFastForward: remote has commits local does not; local has none
	// ahead. A rebase is a fast-forward with no conflict risk.
	SyncStateFastForward SyncState = "fast-forward"
	// SyncStateCleanRebase: both sides have commits, but git merge-tree
	// predicts the rebase would apply cleanly.
	SyncStateCleanRebase SyncState = "clean-rebase"
	// SyncStateConflictPredicted: both sides have diverged and the
	// git merge-tree prediction reports conflicts. Rebase is unsafe without
	// manual resolution.
	SyncStateConflictPredicted SyncState = "conflict-predicted"
	// SyncStateLocalAhead: local has commits remote does not, and remote has
	// none ahead of local. Push is suggested.
	SyncStateLocalAhead SyncState = "local-ahead"
	// SyncStateNoRepo: current directory is not inside a git repository.
	SyncStateNoRepo SyncState = "no-repo"
	// SyncStateNoRemote: repository has no remote configured.
	SyncStateNoRemote SyncState = "no-remote"
	// SyncStateNoUpstream: current branch has no upstream tracking branch.
	SyncStateNoUpstream SyncState = "no-upstream"
	// SyncStateFetchFailed: git fetch was attempted and failed (network,
	// authentication, etc.). The last-fetch timestamp is still updated so
	// subsequent commands within the cooldown window do not retry.
	SyncStateFetchFailed SyncState = "fetch-failed"
	// SyncStateSkipped: the sync check did not run (cooldown window active,
	// command exempt, or environment unavailable). No slog output expected.
	SyncStateSkipped SyncState = "skipped"
)

// SyncStatus is the structured result of a background sync check. Callers
// emit the appropriate slog line based on State; the skill pattern-matches
// on the rendered message.
type SyncStatus struct {
	State         SyncState
	RemoteAhead   int      // count of remote-ahead commits matching ^sdd:
	LocalAhead    int      // count of local-ahead commits matching ^sdd:
	ConflictPaths []string // populated when State == SyncStateConflictPredicted
	// Reason carries a short human-readable detail for failure states —
	// the upstream branch name for NoUpstream, the error message for
	// FetchFailed, etc. Empty for success states.
	Reason string
}

// CountGraphCommits counts non-empty lines from `git log --grep='^sdd:'
// --pretty=format:%H <range>`. Each line is one SDD-written commit in the
// range; we only need the count, not the hashes themselves.
func CountGraphCommits(gitLogOutput string) int {
	n := 0
	for line := range strings.SplitSeq(gitLogOutput, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// ParseMergeTreeConflicts parses the output of
// `git merge-tree --write-tree --name-only --merge-base=<base> HEAD @{u}`.
// On a clean merge, output is a single tree-OID line → returns nil. On a
// conflict, output is the tree OID followed by conflicted file paths →
// returns the paths. Blank lines and the leading OID are stripped.
func ParseMergeTreeConflicts(output string) []string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) <= 1 {
		return nil
	}
	paths := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		p := strings.TrimSpace(line)
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}
