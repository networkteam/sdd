package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/llm/factory"
	"github.com/networkteam/sdd/internal/meta"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/slogutils"
	"github.com/urfave/cli/v3"
)

// version is stamped at release time via `-ldflags "-X main.version=..."`.
// Default "dev" applies to local `go build` and `go run`.
var version = "dev"

// resolveLLMConfig builds the effective LLMConfig for a command by merging
// .sdd/config.yaml + .sdd/config.local.yaml and applying CLI flag overrides.
// Missing .sdd/ or missing config files yield a zero-value config — the
// factory supplies defaults (claude-cli provider, default model). Flags that
// were not explicitly set by the user are skipped so defaults from file
// config remain in effect.
func resolveLLMConfig(cmd *cli.Command) model.LLMConfig {
	var cfg model.LLMConfig
	if sddDir, err := resolveSDDDir(); err == nil {
		if fileCfg, err := meta.ReadConfig(sddDir); err == nil && fileCfg != nil {
			cfg = fileCfg.LLM
		}
	}
	if cmd.IsSet("provider") {
		cfg.Provider = cmd.String("provider")
	}
	if cmd.IsSet("model") {
		cfg.Model = cmd.String("model")
	}
	// preflight-model is a legacy alias for --model in the `sdd new` context.
	if cmd.IsSet("preflight-model") {
		cfg.Model = cmd.String("preflight-model")
	}
	if cmd.IsSet("concurrency") {
		cfg.Concurrency = int(cmd.Int("concurrency"))
	}
	return cfg
}

// newRunner builds a llm.Runner from the resolved LLMConfig. Errors surface
// misconfiguration (unknown provider, missing API key) at CLI entry so
// failures are visible before graph work begins.
func newRunner(cmd *cli.Command) (llm.Runner, error) {
	return factory.New(resolveLLMConfig(cmd))
}

// resolveTimeout returns the per-call LLM timeout for the given flag name,
// preferring the user's explicit --<flag> over the llm.timeout field in
// config. Falls back to the flag's default Value when neither is set.
func resolveTimeout(cmd *cli.Command, flagName string) time.Duration {
	if cmd.IsSet(flagName) {
		return cmd.Duration(flagName)
	}
	cfg := resolveLLMConfig(cmd)
	if cfg.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Timeout); err == nil && d > 0 {
			return d
		}
	}
	return cmd.Duration(flagName)
}

// readOnlyRunner satisfies llm.Runner but always errors on Run. Used by
// read-only CLI commands (status, list, show, lint, wip list) so they
// don't need LLM configuration to operate.
type readOnlyRunner struct{}

func (readOnlyRunner) Run(context.Context, llm.Request) (*llm.RunResult, error) {
	return nil, fmt.Errorf("no llm runner configured for this command")
}

// newReadFinder builds a Finder suitable for read-only operations. The
// runner errors on invocation so accidental use in a code path that does
// call Preflight is loud. Config load failures propagate — a malformed
// config is a real problem and the caller decides how to surface it.
// Returns nil cfg silently only when the CWD is outside an sdd repo or
// config files simply don't exist (legitimate "no config" states).
func newReadFinder() (*finders.Finder, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	return finders.New(finders.Options{
		PreflightRunner: readOnlyRunner{},
		Config:          cfg,
	}), nil
}

// loadConfig reads .sdd/config.yaml + config.local.yaml when present.
// Returns (nil, nil) when the CWD is outside an sdd repo or no config
// files exist — both are legitimate "no config" states. Returns (nil,
// err) when discovery succeeded but parsing failed, so callers can
// fail hard on broken config.
func loadConfig() (*model.Config, error) {
	sddDir, err := resolveSDDDir()
	if err != nil {
		return nil, nil
	}
	return meta.ReadConfig(sddDir)
}

// splitCSV returns the comma-split fields of s with each element trimmed of
// surrounding whitespace; empty elements after trimming are dropped. Returns
// nil if s is empty or contains no non-empty fields.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// gitCommitterFunc adapts a plain commit function to the handlers.Committer interface.
type gitCommitterFunc func(message string, paths ...string) error

func (f gitCommitterFunc) Commit(message string, paths ...string) error {
	return f(message, paths...)
}

// gitMover is the production handlers.Mover: shells out to `git mv` so the
// rename is recorded in the git index atomically with the working-tree change.
type gitMover struct{}

func (gitMover) Move(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}
	if out, err := exec.Command("git", "mv", src, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("git mv %s %s: %s (%w)", src, dst, out, err)
	}
	return nil
}

// gitBrancher is the production handlers.Brancher: shells out to git for
// checkout, merge-status check, and branch deletion.
type gitBrancher struct{}

func (gitBrancher) Checkout(branch string, create bool) error {
	args := []string{"checkout"}
	if create {
		args = append(args, "-b")
	}
	args = append(args, branch)
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout: %s (%w)", out, err)
	}
	return nil
}

func (gitBrancher) BranchMerged(branch string) bool {
	return isBranchMerged(branch)
}

func (gitBrancher) DeleteBranch(branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	if out, err := exec.Command("git", "branch", flag, branch).CombinedOutput(); err != nil {
		return fmt.Errorf("git branch %s: %s (%w)", flag, out, err)
	}
	return nil
}

