package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/networkteam/sdd/internal/cliout"
	clitui "github.com/networkteam/sdd/internal/cliout/tui"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/repos"
)

// repoCmd manages connected repos — the machine-level setup behind
// cross-repo references. Deliberately CLI-only (no MCP surface): connecting
// a repo is environment configuration, not dialogue.
func repoCmd() *cli.Command {
	return &cli.Command{
		Name:  "repo",
		Usage: "Manage connected repos for cross-repo references",
		Commands: []*cli.Command{
			{
				Name:      "add",
				Usage:     "Connect a repo: clone its cache, verify its declared repo_id, register it",
				ArgsUsage: "<clone-url>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() != 1 {
						return fmt.Errorf("usage: sdd repo add <clone-url>")
					}
					h, err := repoHandler()
					if err != nil {
						return err
					}

					// Result fields are captured in the callbacks and rendered
					// after the transient view tears down — printing to stdout
					// mid-view would corrupt the footer the coordinator owns.
					var addedRepoID, addedCacheDir, declaredRepoID string
					var alreadyDeclared, haveAdded, haveDeclared bool
					addCmd := &command.RepoAddCmd{
						CloneURL: cmd.Args().First(),
						OnAdded: func(repoID, cacheDir string) {
							addedRepoID, addedCacheDir, haveAdded = repoID, cacheDir, true
						},
						OnDeclared: func(repoID string, already bool) {
							declaredRepoID, alreadyDeclared, haveDeclared = repoID, already, true
						},
					}
					work := func(ctx context.Context) (struct{}, error) {
						return struct{}{}, h.RepoAdd(ctx, addCmd)
					}

					// The clone is the long, previously-silent step. On a TTY it
					// runs under the inline coordinator (spinner + streamed
					// "cloning" log); off-TTY it stays at the plain slog floor.
					if cliout.IsInteractive(os.Stderr) {
						_, err = clitui.Interactive(ctx, transientViewPolicy(),
							clitui.View{Label: "connecting", StreamLogs: true}, work)
					} else {
						_, err = work(ctx)
					}
					if err != nil {
						return err
					}

					if haveAdded {
						presenters.RenderResultLine(os.Stdout,
							fmt.Sprintf("connected %s", addedRepoID),
							fmt.Sprintf("cache: %s", addedCacheDir))
					}
					if haveDeclared {
						if alreadyDeclared {
							presenters.RenderResultLine(os.Stdout,
								fmt.Sprintf("dependency %s already declared in .sdd/config.yaml", declaredRepoID), "")
						} else {
							presenters.RenderResultLine(os.Stdout,
								fmt.Sprintf("declared dependency %s in .sdd/config.yaml", declaredRepoID),
								"commit it so clones know what to connect")
						}
					}
					return nil
				},
			},
			{
				Name:  "list",
				Usage: "List connected repos",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					reg, _, err := defaultRepos()
					if err != nil {
						return err
					}
					cfg, err := reg.Load()
					if err != nil {
						return err
					}
					if len(cfg.Repos) == 0 {
						fmt.Println("no connected repos — add one with `sdd repo add <clone-url>`")
						return nil
					}
					for _, r := range cfg.Repos {
						status := "not cached"
						if dir, err := reg.CacheDir(r.RepoID); err == nil && repos.IsCloned(dir) {
							status = "cached"
						}
						fmt.Printf("%s\n  clone_url: %s (%s)\n", r.RepoID, r.CloneURL, status)
					}
					return nil
				},
			},
			{
				Name:      "remove",
				Usage:     "Disconnect a repo (its cache stays on disk)",
				ArgsUsage: "<repo-id>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() != 1 {
						return fmt.Errorf("usage: sdd repo remove <repo-id>")
					}
					h, err := repoHandler()
					if err != nil {
						return err
					}
					return h.RepoRemove(ctx, &command.RepoRemoveCmd{
						RepoID: cmd.Args().First(),
						OnRemoved: func(repoID string) {
							fmt.Printf("disconnected %s\n", repoID)
						},
					})
				},
			},
			{
				Name:      "sync",
				Usage:     "Force-pull connected repo caches (all, or the named repos)",
				ArgsUsage: "[repo-id ...]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					h, err := repoHandler()
					if err != nil {
						return err
					}
					return h.RepoSync(ctx, &command.RepoSyncCmd{
						RepoIDs: cmd.Args().Slice(),
						OnSynced: func(repoID string) {
							fmt.Printf("synced %s\n", repoID)
						},
					})
				},
			},
		},
	}
}

// repoHandler builds the handler for repo commands: a read finder for
// handler construction plus the connected-repos manager the repo
// operations run through.
func repoHandler() (*handlers.Handler, error) {
	reader, err := newReadFinder()
	if err != nil {
		return nil, err
	}
	_, mgr, err := defaultRepos()
	if err != nil {
		return nil, err
	}
	// SDDDir is optional: outside a repo the dependency declaration is
	// skipped and only the global registration happens.
	sddDir, err := resolveSDDDir()
	if err != nil {
		sddDir = ""
	}
	return handlers.New(handlers.Options{Reader: reader, Repos: mgr, SDDDir: sddDir}), nil
}

// freshenRepoCaches brings the named connected repos' caches up to date
// before a read (lazy clone + cooldown pull). A no-op for an empty list.
func freshenRepoCaches(ctx context.Context, repoIDs []string) error {
	if len(repoIDs) == 0 {
		return nil
	}
	h, err := repoHandler()
	if err != nil {
		return err
	}
	_, err = h.EnsureReposFresh(ctx, repoIDs)
	return err
}
