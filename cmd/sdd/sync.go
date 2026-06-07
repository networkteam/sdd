package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/meta"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/slogutils"
	"github.com/urfave/cli/v3"
)

// syncCheckTimeout caps the full sync check (fetch + log range + merge-tree).
// Bounded so broken networks don't freeze a command for minutes — git fetch
// alone can hang on unreachable remotes without this.
const syncCheckTimeout = 30 * time.Second

// gitSyncerImpl is the production finders.GitSyncer that shells out to the
// git binary. Methods are deliberately non-chatty: each returns the minimal
// structured answer the finder needs, letting orchestration logic live in
// one place.
type gitSyncerImpl struct{}

func (gitSyncerImpl) InRepo(ctx context.Context) bool {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func (gitSyncerImpl) HasRemote(ctx context.Context) bool {
	out, err := exec.CommandContext(ctx, "git", "remote").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func (gitSyncerImpl) UpstreamRef(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "@{u}").Output()
	if err != nil {
		// Non-zero exit (typically 128) means no upstream is configured;
		// report as empty string rather than propagating so the finder can
		// emit a dedicated state instead of a generic error.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", nil
		}
		return "", fmt.Errorf("git rev-parse --abbrev-ref @{u}: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (gitSyncerImpl) Fetch(ctx context.Context) error {
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

func (gitSyncerImpl) CountCommits(ctx context.Context, rangeSpec, grepPattern string) (int, error) {
	// -E enables ERE so patterns like ^sdd: match. --pretty=format:%H gives
	// one hash per commit; an empty output (no matches) yields zero lines.
	cmd := exec.CommandContext(ctx, "git", "log", "--grep="+grepPattern, "-E", "--pretty=format:%H", rangeSpec)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("git log %s: %w", rangeSpec, err)
	}
	return model.CountGraphCommits(string(out)), nil
}

func (gitSyncerImpl) MergeTreePredict(ctx context.Context, ourRef, theirRef string) ([]string, error) {
	// --no-messages suppresses the trailing informational/conflict-message
	// section so stdout after the OID is purely the conflicted path list.
	// --merge-base is omitted so git computes the base internally (2.38
	// compatibility — the explicit flag was added in 2.40).
	cmd := exec.CommandContext(ctx, "git", "merge-tree", "--write-tree", "--name-only", "--no-messages", ourRef, theirRef)
	out, err := cmd.Output()
	if err != nil {
		// Exit 1 is the documented conflict signal; parse stdout for paths.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return model.ParseMergeTreeConflicts(string(out)), nil
		}
		return nil, fmt.Errorf("git merge-tree: %w", err)
	}
	return model.ParseMergeTreeConflicts(string(out)), nil
}

// syncCmd exposes graph synchronization. Today it carries a single mode,
// --pull, which performs a merge-only pull so background sync never rewrites
// the shared graph's history; the bare command prints guidance.
func syncCmd() *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "Synchronize the shared graph with the upstream branch",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "pull",
				Usage: "Pull upstream changes with a merge (never a rebase); refuses on a dirty working tree",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if !cmd.Bool("pull") {
				return errors.New("sync: specify --pull to merge upstream changes into the shared graph")
			}
			handler := handlers.New(handlers.Options{Puller: gitPuller{}})
			return handler.SyncPull(ctx, &command.SyncPullCmd{
				OnPulled: func(output string) {
					if output != "" {
						fmt.Fprintln(os.Stdout, output)
					}
				},
			})
		},
	}
}

// gitPuller is the production handlers.Puller: shells out to git for the
// working-tree cleanliness check and the merge-only pull.
type gitPuller struct{}

func (gitPuller) IsClean(ctx context.Context) (bool, error) {
	out, err := exec.CommandContext(ctx, "git", "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("git status --porcelain: %w", err)
	}
	return strings.TrimSpace(string(out)) == "", nil
}

func (gitPuller) MergePull(ctx context.Context) (string, error) {
	// --no-rebase forces a merge pull regardless of the user's pull.rebase
	// config, so background sync never rewrites the shared graph's history.
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

// runSyncCheck performs the background sync check and emits slog lines.
// Swallows all errors — sync check failure must never fail a user command.
// Bounded by syncCheckTimeout so network pathologies cannot stall the CLI.
func runSyncCheck(ctx context.Context) {
	logger := slogutils.FromContext(ctx)

	sddDir, err := resolveSDDDir()
	if err != nil {
		// Outside an SDD repo (or no cwd) — nothing to sync. Silent: this
		// is the normal state for `sdd --help` and for the first `sdd init`.
		return
	}
	cfg, err := meta.ReadConfig(sddDir)
	if err != nil {
		logger.Debug("sync check: config unreadable", "err", err)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, syncCheckTimeout)
	defer cancel()

	f := finders.New(finders.Options{
		Config:    cfg,
		GitSyncer: gitSyncerImpl{},
	})
	status, err := f.SyncStatus(ctx, query.SyncStatusQuery{
		SDDDir:          sddDir,
		RespectCooldown: true,
	})
	if err != nil {
		logger.Debug("sync check failed", "err", err)
		return
	}
	emitSyncStatus(logger, status)
}

// emitSyncStatus renders a SyncStatus as one slog line. The phrasing is
// the skill's pattern-match surface — changes here must stay coordinated
// with `internal/bundledskills/claude/sdd/SKILL.md`.
func emitSyncStatus(logger *slog.Logger, s model.SyncStatus) {
	switch s.State {
	case model.SyncStateUpToDate, model.SyncStateSkipped:
		// Quiet: nothing to tell the user or the skill.
	case model.SyncStateFastForward:
		logger.Info(fmt.Sprintf("sync: fast-forward available, %d commits behind", s.RemoteAhead))
	case model.SyncStateCleanRebase:
		logger.Info(fmt.Sprintf("sync: merge is clean, %d remote / %d local", s.RemoteAhead, s.LocalAhead))
	case model.SyncStateConflictPredicted:
		logger.Warn(fmt.Sprintf("sync: merge would conflict in %s, %d remote / %d local",
			strings.Join(s.ConflictPaths, ", "), s.RemoteAhead, s.LocalAhead))
	case model.SyncStateLocalAhead:
		logger.Info(fmt.Sprintf("sync: local ahead by %d, consider push", s.LocalAhead))
	case model.SyncStateNoRepo:
		logger.Warn("sync: not a git repo")
	case model.SyncStateNoRemote:
		logger.Warn("sync: no remote configured")
	case model.SyncStateNoUpstream:
		logger.Warn("sync: no upstream for current branch — set with `git branch --set-upstream-to=origin/<branch>`")
	case model.SyncStateFetchFailed:
		logger.Warn(fmt.Sprintf("sync: fetch failed: %s", s.Reason))
	}
}
