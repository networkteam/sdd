package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/networkteam/sdd/internal/model"
)

// The finders.GitSyncer surface: methods are deliberately non-chatty — each
// returns the minimal structured answer the sync finder needs, letting
// orchestration logic live in one place.

// InRepo reports whether the process is inside a git working tree.
func (CLI) InRepo(ctx context.Context) bool {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// HasRemote reports whether the repository has at least one remote configured.
func (CLI) HasRemote(ctx context.Context) bool {
	out, err := exec.CommandContext(ctx, "git", "remote").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// UpstreamRef returns the upstream ref name for the current branch.
func (CLI) UpstreamRef(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "@{u}").Output()
	if err != nil {
		// Non-zero exit (typically 128) means no upstream is configured;
		// report as empty string rather than propagating so the finder can
		// emit a dedicated state instead of a generic error.
		if errExitCode(err) >= 0 {
			return "", nil
		}
		return "", fmt.Errorf("git rev-parse --abbrev-ref @{u}: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Fetch runs `git fetch` with no args.
func (CLI) Fetch(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "git", "fetch").CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("git fetch: %w", err)
		}
		return fmt.Errorf("git fetch: %s", msg)
	}
	return nil
}

// CountCommits counts commits in rangeSpec whose messages match grepPattern.
func (CLI) CountCommits(ctx context.Context, rangeSpec, grepPattern string) (int, error) {
	// -E enables ERE so patterns like ^sdd: match. --pretty=format:%H gives
	// one hash per commit; an empty output (no matches) yields zero lines.
	cmd := exec.CommandContext(ctx, "git", "log", "--grep="+grepPattern, "-E", "--pretty=format:%H", rangeSpec)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("git log %s: %w", rangeSpec, err)
	}
	return model.CountGraphCommits(string(out)), nil
}

// MergeTreePredict simulates a three-way merge in memory and returns the
// paths that would conflict.
func (CLI) MergeTreePredict(ctx context.Context, ourRef, theirRef string) ([]string, error) {
	// --no-messages suppresses the trailing informational/conflict-message
	// section so stdout after the OID is purely the conflicted path list.
	// --merge-base is omitted so git computes the base internally (2.38
	// compatibility — the explicit flag was added in 2.40).
	cmd := exec.CommandContext(ctx, "git", "merge-tree", "--write-tree", "--name-only", "--no-messages", ourRef, theirRef)
	out, err := cmd.Output()
	if err != nil {
		// Exit 1 is the documented conflict signal; parse stdout for paths.
		if errExitCode(err) == 1 {
			return model.ParseMergeTreeConflicts(string(out)), nil
		}
		return nil, fmt.Errorf("git merge-tree: %w", err)
	}
	return model.ParseMergeTreeConflicts(string(out)), nil
}
