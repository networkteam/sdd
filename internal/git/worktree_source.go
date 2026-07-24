package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AuthorizeInTreeSessionSource proves that a persisted in-tree relocation
// source belongs to one of the current repository's registered worktrees.
// The source path itself is evidence only: common-directory identity and
// `git worktree list` provide authority.
func AuthorizeInTreeSessionSource(
	ctx context.Context,
	currentRepoRoot string,
	stableRepoAuthority string,
	sessions string,
	stagedBlobs string,
) error {
	currentAuthority, err := StableRepoRoot(currentRepoRoot)
	if err != nil {
		return err
	}
	if currentAuthority != filepath.Clean(stableRepoAuthority) {
		return fmt.Errorf(
			"current repository authority %s does not match relocation authority %s",
			currentAuthority, stableRepoAuthority,
		)
	}
	sddDir := filepath.Dir(filepath.Clean(sessions))
	checkout := filepath.Dir(sddDir)
	if filepath.Clean(sessions) != filepath.Join(checkout, ".sdd", "sessions") ||
		filepath.Clean(stagedBlobs) != filepath.Join(checkout, ".sdd", "staged-blobs") {
		return fmt.Errorf("in-tree relocation source does not have the expected checkout-local store shape")
	}
	for _, path := range []string{sddDir, sessions, stagedBlobs} {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			if errors.Is(statErr, fs.ErrNotExist) {
				continue
			}
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("in-tree relocation source component %s is a symbolic link", path)
		}
	}
	sourceAuthority, err := StableRepoRoot(checkout)
	if err != nil {
		return fmt.Errorf("resolving in-tree source repository identity: %w", err)
	}
	if sourceAuthority != currentAuthority {
		return fmt.Errorf(
			"in-tree relocation source belongs to Git authority %s, not %s",
			sourceAuthority, currentAuthority,
		)
	}
	cmd := exec.CommandContext(ctx, "git", "-C", currentRepoRoot, "worktree", "list", "--porcelain")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("listing authorized Git worktrees: %w: %s", err, strings.TrimSpace(string(output)))
	}
	checkout, err = filepath.EvalSymlinks(checkout)
	if err != nil {
		return fmt.Errorf("resolving in-tree source checkout path: %w", err)
	}
	checkout = filepath.Clean(checkout)
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		listed := filepath.Clean(strings.TrimPrefix(line, "worktree "))
		if resolved, resolveErr := filepath.EvalSymlinks(listed); resolveErr == nil {
			listed = filepath.Clean(resolved)
		}
		if listed == checkout {
			return nil
		}
	}
	return fmt.Errorf("in-tree relocation source checkout %s is not a registered worktree", checkout)
}