func main() {
	// Drop the default `-v` alias on --version; our root command already uses
	// -v for --verbose.
	cli.VersionFlag = &cli.BoolFlag{
		Name:  "version",
		Usage: "Print the version",
	}

	app := &cli.Command{
		Name:    "sdd",
		Usage:   "Signal-Dialogue-Decision graph tool",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "graph-dir",
				Aliases: []string{"d"},
				Usage:   "Override graph directory (auto-discovered from .sdd/config.yaml)",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Enable info-level logging",
			},
			&cli.BoolFlag{
				Name:    "extra-verbose",
				Aliases: []string{"vv"},
				Usage:   "Enable debug-level logging",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			level := slog.LevelWarn
			if cmd.Bool("extra-verbose") {
				level = slog.LevelDebug
			} else if cmd.Bool("verbose") {
				level = slog.LevelInfo
			}
			logger := slog.New(slogutils.NewCLIHandler(os.Stderr, &slogutils.CLIHandlerOptions{
				Level: level,
			}))
			slog.SetDefault(logger)
			ctx = slogutils.WithLogger(ctx, logger)

			// Background sync check runs on every command except `sdd init`
			// (bootstrap may precede remote configuration — sync would emit
			// spurious warnings). Cooldown-bound and timeout-bound internally;
			// any failure logs at Debug and does not affect the command.
			if cmd.Args().First() != "init" {
				runSyncCheck(ctx)
			}
			return ctx, nil
		},
		Commands: []*cli.Command{
			initCmd(),
			statusCmd(),
			showCmd(),
			listCmd(),
			newCmd(),
			rewriteCmd(),
			wipCmd(),
			lintCmd(),
			summarizeCmd(),
		},
		DefaultCommand: "status",
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func loadGraph(cmd *cli.Command) (*model.Graph, error) {
	dir, err := resolveGraphDir(cmd)
	if err != nil {
		return nil, err
	}
	f, err := newReadFinder()
	if err != nil {
		return nil, err
	}
	return f.LoadGraph(dir)
}

func statusCmd() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show current state of the decision graph",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			g, err := loadGraph(cmd)
			if err != nil {
				return err
			}

			f, err := newReadFinder()
			if err != nil {
				return err
			}
			result, err := f.Status(query.StatusQuery{Graph: g})
			if err != nil {
				return err
			}
			presenters.RenderStatus(os.Stdout, result)
			return nil
		},
	}
}

func showCmd() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show entry with upstream and downstream summary chains",
		ArgsUsage: "<id> [id2 id3 ...]",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "max-depth",
				Value: query.DefaultMaxDepth,
				Usage: "Maximum depth for upstream/downstream expansion (0 = primary only)",
			},
			&cli.BoolFlag{
				Name:  "downstream",
				Usage: "Include downstream entries (refd-by, closed-by, superseded-by)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			ids := cmd.Args().Slice()
			if len(ids) == 0 {
				return fmt.Errorf("usage: sdd show <id> [id2 id3 ...]")
			}

			g, err := loadGraph(cmd)
			if err != nil {
				return err
			}

			f, err := newReadFinder()
			if err != nil {
				return err
			}
			result, err := f.Show(query.ShowQuery{
				Graph:      g,
				IDs:        ids,
				MaxDepth:   int(cmd.Int("max-depth")),
				Downstream: cmd.Bool("downstream"),
			})
			if err != nil {
				return err
			}
			presenters.RenderShow(os.Stdout, result)
			return nil
		},
	}
}

func listCmd() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List entries with optional filters (open/active only by default)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "type",
				Aliases: []string{"t"},
				Usage:   "Filter by type (d, s)",
			},
			&cli.StringFlag{
				Name:    "layer",
				Aliases: []string{"l"},
				Usage:   "Filter by layer (stg, cpt, tac, ops, prc)",
			},
			&cli.StringFlag{
				Name:    "kind",
				Aliases: []string{"k"},
				Usage:   "Filter by kind — signals: gap, fact, question, insight, done, actor; decisions: directive, activity, plan, contract, aspiration, role",
			},
			&cli.BoolFlag{
				Name:  "missing-kind",
				Usage: "Show only entries without an explicit kind field (migration helper)",
			},
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Show all entries including addressed signals and superseded decisions",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			g, err := loadGraph(cmd)
			if err != nil {
				return err
			}

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

			f, err := newReadFinder()
			if err != nil {
				return err
			}
			result, err := f.List(query.ListQuery{
				Graph: g,
				Filter: model.GraphFilter{
					Type:        typ,
					Layer:       layer,
					Kind:        kind,
					MissingKind: cmd.Bool("missing-kind"),
					OpenOnly:    !cmd.Bool("all"),
				},
			})
			if err != nil {
				return err
			}
			presenters.RenderList(os.Stdout, result)
			return nil
		},
	}
}

