package git

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// The handlers.ChangeReader surface: the working tree read against HEAD, so
// files a failed commit left on disk can be found and committed.

// ModifiedFiles lists tracked files under dir whose working copy differs from
// HEAD, as absolute paths.
func (CLI) ModifiedFiles(ctx context.Context, dir string) ([]string, error) {
	root, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain=v1", "-z", "--untracked-files=no", "--", ".").Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	top := strings.TrimSpace(string(root))
	var paths []string
	records := strings.Split(string(out), "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 4 {
			continue
		}
		status, path := record[:2], record[3:]
		if status[0] == 'R' || status[0] == 'C' {
			i++ // the source path follows as its own record
			continue
		}
		if status[0] == 'M' || status[1] == 'M' {
			paths = append(paths, filepath.Join(top, filepath.FromSlash(path)))
		}
	}
	return paths, nil
}

// HeadContent returns the file at path as HEAD records it.
func (CLI) HeadContent(ctx context.Context, path string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", filepath.Dir(path), "show", "HEAD:./"+filepath.Base(path)).Output()
	if err != nil {
		return nil, fmt.Errorf("git show HEAD:%s: %w", path, err)
	}
	return out, nil
}
