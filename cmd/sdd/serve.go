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
func buildServeSearcher(cmd *cli.Command, graphDir string, reader *finders.Finder, reg *repos.Registry, mgr *repos.Manager) (mcpserver.Searcher, bool, error) {
	emb, err := buildEmbedder(cmd)
	if err != nil {
		return nil, false, err
	}
	if emb == nil {
		sf := finders.NewSearchFinder(finders.SearchFinderOptions{GraphDir: graphDir, Repos: reg})
		return lazyFillSearcher{sf: sf, prepare: handlers.New(handlers.Options{Reader: reader, Repos: mgr})}, false, nil
	}
	idxDir, err := resolveIndexStore(emb)
	if err != nil {
		return nil, false, err
	}
	idxStore, err := index.Open(idxDir)
	if err != nil {
		return nil, false, err
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
	return lazyFillSearcher{sf: sf, ih: ih, prepare: handlers.New(handlers.Options{Reader: reader, Repos: mgr}), emb: emb}, true, nil
}

// lazyFillSearcher fills missing or stale index entries before a vector
// query — the same flow `sdd search` runs, without the TTY progress view.
// A query selecting connected repos additionally freshens their caches and
// member indexes (handler side), then reads via the cross-graph finder.
type lazyFillSearcher struct {
	sf      *finders.SearchFinder
	ih      *handlers.IndexHandler
	prepare *handlers.Handler
	emb     llm.Embedder
}

func (l lazyFillSearcher) Search(ctx context.Context, q query.SearchQuery) (*query.SearchResult, error) {
	if q.Phrase != "" && l.ih != nil {
		if err := l.ih.LazyFill(ctx, &command.LazyFillIndexCmd{}); err != nil {
			return nil, err
		}
	}
	if q.AllRepos || len(q.Repos) > 0 {
		if err := l.prepare.PrepareCrossRepoSearch(ctx, q, l.emb); err != nil {
			return nil, err
		}
		return finders.MultiSearch(ctx, l.sf, q)
	}
	return l.sf.Search(ctx, q)
}
