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
	"github.com/networkteam/resonance/framework/sdd/command"
	"github.com/networkteam/resonance/framework/sdd/finders"
	"github.com/networkteam/resonance/framework/sdd/handlers"
	"github.com/networkteam/resonance/framework/sdd/llm/claude"
	"github.com/networkteam/resonance/framework/sdd/meta"
	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/presenters"
	"github.com/networkteam/resonance/framework/sdd/query"
	"github.com/networkteam/slogutils"
	"github.com/urfave/cli/v3"
)

// newFinder constructs a Finder with a production claude runner. The runner
// model is resolved per-call so flag overrides (--preflight-model on `sdd new`)
// take effect.
func newFinder(model string) *finders.Finder {
	return finders.New(claude.NewRunner(model))
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
	app := &cli.Command{
		Name:  "sdd",
		Usage: "Signal-Dialogue-Decision graph tool",
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
			return slogutils.WithLogger(ctx, logger), nil
		},
		Commands: []*cli.Command{
			initCmd(),
			statusCmd(),
			showCmd(),
			listCmd(),
			newCmd(),
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
	return newFinder("").LoadGraph(dir)
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

			result, err := newFinder("").Status(query.StatusQuery{Graph: g})
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

			result, err := newFinder("").Show(query.ShowQuery{
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
				Usage:   "Filter by type (d, s, a)",
			},
			&cli.StringFlag{
				Name:    "layer",
				Aliases: []string{"l"},
				Usage:   "Filter by layer (stg, cpt, tac, ops, prc)",
			},
			&cli.StringFlag{
				Name:    "kind",
				Aliases: []string{"k"},
				Usage:   "Filter decisions by kind (contract, directive, plan)",
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

			result, err := newFinder("").List(query.ListQuery{
				Graph: g,
				Filter: model.GraphFilter{
					Type:     typ,
					Layer:    layer,
					Kind:     kind,
					OpenOnly: !cmd.Bool("all"),
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
				Name:  "confidence",
				Usage: "Confidence level (high, medium, low)",
			},
			&cli.StringFlag{
				Name:  "kind",
				Usage: "Decision kind (contract, directive, plan). Only applies to decisions.",
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
				Name:  "preflight-model",
				Usage: "Model to use for pre-flight validation",
				Value: "claude-haiku-4-5-20251001",
			},
			&cli.DurationFlag{
				Name:  "preflight-timeout",
				Usage: "Timeout for pre-flight validation (e.g. 120s, 2m)",
				Value: 120 * time.Second,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			ctx = slogutils.WithLogger(ctx, slogutils.FromContext(ctx).With("command", "new"))

			args := cmd.Args()
			if args.Len() < 2 {
				return fmt.Errorf("usage: sdd new <type> <layer> [description]")
			}

			typeArg := args.Get(0)
			layerArg := args.Get(1)

			// Resolve type
			typ, ok := model.TypeFromAbbrev[typeArg]
			if !ok {
				typ = model.EntryType(typeArg)
				if _, exists := model.TypeAbbrev[typ]; !exists {
					return fmt.Errorf("invalid type: %s (use d, s, or a)", typeArg)
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

			ncmd := &command.NewEntryCmd{
				Type:             typ,
				Layer:            layer,
				Kind:             kind,
				Description:      description,
				Participants:     splitCSV(cmd.String("participants")),
				Refs:             splitCSV(cmd.String("refs")),
				Supersedes:       splitCSV(cmd.String("supersedes")),
				Closes:           splitCSV(cmd.String("closes")),
				Confidence:       cmd.String("confidence"),
				Attachments:      atts,
				SkipPreflight:    cmd.Bool("skip-preflight"),
				DryRun:           cmd.Bool("dry-run"),
				PreflightModel:   cmd.String("preflight-model"),
				PreflightTimeout: cmd.Duration("preflight-timeout"),
				OnNewEntry: func(id string) {
					fmt.Println(id + ".md")
					if rel, err := model.IDToRelPath(id); err == nil {
						fmt.Printf("  → %s\n", filepath.Join(dir, rel))
					}
				},
			}

			finder := newFinder(cmd.String("preflight-model"))
			handler := handlers.New(handlers.Options{
				GraphDir:  dir,
				SDDDir:    sddDir,
				Reader:    finder,
				LLMRunner: claude.NewRunner(cmd.String("preflight-model")),
				Committer: gitCommitterFunc(gitCommit),
			})

			return handler.NewEntry(ctx, ncmd)
		},
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
				handler := handlers.New(handlers.Options{
					GraphDir:  dir,
					Reader:    newFinder(""),
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

			result, err := newFinder("").Lint(query.LintQuery{Graph: g})
			if err != nil {
				return err
			}
			presenters.RenderLint(os.Stdout, result)
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
				Name:  "model",
				Usage: "Model to use for summary generation",
				Value: "claude-haiku-4-5-20251001",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "Timeout per summary generation (e.g. 60s, 1m)",
				Value: 60 * time.Second,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			ctx = slogutils.WithLogger(ctx, slogutils.FromContext(ctx).With("command", "summarize"))

			ids := cmd.Args().Slice()
			if len(ids) == 0 && !cmd.Bool("all") {
				return fmt.Errorf("usage: sdd summarize <id>... or sdd summarize --all")
			}

			sumCmd := &command.SummarizeCmd{
				EntryIDs: ids,
				Force:    cmd.Bool("force"),
				Model:    cmd.String("model"),
				Timeout:  cmd.Duration("timeout"),
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
			handler := handlers.New(handlers.Options{
				GraphDir:  dir,
				Reader:    newFinder(cmd.String("model")),
				LLMRunner: claude.NewRunner(cmd.String("model")),
				Committer: gitCommitterFunc(gitCommit),
			})
			return handler.Summarize(ctx, sumCmd)
		},
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

// isTerminal returns true if the given file descriptor is a terminal.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func initCmd() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Initialize .sdd/ metadata directory",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "graph-dir",
				Usage: "Graph directory relative to repo root",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			repoRoot, err := findRepoRoot()
			if err != nil {
				return fmt.Errorf("finding repo root: %w", err)
			}

			graphDir := cmd.String("graph-dir")
			if graphDir == "" && isTerminal(os.Stdin) {
				prompted, err := promptGraphDir(model.DefaultGraphDir)
				if err != nil {
					return fmt.Errorf("prompt: %w", err)
				}
				graphDir = prompted
			}
			if graphDir == "" {
				graphDir = model.DefaultGraphDir
			}

			icmd := &command.InitCmd{
				RepoRoot: repoRoot,
				GraphDir: graphDir,
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
			}

			handler := handlers.New(handlers.Options{
				Committer: gitCommitterFunc(gitCommit),
			})
			return handler.Init(ctx, icmd)
		},
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
				Usage: "Participant name (defaults to git user.name)",
			},
			&cli.BoolFlag{
				Name:  "branch",
				Usage: "Create a git branch and check out to it",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			args := cmd.Args()
			if args.Len() < 1 {
				return fmt.Errorf("usage: sdd wip start <entry-id> [description]")
			}

			startCmd := &command.StartWIPCmd{
				EntryID:     args.Get(0),
				Description: strings.Join(args.Slice()[1:], " "),
				Participant: cmd.String("participant"),
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
			handler := handlers.New(handlers.Options{
				GraphDir:  dir,
				Reader:    newFinder(""),
				Committer: gitCommitterFunc(gitCommit),
				Brancher:  gitBrancher{},
			})
			return handler.StartWIP(ctx, startCmd)
		},
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
		Action: func(ctx context.Context, cmd *cli.Command) error {
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
			handler := handlers.New(handlers.Options{
				GraphDir:  dir,
				Reader:    newFinder(""),
				Committer: gitCommitterFunc(gitRemoveAndCommit),
				Brancher:  gitBrancher{},
			})
			return handler.FinishWIP(ctx, doneCmd)
		},
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
			result, err := newFinder("").WIPList(query.WIPListQuery{GraphDir: dir})
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
