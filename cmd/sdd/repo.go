package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/model"
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
					h := handlers.New(handlers.Options{Reader: mustReadFinder()})
					return h.RepoAdd(ctx, &command.RepoAddCmd{
						CloneURL: cmd.Args().First(),
						OnAdded: func(repoID, cacheDir string) {
							fmt.Printf("connected %s\n", repoID)
							fmt.Printf("  cache: %s\n", cacheDir)
						},
					})
				},
			},
			{
				Name:  "list",
				Usage: "List connected repos",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					cfg, err := repos.LoadConfig()
					if err != nil {
						return err
					}
					if len(cfg.Repos) == 0 {
						fmt.Println("no connected repos — add one with `sdd repo add <clone-url>`")
						return nil
					}
					for _, r := range cfg.Repos {
						status := "not cached"
						if dir, err := repos.CacheDir(r.RepoID); err == nil && repos.IsCloned(dir) {
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
					h := handlers.New(handlers.Options{Reader: mustReadFinder()})
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
					h := handlers.New(handlers.Options{Reader: mustReadFinder()})
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

// mustReadFinder builds the read finder or exits — repo commands need it
// only for handler construction, not for graph reads.
func mustReadFinder() handlers.Reader {
	reader, err := newReadFinder()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return reader
}

// crossRepoIDsIn collects the distinct repo IDs named by cross-repo
// (<repo-id>:<entry-id>) arguments.
func crossRepoIDsIn(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if repoID, _, ok := model.SplitCrossRepoID(id); ok && !seen[repoID] {
			seen[repoID] = true
			out = append(out, repoID)
		}
	}
	return out
}

// freshenRepoCaches brings the named connected repos' caches up to date
// before a read (lazy clone + cooldown pull). A no-op for an empty list.
func freshenRepoCaches(ctx context.Context, repoIDs []string) error {
	if len(repoIDs) == 0 {
		return nil
	}
	reader, err := newReadFinder()
	if err != nil {
		return err
	}
	h := handlers.New(handlers.Options{Reader: reader})
	_, err = h.EnsureReposFresh(ctx, repoIDs)
	return err
}
