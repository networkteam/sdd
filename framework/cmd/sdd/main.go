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

	"github.com/networkteam/resonance/framework/sdd"
	"github.com/networkteam/resonance/framework/sdd/command"
	"github.com/networkteam/resonance/framework/sdd/finders"
	"github.com/networkteam/resonance/framework/sdd/handlers"
	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/urfave/cli/v3"
)

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
		},
		DefaultCommand: "status",
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func loadGraph(cmd *cli.Command) (*model.Graph, error) {
	dir := cmd.String("graph-dir")
	if !filepath.IsAbs(dir) {
		// Resolve relative to git root or cwd
		dir, _ = filepath.Abs(dir)
	}
	return sdd.LoadGraph(dir)
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

			// Summary
			decisions := g.Filter(model.GraphFilter{Type: model.TypeDecision})
			signals := g.Filter(model.GraphFilter{Type: model.TypeSignal})
			actions := g.Filter(model.GraphFilter{Type: model.TypeAction})
			fmt.Printf("Graph: %d entries (%d decisions, %d signals, %d actions)\n\n",
				len(g.Entries), len(decisions), len(signals), len(actions))

			// Contracts grouped by layer
			contracts := g.Contracts()
			if len(contracts) > 0 {
				fmt.Println("## Contracts")
				fmt.Println()
				byLayer := groupByLayer(contracts)
				for _, layer := range layerOrder() {
					entries, ok := byLayer[layer]
					if !ok {
						continue
					}
					fmt.Printf("### %s\n", layer)
					for _, e := range entries {
						printEntry(e, int(cmd.Int("width")))
					}
					fmt.Println()
				}
			}

			// Plans grouped by layer
			plans := g.Plans()
			if len(plans) > 0 {
				fmt.Println("## Plans")
				fmt.Println()
				byLayer := groupByLayer(plans)
				for _, layer := range layerOrder() {
					entries, ok := byLayer[layer]
					if !ok {
						continue
					}
					fmt.Printf("### %s\n", layer)
					for _, e := range entries {
						printEntry(e, int(cmd.Int("width")))
					}
					fmt.Println()
				}
			}

			// Active decisions grouped by layer
			active := g.ActiveDecisions()
			if len(active) > 0 {
				fmt.Println("## Active Decisions")
				fmt.Println()
				byLayer := groupByLayer(active)
				for _, layer := range layerOrder() {
					entries, ok := byLayer[layer]
					if !ok {
						continue
					}
					fmt.Printf("### %s\n", layer)
					for _, e := range entries {
						printEntry(e, int(cmd.Int("width")))
					}
					fmt.Println()
				}
			}

			// Open signals
			open := g.OpenSignals()
			if len(open) > 0 {
				fmt.Println("## Open Signals")
				fmt.Println()
				for _, e := range open {
					printEntry(e, int(cmd.Int("width")))
				}
				fmt.Println()
			}

			// Recent actions
			recent := g.RecentActions(5)
			if len(recent) > 0 {
				fmt.Println("## Recent Actions")
				fmt.Println()
				for _, e := range recent {
					printEntry(e, int(cmd.Int("width")))
				}
				fmt.Println()
			}

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

			downstream := cmd.Bool("downstream")

			return sdd.RenderShow(os.Stdout, g, ids, downstream)
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

			var entries []*model.Entry
			f := model.GraphFilter{Type: typ, Layer: layer, Kind: kind, OpenOnly: !cmd.Bool("all")}
			entries = g.Filter(f)
			for _, e := range entries {
				printEntry(e, int(cmd.Int("width")))
			}

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

			runner := &claudeRunner{model: cmd.String("preflight-model")}
			finder := finders.New(runner)
			handler := handlers.New(handlers.Options{
				GraphDir:    dir,
				Preflighter: finder,
				Committer:   gitCommitterFunc(gitCommit),
				LoadGraph:   sdd.LoadGraph,
			})

			return handler.NewEntry(ctx, ncmd)
		},
	}
}