func newCmd() *cli.Command {
	return &cli.Command{
		Name:      "new",
		Usage:     "Create a new graph entry",
		ArgsUsage: "<type> <layer> [description]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "refs",
				Usage: "Comma-separated list of referenced entry IDs",
			},
			&cli.StringFlag{
				Name:  "supersedes",
				Usage: "Comma-separated list of entry IDs this supersedes",
			},
			&cli.StringFlag{
				Name:  "closes",
				Usage: "Comma-separated list of entry IDs this closes/resolves",
			},
			&cli.StringFlag{
				Name:  "participants",
				Usage: "Comma-separated list of participants",
			},
			&cli.StringFlag{
				Name:     "confidence",
				Usage:    "Confidence level (high, medium, low)",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "kind",
				Usage: "Entry kind: signals — gap (default), fact, question, insight, done, actor; decisions — directive (default), activity, plan, contract, aspiration, role",
			},
			&cli.StringSliceFlag{
				Name:  "attach",
				Usage: "File to attach (repeatable). Supports source:target mapping and -:name for stdin",
			},
			&cli.BoolFlag{
				Name:  "skip-preflight",
				Usage: "Skip pre-flight validation (entry is annotated with preflight: skipped)",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Run validation and pre-flight only, without writing or committing the entry",
			},
			&cli.StringFlag{
				Name:  "provider",
				Usage: "LLM provider (claude-cli, anthropic, openai, ollama) — overrides config",
			},
			&cli.StringFlag{
				Name:  "model",
				Usage: "LLM model identifier — overrides config",
			},
			&cli.StringFlag{
				Name:  "preflight-model",
				Usage: "Legacy alias for --model in the pre-flight context",
			},
			&cli.DurationFlag{
				Name:  "preflight-timeout",
				Usage: "Timeout for pre-flight validation (e.g. 120s, 2m)",
				Value: 120 * time.Second,
			},
		},
		Action: withWriteGate(func(ctx context.Context, cmd *cli.Command) error {
			ctx = slogutils.WithLogger(ctx, slogutils.FromContext(ctx).With("command", "new"))

			args := cmd.Args()
			if args.Len() < 2 {
				return fmt.Errorf("usage: sdd new <type> <layer> [description]")
			}

			typeArg := args.Get(0)
			layerArg := args.Get(1)

			// Migration hint: `sdd new a` was removed in the two-type redesign.
			// Intercept here so the user gets actionable guidance rather than a
			// generic "unknown type" error.
			if typeArg == "a" || typeArg == "action" {
				return fmt.Errorf(`"sdd new %s" was removed in the two-type migration; actions are now done signals — use "sdd new s --kind done" (see README for the kind vocabulary)`, typeArg)
			}

			// Resolve type
			typ, ok := model.TypeFromAbbrev[typeArg]
			if !ok {
				typ = model.EntryType(typeArg)
				if _, exists := model.TypeAbbrev[typ]; !exists {
					return fmt.Errorf("invalid type: %s (use d or s)", typeArg)
				}
			}

			// Resolve layer
			layer, ok := model.LayerFromAbbrev[layerArg]
			if !ok {
				layer = model.Layer(layerArg)
				if _, exists := model.LayerAbbrev[layer]; !exists {
					return fmt.Errorf("invalid layer: %s (use stg, cpt, tac, ops, or prc)", layerArg)
				}
			}

			description := strings.Join(args.Slice()[2:], " ")
			if description == "" {
				description = "[TODO: describe this " + string(typ) + "]"
			}

			dir, err := resolveGraphDir(cmd)
			if err != nil {
				return err
			}
			sddDir, err := resolveSDDDir()
			if err != nil {
				return err
			}

			// Parse attachment specs into command.Attachment values. For stdin,
			// this reads stdin bytes into Attachment.Data — after this point
			// the handler receives fully-materialized command values.
			cliAtts, err := parseAttachFlags(cmd.StringSlice("attach"), os.Stdin)
			if err != nil {
				return err
			}
			var atts []command.Attachment
			for _, a := range cliAtts {
				atts = append(atts, command.Attachment{
					Source: a.source,
					Target: a.target,
					Data:   a.data,
				})
			}

			var kind model.Kind
			if k := cmd.String("kind"); k != "" {
				kind = model.Kind(k)
			}

			confidence := cmd.String("confidence")
			switch confidence {
			case "high", "medium", "low":
			default:
				return fmt.Errorf("invalid confidence: %q (expected high, medium, or low)", confidence)
			}

			participants, err := resolveParticipantsFlag(cmd.String("participants"), sddDir)
			if err != nil {
				return err
			}

			ncmd := &command.NewEntryCmd{
				Type:             typ,
				Layer:            layer,
				Kind:             kind,
				Description:      description,
				Participants:     participants,
				Refs:             splitCSV(cmd.String("refs")),
				Supersedes:       splitCSV(cmd.String("supersedes")),
				Closes:           splitCSV(cmd.String("closes")),
				Confidence:       confidence,
				Attachments:      atts,
				SkipPreflight:    cmd.Bool("skip-preflight"),
				DryRun:           cmd.Bool("dry-run"),
				PreflightModel:   cmd.String("preflight-model"),
				PreflightTimeout: resolveTimeout(cmd, "preflight-timeout"),
				OnNewEntry: func(id string) {
					fmt.Println(id + ".md")
					if rel, err := model.IDToRelPath(id); err == nil {
						fmt.Printf("  → %s\n", filepath.Join(dir, rel))
					}
				},
			}

			runner, err := newRunner(cmd)
			if err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			handler := handlers.New(handlers.Options{
				GraphDir: dir,
				SDDDir:   sddDir,
				Reader: finders.New(finders.Options{
					PreflightRunner: runner,
					Config:          cfg,
				}),
				LLMRunner: runner,
				Committer: gitCommitterFunc(gitCommit),
			})

			return handler.NewEntry(ctx, ncmd)
		}),
	}
}

