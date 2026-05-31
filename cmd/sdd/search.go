package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/llm/embed"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

// resolveEmbeddingConfig builds the effective EmbeddingConfig by merging
// .sdd/config.yaml + .sdd/config.local.yaml and applying CLI flag
// overrides. Mirrors resolveLLMConfig — same precedence rules, same
// IsSet check so unset flags don't clobber config defaults.
func resolveEmbeddingConfig(cmd *cli.Command) model.EmbeddingConfig {
	var cfg model.EmbeddingConfig
	if fileCfg, err := loadConfig(); err == nil && fileCfg != nil {
		cfg = fileCfg.Embedding
	}
	if cmd.IsSet("embedding-provider") {
		cfg.Provider = cmd.String("embedding-provider")
	}
	if cmd.IsSet("embedding-model") {
		cfg.Model = cmd.String("embedding-model")
	}
	if cmd.IsSet("embedding-endpoint") {
		cfg.Endpoint = cmd.String("embedding-endpoint")
	}
	if cmd.IsSet("embedding-ollama-endpoint") {
		cfg.OllamaEndpoint = cmd.String("embedding-ollama-endpoint")
	}
	if cmd.IsSet("embedding-batch-size") {
		cfg.BatchSize = int(cmd.Int("embedding-batch-size"))
	}
	if cmd.IsSet("embedding-dimensions") {
		cfg.Dimensions = int(cmd.Int("embedding-dimensions"))
	}
	if cmd.IsSet("embedding-rate-limit-rps") {
		cfg.RateLimitRPS = cmd.Float("embedding-rate-limit-rps")
	}
	return cfg
}

// embeddingFlags returns the CLI flag set used by both `sdd index` and
// `sdd search` — they share the same surface so users tune embedding
// behaviour the same way regardless of entry point.
func embeddingFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "embedding-provider",
			Usage: "Embedding provider (openai, ollama) — overrides config",
		},
		&cli.StringFlag{
			Name:  "embedding-model",
			Usage: "Embedding model identifier — overrides config",
		},
		&cli.StringFlag{
			Name:  "embedding-endpoint",
			Usage: "OpenAI-compatible base URL — overrides config",
		},
		&cli.StringFlag{
			Name:  "embedding-ollama-endpoint",
			Usage: "Ollama base URL — overrides config",
		},
		&cli.IntFlag{
			Name:  "embedding-batch-size",
			Usage: "Inputs per embedding call — overrides config",
		},
		&cli.IntFlag{
			Name:  "embedding-dimensions",
			Usage: "Override the embedded vector size (e.g. for OpenAI matryoshka)",
		},
		&cli.FloatFlag{
			Name:  "embedding-rate-limit-rps",
			Usage: "Rate limit for remote embedding providers — overrides config",
		},
	}
}

// populateIndexLint fills the index-side fields on a LintResult when an
// embedding provider is configured. Loading the manifest and building an
// embedder are both pure operations against config — no graph mutation.
// Errors degrade silently to "index not configured" so a missing
// dependency in lint shouldn't block graph-side validation.
func populateIndexLint(cmd *cli.Command, result *query.LintResult) {
	cfg := resolveEmbeddingConfig(cmd)
	if cfg.Provider == "" {
		return
	}
	emb, err := embed.New(cfg)
	if err != nil {
		return
	}
	idxDir, err := indexDir()
	if err != nil {
		return
	}
	manifest, err := index.LoadManifest(idxDir)
	if err != nil {
		return
	}
	result.IndexConfigured = true
	result.IndexFingerprint = emb.Fingerprint()
	result.IndexEntryCount = len(manifest.Entries)
	result.IndexDriftCount = manifest.MismatchCount(emb.Fingerprint())
}

// indexDir returns the index directory rooted at the discovered .sdd dir.
// Returns ("", err) when no .sdd is found.
func indexDir() (string, error) {
	sddDir, err := resolveSDDDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(sddDir, "index"), nil
}

// buildEmbedder constructs the configured embedder, or returns nil when
// no provider is configured. Errors only on misconfiguration (unknown
// provider, missing required fields).
func buildEmbedder(cmd *cli.Command) (llm.Embedder, error) {
	cfg := resolveEmbeddingConfig(cmd)
	if cfg.Provider == "" {
		return nil, nil
	}
	return embed.New(cfg)
}

func indexCmd() *cli.Command {
	return &cli.Command{
		Name:  "index",
		Usage: "Build or refresh the search index over .sdd/graph",
		Flags: append(embeddingFlags(),
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Re-embed every entry, even those whose hash and fingerprint match the manifest",
			},
		),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			emb, err := buildEmbedder(cmd)
			if err != nil {
				return err
			}
			if emb == nil {
				return fmt.Errorf("no embedding provider configured (set embedding.provider in .sdd/config.local.yaml or pass --embedding-provider)")
			}

			graphDir, err := resolveGraphDir(cmd)
			if err != nil {
				return err
			}
			idxDir, err := indexDir()
			if err != nil {
				return err
			}
			idxStore, err := index.Open(idxDir)
			if err != nil {
				return err
			}
			reader, err := newReadFinder()
			if err != nil {
				return err
			}

			h := handlers.NewIndexHandler(handlers.IndexHandlerOptions{
				GraphDir:   graphDir,
				IndexDir:   idxDir,
				Embedder:   emb,
				IndexStore: idxStore,
				Reader:     reader,
			})

			start := time.Now()
			buildCmd := &command.BuildIndexCmd{
				Force: cmd.Bool("force"),
				OnEntryIndexed: func(id string, n int) {
					fmt.Fprintf(os.Stderr, "  indexed %s (%d chunks)\n", id, n)
				},
				OnEntrySkipped: func(id string) {
					fmt.Fprintf(os.Stderr, "  skipped %s\n", id)
				},
				OnComplete: func(indexed, skipped int) {
					fmt.Printf("indexed %d entries (%d skipped) in %s\n",
						indexed, skipped, time.Since(start).Round(time.Millisecond))
				},
			}
			return h.Build(ctx, buildCmd)
		},
	}
}

