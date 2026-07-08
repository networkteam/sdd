package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// The repos.Git surface: clone and fast-forward pull for the connected-repo
// caches.

// Clone clones url into dir.
func (CLI) Clone(ctx context.Context, url, dir string) error {
	return run(ctx, "", "clone", "--quiet", url, dir)
}

// PullFFOnly pulls the checkout at dir, fast-forward only — the caches are
// read-only, so a non-ff state means the remote rewrote history; that
// surfaces as the error rather than merging.
func (CLI) PullFFOnly(ctx context.Context, dir string) error {
	return run(ctx, dir, "pull", "--quiet", "--ff-only")
}

// run executes a git command, in dir when non-empty, folding stderr into the
// returned error so failures carry git's own message.
func run(ctx context.Context, dir string, args ...string) error {
	verb := args[0]
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = nil
	if err := cmd.Run(); err != nil {
		msg := bytes.TrimSpace(stderr.Bytes())
		if len(msg) > 0 {
			return fmt.Errorf("git %s: %w: %s", verb, err, msg)
		}
		return fmt.Errorf("git %s: %w", verb, err)
	}
	return nil
}
