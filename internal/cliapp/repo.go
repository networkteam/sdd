package cliapp

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/networkteam/sdd/internal/cliout"
	clitui "github.com/networkteam/sdd/internal/cliout/tui"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/meta"
	"github.com/networkteam/sdd/internal/model"
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
					h, err := repoHandler(cmd.ErrWriter)
					if err != nil {
						return err
					}

					// Result fields are captured in the callbacks and rendered
					// after the transient view tears down — printing to stdout
					// mid-view would corrupt the footer the coordinator owns.
					var addedRepoID, declaredRepoID model.RepoID
					var addedCacheDir string
					var alreadyDeclared, haveAdded, haveDeclared bool
					// A phase-only reporter (no chunk total) so the footer label
					// tracks the reported stage — connecting → cloning — rather
					// than a bare eternal "connecting".
					reporter := cliout.NewReporter()
					addCmd := &command.RepoAddCmd{
						CloneURL: cmd.Args().First(),
						OnAdded: func(repoID model.RepoID, cacheDir string) {
							addedRepoID, addedCacheDir, haveAdded = repoID, cacheDir, true
						},
						OnDeclared: func(repoID model.RepoID, already bool) {
							declaredRepoID, alreadyDeclared, haveDeclared = repoID, already, true
						},
						OnPhase: reporter.SetPhase,
					}
					work := func(ctx context.Context) (struct{}, error) {
						return struct{}{}, h.RepoAdd(ctx, addCmd)
					}

					// On a TTY the work runs under the coordinator; its dormant /
					// armed states keep the already-connected no-op silent (no
					// program, no escape leak) while a real clone gets the inline
					// spinner, phase label, and streamed "cloning" log. Off-TTY it
					// stays at the plain slog floor.
					if cliout.IsInteractive(cmd.ErrWriter) {
						_, err = clitui.Interactive(ctx, transientViewPolicy(),
							clitui.View{Reader: cmd.Reader, Writer: cmd.ErrWriter, InitialPhase: model.PhaseConnecting, Progress: reporter, StreamLogs: true}, work)
					} else {
						_, err = work(ctx)
					}
					if err != nil {
						return err
					}

					if haveAdded {
						presenters.RenderResultLine(cmd.Writer,
							fmt.Sprintf("connected %s", addedRepoID),
							fmt.Sprintf("cache: %s", addedCacheDir))
					}
					if haveDeclared {
						if alreadyDeclared {
							presenters.RenderResultLine(cmd.Writer,
								fmt.Sprintf("dependency %s already declared in .sdd/config.yaml", declaredRepoID), "")
						} else {
							presenters.RenderResultLine(cmd.Writer,
								fmt.Sprintf("declared dependency %s in .sdd/config.yaml", declaredRepoID),
								"committed so clones know what to connect")
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
						fmt.Fprintln(cmd.Writer, "no connected repos — add one with `sdd repo add <clone-url>`")
						return nil
					}
					for _, r := range cfg.Repos {
						status := "not cached"
						if dir, err := reg.CacheDir(r.RepoID); err == nil && repos.IsCloned(dir) {
							status = "cached"
						}
						fmt.Fprintf(cmd.Writer, "%s\n  clone_url: %s (%s)\n", r.RepoID, r.CloneURL, status)
					}
					return nil
				},
			},
			{
				Name:      "remove",
				Usage:     "Drop a declared cross-repo dependency from .sdd/config.yaml (ref-safety-guarded)",
				ArgsUsage: "<repo-id>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "force",
						Usage: "remove even when local entries still reference the repo, stranding those refs",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() != 1 {
						return fmt.Errorf("usage: sdd repo remove <repo-id>")
					}
					h, err := repoHandler(cmd.ErrWriter)
					if err != nil {
						return err
					}
					return h.RepoRemove(ctx, &command.RepoRemoveCmd{
						RepoID: model.RepoID(cmd.Args().First()),
						Force:  cmd.Bool("force"),
						OnRemoved: func(repoID model.RepoID) {
							fmt.Fprintf(cmd.Writer, "removed dependency %s from .sdd/config.yaml\n", repoID)
						},
						OnStranded: func(repoID model.RepoID, stranded []command.StrandedRef) {
							fmt.Fprintf(cmd.ErrWriter, "warning: --force stranded %d ref(s) into %s:\n", len(stranded), repoID)
							for _, s := range stranded {
								fmt.Fprintf(cmd.ErrWriter, "  %s  %s  %s\n", s.EntryID, s.Kind, s.RefID)
							}
						},
					})
				},
			},
			{
				Name:      "sync",
				Usage:     "Force-pull connected repo caches (all, or the named repos)",
				ArgsUsage: "[repo-id ...]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					h, err := repoHandler(cmd.ErrWriter)
					if err != nil {
						return err
					}
					return h.RepoSync(ctx, &command.RepoSyncCmd{
						RepoIDs: repoIDArgs(cmd.Args().Slice()),
						OnSynced: func(repoID model.RepoID) {
							fmt.Fprintf(cmd.Writer, "synced %s\n", repoID)
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
func repoHandler(errWriter io.Writer) (*handlers.Handler, error) {
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
	// GraphDir backs the ref-safety scan on repo remove; resolved best-effort
	// since add/sync run fine outside a repo (where it stays empty).
	graphDir := ""
	if sddDir != "" {
		cfg, _ := meta.ReadConfig(sddDir)
		graphDir = meta.ResolveGraphDir(filepath.Dir(sddDir), cfg)
	}
	return handlers.New(handlers.Options{
		Stderr:    errWriter,
		Reader:    reader,
		Repos:     mgr,
		SDDDir:    sddDir,
		GraphDir:  graphDir,
		Committer: git.CLI{},
	}), nil
}

// repoIDArgs types repo IDs given as command-line text.
func repoIDArgs(args []string) []model.RepoID {
	if len(args) == 0 {
		return nil
	}
	ids := make([]model.RepoID, len(args))
	for i, arg := range args {
		ids[i] = model.RepoID(arg)
	}
	return ids
}

// freshenRepoCaches brings the named connected repos' caches up to date
// before a read (lazy clone + cooldown pull). A no-op for an empty list.
func freshenRepoCaches(ctx context.Context, errWriter io.Writer, repoIDs []model.RepoID) error {
	if len(repoIDs) == 0 {
		return nil
	}
	h, err := repoHandler(errWriter)
	if err != nil {
		return err
	}
	_, err = h.EnsureReposFresh(ctx, command.EnsureReposFreshCmd{RepoIDs: repoIDs})
	return err
}