func searchCmd() *cli.Command {
	return &cli.Command{
		Name:  "search",
		Usage: "Search the graph by keyword (--term), semantic phrase (--query), or both (hybrid)",
		Flags: append(embeddingFlags(),
			&cli.StringSliceFlag{
				Name:    "term",
				Aliases: []string{"t"},
				Usage:   "Keyword (regex) — repeatable; multiple values combine with AND",
			},
			&cli.StringFlag{
				Name:    "query",
				Aliases: []string{"q"},
				Usage:   "Phrase for semantic vector search; required for --query / hybrid mode",
			},
			&cli.StringFlag{
				Name:  "type",
				Usage: "Filter by type (d, s)",
			},
			&cli.StringFlag{
				Name:  "layer",
				Usage: "Filter by layer (stg, cpt, tac, ops, prc)",
			},
			&cli.StringFlag{
				Name:  "kind",
				Usage: "Filter by kind",
			},
			&cli.BoolFlag{
				Name:  "include-superseded",
				Usage: "Include entries whose status is superseded-by another (excluded by default)",
			},
			&cli.IntFlag{
				Name:  "limit",
				Usage: "Maximum entries returned (default 10)",
				Value: query.DefaultSearchLimit,
			},
			&cli.IntFlag{
				Name:  "max-citations",
				Usage: "Maximum citations per entry — 0 for entry headers only (no snippets), 1 for one-line-per-entry, 3 to surface multiple matching chunks per entry",
				Value: query.DefaultMaxCitationsPerEntry,
			},
		),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			terms := cmd.StringSlice("term")
			phrase := cmd.String("query")
			if len(terms) == 0 && phrase == "" {
				return fmt.Errorf("at least one of --term or --query is required")
			}

			g, err := loadGraph(cmd)
			if err != nil {
				return err
			}

			graphDir, err := resolveGraphDir(cmd)
			if err != nil {
				return err
			}

			var emb llm.Embedder
			var idxStore *index.Index
			needsVector := phrase != ""
			if needsVector {
				emb, err = buildEmbedder(cmd)
				if err != nil {
					return err
				}
				if emb == nil {
					return fmt.Errorf("vector search requires an embedding provider — set embedding.provider in .sdd/config.local.yaml")
				}
				idxDir, err := indexDir()
				if err != nil {
					return err
				}
				idxStore, err = index.Open(idxDir)
				if err != nil {
					return err
				}
				// Lazy-fill before query so a fresh clone or branch
				// switch still returns useful seeds.
				reader, err := newReadFinder()
				if err != nil {
					return err
				}
				ih := handlers.NewIndexHandler(handlers.IndexHandlerOptions{
					GraphDir:   graphDir,
					IndexDir:   idxDir,
					Embedder:   emb,
					IndexStore: idxStore,
					Reader:     reader,
				})
				if err := ih.LazyFill(ctx, &command.LazyFillIndexCmd{
					OnEntryIndexed: func(id string, n int) {
						fmt.Fprintf(os.Stderr, "  lazy-indexed %s (%d chunks)\n", id, n)
					},
				}); err != nil {
					return err
				}
			}

			finder := finders.NewSearchFinder(finders.SearchFinderOptions{
				GraphDir:   graphDir,
				Embedder:   emb,
				IndexStore: idxStore,
			})

			var typ model.EntryType
			if t := cmd.String("type"); t != "" {
				if resolved, ok := model.TypeFromAbbrev[t]; ok {
					typ = resolved
				} else {
					typ = model.EntryType(t)
				}
			}
			var layer model.Layer
			if l := cmd.String("layer"); l != "" {
				if resolved, ok := model.LayerFromAbbrev[l]; ok {
					layer = resolved
				} else {
					layer = model.Layer(l)
				}
			}
			var kind model.Kind
			if k := cmd.String("kind"); k != "" {
				kind = model.Kind(k)
			}

			maxCitations := int(cmd.Int("max-citations"))
			res, err := finder.Search(ctx, query.SearchQuery{
				Graph:                g,
				Terms:                terms,
				Phrase:               phrase,
				Filter:               model.GraphFilter{Type: typ, Layer: layer, Kind: kind},
				IncludeSuperseded:    cmd.Bool("include-superseded"),
				Limit:                int(cmd.Int("limit")),
				MaxCitationsPerEntry: maxCitations,
				// Explicit --max-citations 0 means "headers only". The struct's
				// zero value already means "default", so the intent rides on a
				// dedicated flag rather than the ambiguous integer.
				SuppressCitations: maxCitations == 0,
			})
			if err != nil {
				return err
			}

			presenters.RenderSearch(os.Stdout, res, g)
			return nil
		},
	}
}