func rewriteCmd() *cli.Command {
	return &cli.Command{
		Name:      "rewrite",
		Usage:     "Rewrite an entry's type and kind, updating inbound references",
		ArgsUsage: "<id>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "type",
				Aliases:  []string{"t"},
				Usage:    "New entry type (s, d)",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "kind",
				Aliases:  []string{"k"},
				Usage:    "New entry kind",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "message",
				Aliases: []string{"m"},
				Usage:   "Override the default commit message",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Report intended changes without writing or committing",
			},
			&cli.BoolFlag{
				Name:  "no-commit",
				Usage: "Write changes to disk but skip the git commit",
			},
		},
		Action: withWriteGate(func(ctx context.Context, cmd *cli.Command) error {
			ctx = slogutils.WithLogger(ctx, slogutils.FromContext(ctx).With("command", "rewrite"))

			args := cmd.Args()
			if args.Len() < 1 {
				return fmt.Errorf("usage: sdd rewrite <id> --type <t> --kind <k>")
			}

			typeArg := cmd.String("type")
			typ, ok := model.TypeFromAbbrev[typeArg]
			if !ok {
				typ = model.EntryType(typeArg)
				if _, exists := model.TypeAbbrev[typ]; !exists {
					return fmt.Errorf("invalid type: %s (use d or s)", typeArg)
				}
			}

			rcmd := &command.RewriteEntryCmd{
				EntryID:  args.Get(0),
				NewType:  typ,
				NewKind:  model.Kind(cmd.String("kind")),
				Message:  cmd.String("message"),
				DryRun:   cmd.Bool("dry-run"),
				NoCommit: cmd.Bool("no-commit"),
				OnRewritten: func(oldID, newID string, inbound []string) {
					if cmd.Bool("dry-run") {
						fmt.Printf("would rewrite %s → %s\n", oldID, newID)
					} else {
						fmt.Printf("%s → %s\n", oldID, newID)
					}
					if len(inbound) > 0 {
						fmt.Printf("  inbound updates: %d\n", len(inbound))
						for _, id := range inbound {
							fmt.Printf("    %s\n", id)
						}
					}
				},
			}

			dir, err := resolveGraphDir(cmd)
			if err != nil {
				return err
			}
			reader, err := newReadFinder()
			if err != nil {
				return err
			}
			handler := handlers.New(handlers.Options{
				GraphDir:  dir,
				Reader:    reader,
				Committer: gitCommitterFunc(gitCommit),
				Mover:     gitMover{},
			})
			return handler.RewriteEntry(ctx, rcmd)
		}),
	}
}

func lintCmd() *cli.Command {
	return &cli.Command{
		Name:  "lint",
		Usage: "Check graph entries for integrity issues",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "fix", Usage: "Automatically fix mechanical issues"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Bool("fix") {
				dir, err := resolveGraphDir(cmd)
				if err != nil {
					return err
				}
				fixCmd := &command.LintFixCmd{
					OnFixed: func(id string, fixes []string) {
						for _, f := range fixes {
							fmt.Fprintf(os.Stderr, "  fixed %s: %s\n", id, f)
						}
					},
				}
				reader, err := newReadFinder()
				if err != nil {
					return err
				}
				handler := handlers.New(handlers.Options{
					GraphDir:  dir,
					Reader:    reader,
					Committer: gitCommitterFunc(gitCommit),
				})
				if err := handler.LintFix(ctx, fixCmd); err != nil {
					return err
				}
			}

			g, err := loadGraph(cmd)
			if err != nil {
				return err
			}

			f, err := newReadFinder()
			if err != nil {
				return err
			}
			result, err := f.Lint(query.LintQuery{Graph: g})
			if err != nil {
				return err
			}
			presenters.RenderLint(os.Stdout, result, g)
			if result.TotalIssues > 0 {
				return fmt.Errorf("lint found %d issue(s)", result.TotalIssues)
			}
			return nil
		},
	}
}

func summarizeCmd() *cli.Command {
	return &cli.Command{
		Name:      "summarize",
		Usage:     "Generate or regenerate entry summaries",
		ArgsUsage: "[id...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Summarize all entries (in topological order)",
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Regenerate even if summary hash matches",
			},
			&cli.StringFlag{
				Name:  "provider",
				Usage: "LLM provider (claude-cli, anthropic, openai, ollama) — overrides config",
			},
			&cli.StringFlag{
				Name:  "model",
				Usage: "LLM model identifier — overrides config",
			},
			&cli.IntFlag{
				Name:  "concurrency",
				Usage: "Worker pool size for batch summarize — overrides config",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "Timeout per summary generation (e.g. 60s, 1m)",
				Value: 60 * time.Second,
			},
		},
		Action: withWriteGate(func(ctx context.Context, cmd *cli.Command) error {
			ctx = slogutils.WithLogger(ctx, slogutils.FromContext(ctx).With("command", "summarize"))

			ids := cmd.Args().Slice()
			if len(ids) == 0 && !cmd.Bool("all") {
				return fmt.Errorf("usage: sdd summarize <id>... or sdd summarize --all")
			}

			sumCmd := &command.SummarizeCmd{
				EntryIDs:    ids,
				Force:       cmd.Bool("force"),
				Model:       cmd.String("model"),
				Timeout:     resolveTimeout(cmd, "timeout"),
				Concurrency: int(cmd.Int("concurrency")),
				OnSummarized: func(id, summary string) {
					fmt.Fprintf(os.Stderr, "  summarized %s\n", id)
				},
				OnSkipped: func(id string) {
					fmt.Fprintf(os.Stderr, "  skipped %s (hash matches)\n", id)
				},
			}

			dir, err := resolveGraphDir(cmd)
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
			handler := handlers.New(handlers.Options{
				GraphDir: dir,
				Reader: finders.New(finders.Options{
					PreflightRunner: runner,
					Config:          cfg,
				}),
				LLMRunner: runner,
				Committer: gitCommitterFunc(gitCommit),
			})
			return handler.Summarize(ctx, sumCmd)
		}),
	}
}

