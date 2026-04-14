package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/networkteam/resonance/framework/sdd/command"
	"github.com/networkteam/resonance/framework/sdd/finders"
	"github.com/networkteam/resonance/framework/sdd/handlers"
	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/presenters"
	"github.com/networkteam/resonance/framework/sdd/query"
	"github.com/urfave/cli/v3"
)

// newFinder constructs a Finder with a production claudeRunner. The runner
// model is resolved per-call so flag overrides (--preflight-model on `sdd new`)
// take effect.
func newFinder(model string) *finders.Finder {
	return finders.New(&claudeRunner{model: model})
}

// splitCSV returns the comma-split fields of s, or nil if s is empty.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
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
				Usage:   "Path to graph directory",
				Value:   "docs/framework/graph",
			},
			&cli.IntFlag{
				Name:    "width",
				Aliases: []string{"w"},
				Usage:   "Max content width for entry summaries",
				Value:   160,
			},
		},
		Commands: []*cli.Command{
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
		log.Fatal(err)
	}
}

func loadGraph(cmd *cli.Command) (*model.Graph, error) {
	return newFinder("").LoadGraph(graphDir(cmd))
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
			presenters.RenderStatus(os.Stdout, result, int(cmd.Int("width")))
			return nil
		},
	}
}

func showCmd() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show entries with their reference chains",
		ArgsUsage: "<id> [id2 id3 ...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "downstream",
				Usage: "Show entries that reference, close, or supersede the target (instead of upstream chain)",
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
			presenters.RenderList(os.Stdout, result, int(cmd.Int("width")))
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
			&cli.IntFlag{
				Name:  "preflight-timeout",
				Usage: "Timeout in seconds for pre-flight validation",
				Value: 120,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
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

			dir := cmd.String("graph-dir")
			if !filepath.IsAbs(dir) {
				dir, _ = filepath.Abs(dir)
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
				PreflightTimeout: time.Duration(cmd.Int("preflight-timeout")) * time.Second,
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
				Reader:    finder,
				LLMRunner: &claudeRunner{model: cmd.String("preflight-model")},
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
				fixCmd := &command.LintFixCmd{
					OnFixed: func(id string, fixes []string) {
						for _, f := range fixes {
							fmt.Fprintf(os.Stderr, "  fixed %s: %s\n", id, f)
						}
					},
				}
				handler := handlers.New(handlers.Options{
					GraphDir:  graphDir(cmd),
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
			presenters.RenderLint(os.Stdout, result, int(cmd.Int("width")))
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
			&cli.IntFlag{
				Name:  "timeout",
				Usage: "Timeout in seconds per summary generation",
				Value: 60,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			ids := cmd.Args().Slice()
			if len(ids) == 0 && !cmd.Bool("all") {
				return fmt.Errorf("usage: sdd summarize <id>... or sdd summarize --all")
			}

			sumCmd := &command.SummarizeCmd{
				EntryIDs: ids,
				Force:    cmd.Bool("force"),
				Model:    cmd.String("model"),
				Timeout:  time.Duration(cmd.Int("timeout")) * time.Second,
				OnSummarized: func(id, summary string) {
					fmt.Fprintf(os.Stderr, "  summarized %s\n", id)
				},
				OnSkipped: func(id string) {
					fmt.Fprintf(os.Stderr, "  skipped %s (hash matches)\n", id)
				},
			}

			runner := &claudeRunner{model: cmd.String("model")}
			handler := handlers.New(handlers.Options{
				GraphDir:  graphDir(cmd),
				Reader:    newFinder(cmd.String("model")),
				LLMRunner: runner,
				Committer: gitCommitterFunc(gitCommit),
			})
			return handler.Summarize(ctx, sumCmd)
		},
	}
}

// claudeRunner implements finders.PreflightRunner by invoking the claude CLI.
type claudeRunner struct {
	model string
}

func (r *claudeRunner) Run(ctx context.Context, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, "claude", "-p", "--model", r.model)
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("claude -p timed out (increase with --preflight-timeout)")
		}
		return "", fmt.Errorf("claude -p: %w", err)
	}
	return string(out), nil
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

func graphDir(cmd *cli.Command) string {
	dir := cmd.String("graph-dir")
	if !filepath.IsAbs(dir) {
		dir, _ = filepath.Abs(dir)
	}
	return dir
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

			handler := handlers.New(handlers.Options{
				GraphDir:  graphDir(cmd),
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

			handler := handlers.New(handlers.Options{
				GraphDir:  graphDir(cmd),
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
			result, err := newFinder("").WIPList(query.WIPListQuery{GraphDir: graphDir(cmd)})
			if err != nil {
				return fmt.Errorf("loading WIP markers: %w", err)
			}
			presenters.RenderWIPList(os.Stdout, result, int(cmd.Int("width")))
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