func lintCmd() *cli.Command {
	return &cli.Command{
		Name:  "lint",
		Usage: "Check graph entries for integrity issues",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			g, err := loadGraph(cmd)
			if err != nil {
				return err
			}

			entries := g.Lint()
			if len(entries) == 0 {
				fmt.Println("No issues found.")
				return nil
			}

			total := 0
			for _, e := range entries {
				total += len(e.Warnings)
			}

			fmt.Printf("%d issue(s) in %d entry/entries:\n\n", total, len(entries))
			for _, e := range entries {
				fmt.Printf("  %s  %s  %s\n", e.ID, e.TypeLabel(), e.ShortContent(int(cmd.Int("width"))))
				for _, w := range e.Warnings {
					fmt.Printf("    ⚠ %s\n", w.Message)
				}
				fmt.Println()
			}

			return fmt.Errorf("lint found %d issue(s)", total)
		},
	}
}

func printEntry(e *model.Entry, width int) {
	conf := ""
	if e.Confidence != "" {
		conf = fmt.Sprintf(" [%s]", e.Confidence)
	}
	fmt.Printf("  %s  %-8s %-12s%s  %s\n",
		e.ID, e.TypeLabel(), e.LayerLabel(), conf, e.ShortContent(width))
}


func groupByLayer(entries []*model.Entry) map[model.Layer][]*model.Entry {
	m := make(map[model.Layer][]*model.Entry)
	for _, e := range entries {
		m[e.Layer] = append(m[e.Layer], e)
	}
	return m
}

func layerOrder() []model.Layer {
	return []model.Layer{
		model.LayerStrategic,
		model.LayerConceptual,
		model.LayerTactical,
		model.LayerOperational,
		model.LayerProcess,
	}
}

// claudeRunner implements sdd.PreflightRunner by invoking the claude CLI.
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

