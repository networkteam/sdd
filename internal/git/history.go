package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
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
