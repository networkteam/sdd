package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/slogutils"
	"github.com/urfave/cli/v3"
)

// syncCheckTimeout caps the full sync check (fetch + log range + merge-tree).
// Bounded so broken networks don't freeze a command for minutes — git fetch
// alone can hang on unreachable remotes without this.
const syncCheckTimeout = 30 * time.Second

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
			handler := handlers.New(handlers.Options{Puller: git.CLI{}})
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
	cfg, err := loadConfig()
	if err != nil {
		logger.Debug("sync check: config unreadable", "err", err)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, syncCheckTimeout)
	defer cancel()

	f := finders.New(finders.Options{
		Config:    cfg,
		GitSyncer: git.CLI{},
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
// with `internal/bundledskills/templates/sdd/SKILL.md.tmpl`.
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
