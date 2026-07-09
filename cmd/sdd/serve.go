package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/urfave/cli/v3"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/mcpserver"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
)

func serveCmd() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Run the workflow MCP server: dialogue-loop tools over stdio or HTTP",
		Flags: append(embeddingFlags(),
			&cli.StringFlag{
				Name:  "transport",
				Value: "stdio",
				Usage: "transport: stdio or http",
			},
			&cli.StringFlag{
				Name:  "addr",
				Value: "127.0.0.1:8765",
				Usage: "HTTP listen address (transport=http only)",
			},
			&cli.StringFlag{
				Name:    "auth-token",
				Usage:   "bearer token required on every HTTP request (transport=http only)",
				Sources: cli.EnvVars("SDD_SERVE_TOKEN"),
			},
		),
		Action: withWriteGate(func(ctx context.Context, cmd *cli.Command) error {
			dir, err := resolveGraphDir(cmd)
			if err != nil {
				return err
			}
			sddDir, err := resolveSDDDir()
			if err != nil {
				return err
			}
			runner, err := newRunner(cmd)
			if err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			reg, mgr, err := defaultRepos()
			if err != nil {
				return err
			}
			finder := finders.New(finders.Options{
				PreflightRunner: runner,
				Config:          cfg,
				Repos:           reg,
			})
			handler := handlers.New(handlers.Options{
				GraphDir:  dir,
				SDDDir:    sddDir,
				Reader:    finder,
				LLMRunner: runner,
				Committer: git.CLI{},
				Repos:     mgr,
			})

			searcher, vector, err := buildServeSearcher(cmd, dir, finder, reg, mgr)
			if err != nil {
				return err
			}

			transport := cmd.String("transport")
			srv, err := mcpserver.New(mcpserver.Options{
				Handler:      handler,
				Finder:       finder,
				Searcher:     searcher,
				VectorSearch: vector,
				GraphDir:     dir,
				SessionsDir:  filepath.Join(sddDir, "sessions"),
				LocalClient:  transport == "stdio",
				Version:      version,
				Repos:        reg,
			})
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()

			switch transport {
			case "stdio":
				return srv.RunStdio(ctx)
			case "http":
				token := cmd.String("auth-token")
				if token == "" {
					return fmt.Errorf("transport=http requires --auth-token (or SDD_SERVE_TOKEN) — the write path must not be open to anyone who can reach the address")
				}
				addr := cmd.String("addr")
				fmt.Fprintf(os.Stderr, "sdd serve: listening on http://%s (bearer token required)\n", addr)
				return srv.RunHTTP(ctx, addr, token)
			default:
				return fmt.Errorf("invalid transport %q: use stdio or http", cmd.String("transport"))
			}
		}),
	}
}

// buildServeSearcher wires ground-tool retrieval: vector search with
// per-call lazy index fill when an embedding provider is configured, plain
// text-term search otherwise. Cross-repo selection on the query fans out
// through the same prepare-then-read split the CLI uses.
//
// It builds two vector stacks so the MCP server matches the CLI per mode: a
// local stack under the local-overlay embedder for local-only queries, and a
// cross-repo stack under the global-first embedder (crossRepoEmbedder) so a
// cross-repo query builds and queries the same one (repo-id, fingerprint)
// vector space the CLI does. When the two embedders coincide (no local
// override) the local stack is reused for both, so no second index store is
// opened.
func buildServeSearcher(cmd *cli.Command, graphDir string, reader *finders.Finder, reg *repos.Registry, mgr *repos.Manager) (mcpserver.Searcher, bool, error) {
	localEmb, err := buildEmbedder(cmd)
	if err != nil {
		return nil, false, err
	}
	crossEmb, err := crossRepoEmbedder(cmd)
	if err != nil {
		return nil, false, err
	}
	prepare := handlers.New(handlers.Options{Reader: reader, Repos: mgr})

	// No provider anywhere: text-only. localEmb == nil iff crossEmb == nil
	// (both are driven by whether any provider resolves), so one finder serves
	// both local and cross-repo queries.
	if localEmb == nil {
		sf := finders.NewSearchFinder(finders.SearchFinderOptions{GraphDir: graphDir, Repos: reg})
		return lazyFillSearcher{localSF: sf, crossSF: sf, prepare: prepare}, false, nil
	}

	localSF, localIH, err := buildVectorStack(graphDir, reader, reg, localEmb)
	if err != nil {
		return nil, false, err
	}

	// The cross-repo stack uses the global-first embedder. Reuse the local
	// stack when the fingerprints match (no local override) — same vector
	// space, and opening the same store twice would contend on its lock.
	crossSF, crossIH := localSF, localIH
	if crossEmb.Fingerprint() != localEmb.Fingerprint() {
		crossSF, crossIH, err = buildVectorStack(graphDir, reader, reg, crossEmb)
		if err != nil {
			return nil, false, err
		}
	}

	return lazyFillSearcher{
		localSF: localSF, localIH: localIH,
		crossSF: crossSF, crossIH: crossIH, crossEmb: crossEmb,
		prepare: prepare,
	}, true, nil
}

