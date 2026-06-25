package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/llm/factory"
	"github.com/networkteam/sdd/internal/llmstats"
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
			// sdd index is an explicit progress command: on a non-TTY it shows
			// its per-entry indexed lines (Info) by default. The transient TTY
			// view governs its own display level independently.
			if cmd.Args().First() == "index" {
				level = slog.LevelInfo
			}
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

			// Attach a stats sink so LLM calls (pre-flight, summarize) record
			// token + prompt-cache metrics to .sdd/stats/llm.jsonl. Best-effort:
			// if .sdd/ isn't discoverable or the sink can't be created, LLM
			// calls simply record nothing.
			if sddDir, err := resolveSDDDir(); err == nil {
				if sink, err := llmstats.NewFileSink(filepath.Join(sddDir, "stats")); err == nil {
					ctx = llm.WithStatsSink(ctx, sink)
				}
			}

			// Background sync check runs on every command except `sdd init`
			// (bootstrap may precede remote configuration — sync would emit
			// spurious warnings). Cooldown-bound and timeout-bound internally;
			// any failure logs at Debug and does not affect the command.
			if cmd.Args().First() != "init" {
				runSyncCheck(ctx)
				// Participant-missing nudge (AC 8): one-line stderr
				// warning naming `sdd init` as the fix. Suppressed for
				// `sdd init` itself (it's the resolution path) and
				// silently dropped when no .sdd/ is discoverable (the
				// inner command will emit its own "run sdd init"
				// guidance). The `--quiet` / structured-output
				// suppression branch is deferred until those flags
				// exist as a real surface.
				warnIfParticipantMissing()
			}
			return ctx, nil
		},
		Commands: []*cli.Command{
			initCmd(),
			infoCmd(),
			showCmd(),
			viewCmd(),
			newCmd(),
			rewriteCmd(),
			wipCmd(),
			lintCmd(),
			summarizeCmd(),
			indexCmd(),
			searchCmd(),
			serveCmd(),
			syncCmd(),
			statsCmd(),
		},
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

func infoCmd() *cli.Command {
	return &cli.Command{
		Name:  "info",
		Usage: "Show session framing — participant, language, search capability",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			f, err := newReadFinder()
			if err != nil {
				return err
			}
			result, err := f.Info(query.InfoQuery{})
			if err != nil {
				return err
			}
			presenters.RenderInfo(os.Stdout, result)
			return nil
		},
	}
}