func gitCommit(message string, filePaths ...string) error {
	args := append([]string{"add"}, filePaths...)
	add := exec.Command("git", args...)
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s (%w)", out, err)
	}

	commit := exec.Command("git", "commit", "-m", message)
	if out, err := commit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s (%w)", out, err)
	}

	return nil
}

func isBranchMerged(branch string) bool {
	out, err := exec.Command("git", "branch", "--merged").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		// git branch prefixes: * = current, + = worktree checkout
		name := strings.TrimLeft(line, " *+")
		name = strings.TrimSpace(name)
		if name == branch {
			return true
		}
	}
	return false
}

// resolveGraphDir determines the graph directory from the --graph-dir flag
// or by discovering .sdd/config.yaml. Errors if neither is available.
func resolveGraphDir(cmd *cli.Command) (string, error) {
	// Explicit flag takes priority.
	if dir := cmd.String("graph-dir"); dir != "" {
		if !filepath.IsAbs(dir) {
			dir, _ = filepath.Abs(dir)
		}
		return dir, nil
	}

	// Auto-discover from .sdd/config.yaml.
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	repoRoot := meta.DiscoverRoot(cwd)
	if repoRoot == "" {
		return "", fmt.Errorf("no .sdd/ directory found; run 'sdd init' first")
	}
	sddDir := meta.SDDDir(repoRoot)
	cfg, err := meta.ReadConfig(sddDir)
	if err != nil {
		return "", fmt.Errorf("reading .sdd/config.yaml: %w", err)
	}
	return meta.ResolveGraphDir(repoRoot, cfg), nil
}

// resolveParticipantsFlag is the shared fallback rule for capture commands.
// An explicit flag value is used exactly as given — splitCSV, no merging with
// the local config (matches --refs semantics per d-tac-q5p). Flag empty falls
// back to the canonical participant in .sdd/config.local.yaml. Missing both
// is an error pointing the user at `sdd init`.
func resolveParticipantsFlag(flagValue, sddDir string) ([]string, error) {
	if flagValue != "" {
		return splitCSV(flagValue), nil
	}
	cfg, err := meta.ReadConfig(sddDir)
	if err != nil {
		return nil, fmt.Errorf("reading .sdd/config.local.yaml: %w", err)
	}
	if cfg == nil || cfg.Participant == "" {
		return nil, fmt.Errorf("no participant configured; run `sdd init` or pass --participants")
	}
	return []string{cfg.Participant}, nil
}

// resolveParticipantFlag is the singular counterpart for `sdd wip start`.
// Same rule as resolveParticipantsFlag but returns a single string.
func resolveParticipantFlag(flagValue, sddDir string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	cfg, err := meta.ReadConfig(sddDir)
	if err != nil {
		return "", fmt.Errorf("reading .sdd/config.local.yaml: %w", err)
	}
	if cfg == nil || cfg.Participant == "" {
		return "", fmt.Errorf("no participant configured; run `sdd init` or pass --participant")
	}
	return cfg.Participant, nil
}

// resolveSDDDir discovers the .sdd/ directory by walking up from cwd.
// Errors if not found.
func resolveSDDDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	repoRoot := meta.DiscoverRoot(cwd)
	if repoRoot == "" {
		return "", fmt.Errorf("no .sdd/ directory found; run 'sdd init' first")
	}
	return meta.SDDDir(repoRoot), nil
}

// findRepoRoot returns the git repository root, falling back to cwd.
func findRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return os.Getwd()
	}
	return strings.TrimSpace(string(out)), nil
}

// graphDirPromptModel is a bubbletea model for the graph directory prompt.
type graphDirPromptModel struct {
	textInput textinput.Model
	done      bool
}

func newGraphDirPromptModel(defaultValue string) graphDirPromptModel {
	ti := textinput.New()
	ti.Placeholder = defaultValue
	ti.Focus()
	ti.Width = 60
	return graphDirPromptModel{textInput: ti}
}

func (m graphDirPromptModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m graphDirPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m graphDirPromptModel) View() string {
	return fmt.Sprintf("Graph directory (relative to repo root) [%s]: %s",
		m.textInput.Placeholder, m.textInput.View())
}

// promptGraphDir runs an interactive prompt for the graph directory.
func promptGraphDir(defaultValue string) (string, error) {
	m := newGraphDirPromptModel(defaultValue)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return "", err
	}
	final := result.(graphDirPromptModel)
	if !final.done {
		return "", fmt.Errorf("prompt cancelled")
	}
	value := strings.TrimSpace(final.textInput.Value())
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

// participantPromptModel is a bubbletea model for the participant-name prompt.
type participantPromptModel struct {
	textInput textinput.Model
	done      bool
}