// buildVectorStack opens the machine-global index store for emb and returns
// the (search finder, index handler) pair that reads and lazy-fills it.
func buildVectorStack(graphDir string, reader *finders.Finder, reg *repos.Registry, emb llm.Embedder) (*finders.SearchFinder, *handlers.IndexHandler, error) {
	idxDir, err := resolveIndexStore(emb)
	if err != nil {
		return nil, nil, err
	}
	idxStore, err := index.Open(idxDir)
	if err != nil {
		return nil, nil, err
	}
	sf := finders.NewSearchFinder(finders.SearchFinderOptions{
		GraphDir:   graphDir,
		Embedder:   emb,
		IndexStore: idxStore,
		Repos:      reg,
	})
	ih := handlers.NewIndexHandler(handlers.IndexHandlerOptions{
		GraphDir:   graphDir,
		IndexDir:   idxDir,
		Embedder:   emb,
		IndexStore: idxStore,
		Reader:     reader,
	})
	return sf, ih, nil
}

// lazyFillSearcher fills missing or stale index entries before a vector
// query — the same flow `sdd search` runs, without the TTY progress view.
// It holds two stacks: a local-overlay stack for local-only queries and a
// global-first stack for cross-repo queries, matching the CLI's per-mode
// embedder resolution so both surfaces share one cross-repo vector space. A
// query selecting connected repos additionally freshens their caches and
// member indexes (handler side), then reads via the cross-graph finder.
type lazyFillSearcher struct {
	localSF  *finders.SearchFinder
	localIH  *handlers.IndexHandler
	crossSF  *finders.SearchFinder
	crossIH  *handlers.IndexHandler
	crossEmb llm.Embedder
	prepare  *handlers.Handler
}

func (l lazyFillSearcher) Search(ctx context.Context, q query.SearchQuery) (*query.SearchResult, error) {
	if q.AllRepos || len(q.Repos) > 0 {
		// Cross-repo: the global-first embedder unifies the local index and
		// every member index into one (repo-id, fingerprint) vector space, so
		// the MCP server queries exactly what the CLI's cross-repo path does.
		if q.Phrase != "" && l.crossIH != nil {
			if err := l.crossIH.LazyFill(ctx, &command.LazyFillIndexCmd{}); err != nil {
				return nil, err
			}
		}
		if err := l.prepare.PrepareCrossRepoSearch(ctx, q, l.crossEmb, nil); err != nil {
			return nil, err
		}
		return finders.MultiSearch(ctx, l.crossSF, q)
	}
	// Local-only: the local-overlay embedder.
	if q.Phrase != "" && l.localIH != nil {
		if err := l.localIH.LazyFill(ctx, &command.LazyFillIndexCmd{}); err != nil {
			return nil, err
		}
	}
	return l.localSF.Search(ctx, q)
}
