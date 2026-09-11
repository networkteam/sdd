package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
)

// indexStore is one store `sdd index gc` reports or drops from: the local
// repo's, or a connected repo's cache.
type indexStore struct {
	label           string
	graphDir        string
	indexDir        string
	excludeEmbedded bool
}

// indexGCCmd is the deliberate collection path for stored index versions
// (d-tac-c9c): without --drop it only reports.
func indexGCCmd() *cli.Command {
	return &cli.Command{
		Name:  "gc",
		Usage: "Report stored index versions by derivation rule; --drop removes a group",
		Description: `Without --drop, lists each store's version groups with counts, age and size.
Groups: ` + index.GroupCurrent + ` is what this checkout searches and is never dropped;
` + index.GroupBranch + ` holds current-rule versions this checkout has no entry for;
older derivation rules (` + index.GroupPreDerivation + `, ` + index.GroupLegacy + `) are what
binaries of previous versions wrote and still read while they run.

--drop <group> removes that group; --drop ` + index.GroupStale + ` removes every group
but ` + index.GroupCurrent + `. Drop an older rule's group only after every process of
that version has stopped.`,
		Flags: append(embeddingFlags(),
			&cli.StringSliceFlag{
				Name:  "drop",
				Usage: "Group to delete (repeatable), or '" + index.GroupStale + "' for every version not current for this checkout",
			},
			&cli.StringSliceFlag{
				Name:  "repo",
				Usage: "Also cover a connected repo's store by its repo-id (repeatable)",
			},
			&cli.BoolFlag{
				Name:  "all-repos",
				Usage: "Also cover every connected repo's store",
			},
			&cli.StringFlag{
				Name:  "format",
				Usage: "Report format: auto (default — styled table on a TTY, JSON otherwise) or json",
			},
		),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			stores, emb, reader, err := selectIndexStores(cmd)
			if err != nil {
				return err
			}
			drop := cmd.StringSlice("drop")
			if len(drop) == 0 {
				var results []*query.IndexVersionsResult
				for _, s := range stores {
					r, err := reader.IndexVersions(ctx, query.IndexVersionsQuery{
						Label: s.label, GraphDir: s.graphDir, IndexDir: s.indexDir, ExcludeEmbedded: s.excludeEmbedded,
					})
					if err != nil {
						return fmt.Errorf("%s: %w", s.label, err)
					}
					results = append(results, r)
				}
				// Same split as sdd stats: agents and pipes get JSON, a
				// terminal gets the styled table (d-cpt-owo).
				if cmd.String("format") == "json" || !isTerminal(os.Stdout) {
					return presenters.RenderIndexVersionsJSON(os.Stdout, results)
				}
				presenters.RenderIndexVersionsTable(os.Stdout, results)
				return nil
			}
			for _, s := range stores {
				h := handlers.NewIndexHandler(handlers.IndexHandlerOptions{
					GraphDir: s.graphDir, IndexDir: s.indexDir, Embedder: emb, Reader: reader, ExcludeEmbedded: s.excludeEmbedded,
				})
				err := h.DropVersions(ctx, &command.DropIndexVersionsCmd{
					Groups: drop,
					OnDropped: func(versions, chunks int, bytes int64) {
						presenters.RenderResultLine(os.Stdout,
							fmt.Sprintf("dropped %d versions from %s", versions, s.label),
							fmt.Sprintf("%d chunks, %s freed", chunks, presenters.HumanBytes(bytes)))
					},
				})
				if err != nil {
					return fmt.Errorf("%s: %w", s.label, err)
				}
			}
			return nil
		},
	}
}

// selectIndexStores resolves the local store plus the connected stores the
// --repo/--all-repos flags select, under the same embedder rule `sdd index`
// uses: cross-repo work runs in the shared global vector space. Connected
// caches are read as they are; nothing is pulled.
func selectIndexStores(cmd *cli.Command) ([]indexStore, handlers.IndexEmbedder, *finders.Finder, error) {
	repoSelection := cmd.StringSlice("repo")
	allRepos := cmd.Bool("all-repos")
	crossRepo := allRepos || len(repoSelection) > 0

	var emb handlers.IndexEmbedder
	var err error
	if crossRepo {
		emb, err = crossRepoEmbedder(cmd)
	} else {
		emb, err = buildEmbedder(cmd)
	}
	if err != nil {
		return nil, emb, nil, err
	}
	if emb.Embedder == nil {
		return nil, emb, nil, fmt.Errorf("no embedding provider configured (set embedding.provider in .sdd/config.local.yaml or pass --embedding-provider)")
	}
	graphDir, err := resolveGraphDir(cmd)
	if err != nil {
		return nil, emb, nil, err
	}
	idxDir, err := resolveIndexStore(emb)
	if err != nil {
		return nil, emb, nil, err
	}
	reader, err := newReadFinder()
	if err != nil {
		return nil, emb, nil, err
	}
	stores := []indexStore{{label: "local", graphDir: graphDir, indexDir: idxDir}}
	if !crossRepo {
		return stores, emb, reader, nil
	}
	reg, _, err := defaultRepos()
	if err != nil {
		return nil, emb, nil, err
	}
	repoIDs, err := reg.SelectRepoIDs(repoSelection, allRepos)
	if err != nil {
		return nil, emb, nil, err
	}
	for _, repoID := range repoIDs {
		cacheDir, err := reg.CacheDir(repoID)
		if err != nil {
			return nil, emb, nil, err
		}
		if !repos.IsCloned(cacheDir) {
			continue
		}
		cacheGraph, err := repos.GraphDir(cacheDir)
		if err != nil {
			return nil, emb, nil, err
		}
		stores = append(stores, indexStore{
			label: repoID, graphDir: cacheGraph, indexDir: index.StoreDir(reg.CacheRoot(), repoID, emb.Fingerprint()), excludeEmbedded: true,
		})
	}
	return stores, emb, reader, nil
}