func newParticipantPromptModel(defaultValue string) participantPromptModel {
	ti := textinput.New()
	ti.Placeholder = defaultValue
	ti.Focus()
	ti.Width = 60
	return participantPromptModel{textInput: ti}
}

func (m participantPromptModel) Init() tea.Cmd { return textinput.Blink }

func (m participantPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m participantPromptModel) View() string {
	return fmt.Sprintf("Participant name [%s]: %s",
		m.textInput.Placeholder, m.textInput.View())
}

// promptParticipant runs an interactive prompt for the local participant name.
// Empty input accepts the default. Cancellation returns an error.
func promptParticipant(defaultValue string) (string, error) {
	m := newParticipantPromptModel(defaultValue)
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}
	final := result.(participantPromptModel)
	if !final.done {
		return "", fmt.Errorf("prompt cancelled")
	}
	value := strings.TrimSpace(final.textInput.Value())
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

// languagePromptModel is a bubbletea model for the graph-language prompt.
type languagePromptModel struct {
	textInput textinput.Model
	done      bool
}

func newLanguagePromptModel(defaultValue string) languagePromptModel {
	ti := textinput.New()
	ti.Placeholder = defaultValue
	ti.Focus()
	ti.Width = 20
	return languagePromptModel{textInput: ti}
}

func (m languagePromptModel) Init() tea.Cmd { return textinput.Blink }

func (m languagePromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m languagePromptModel) View() string {
	return fmt.Sprintf("Graph language [%s]: %s",
		m.textInput.Placeholder, m.textInput.View())
}

// promptLanguage runs an interactive prompt for the graph authoring language.
// Empty input accepts the default. Cancellation returns an error.
func promptLanguage(defaultValue string) (string, error) {
	m := newLanguagePromptModel(defaultValue)
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}
	final := result.(languagePromptModel)
	if !final.done {
		return "", fmt.Errorf("prompt cancelled")
	}
	value := strings.TrimSpace(final.textInput.Value())
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

// gitUserName reads git config user.name, returning an empty string when
// git is unavailable or the setting isn't configured. Best-effort — used
// only as a pre-filled default for the sdd init participant prompt.
func gitUserName() string {
	out, err := exec.Command("git", "config", "--get", "user.name").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// isTerminal returns true if f is attached to an interactive terminal. Uses
// term.IsTerminal rather than os.FileMode checks because special devices
// like /dev/null are character devices but not terminals — the distinction
// matters when stdin is redirected, since bubbletea opens /dev/tty directly
// and fails in non-interactive contexts.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(f.Fd())
}

// confirmPromptModel is a bubbletea model for a single-char [y/N]
// confirmation. Reuses the same textinput.Model infrastructure as
// graphDirPromptModel for stylistic consistency with the d-tac-s2g flow.
type confirmPromptModel struct {
	textInput textinput.Model
	prompt    string
	done      bool
}

func newConfirmPromptModel(prompt string) confirmPromptModel {
	ti := textinput.New()
	ti.Placeholder = "N"
	ti.CharLimit = 1
	ti.Width = 3
	ti.Focus()
	return confirmPromptModel{textInput: ti, prompt: prompt}
}

func (m confirmPromptModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m confirmPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m confirmPromptModel) View() string {
	return fmt.Sprintf("%s [y/N]: %s", m.prompt, m.textInput.View())
}

// promptOverwriteModified asks the user whether to overwrite a user-edited
// skill file during sdd init. Default N (preserve). Returns false on empty
// input, EOF, or cancellation — the safe side is always "leave it alone."
func promptOverwriteModified(absPath string) (bool, error) {
	m := newConfirmPromptModel(fmt.Sprintf("Overwrite user-edited %s?", absPath))
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return false, err
	}
	final := result.(confirmPromptModel)
	if !final.done {
		return false, nil
	}
	v := strings.ToLower(strings.TrimSpace(final.textInput.Value()))
	return v == "y" || v == "yes", nil
}