func deleteBranch(branch string, force bool) {
	flag := "-d"
	if force {
		flag = "-D"
	}
	if out, err := exec.Command("git", "branch", flag, branch).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: git branch %s: %s (%v)\n", flag, out, err)
	}
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

			entryID := args.Get(0)
			description := strings.Join(args.Slice()[1:], " ")

			// Validate entry exists
			g, err := loadGraph(cmd)
			if err != nil {
				return err
			}
			if _, ok := g.ByID[entryID]; !ok {
				return fmt.Errorf("entry not found: %s", entryID)
			}

			// Resolve participant
			participant := cmd.String("participant")
			if participant == "" {
				return fmt.Errorf("--participant is required")
			}

			// Check for existing exclusive markers
			dir := graphDir(cmd)
			markers, err := sdd.LoadWIPMarkers(dir)
			if err != nil {
				return fmt.Errorf("loading WIP markers: %w", err)
			}
			if existing, ok := model.HasExclusiveMarker(markers, entryID); ok {
				fmt.Fprintf(os.Stderr, "warning: exclusive marker exists for %s by %s (%s)\n",
					entryID, existing.Participant, existing.ID)
			}

			// Derive branch name if --branch is set
			var branchName string
			if cmd.Bool("branch") {
				branchName = model.DeriveBranchName(entryID, description)
			}

			// Create marker
			markerID := model.GenerateWIPMarkerID(participant)
			marker := &model.WIPMarker{
				ID:          markerID,
				Entry:       entryID,
				Participant: participant,
				Exclusive:   cmd.Bool("exclusive"),
				Branch:      branchName,
				Content:     description,
			}

			markerPath := filepath.Join(dir, model.WIPMarkerPath(markerID))
			if err := os.MkdirAll(filepath.Dir(markerPath), 0755); err != nil {
				return fmt.Errorf("creating wip directory: %w", err)
			}

			content := model.FormatWIPMarker(marker)
			if err := os.WriteFile(markerPath, []byte(content), 0644); err != nil {
				return fmt.Errorf("writing marker: %w", err)
			}

			fmt.Printf("%s\n", markerID)
			fmt.Printf("  → %s\n", markerPath)

			// Commit marker on current branch (before creating the new branch)
			if err := gitCommit(fmt.Sprintf("sdd: wip start %s (%s)", entryID, participant), markerPath); err != nil {
				fmt.Fprintf(os.Stderr, "warning: git commit failed: %v\n", err)
			}

			// Create branch and check out if --branch
			if branchName != "" {
				checkoutCmd := exec.Command("git", "checkout", "-b", branchName)
				if out, err := checkoutCmd.CombinedOutput(); err != nil {
					return fmt.Errorf("creating branch %s: %s (%w)", branchName, out, err)
				}

				fmt.Printf("  branch: %s (checked out)\n", branchName)
			}

			return nil
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

			markerID := args.Get(0)
			dir := graphDir(cmd)
			markerPath := filepath.Join(dir, model.WIPMarkerPath(markerID))

			if _, err := os.Stat(markerPath); err != nil {
				return fmt.Errorf("marker not found: %s", markerID)
			}

			// Read marker to check for branch
			data, err := os.ReadFile(markerPath)
			if err != nil {
				return fmt.Errorf("reading marker: %w", err)
			}
			marker, err := model.ParseWIPMarker(filepath.Base(markerPath), string(data))
			if err != nil {
				return fmt.Errorf("parsing marker: %w", err)
			}

			if err := os.Remove(markerPath); err != nil {
				return fmt.Errorf("removing marker: %w", err)
			}

			fmt.Printf("removed %s\n", markerID)

			// Git rm and commit
			rm := exec.Command("git", "rm", "--cached", "-f", markerPath)
			if out, err := rm.CombinedOutput(); err != nil {
				// File already removed from disk, try git add the deletion
				add := exec.Command("git", "add", markerPath)
				if out2, err2 := add.CombinedOutput(); err2 != nil {
					fmt.Fprintf(os.Stderr, "warning: git stage failed: %s (%v); %s (%v)\n", out, err, out2, err2)
					return nil
				}
			}

			commit := exec.Command("git", "commit", "-m", fmt.Sprintf("sdd: wip done %s", markerID))
			if out, err := commit.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: git commit failed: %s (%v)\n", out, err)
			}

			// Branch cleanup
			if marker.Branch != "" {
				merged := isBranchMerged(marker.Branch)

				if merged {
					deleteBranch(marker.Branch, false)
					fmt.Printf("  deleted branch %s (merged)\n", marker.Branch)
				} else if cmd.Bool("force") {
					deleteBranch(marker.Branch, true)
					fmt.Printf("  force-deleted branch %s (unmerged)\n", marker.Branch)
				} else {
					fmt.Fprintf(os.Stderr, "  warning: branch %s has unmerged changes — marker removed but branch preserved\n", marker.Branch)
					fmt.Fprintf(os.Stderr, "  use --force to delete the unmerged branch, or merge it first\n")
				}
			}

			return nil
		},
	}
}

func wipListCmd() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List all active WIP markers",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dir := graphDir(cmd)
			markers, err := sdd.LoadWIPMarkers(dir)
			if err != nil {
				return fmt.Errorf("loading WIP markers: %w", err)
			}

			if len(markers) == 0 {
				fmt.Println("No active WIP markers.")
				return nil
			}

			for _, m := range markers {
				excl := ""
				if m.Exclusive {
					excl = " [exclusive]"
				}
				branch := ""
				if m.Branch != "" {
					branch = fmt.Sprintf("  branch:%s", m.Branch)
				}
				fmt.Printf("  %s  %-15s%s  %s%s  %s\n",
					m.ID, m.Participant, excl, m.Entry, branch, m.ShortContent(int(cmd.Int("width"))))
			}

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

