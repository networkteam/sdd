package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// The finders.GitHistory surface: read-only provenance for the graph export.

// InWorkTree reports whether dir lies inside a git work tree.
func (CLI) InWorkTree(ctx context.Context, dir string) bool {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--is-inside-work-tree").Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// HeadRevision returns the commit hash HEAD points at in dir's repository, or
// "" when HEAD has no commit yet.
func (CLI) HeadRevision(ctx context.Context, dir string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--verify", "--quiet", "HEAD").Output()
	if err != nil {
		if errExitCode(err) == 1 && len(out) == 0 {
			return "", nil
		}
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// FileArrivals maps each file under dir that HEAD's first-parent history added
// (path relative to dir) to the committer time of the first-parent commit that
// brought it — for a file that arrived through a merge, the merge commit. One
// log pass; renames count as additions so a rewritten entry arrives when its
// new path did.
func (CLI) FileArrivals(ctx context.Context, dir string) (map[string]time.Time, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "log",
		"--first-parent", "--diff-merges=first-parent", "--diff-filter=A", "--no-renames",
		"--name-only", "--relative", "--no-color", "--format=%x00%cI", "--", ".",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("git log in %s: %w", dir, err)
	}
	return model.ParseFileArrivals(string(out))
}
