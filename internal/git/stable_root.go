package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// StableRepoRoot resolves the absolute Git common-directory identity that is
// invariant across linked worktrees while remaining distinct for submodules
// and repositories using a separate git directory. Confirmed non-Git
// directories preserve the normalized absolute supplied directory; failures
// while resolving an actual Git worktree surface.
func StableRepoRoot(dir string) (string, error) {
	return stableRepoRootWith(dir, func(args ...string) ([]byte, error) {
		return exec.Command(args[0], args[1:]...).CombinedOutput()
	})
}

func stableRepoRootWith(dir string, run func(...string) ([]byte, error)) (string, error) {
	absoluteDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving absolute repository path %s: %w", dir, err)
	}
	absoluteDir = filepath.Clean(absoluteDir)
	out, err := run("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		if strings.Contains(strings.ToLower(string(out)), "not a git repository") {
			return absoluteDir, nil
		}
		return "", fmt.Errorf("checking Git repository at %s: %w: %s", dir, err, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(string(out)) != "true" {
		return absoluteDir, nil
	}
	out, err = run("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("resolving Git common directory at %s: %w: %s", dir, err, strings.TrimSpace(string(out)))
	}
	commonDir := strings.TrimSpace(string(out))
	if commonDir == "" {
		return "", fmt.Errorf("resolving Git common directory at %s returned an empty path", dir)
	}
	if !filepath.IsAbs(commonDir) {
		return "", fmt.Errorf("resolving Git common directory at %s returned a non-absolute path %q", dir, commonDir)
	}
	return filepath.Clean(commonDir), nil
}