func initCmd() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Initialize or refresh the SDD project (idempotent)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "graph-dir",
				Usage: "Graph directory relative to repo root (prompted interactively on fresh init)",
			},
			&cli.StringFlag{
				Name:  "participant",
				Usage: "Canonical author name recorded in .sdd/config.local.yaml (prompted interactively with git user.name as default when unset)",
			},
			&cli.StringFlag{
				Name:  "language",
				Usage: "Graph authoring language as a locale code, e.g. en, de (prompted interactively on fresh init with en as default)",
			},
			&cli.StringFlag{
				Name:  "scope",
				Usage: "Where to install skills: user (~/.claude/skills) or project (.claude/skills)",
				Value: string(model.DefaultScope),
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Overwrite user-modified skill files without prompting",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			repoRoot, err := findRepoRoot()
			if err != nil {
				return fmt.Errorf("finding repo root: %w", err)
			}

			sddDir := filepath.Join(repoRoot, model.SDDDirName)
			sddExists := false
			if _, err := os.Stat(sddDir); err == nil {
				sddExists = true
			}

			graphDir := cmd.String("graph-dir")
			if graphDir == "" && !sddExists && isTerminal(os.Stdin) {
				prompted, err := promptGraphDir(model.DefaultGraphDir)
				if err != nil {
					return fmt.Errorf("prompt: %w", err)
				}
				graphDir = prompted
			}
			if graphDir == "" {
				graphDir = model.DefaultGraphDir
			}

			// Resolve graph language: explicit flag wins; otherwise prompt
			// only on fresh init with stdin interactive. The default is
			// `en` (English); choosing it still writes the key so future
			// readers of the file don't have to infer default vs. unset.
			language := strings.TrimSpace(cmd.String("language"))
			if language == "" && !sddExists && isTerminal(os.Stdin) {
				prompted, err := promptLanguage("en")
				if err != nil {
					return fmt.Errorf("prompt: %w", err)
				}
				language = prompted
			}

			scope := model.Scope(cmd.String("scope"))
			if scope != model.ScopeUser && scope != model.ScopeProject {
				return fmt.Errorf("invalid --scope: %s (use user or project)", scope)
			}
			userHome, _ := os.UserHomeDir()

			// Resolve participant name: explicit flag wins; otherwise we
			// only prompt when the config doesn't already carry a
			// participant *and* stdin is interactive. Re-runs with a
			// populated config stay silent (idempotent housekeeping).
			participant := strings.TrimSpace(cmd.String("participant"))
			if participant == "" {
				var existing *model.Config
				if sddExists {
					existing, _ = meta.ReadConfig(sddDir)
				}
				if existing == nil || existing.Participant == "" {
					if isTerminal(os.Stdin) {
						def := gitUserName()
						prompted, err := promptParticipant(def)
						if err != nil {
							return fmt.Errorf("prompt: %w", err)
						}
						participant = prompted
					}
				}
			}

			icmd := &command.InitCmd{
				RepoRoot:      repoRoot,
				GraphDir:      graphDir,
				Participant:   participant,
				Language:      language,
				BinaryVersion: version,
				Target:        model.DefaultAgentTarget,
				Scope:         scope,
				UserHome:      userHome,
				Force:         cmd.Bool("force"),
				PromptOverwrite: func(path string) (bool, error) {
					if !isTerminal(os.Stdin) {
						return false, nil
					}
					return promptOverwriteModified(path)
				},
				OnCreated: func(sddDir, absGraphDir string) {
					fmt.Printf("created %s\n", sddDir)
					fmt.Printf("  graph: %s\n", absGraphDir)
				},
				OnMigrated: func(count int) {
					fmt.Fprintf(os.Stderr, "  migrated %d file(s) from .sdd-tmp/\n", count)
				},
				OnGitignoreUpdated: func(path string) {
					fmt.Fprintf(os.Stderr, "  updated %s\n", path)
				},
				OnMetaWritten: func(path string) {
					fmt.Printf("  meta: %s\n", path)
				},
				OnParticipantWritten: func(path, name string) {
					fmt.Printf("  participant: %s → %s\n", name, path)
				},
				OnSkillsInstalled: func(result command.SkillInstallResult) {
					installDir := scopeInstallDir(repoRoot, userHome, scope)
					presenters.RenderInitSkills(os.Stdout, installDir, result)
				},
			}

			reader, err := newReadFinder()
			if err != nil {
				return err
			}
			handler := handlers.New(handlers.Options{
				Reader:    reader,
				Committer: gitCommitterFunc(gitCommit),
			})
			return handler.Init(ctx, icmd)
		},
	}
}

// scopeInstallDir resolves the install directory for the selected scope.
// Errors are swallowed because this is a display-only derivation — the
// handler already validated the scope.
func scopeInstallDir(repoRoot, userHome string, scope model.Scope) string {
	d, err := model.SkillInstallDir(model.DefaultAgentTarget, scope, repoRoot, userHome)
	if err != nil {
		return ""
	}
	return d
}

func wipCmd() *cli.Command {
	return &cli.Command{
		Name:  "wip",
		Usage: "Manage work-in-progress markers",
		Commands: []*cli.Command{
			wipStartCmd(),
			wipDoneCmd(),
			wipListCmd(),
		},
	}
}

func wipStartCmd() *cli.Command {
	return &cli.Command{
		Name:      "start",
		Usage:     "Create a WIP marker for a graph entry",
		ArgsUsage: "<entry-id> [description]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "exclusive",
				Usage: "Discourage parallel work on this entry",
			},
			&cli.StringFlag{
				Name:  "participant",
				Usage: "Participant name (falls back to .sdd/config.local.yaml; run `sdd init` to configure)",
			},
			&cli.BoolFlag{
				Name:  "branch",
				Usage: "Create a git branch and check out to it",
			},
		},
		Action: withWriteGate(func(ctx context.Context, cmd *cli.Command) error {
			args := cmd.Args()
			if args.Len() < 1 {
				return fmt.Errorf("usage: sdd wip start <entry-id> [description]")
			}

			sddDir, err := resolveSDDDir()
			if err != nil {
				return err
			}
			participant, err := resolveParticipantFlag(cmd.String("participant"), sddDir)
			if err != nil {
				return err
			}

			startCmd := &command.StartWIPCmd{
				EntryID:     args.Get(0),
				Description: strings.Join(args.Slice()[1:], " "),
				Participant: participant,
				Exclusive:   cmd.Bool("exclusive"),
				Branch:      cmd.Bool("branch"),
				OnStarted: func(markerID, markerPath string) {
					fmt.Println(markerID)
					fmt.Printf("  → %s\n", markerPath)
				},
				OnBranchCreated: func(branch string) {
					fmt.Printf("  branch: %s (checked out)\n", branch)
				},
				OnExclusiveCollision: func(existing *model.WIPMarker) {
					fmt.Fprintf(os.Stderr, "warning: exclusive marker exists for %s by %s (%s)\n",
						existing.Entry, existing.Participant, existing.ID)
				},
			}

			dir, err := resolveGraphDir(cmd)
			if err != nil {
				return err
			}
			reader, err := newReadFinder()
			if err != nil {
				return err
			}
			handler := handlers.New(handlers.Options{
				GraphDir:  dir,
				Reader:    reader,
				Committer: gitCommitterFunc(gitCommit),
				Brancher:  gitBrancher{},
			})
			return handler.StartWIP(ctx, startCmd)
		}),
	}
}

