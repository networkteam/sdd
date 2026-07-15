package local

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	app "github.com/networkteam/sdd/application"
)

type TargetRuntimeFactory func(context.Context, string, app.MutationTarget) (app.GraphStore, []app.MutationFinalizer, func() error, error)

type GitWorktreeAcquirerOptions struct {
	Project        app.ProjectID
	ServerCheckout string
	Factory        TargetRuntimeFactory
}

// GitWorktreeAcquirer rediscoveries one registered checkout for every
// acquisition. It persists branch authority, never checkout paths.
type GitWorktreeAcquirer struct {
	project        app.ProjectID
	serverCheckout string
	factory        TargetRuntimeFactory
}

func NewGitWorktreeAcquirer(options GitWorktreeAcquirerOptions) (*GitWorktreeAcquirer, error) {
	if options.Project == "" || strings.TrimSpace(options.ServerCheckout) == "" || options.Factory == nil {
		return nil, fmt.Errorf("sdd: local target project, server checkout, and factory are required")
	}
	root, err := filepath.Abs(options.ServerCheckout)
	if err != nil {
		return nil, err
	}
	return &GitWorktreeAcquirer{project: options.Project, serverCheckout: root, factory: options.Factory}, nil
}

func (a *GitWorktreeAcquirer) Acquire(ctx context.Context, target app.MutationTarget) (*app.AcquiredTarget, error) {
	if err := target.Validate(a.project); err != nil {
		return nil, err
	}
	if output, err := exec.CommandContext(ctx, "git", "check-ref-format", "--branch", target.Branch).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("sdd: invalid mutation target branch %q: %s (%w)", target.Branch, strings.TrimSpace(string(output)), err)
	}
	output, err := exec.CommandContext(ctx, "git", "-C", a.serverCheckout, "worktree", "list", "--porcelain", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("sdd: listing registered worktrees: %w", err)
	}
	matches := matchingWorktrees(output, "refs/heads/"+target.Branch)
	if len(matches) != 1 {
		return nil, fmt.Errorf("sdd: mutation target branch %q must have exactly one registered checkout (found %d)", target.Branch, len(matches))
	}
	checkout := matches[0]
	head, err := exec.CommandContext(ctx, "git", "-C", checkout, "symbolic-ref", "--quiet", "HEAD").Output()
	if err != nil {
		return nil, fmt.Errorf("sdd: mutation target checkout %q is detached or unreadable: %w", checkout, err)
	}
	if actual := strings.TrimSpace(string(head)); actual != "refs/heads/"+target.Branch {
		return nil, fmt.Errorf("sdd: mutation target checkout HEAD changed: got %q, want %q", actual, "refs/heads/"+target.Branch)
	}
	graph, finalizers, release, err := a.factory(ctx, checkout, target)
	if err != nil {
		return nil, err
	}
	if release == nil {
		release = func() error { return nil }
	}
	return &app.AcquiredTarget{Target: target, Graph: graph, Finalizers: finalizers, Release: release}, nil
}

func matchingWorktrees(output []byte, branchRef string) []string {
	var matches []string
	var path, branch string
	flush := func() {
		if path != "" && branch == branchRef {
			matches = append(matches, path)
		}
		path, branch = "", ""
	}
	for _, raw := range bytes.Split(output, []byte{0}) {
		line := string(raw)
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			if path != "" {
				flush()
			}
			path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			branch = strings.TrimPrefix(line, "branch ")
		}
	}
	flush()
	return matches
}
