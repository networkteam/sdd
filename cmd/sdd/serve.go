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
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/mcpserver"
	"github.com/networkteam/sdd/internal/query"
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
			finder := finders.New(finders.Options{
				PreflightRunner: runner,
				Config:          cfg,
			})
			handler := handlers.New(handlers.Options{
				GraphDir:  dir,
				SDDDir:    sddDir,
				Reader:    finder,
				LLMRunner: runner,
				Committer: gitCommitterFunc(gitCommit),
			})

			searcher, vector, err := buildServeSearcher(cmd, dir, finder)
			if err != nil {
				return err
			}

			srv, err := mcpserver.New(mcpserver.Options{
				Handler:      handler,
				Finder:       finder,
				Searcher:     searcher,
				VectorSearch: vector,
				GraphDir:     dir,
				SessionsDir:  filepath.Join(sddDir, "sessions"),
				Version:      version,
			})
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()

			switch cmd.String("transport") {
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
// text-term search otherwise.
func buildServeSearcher(cmd *cli.Command, graphDir string, reader *finders.Finder) (mcpserver.Searcher, bool, error) {
	emb, err := buildEmbedder(cmd)
	if err != nil {
		return nil, false, err
	}
	if emb == nil {
		return finders.NewSearchFinder(finders.SearchFinderOptions{GraphDir: graphDir}), false, nil
	}
	idxDir, err := indexDir()
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
	})
	ih := handlers.NewIndexHandler(handlers.IndexHandlerOptions{
		GraphDir:   graphDir,
		IndexDir:   idxDir,
		Embedder:   emb,
		IndexStore: idxStore,
		Reader:     reader,
	})
	return lazyFillSearcher{sf: sf, ih: ih}, true, nil
}

// lazyFillSearcher fills missing or stale index entries before a vector
// query — the same flow `sdd search` runs, without the TTY progress view.
type lazyFillSearcher struct {
	sf *finders.SearchFinder
	ih *handlers.IndexHandler
}

func (l lazyFillSearcher) Search(ctx context.Context, q query.SearchQuery) (*query.SearchResult, error) {
	if q.Phrase != "" {
		if err := l.ih.LazyFill(ctx, &command.LazyFillIndexCmd{}); err != nil {
			return nil, err
		}
	}
	return l.sf.Search(ctx, q)
}