func wipDoneCmd() *cli.Command {
	return &cli.Command{
		Name:      "done",
		Usage:     "Remove a WIP marker",
		ArgsUsage: "<marker-id>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Force-delete unmerged branch (discard flow)",
			},
		},
		Action: withWriteGate(func(ctx context.Context, cmd *cli.Command) error {
			args := cmd.Args()
			if args.Len() < 1 {
				return fmt.Errorf("usage: sdd wip done <marker-id>")
			}

			doneCmd := &command.FinishWIPCmd{
				MarkerID: args.Get(0),
				Force:    cmd.Bool("force"),
				OnRemoved: func(id string) {
					fmt.Printf("removed %s\n", id)
				},
				OnBranchDeleted: func(branch string, forced bool) {
					if forced {
						fmt.Printf("  force-deleted branch %s (unmerged)\n", branch)
					} else {
						fmt.Printf("  deleted branch %s (merged)\n", branch)
					}
				},
				OnBranchPreserved: func(branch string) {
					fmt.Fprintf(os.Stderr, "  warning: branch %s has unmerged changes — marker removed but branch preserved\n", branch)
					fmt.Fprintln(os.Stderr, "  use --force to delete the unmerged branch, or merge it first")
				},
			}

			dir, err := resolveGraphDir(cmd)
			if err != nil {
				return err
			}
			reader, err := newReadFinder()
			if err != nil {
				return err
			}
			handler := handlers.New(handlers.Options{
				GraphDir:  dir,
				Reader:    reader,
				Committer: gitCommitterFunc(gitRemoveAndCommit),
				Brancher:  gitBrancher{},
			})
			return handler.FinishWIP(ctx, doneCmd)
		}),
	}
}

// gitRemoveAndCommit stages the deletion of the given paths and commits.
// Used by FinishWIP — the marker file has already been removed from disk
// when this runs, so we use `git rm --cached` (or `git add` as fallback)
// to stage the deletion before committing.
func gitRemoveAndCommit(message string, paths ...string) error {
	for _, p := range paths {
		rm := exec.Command("git", "rm", "--cached", "-f", p)
		if out, err := rm.CombinedOutput(); err != nil {
			add := exec.Command("git", "add", p)
			if out2, err2 := add.CombinedOutput(); err2 != nil {
				return fmt.Errorf("git stage: %s (%v); fallback %s (%w)", out, err, out2, err2)
			}
		}
	}
	commit := exec.Command("git", "commit", "-m", message)
	if out, err := commit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s (%w)", out, err)
	}
	return nil
}

func wipListCmd() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List all active WIP markers",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dir, err := resolveGraphDir(cmd)
			if err != nil {
				return err
			}
			f, err := newReadFinder()
			if err != nil {
				return err
			}
			result, err := f.WIPList(query.WIPListQuery{GraphDir: dir})
			if err != nil {
				return fmt.Errorf("loading WIP markers: %w", err)
			}
			presenters.RenderWIPList(os.Stdout, result)
			return nil
		},
	}
}

// parseAttachSpec splits an --attach value into source and target.
// Formats: "path" (target=""), "source:target", "-:target" (stdin).
// Splits on the last colon to tolerate colons in source paths.
func parseAttachSpec(spec string) (source, target string) {
	i := strings.LastIndex(spec, ":")
	if i < 0 {
		return spec, ""
	}
	return spec[:i], spec[i+1:]
}

type attachment struct {
	source string // file path or "-" for stdin
	target string // destination filename
	data   []byte // populated for stdin
}

// parseAttachFlags parses and validates a list of --attach flag values.
// stdinReader is used when source is "-"; pass nil if stdin is not available.
func parseAttachFlags(specs []string, stdinReader io.Reader) ([]attachment, error) {
	var attachments []attachment
	stdinUsed := false
	for _, spec := range specs {
		src, tgt := parseAttachSpec(spec)
		if src == "-" {
			if stdinUsed {
				return nil, fmt.Errorf("stdin (-) can only be used once in --attach")
			}
			if tgt == "" {
				return nil, fmt.Errorf("stdin (-) requires a target name: --attach -:filename")
			}
			stdinUsed = true
			if stdinReader == nil {
				return nil, fmt.Errorf("stdin not available")
			}
			data, err := io.ReadAll(stdinReader)
			if err != nil {
				return nil, fmt.Errorf("reading stdin for attachment: %w", err)
			}
			attachments = append(attachments, attachment{source: "-", target: tgt, data: data})
		} else {
			if _, err := os.Stat(src); err != nil {
				return nil, fmt.Errorf("attachment file not found: %s", src)
			}
			if tgt == "" {
				tgt = filepath.Base(src)
			}
			attachments = append(attachments, attachment{source: src, target: tgt})
		}
	}
	return attachments, nil
}