func statsCmd() *cli.Command {
	return &cli.Command{
		Name:  "stats",
		Usage: "Show LLM/embedding usage analytics from the local stats sink",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "since",
				Usage: "Date range: all-time (default), a duration (7d, 30d, 2w, 1m), or YYYY-MM-DD",
			},
			&cli.StringFlag{
				Name:  "op",
				Usage: "Filter by operation (preflight, summarize, embed-documents, embed-queries)",
			},
			&cli.StringFlag{
				Name:  "provider",
				Usage: "Filter by provider (e.g. anthropic, ollama, openai)",
			},
			&cli.StringFlag{
				Name:  "model",
				Usage: "Filter by model name",
			},
			&cli.StringFlag{
				Name:  "format",
				Usage: "Output format: auto (default — styled table on a TTY, JSON otherwise) or json",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			sddDir, err := resolveSDDDir()
			if err != nil {
				return err
			}

			now := time.Now()
			var since *time.Time
			if spec := strings.TrimSpace(cmd.String("since")); spec != "" && spec != "all-time" && spec != "all" {
				cutoff, err := model.ResolveSinceSpec(spec, now)
				if err != nil {
					return err
				}
				since = &cutoff
			}

			f, err := newReadFinder()
			if err != nil {
				return err
			}
			result, err := f.Stats(query.StatsQuery{
				StatsDir: filepath.Join(sddDir, "stats"),
				Since:    since,
				Until:    now,
				Op:       cmd.String("op"),
				Provider: cmd.String("provider"),
				Model:    cmd.String("model"),
			})
			if err != nil {
				return err
			}

			// Non-TTY consumers (and an explicit --format json) get clean
			// structured stdout; interactive terminals get the styled table
			// (per d-cpt-mvb / d-cpt-5f4).
			if cmd.String("format") == "json" || !isTerminal(os.Stdout) {
				return presenters.RenderStatsJSON(os.Stdout, result)
			}
			presenters.RenderStatsTable(os.Stdout, result)
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
				Name:  "up",
				Value: query.DefaultUpDepth,
				Usage: "Upstream (grounding) expansion depth; 0 = no upstream",
			},
			&cli.IntFlag{
				Name:  "down",
				Value: query.DefaultDownDepth,
				Usage: "Downstream (consumers) expansion depth; 0 = no downstream",
			},
			&cli.BoolFlag{
				Name:  "with-summary",
				Usage: "Include the primary's stored summary in the envelope (for drift review)",
			},
			&cli.StringFlag{
				Name:  "format",
				Usage: "Output format: auto (default — styled on a TTY, plain markdown otherwise) or text",
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
				Graph:     g,
				IDs:       ids,
				UpDepth:   int(cmd.Int("up")),
				DownDepth: int(cmd.Int("down")),
			})
			if err != nil {
				return err
			}
			opts := presenters.ShowOptions{WithSummary: cmd.Bool("with-summary")}

			// Renderer selection: an explicit --format text, NO_COLOR, or a
			// non-terminal stdout all take the plain markdown renderer; an
			// interactive terminal gets the styled view (d-cpt-5f4 / d-cpt-mvb).
			if cmd.String("format") == "text" || os.Getenv("NO_COLOR") != "" || !isTerminal(os.Stdout) {
				presenters.RenderShow(os.Stdout, result, opts)
				return nil
			}
			if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil {
				opts.Width = w
			}
			presenters.RenderShowStyled(os.Stdout, result, opts)
			return nil
		},
	}
}

