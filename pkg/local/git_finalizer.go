package local

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	app "github.com/networkteam/sdd/pkg/application"
)

// GitFinalizer commits only the graph paths in one applied mutation, bound to
// the acquired checkout rather than process cwd. It is the sole committer for
// a Git-backed target: the graph store writes files, this commits them once.
type GitFinalizer struct {
	Checkout string
	GraphDir string
	Branch   string
	Timeout  time.Duration
}

func (GitFinalizer) Name() string { return "git" }

func mutationTrailer(batchID string) string { return "SDD-Mutation: " + batchID }

var trailerBlock = regexp.MustCompile(`\n\n(?:[A-Za-z0-9-]+: [^\n]+\n?)+$`)

// withTrailer appends trailer to message, as the last line of the trailer
// block message ends in when it has one, so git reads both as trailers.
func withTrailer(message, trailer string) string {
	if trailerBlock.MatchString(message) {
		return strings.TrimRight(message, "\n") + "\n" + trailer
	}
	return message + "\n\n" + trailer
}

func (f GitFinalizer) Finalize(ctx context.Context, mutation app.AppliedMutation) error {
	lock, err := lockGraph(filepath.Join(f.Checkout, f.GraphDir))
	if err != nil {
		return err
	}
	defer unlock(lock)
	if f.Timeout <= 0 {
		f.Timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, f.Timeout)
	defer cancel()
	trailer := mutationTrailer(mutation.BatchID)
	revision, err := f.lookupTrailer(ctx, trailer)
	if err != nil {
		return err
	}
	if revision != "" {
		return nil
	}
	seen := map[string]bool{}
	var paths, added, removed []string
	addPath := func(logical string, remove bool) {
		if logical == "" {
			return
		}
		path := filepath.Join(f.GraphDir, filepath.FromSlash(logical))
		if seen[path] {
			return
		}
		seen[path] = true
		paths = append(paths, path)
		if remove {
			removed = append(removed, path)
		} else {
			added = append(added, path)
		}
	}
	for _, change := range mutation.Batch.Changes {
		addPath(change.LogicalPath, change.Delete)
	}
	for _, attachment := range mutation.Batch.Attachments {
		addPath(attachment.LogicalPath, false)
	}
	if len(paths) == 0 {
		return fmt.Errorf("git finalizer: mutation %s has no paths", mutation.BatchID)
	}
	if len(added) > 0 {
		addArgs := append([]string{"-C", f.Checkout, "add", "--all", "--"}, added...)
		if out, err := exec.CommandContext(ctx, "git", addArgs...).CombinedOutput(); err != nil {
			return fmt.Errorf("git finalizer add: %s (%w)", strings.TrimSpace(string(out)), err)
		}
	}
	if len(removed) > 0 {
		// A removal may already be staged by an attempt whose commit failed;
		// --ignore-unmatch keeps the retry from failing on the missing path.
		rmArgs := append([]string{"-C", f.Checkout, "rm", "--quiet", "--cached", "--ignore-unmatch", "--"}, removed...)
		if out, err := exec.CommandContext(ctx, "git", rmArgs...).CombinedOutput(); err != nil {
			return fmt.Errorf("git finalizer rm: %s (%w)", strings.TrimSpace(string(out)), err)
		}
	}
	message := mutation.Batch.Message
	if message == "" {
		message = "sdd: apply " + mutation.BatchID
	}
	commitArgs := append([]string{"-C", f.Checkout, "commit", "-m", withTrailer(message, trailer), "--"}, paths...)
	if out, err := exec.CommandContext(ctx, "git", commitArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("git finalizer commit: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// lookupTrailer returns the newest commit on the branch carrying the trailer
// as a whole line, or "" when none does.
func (f GitFinalizer) lookupTrailer(ctx context.Context, trailer string) (string, error) {
	if strings.TrimSpace(f.Branch) == "" {
		return "", fmt.Errorf("git finalizer: concrete branch is required")
	}
	trailerPattern := "^" + regexp.QuoteMeta(trailer) + "$"
	out, err := exec.CommandContext(ctx, "git", "-C", f.Checkout, "log", f.Branch, "--extended-regexp", "--grep="+trailerPattern, "--format=%H", "-n", "1").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git finalizer log: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// readCommittedFile returns a graph file's bytes at a revision, and whether the
// revision carries it at all.
func (f GitFinalizer) readCommittedFile(ctx context.Context, revision, logicalPath string) ([]byte, bool, error) {
	filename := path.Join(f.GraphDir, logicalPath)
	listed, err := exec.CommandContext(ctx, "git", "-C", f.Checkout, "ls-tree", "--name-only", revision, "--", filename).Output()
	if err != nil {
		return nil, false, fmt.Errorf("listing %s at %s: %w", filename, revision, err)
	}
	if strings.TrimSpace(string(listed)) == "" {
		return nil, false, nil
	}
	content, err := exec.CommandContext(ctx, "git", "-C", f.Checkout, "show", revision+":"+filename).Output()
	if err != nil {
		return nil, false, fmt.Errorf("reading %s at %s: %w", filename, revision, err)
	}
	return content, true, nil
}

// isAncestor reports whether the revision is reachable from the branch head.
func (f GitFinalizer) isAncestor(ctx context.Context, revision string) (bool, error) {
	err := exec.CommandContext(ctx, "git", "-C", f.Checkout, "merge-base", "--is-ancestor", revision, f.Branch).Run()
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git ancestry of %s on %s: %w", revision, f.Branch, err)
}
