package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/networkteam/sdd/internal/cliout"
	clitui "github.com/networkteam/sdd/internal/cliout/tui"
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

			// Pre-pass: how many entries need (re-)embedding? This sets the
			// progress total and decides whether to show a transient view at
			// all — a fully warm index does no work, so skip the program and
			// its alt-screen flash.
			g, err := reader.LoadGraph(graphDir)
			if err != nil {
				return err
			}
			manifest, err := index.LoadManifest(idxDir)
			if err != nil {
				return err
			}
			force := cmd.Bool("force")
			total := manifest.PendingCount(entryIDs(g), emb.Fingerprint())
			if force {
				total = len(g.Entries) // force re-embeds everything
			}

			start := time.Now()
			reporter := cliout.NewReporter()
			reporter.SetUnit("entries")
			reporter.SetTotal(total)

			var doneIndexed, doneSkipped int
			var haveSummary bool
			buildCmd := &command.BuildIndexCmd{
				Force:        force,
				OnBatchStart: func(batchIDs []string) { reporter.Add(len(batchIDs)) },
				OnComplete: func(indexed, skipped int) {
					doneIndexed, doneSkipped, haveSummary = indexed, skipped, true
				},
			}
			work := func(ctx context.Context) (struct{}, error) {
				return struct{}{}, h.Build(ctx, buildCmd)
			}

			// Indexing logs persist (the per-entry indexed lines scroll above
			// the footer); on a TTY with work to do, run under the inline view.
			if cliout.IsInteractive(os.Stderr) && total > 0 {
				_, err = clitui.Interactive(ctx, transientViewPolicy(),
					clitui.View{Label: "indexing", Progress: reporter, StreamLogs: true}, work)
			} else {
				_, err = work(ctx)
			}
			if err != nil {
				return err
			}
			if haveSummary { // styled summary to clean stdout, after teardown
				presenters.RenderResultLine(os.Stdout,
					fmt.Sprintf("indexed %d entries", doneIndexed),
					fmt.Sprintf("(%d skipped) in %s", doneSkipped, time.Since(start).Round(time.Millisecond)))
			}
			return nil
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
				Usage: "Maximum citations per entry (default 3) — 0 for entry headers only (no snippets), 1 for one-line-per-entry, higher to surface multiple matching chunks",
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

			var (
				emb      llm.Embedder
				idxStore *index.Index
				ih       *handlers.IndexHandler
				willFill bool
				pending  int
			)
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
				reader, err := newReadFinder()
				if err != nil {
					return err
				}
				ih = handlers.NewIndexHandler(handlers.IndexHandlerOptions{
					GraphDir:   graphDir,
					IndexDir:   idxDir,
					Embedder:   emb,
					IndexStore: idxStore,
					Reader:     reader,
				})
				// Will lazy-fill actually embed anything? A warm index does no
				// work, so the transient view is skipped and the result path
				// stays quiet for agents.
				manifest, err := index.LoadManifest(idxDir)
				if err != nil {
					return err
				}
				pending = manifest.PendingCount(entryIDs(g), emb.Fingerprint())
				willFill = pending > 0
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

			// The default is applied here, not in the struct: an unset flag
			// uses DefaultMaxCitationsPerEntry, while an explicit value —
			// including --max-citations 0 (headers only) — passes through
			// literally. cmd.IsSet distinguishes "user gave 0" from "unset".
			maxCitations := query.DefaultMaxCitationsPerEntry
			if cmd.IsSet("max-citations") {
				maxCitations = int(cmd.Int("max-citations"))
			}
			sq := query.SearchQuery{
				Graph:                g,
				Terms:                terms,
				Phrase:               phrase,
				Filter:               model.GraphFilter{Type: typ, Layer: layer, Kind: kind},
				IncludeSuperseded:    cmd.Bool("include-superseded"),
				Limit:                int(cmd.Int("limit")),
				MaxCitationsPerEntry: maxCitations,
			}

			// Lazy-fill (when vector search needs a warm index) then query.
			// On a TTY with fill work pending, the inline footer shows
			// determinate progress and clears before results render — indexing
			// is transient for search, so its per-entry log lines are not
			// streamed. Off-TTY (and for agents) the plain path stays quiet at
			// the Warn floor.
			var reporter *cliout.Reporter
			if willFill {
				reporter = cliout.NewReporter()
				reporter.SetUnit("entries")
				reporter.SetTotal(pending)
			}
			work := func(ctx context.Context) (*query.SearchResult, error) {
				if needsVector {
					lazy := &command.LazyFillIndexCmd{}
					if reporter != nil {
						lazy.OnBatchStart = func(batchIDs []string) { reporter.Add(len(batchIDs)) }
					}
					if err := ih.LazyFill(ctx, lazy); err != nil {
						return nil, err
					}
				}
				return finder.Search(ctx, sq)
			}

			var res *query.SearchResult
			if cliout.IsInteractive(os.Stderr) && willFill {
				// The footer tracks the lazy-fill (embedding) — that's the work
				// taking time; the vector query after it is instant. So label it
				// "indexing", matching what the bar actually measures.
				res, err = clitui.Interactive(ctx, transientViewPolicy(),
					clitui.View{Label: "indexing", Progress: reporter, StreamLogs: false}, work)
			} else {
				res, err = work(ctx)
			}
			if err != nil {
				return err
			}

			presenters.RenderSearch(os.Stdout, res, g)
			return nil
		},
	}
}

// entryIDs collects the IDs of every entry in the graph — the universe the
// index reconciles against when counting pending work.
func entryIDs(g *model.Graph) []string {
	ids := make([]string, len(g.Entries))
	for i, e := range g.Entries {
		ids[i] = e.ID
	}
	return ids
}

// transientViewPolicy is the durable-vs-ephemeral policy shared by the
// indexing commands: the live view is chatty (Info), warnings and above
// survive to the durable stderr sink, and on an error the last entries (any
// level) are flushed so the context around a failure isn't lost with the view.
func transientViewPolicy() cliout.Policy {
	return cliout.Policy{
		Display:        slog.LevelInfo,
		KeepAtOrAbove:  slog.LevelWarn,
		FingersCrossed: &cliout.FingersCrossed{Trigger: slog.LevelError, Tail: 50},
	}
}