func newCmd() *cli.Command {
	return &cli.Command{
		Name:      "new",
		Usage:     "Create a new graph entry",
		ArgsUsage: "<type> <layer> [description]",
		// Disable v3's slice-flag CSV-splitting at the subcommand level too —
		// the root toggle does not propagate to subcommands in v3, and our
		// JSON-bearing flags (--involvement, --topic) live here.
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:  "refs",
				Usage: "Reference (repeatable) — JSON object {\"id\":\"<id>\",\"kind\":\"<kind>\",\"desc\":\"<optional>\"}. Kind is one of: grounded-in, builds-on, refines, addresses, surfaces, surfaced-by, depends-on, required-by, related.",
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
				Name:  "intent",
				Usage: "Directive lifecycle posture (pending, guiding, settled) — required on directives, rejected on other kinds",
			},
			&cli.StringFlag{
				Name:  "kind",
				Usage: "Entry kind: signals — gap (default), fact, question, insight, done, actor, annotation; decisions — directive (default), activity, plan, contract, aspiration, role, focus",
			},
			&cli.StringFlag{
				Name:  "canonical",
				Usage: "Canonical identity name (kind: actor only) — written into frontmatter",
			},
			&cli.StringFlag{
				Name:  "aliases",
				Usage: "Comma-separated aliases (kind: actor only) — read-side convenience for mining and comprehension",
			},
			&cli.StringFlag{
				Name:  "actor",
				Usage: "Canonical name the role binds to (kind: role only) — must match an active actor chain's head canonical",
			},
			&cli.StringFlag{
				Name:  "topics",
				Usage: "Comma-separated topic labels (any kind) — inline topic membership written as `topics:` strings",
			},
			&cli.StringSliceFlag{
				Name:  "topic",
				Usage: "Topic cluster (kind: annotation only) — JSON object {\"label\":\"path\",\"members\":[\"id\",...]}; repeatable. A plain string in --topics works too for whole-refs membership.",
			},
			&cli.StringFlag{
				Name:  "actors",
				Usage: "Comma-separated focus-level default actor canonicals (kind: focus only)",
			},
			&cli.StringFlag{
				Name:  "when",
				Usage: "Focus-level default temporal scope (kind: focus only) — JSON object {\"from\":\"YYYY-MM-DD\",\"to\":\"YYYY-MM-DD\"}; at least one of from/to required",
			},
			&cli.StringSliceFlag{
				Name:  "involvement",
				Usage: "Involvement triple (kind: focus only) — JSON object {\"target\":\"<id>\",\"actors\":[\"...\"],\"when\":{\"from\":\"...\",\"to\":\"...\"}}; repeatable. actors omitted inherits focus default; explicit \"actors\":[] means pull-available.",
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

			focusActors := splitCSV(cmd.String("actors"))
			focusWhen, err := parseWhenFlag(cmd.String("when"))
			if err != nil {
				return err
			}
			involvement, err := parseInvolvementFlags(cmd.StringSlice("involvement"))
			if err != nil {
				return err
			}
			annotationTopics, err := parseAnnotationTopicFlags(cmd.StringSlice("topic"))
			if err != nil {
				return err
			}
			refs, err := parseRefFlags(cmd.StringSlice("refs"))
			if err != nil {
				return err
			}

			ncmd := &command.NewEntryCmd{
				Type:             typ,
				Layer:            layer,
				Kind:             kind,
				Intent:           strings.TrimSpace(cmd.String("intent")),
				Description:      description,
				Participants:     participants,
				Refs:             refs,
				Supersedes:       splitCSV(cmd.String("supersedes")),
				Closes:           splitCSV(cmd.String("closes")),
				Confidence:       confidence,
				Canonical:        strings.TrimSpace(cmd.String("canonical")),
				Aliases:          splitCSV(cmd.String("aliases")),
				Actor:            strings.TrimSpace(cmd.String("actor")),
				TopicLabels:      splitCSV(cmd.String("topics")),
				AnnotationTopics: annotationTopics,
				FocusActors:      focusActors,
				FocusWhen:        focusWhen,
				Involvement:      involvement,
				Attachments:      atts,
				SkipPreflight:    cmd.Bool("skip-preflight"),
				DryRun:           cmd.Bool("dry-run"),
				PreflightModel:   cmd.String("preflight-model"),
				PreflightTimeout: resolveTimeout(cmd, "preflight-timeout"),
				OnNewEntry: func(id, summary string) {
					fmt.Println(id + ".md")
					if rel, err := model.IDToRelPath(id); err == nil {
						fmt.Printf("  → %s\n", filepath.Join(dir, rel))
					}
					if summary != "" {
						fmt.Printf("  Summary: %s\n", summary)
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
			populateIndexLint(cmd, result)
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
			&cli.StringFlag{
				Name:  "text",
				Usage: "Write the supplied text as the summary, bypassing the LLM. Use '-' to read from stdin. Single entry only.",
			},
		},
		Action: withWriteGate(func(ctx context.Context, cmd *cli.Command) error {
			ctx = slogutils.WithLogger(ctx, slogutils.FromContext(ctx).With("command", "summarize"))

			ids := cmd.Args().Slice()
			if len(ids) == 0 && !cmd.Bool("all") {
				return fmt.Errorf("usage: sdd summarize <id>... or sdd summarize --all")
			}

			var explicitText *string
			if cmd.IsSet("text") {
				if cmd.Bool("all") {
					return fmt.Errorf("--text cannot be combined with --all (single entry only)")
				}
				if len(ids) != 1 {
					return fmt.Errorf("--text requires exactly one entry ID (got %d)", len(ids))
				}
				value := cmd.String("text")
				if value == "-" {
					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						return fmt.Errorf("reading --text from stdin: %w", err)
					}
					value = string(data)
				}
				explicitText = &value
			}

			sumCmd := &command.SummarizeCmd{
				EntryIDs:     ids,
				Force:        cmd.Bool("force"),
				Model:        cmd.String("model"),
				Timeout:      resolveTimeout(cmd, "timeout"),
				Concurrency:  int(cmd.Int("concurrency")),
				ExplicitText: explicitText,
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

	// Scope the commit to exactly the staged paths with an explicit pathspec.
	// Without `-- <paths>`, `git commit` records the whole index, sweeping any
	// pre-staged unrelated work into the CLI's own commit.
	commitArgs := append([]string{"commit", "-m", message, "--"}, filePaths...)
	commit := exec.Command("git", commitArgs...)
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
	ti.SetWidth(60)
	return graphDirPromptModel{textInput: ti}
}

func (m graphDirPromptModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m graphDirPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m graphDirPromptModel) View() tea.View {
	return tea.NewView(fmt.Sprintf("Graph directory (relative to repo root) [%s]: %s",
		m.textInput.Placeholder, m.textInput.View()))
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
	ti.SetWidth(60)
	return participantPromptModel{textInput: ti}
}

func (m participantPromptModel) Init() tea.Cmd { return textinput.Blink }

func (m participantPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m participantPromptModel) View() tea.View {
	return tea.NewView(fmt.Sprintf("Participant name [%s]: %s",
		m.textInput.Placeholder, m.textInput.View()))
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
	ti.SetWidth(20)
	return languagePromptModel{textInput: ti}
}

func (m languagePromptModel) Init() tea.Cmd { return textinput.Blink }

func (m languagePromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m languagePromptModel) View() tea.View {
	return tea.NewView(fmt.Sprintf("Graph language [%s]: %s",
		m.textInput.Placeholder, m.textInput.View()))
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

// scopePromptModel is a bubbletea model for the skill-scope selector. Two
// fixed options (project, user) navigated by ↑/↓ or j/k; Enter confirms.
// Per d-tac-07q the cursor starts on `project` so the keystroke-free path
// installs into the repo-local tree — friction-minimising for contributors
// cloning an SDD-instrumented repo.
type scopePromptModel struct {
	options []scopeOption
	cursor  int
	done    bool
}

type scopeOption struct {
	value model.Scope
	label string
	hint  string
}

func newScopePromptModel() scopePromptModel {
	return scopePromptModel{
		options: []scopeOption{
			{value: model.ScopeProject, label: "project", hint: ".claude/skills/ in this repo (recommended for shared SDD-instrumented repos)"},
			{value: model.ScopeUser, label: "user", hint: "~/.claude/skills/ shared across all projects on this machine"},
		},
		cursor: 0,
	}
}

func (m scopePromptModel) Init() tea.Cmd { return nil }

func (m scopePromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m scopePromptModel) View() tea.View {
	var b strings.Builder
	b.WriteString("Where should skills be installed? (↑/↓ to navigate, enter to confirm)\n")
	for i, opt := range m.options {
		marker := "  "
		if i == m.cursor {
			marker = "› "
		}
		fmt.Fprintf(&b, "%s%s — %s\n", marker, opt.label, opt.hint)
	}
	return tea.NewView(b.String())
}

// promptScope runs the interactive scope selector. Returns the chosen scope
// or an error on cancellation. The caller is responsible for the
// non-interactive branch — this function unconditionally opens a TTY.
func promptScope() (model.Scope, error) {
	m := newScopePromptModel()
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}
	final := result.(scopePromptModel)
	if !final.done {
		return "", fmt.Errorf("prompt cancelled")
	}
	return final.options[final.cursor].value, nil
}

// agentsPromptModel is a bubbletea model for the supported-agents multi-select
// shown on fresh init. Options are toggled with space and confirmed with enter;
// at least one must be selected. Claude starts selected so the keystroke-light
// path matches the pre-multi-agent default.
type agentsPromptModel struct {
	options []agentOption
	cursor  int
	done    bool
}

type agentOption struct {
	value    model.AgentTarget
	label    string
	hint     string
	selected bool
}

func newAgentsPromptModel() agentsPromptModel {
	return agentsPromptModel{
		options: []agentOption{
			{value: model.AgentClaude, label: "claude", hint: ".claude/skills/ — Claude Code", selected: true},
			{value: model.AgentCodex, label: "codex", hint: ".agents/skills/ — Codex (Agent Skills standard)"},
		},
	}
}

func (m agentsPromptModel) Init() tea.Cmd { return nil }

func (m agentsPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			for _, o := range m.options {
				if o.selected {
					m.done = true
					return m, tea.Quit
				}
			}
			// No selection yet — ignore enter until at least one is chosen.
		case " ", "space":
			m.options[m.cursor].selected = !m.options[m.cursor].selected
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m agentsPromptModel) View() tea.View {
	var b strings.Builder
	b.WriteString("Which agents should sdd render skills for? (↑/↓ navigate, space toggle, enter confirm)\n")
	for i, opt := range m.options {
		cursor := "  "
		if i == m.cursor {
			cursor = "› "
		}
		check := "[ ]"
		if opt.selected {
			check = "[x]"
		}
		fmt.Fprintf(&b, "%s%s %s — %s\n", cursor, check, opt.label, opt.hint)
	}
	return tea.NewView(b.String())
}

// promptAgents runs the interactive supported-agents multi-select, returning
// the chosen targets or an error on cancellation. The caller is responsible for
// the non-interactive branch — this function unconditionally opens a TTY.
func promptAgents() ([]model.AgentTarget, error) {
	m := newAgentsPromptModel()
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return nil, err
	}
	final := result.(agentsPromptModel)
	if !final.done {
		return nil, fmt.Errorf("prompt cancelled")
	}
	var chosen []model.AgentTarget
	for _, o := range final.options {
		if o.selected {
			chosen = append(chosen, o.value)
		}
	}
	return chosen, nil
}

// warnIfParticipantMissing emits a one-line stderr nudge when no local
// participant is configured. Silently noops outside SDD-instrumented
// repos and on any read error — the surface is informational, not a
// gate, and downstream commands have their own "no participant
// configured" errors for the cases where one is actually required.
//
// The Before hook in main() guards `sdd init` so the warning never
// fires from inside the resolution path. A future `--quiet` / structured
// output mode would suppress it here too; the call site is the right
// place to attach that branch when those flags land.
func warnIfParticipantMissing() {
	sddDir, err := resolveSDDDir()
	if err != nil {
		return
	}
	cfg, err := meta.ReadConfig(sddDir)
	if err != nil {
		return
	}
	if cfg == nil || strings.TrimSpace(cfg.Participant) == "" {
		fmt.Fprintln(os.Stderr, "sdd: no local participant configured — run `sdd init` to set one")
	}
}

// readRecordedSkillScope reads the persisted skill_scope value from
// .sdd/config.yaml only (no overlay from config.local.yaml). Returns empty
// when the file is missing, unparseable, or the field is absent — every
// non-empty case means the operator (or a prior init) made a deliberate
// choice we should honor.
//
// The CLI uses this before deciding whether to prompt; the handler does
// the same check to gate persistence and the contradiction error. Both
// callers stick to the shared file because skill_scope is a project-level
// decision (everyone on the repo installs to the same place) — local
// override is undefined behavior.
func readRecordedSkillScope(sddDir string) model.Scope {
	data, err := os.ReadFile(filepath.Join(sddDir, "config.yaml"))
	if err != nil {
		return ""
	}
	cfg, err := model.ParseConfig(data)
	if err != nil || cfg == nil {
		return ""
	}
	return cfg.SkillScope
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
	ti.SetWidth(3)
	ti.Focus()
	return confirmPromptModel{textInput: ti, prompt: prompt}
}

func (m confirmPromptModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m confirmPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m confirmPromptModel) View() tea.View {
	return tea.NewView(fmt.Sprintf("%s [y/N]: %s", m.prompt, m.textInput.View()))
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
			&cli.StringSliceFlag{
				Name:  "agents",
				Usage: "Agent targets to render skills for: claude, codex (prompted interactively on fresh init; defaults to claude)",
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Overwrite user-modified skill files without prompting",
			},
			&cli.BoolFlag{
				Name:  "bump",
				Usage: "Raise .sdd/meta.json minimum_version to this binary's version (released builds only)",
			},
		},
		Action: withWriteGate(func(ctx context.Context, cmd *cli.Command) error {
			// `--bump` cannot be honoured from a dev build — fail before
			// any other side effects so the operator gets the explicit
			// guidance instead of a partial run.
			if cmd.Bool("bump") && model.IsDevVersion(version) {
				return fmt.Errorf("cannot bump from a dev build, use a released sdd binary")
			}
			repoRoot, err := findRepoRoot()
			if err != nil {
				return fmt.Errorf("finding repo root: %w", err)
			}

			sddDir := filepath.Join(repoRoot, model.SDDDirName)
			sddExists := false
			if _, err := os.Stat(sddDir); err == nil {
				sddExists = true
			}

			// Read everything we know about the current state up-front so
			// the missing-piece detection (for the non-interactive
			// aggregated error) and the per-piece prompts work from the
			// same picture.
			scope := model.Scope(cmd.String("scope"))
			scopeExplicit := cmd.IsSet("scope")
			if scope != model.ScopeUser && scope != model.ScopeProject {
				return fmt.Errorf("invalid --scope: %s (use user or project)", scope)
			}
			var recordedScope model.Scope
			if sddExists {
				recordedScope = readRecordedSkillScope(sddDir)
			}
			var existingMerged *model.Config
			if sddExists {
				existingMerged, _ = meta.ReadConfig(sddDir)
			}
			recordedParticipant := ""
			if existingMerged != nil {
				recordedParticipant = existingMerged.Participant
			}
			languageFlag := strings.TrimSpace(cmd.String("language"))
			participantFlag := strings.TrimSpace(cmd.String("participant"))

			// Aggregated non-interactive error (AC 5): a single message
			// names every missing piece and the exact flag to fix it,
			// rather than failing the run on the first one. Runs only
			// when stdin is not a TTY — interactive callers fall through
			// to the per-piece prompts below.
			if !isTerminal(os.Stdin) {
				var missing []string
				if !sddExists && languageFlag == "" {
					missing = append(missing, "--language LOCALE   (e.g. --language en — graph authoring language)")
				}
				if !scopeExplicit && recordedScope == "" {
					missing = append(missing, "--scope project|user   (where to install skills)")
				}
				if participantFlag == "" && recordedParticipant == "" {
					missing = append(missing, "--participant NAME   (canonical author name)")
				}
				if len(missing) > 0 {
					return fmt.Errorf(
						"sdd init needs values for the following flags to run non-interactively:\n  %s",
						strings.Join(missing, "\n  "),
					)
				}
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
			language := languageFlag
			if language == "" && !sddExists && isTerminal(os.Stdin) {
				prompted, err := promptLanguage("en")
				if err != nil {
					return fmt.Errorf("prompt: %w", err)
				}
				language = prompted
			}

			// Run the scope selector when no value has been recorded and the
			// operator didn't pass --scope. The non-interactive branch is
			// already handled by the aggregated error above; here we only
			// reach the prompt when stdin is a TTY. The choice flows
			// through with ScopeExplicit=true; the handler's contradiction
			// check is a no-op against an absent recorded value, so
			// flipping the bool doesn't change behavior.
			if !scopeExplicit && recordedScope == "" {
				chosen, err := promptScope()
				if err != nil {
					return fmt.Errorf("prompt: %w", err)
				}
				scope = chosen
				scopeExplicit = true
			}
			userHome, _ := os.UserHomeDir()

			// Resolve participant name: explicit flag wins; otherwise we
			// only prompt when the config doesn't already carry a
			// participant. Non-interactive runs are caught by the
			// aggregated error above; here we only prompt on a TTY.
			participant := participantFlag
			if participant == "" && recordedParticipant == "" && isTerminal(os.Stdin) {
				def := gitUserName()
				prompted, err := promptParticipant(def)
				if err != nil {
					return fmt.Errorf("prompt: %w", err)
				}
				participant = prompted
			}

			// Resolve agent targets: an explicit --agents flag wins; otherwise,
			// on a fresh tree with a TTY, run the multi-select. On existing
			// trees we pass nothing and let the handler honor the recorded
			// supported_agents (or the Claude-only default).
			var targets []model.AgentTarget
			if agentsFlag := cmd.StringSlice("agents"); len(agentsFlag) > 0 {
				for _, a := range agentsFlag {
					t, err := model.ParseAgentTarget(strings.TrimSpace(a))
					if err != nil {
						return fmt.Errorf("invalid --agents: %w", err)
					}
					targets = append(targets, t)
				}
			} else if !sddExists && isTerminal(os.Stdin) {
				chosen, err := promptAgents()
				if err != nil {
					return fmt.Errorf("prompt: %w", err)
				}
				targets = chosen
			}

			icmd := &command.InitCmd{
				RepoRoot:      repoRoot,
				GraphDir:      graphDir,
				Participant:   participant,
				Language:      language,
				BinaryVersion: version,
				Targets:       targets,
				Scope:         scope,
				ScopeExplicit: scopeExplicit,
				UserHome:      userHome,
				Force:         cmd.Bool("force"),
				Bump:          cmd.Bool("bump"),
				OnMinimumVersionBumped: func(previous, current string) {
					if previous == "" {
						fmt.Printf("  minimum_version: → %s\n", current)
					} else {
						fmt.Printf("  minimum_version: %s → %s\n", previous, current)
					}
				},
				OnMinimumVersionUnchanged: func(current string) {
					fmt.Printf("  minimum_version: %s (unchanged)\n", current)
				},
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
					// result.InstallDir reflects the scope the handler
					// actually used — which may be the recorded value
					// rather than the flag default — so it stays
					// authoritative even when the CLI's local `scope`
					// variable is just the fallback.
					presenters.RenderInitSkills(os.Stdout, result.InstallDir, result)
				},
				OnBridgeScaffolded: func(paths []string) {
					for _, p := range paths {
						fmt.Printf("  bridge: %s\n", p)
					}
				},
				OnAgentSkillsPruned: func(result command.AgentPruneResult) {
					presenters.RenderInitPrune(os.Stdout, result)
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
		}),
	}
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
	// Scope to the given paths (see gitCommit) so an unrelated staged index
	// isn't swept into the WIP-marker removal commit.
	commitArgs := append([]string{"commit", "-m", message, "--"}, paths...)
	commit := exec.Command("git", commitArgs...)
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

// parseWhenFlag parses a --when JSON value into a *model.FocusWhen.
// Empty input returns nil so the field is omitted on entries that don't
// declare a focus-level temporal default.
func parseWhenFlag(s string) (*model.FocusWhen, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var w model.FocusWhen
	if err := json.Unmarshal([]byte(s), &w); err != nil {
		return nil, fmt.Errorf("--when: invalid JSON: %w", err)
	}
	if err := w.Validate(); err != nil {
		return nil, fmt.Errorf("--when: %w", err)
	}
	return &w, nil
}

// parseInvolvementFlags parses each --involvement JSON value into a
// model.Involvement, preserving the actors-omitted vs explicit-empty
// distinction (the latter declares pull-available involvement).
func parseInvolvementFlags(specs []string) ([]model.Involvement, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	out := make([]model.Involvement, 0, len(specs))
	for i, spec := range specs {
		raw := struct {
			Target string           `json:"target"`
			Actors *[]string        `json:"actors,omitempty"`
			When   *model.FocusWhen `json:"when,omitempty"`
		}{}
		if err := json.Unmarshal([]byte(spec), &raw); err != nil {
			return nil, fmt.Errorf("--involvement[%d]: invalid JSON: %w", i, err)
		}
		if strings.TrimSpace(raw.Target) == "" {
			return nil, fmt.Errorf("--involvement[%d]: missing required `target`", i)
		}
		if raw.When != nil {
			if err := raw.When.Validate(); err != nil {
				return nil, fmt.Errorf("--involvement[%d].when: %w", i, err)
			}
		}
		inv := model.Involvement{
			Target: raw.Target,
			When:   raw.When,
		}
		if raw.Actors != nil {
			inv.Actors = *raw.Actors
			inv.ActorsSet = true
		}
		out = append(out, inv)
	}
	return out, nil
}

// parseAnnotationTopicFlags parses each --topic JSON value into a
// model.AnnotationTopic. The CLI accepts either a JSON object
// {"label":"...","members":["..."]} or a bare JSON-quoted string for the
// plain "all-refs" form, mirroring the on-disk shape that AnnotationTopic
// already round-trips. A non-JSON value (e.g. plain `foo`) is treated as a
// label string for ergonomics — quoting in shells is fiddly enough that
// this small forgiveness is worth it.
func parseAnnotationTopicFlags(specs []string) ([]model.AnnotationTopic, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	out := make([]model.AnnotationTopic, 0, len(specs))
	for i, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		// Object form first.
		if strings.HasPrefix(spec, "{") {
			var raw struct {
				Label   string   `json:"label"`
				Members []string `json:"members,omitempty"`
			}
			if err := json.Unmarshal([]byte(spec), &raw); err != nil {
				return nil, fmt.Errorf("--topic[%d]: invalid JSON: %w", i, err)
			}
			if strings.TrimSpace(raw.Label) == "" {
				return nil, fmt.Errorf("--topic[%d]: missing required `label`", i)
			}
			out = append(out, model.AnnotationTopic{Label: raw.Label, Members: raw.Members})
			continue
		}
		// JSON-quoted scalar string.
		if strings.HasPrefix(spec, `"`) {
			var label string
			if err := json.Unmarshal([]byte(spec), &label); err != nil {
				return nil, fmt.Errorf("--topic[%d]: invalid JSON: %w", i, err)
			}
			out = append(out, model.AnnotationTopic{Label: label})
			continue
		}
		// Bare label string (shell-friendly shortcut).
		out = append(out, model.AnnotationTopic{Label: spec})
	}
	return out, nil
}

// parseRefFlags parses each --refs JSON value into a model.Ref. The flag
// is strictly JSON object form ({"id":"...","kind":"...","desc":"..."});
// bare ID strings or comma-separated IDs are rejected with a clear error
// pointing at the new shape. The kind value is required and must be one
// of the capturable kinds (the legacy `unknown` sentinel is rejected at
// capture).
func parseRefFlags(specs []string) ([]model.Ref, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	out := make([]model.Ref, 0, len(specs))
	for i, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		if !strings.HasPrefix(spec, "{") {
			return nil, fmt.Errorf("--refs[%d]: expected JSON object like '{\"id\":\"<id>\",\"kind\":\"<kind>\"}'; got %q. Use one --refs flag per reference (the legacy comma-separated form was dropped because each ref now requires a `kind`)", i, spec)
		}
		var raw struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
			Desc string `json:"desc,omitempty"`
		}
		if err := json.Unmarshal([]byte(spec), &raw); err != nil {
			return nil, fmt.Errorf("--refs[%d]: invalid JSON: %w", i, err)
		}
		raw.ID = strings.TrimSpace(raw.ID)
		raw.Kind = strings.TrimSpace(raw.Kind)
		if raw.ID == "" {
			return nil, fmt.Errorf("--refs[%d]: missing required `id`", i)
		}
		if raw.Kind == "" {
			return nil, fmt.Errorf("--refs[%d]: missing required `kind` (one of: %s)", i, refKindsForUsage())
		}
		k := model.RefKind(raw.Kind)
		if !model.IsCapturableRefKind(k) {
			return nil, fmt.Errorf("--refs[%d]: invalid kind %q (expected one of: %s)", i, raw.Kind, refKindsForUsage())
		}
		out = append(out, model.Ref{ID: raw.ID, Kind: k, Desc: raw.Desc})
	}
	return out, nil
}

func refKindsForUsage() string {
	values := model.RefKindValues()
	parts := make([]string, len(values))
	for i, k := range values {
		parts[i] = string(k)
	}
	return strings.Join(parts, ", ")
}
