// Package git is the production git adapter for the sdd binary: every git
// subprocess sdd runs is implemented here, behind the narrow interfaces its
// consumers define (handlers.Committer/Brancher/Mover/Puller,
// finders.GitSyncer, the repos package's clone/pull surface). Interfaces stay
// with their consumers; this package only provides the implementations, so a
// future — possibly partial — swap to an in-process implementation such as
// go-git touches nothing but this package.
package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// CLI is the exec-based adapter: a zero-value struct whose methods shell out
// to the git binary. One value satisfies the consumer-defined git interfaces
// (handlers.Committer/Brancher/Mover/Puller, finders.GitSyncer, repos.Git);
// the one exception is the staged-deletion commit variant, which shares the
// Commit method name and therefore lives on its own type (RemovalCommitter).
type CLI struct{}

// commitTimeout caps the detached auto-commit. A signing or credential helper
// blocking on input would otherwise hang indefinitely when sdd runs as a
// long-lived server; the timeout is a backstop on top of the TTY detach in
// runDetached. See d-tac-zhp.
const commitTimeout = 30 * time.Second

// Commit stages exactly the given paths and commits them, detached from any
// controlling terminal. It is the production handlers.Committer for the
// auto-commit paths (sdd new, summarize, init, ...).
func (CLI) Commit(message string, paths ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), commitTimeout)
	defer cancel()

	addArgs := append([]string{"add", "--all", "--"}, paths...)
	if out, err := runDetached(ctx, addArgs...); err != nil {
		return fmt.Errorf("git add: %s (%w)", out, err)
	}

	// Scope the commit to exactly the staged paths with an explicit pathspec.
	// Without `-- <paths>`, `git commit` records the whole index, sweeping any
	// pre-staged unrelated work into the CLI's own commit.
	commitArgs := append([]string{"commit", "-m", message, "--"}, paths...)
	if out, err := runDetached(ctx, commitArgs...); err != nil {
		return fmt.Errorf("git commit: %s (%w)", out, err)
	}

	return nil
}

// HasCommitMessage reports whether any reachable commit contains text. It is
// used by retryable post-apply finalizers to recognize a commit that landed
// before its durable finalizer outcome could be recorded.
func (CLI) HasCommitMessage(ctx context.Context, text string) (bool, error) {
	out, err := exec.CommandContext(ctx, "git", "log", "--all", "--fixed-strings", "--grep="+text, "--format=%H", "-n", "1").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git log: %s (%w)", out, err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// RemovalCommitter is the handlers.Committer variant for paths already
// removed from disk: it stages the deletions (`git rm --cached`, falling back
// to `git add`) before committing. Used by FinishWIP, where the marker file
// is gone by the time the commit runs.
type RemovalCommitter struct{}

// Commit stages the deletion of the given paths and commits, scoped to those
// paths so an unrelated staged index isn't swept into the removal commit.
func (RemovalCommitter) Commit(message string, paths ...string) error {
	for _, p := range paths {
		rm := exec.Command("git", "rm", "--cached", "-f", p)
		if out, err := rm.CombinedOutput(); err != nil {
			add := exec.Command("git", "add", p)
			if out2, err2 := add.CombinedOutput(); err2 != nil {
				return fmt.Errorf("git stage: %s (%v); fallback %s (%w)", out, err, out2, err2)
			}
		}
	}
	commitArgs := append([]string{"commit", "-m", message, "--"}, paths...)
	commit := exec.Command("git", commitArgs...)
	if out, err := commit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s (%w)", out, err)
	}
	return nil
}

// runDetached runs a git subprocess detached from any controlling terminal
// (Setsid starts a new session with no controlling TTY) and bounded by ctx.
// Detaching is the fix for the auto-commit hang (d-tac-zhp): an interactive
// signing or credential prompt has no TTY to read from, so it fails immediately
// instead of suspending the backgrounded process on SIGTTIN. The context
// timeout is a backstop for any other blocking helper.
func runDetached(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("git %s: timed out after %s (a commit signer or credential helper may be blocking on input)", args[0], commitTimeout)
	}
	return out, err
}

// Move renames a path in the working tree and the git index as one operation
// via `git mv`, so the rename is recorded atomically with the working-tree
// change. Production handlers.Mover.
func (CLI) Move(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}
	if out, err := exec.Command("git", "mv", src, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("git mv %s %s: %s (%w)", src, dst, out, err)
	}
	return nil
}

// Checkout switches to branch, creating it first when create is set.
// Production handlers.Brancher.
func (CLI) Checkout(branch string, create bool) error {
	args := []string{"checkout"}
	if create {
		args = append(args, "-b")
	}
	args = append(args, branch)
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout: %s (%w)", out, err)
	}
	return nil
}

// BranchMerged reports whether branch is merged into the current HEAD.
func (CLI) BranchMerged(branch string) bool {
	out, err := exec.Command("git", "branch", "--merged").Output()
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		// git branch prefixes: * = current, + = worktree checkout
		name := strings.TrimLeft(line, " *+")
		name = strings.TrimSpace(name)
		if name == branch {
			return true
		}
	}
	return false
}

// DeleteBranch removes branch (-d, or -D when force is set).
func (CLI) DeleteBranch(branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	if out, err := exec.Command("git", "branch", flag, branch).CombinedOutput(); err != nil {
		return fmt.Errorf("git branch %s: %s (%w)", flag, out, err)
	}
	return nil
}

// IsClean reports whether the working tree has no uncommitted changes.
// Production handlers.Puller.
func (CLI) IsClean(ctx context.Context) (bool, error) {
	out, err := exec.CommandContext(ctx, "git", "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("git status --porcelain: %w", err)
	}
	return strings.TrimSpace(string(out)) == "", nil
}

// MergePull runs a merge-only pull. --no-rebase forces a merge pull
// regardless of the user's pull.rebase config, so background sync never
// rewrites the shared graph's history.
func (CLI) MergePull(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "pull", "--no-rebase").CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		if msg == "" {
			return "", fmt.Errorf("git pull --no-rebase: %w", err)
		}
		return "", fmt.Errorf("git pull --no-rebase: %s", msg)
	}
	return msg, nil
}

// RepoRoot returns the git repository root, falling back to cwd when not in
// a repository.
func RepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return os.Getwd()
	}
	return strings.TrimSpace(string(out)), nil
}

// UserName reads git config user.name, returning an empty string when git is
// unavailable or the setting isn't configured. Best-effort — used only as a
// pre-filled default for the sdd init participant prompt.
func UserName() string {
	out, err := exec.Command("git", "config", "--get", "user.name").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// RemoteURL returns the repo's origin remote URL, or "" when no remote is
// configured — the local-only case for repo_id derivation.
func RemoteURL(repoRoot string) string {
	out, err := exec.Command("git", "-C", repoRoot, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// errExitCode returns the exit code when err is an exec.ExitError, or -1.
func errExitCode(err error) int {
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode()
	}
	return -1
}
