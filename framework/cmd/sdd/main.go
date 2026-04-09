package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/networkteam/resonance/framework/sdd"
	"github.com/urfave/cli/v3"
)

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
		},
		DefaultCommand: "status",
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func loadGraph(cmd *cli.Command) (*sdd.Graph, error) {
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
			decisions := g.Filter(sdd.TypeDecision, "")
			signals := g.Filter(sdd.TypeSignal, "")
			actions := g.Filter(sdd.TypeAction, "")
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

			seen := make(map[string]bool)
			for i, id := range ids {
				var entries []*sdd.Entry
				if downstream {
					// Check the target exists
					if _, ok := g.ByID[id]; !ok {
						return fmt.Errorf("entry not found: %s", id)
					}
					entries = g.Downstream(id)
				} else {
					entries = g.RefChain(id)
					if len(entries) == 0 {
						return fmt.Errorf("entry not found: %s", id)
					}
				}

				if i > 0 {
					fmt.Println()
				}

				for j, e := range entries {
					if seen[e.ID] {
						if j > 0 {
							fmt.Println("---")
						}
						fmt.Printf("(shown above) %s  %s\n", e.ID, e.ShortContent(int(cmd.Int("width"))))
					} else {
						if j > 0 {
							fmt.Println("---")
						}
						printEntryFull(e)
						seen[e.ID] = true
					}
				}
			}

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
				Usage:   "Filter decisions by kind (contract, directive)",
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

			var typ sdd.EntryType
			if t := cmd.String("type"); t != "" {
				if resolved, ok := sdd.TypeFromAbbrev[t]; ok {
					typ = resolved
				} else {
					typ = sdd.EntryType(t)
				}
			}

			var layer sdd.Layer
			if l := cmd.String("layer"); l != "" {
				if resolved, ok := sdd.LayerFromAbbrev[l]; ok {
					layer = resolved
				} else {
					layer = sdd.Layer(l)
				}
			}

			var kind sdd.Kind
			if k := cmd.String("kind"); k != "" {
				kind = sdd.Kind(k)
			}

			var entries []*sdd.Entry
			if cmd.Bool("all") {
				entries = g.Filter(typ, layer)
			} else {
				entries = g.FilterOpen(typ, layer, kind)
			}
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
				Usage: "Decision kind (contract, directive). Only applies to decisions.",
			},
			&cli.StringFlag{
				Name:  "attach",
				Usage: "Comma-separated list of file paths to attach",
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
			typ, ok := sdd.TypeFromAbbrev[typeArg]
			if !ok {
				typ = sdd.EntryType(typeArg)
				if _, exists := sdd.TypeAbbrev[typ]; !exists {
					return fmt.Errorf("invalid type: %s (use d, s, or a)", typeArg)
				}
			}

			// Resolve layer
			layer, ok := sdd.LayerFromAbbrev[layerArg]
			if !ok {
				layer = sdd.Layer(layerArg)
				if _, exists := sdd.LayerAbbrev[layer]; !exists {
					return fmt.Errorf("invalid layer: %s (use stg, cpt, tac, ops, or prc)", layerArg)
				}
			}

			// Description from remaining args
			description := strings.Join(args.Slice()[2:], " ")
			if description == "" {
				description = "[TODO: describe this " + string(typ) + "]"
			}

			// Generate random suffix
			suffix, err := randomSuffix(3)
			if err != nil {
				return fmt.Errorf("generating suffix: %w", err)
			}

			id := sdd.GenerateID(typ, layer, suffix)

			// Build entry
			entry := &sdd.Entry{
				ID:      id,
				Type:    typ,
				Layer:   layer,
				Content: description,
			}

			if refs := cmd.String("refs"); refs != "" {
				entry.Refs = strings.Split(refs, ",")
			}
			if supersedes := cmd.String("supersedes"); supersedes != "" {
				entry.Supersedes = strings.Split(supersedes, ",")
			}
			if closes := cmd.String("closes"); closes != "" {
				entry.Closes = strings.Split(closes, ",")
			}
			if participants := cmd.String("participants"); participants != "" {
				entry.Participants = strings.Split(participants, ",")
			}
			if confidence := cmd.String("confidence"); confidence != "" {
				entry.Confidence = confidence
			}
			if kind := cmd.String("kind"); kind != "" {
				entry.Kind = sdd.Kind(kind)
			}

			// Validate refs, closes, supersedes against existing graph
			dir := cmd.String("graph-dir")
			if !filepath.IsAbs(dir) {
				dir, _ = filepath.Abs(dir)
			}

			allIDRefs := make([]string, 0)
			allIDRefs = append(allIDRefs, entry.Refs...)
			allIDRefs = append(allIDRefs, entry.Closes...)
			allIDRefs = append(allIDRefs, entry.Supersedes...)

			if len(allIDRefs) > 0 {
				graph, err := sdd.LoadGraph(dir)
				if err != nil {
					return fmt.Errorf("loading graph for validation: %w", err)
				}
				for _, refID := range allIDRefs {
					if _, exists := graph.ByID[refID]; !exists {
						return fmt.Errorf("referenced entry not found: %s (use full entry IDs)", refID)
					}
				}
			}

			// Parse attachment paths
			var attachPaths []string
			if attach := cmd.String("attach"); attach != "" {
				attachPaths = strings.Split(attach, ",")
				for _, p := range attachPaths {
					if _, err := os.Stat(p); err != nil {
						return fmt.Errorf("attachment file not found: %s", p)
					}
				}
			}

			// Resolve {{attachments}} placeholders in description
			entry.Content = sdd.ResolveAttachmentLinks(entry.Content, id)

			// Write entry file
			relPath, err := sdd.IDToRelPath(id)
			if err != nil {
				return fmt.Errorf("computing path for %s: %w", id, err)
			}
			filePath := filepath.Join(dir, relPath)
			content := sdd.FormatFrontmatter(entry) + "\n" + entry.Content + "\n"

			if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
				return fmt.Errorf("creating directories: %w", err)
			}

			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return fmt.Errorf("writing %s: %w", filePath, err)
			}

			commitPaths := []string{filePath}

			// Copy attachments
			if len(attachPaths) > 0 {
				attachDirRel, err := sdd.AttachDirRelPath(id)
				if err != nil {
					return fmt.Errorf("computing attachment dir for %s: %w", id, err)
				}
				attachDir := filepath.Join(dir, attachDirRel)
				if err := os.MkdirAll(attachDir, 0755); err != nil {
					return fmt.Errorf("creating attachment directory: %w", err)
				}

				for _, src := range attachPaths {
					data, err := os.ReadFile(src)
					if err != nil {
						return fmt.Errorf("reading attachment %s: %w", src, err)
					}
					dst := filepath.Join(attachDir, filepath.Base(src))
					if err := os.WriteFile(dst, data, 0644); err != nil {
						return fmt.Errorf("writing attachment %s: %w", dst, err)
					}
					commitPaths = append(commitPaths, dst)
				}
			}

			fmt.Println(id + ".md")
			fmt.Printf("  → %s\n", filePath)

			// Auto-commit the new entry (with attachments if any)
			if err := gitCommit(fmt.Sprintf("sdd: %s %s %s", entry.TypeLabel(), entry.LayerLabel(), entry.ShortContent(72)), commitPaths...); err != nil {
				fmt.Fprintf(os.Stderr, "warning: git commit failed: %v\n", err)
			}

			return nil
		},
	}
}

func printEntry(e *sdd.Entry, width int) {
	conf := ""
	if e.Confidence != "" {
		conf = fmt.Sprintf(" [%s]", e.Confidence)
	}
	fmt.Printf("  %s  %-8s %-12s%s  %s\n",
		e.ID, e.TypeLabel(), e.LayerLabel(), conf, e.ShortContent(width))
}

func printEntryFull(e *sdd.Entry) {
	fmt.Printf("ID:     %s\n", e.ID)
	fmt.Printf("Type:   %s\n", e.TypeLabel())
	fmt.Printf("Layer:  %s\n", e.LayerLabel())
	if e.Confidence != "" {
		fmt.Printf("Conf:   %s\n", e.Confidence)
	}
	if len(e.Participants) > 0 {
		fmt.Printf("Who:    %s\n", strings.Join(e.Participants, ", "))
	}
	if len(e.Refs) > 0 {
		fmt.Printf("Refs:   %s\n", strings.Join(e.Refs, ", "))
	}
	if len(e.Closes) > 0 {
		fmt.Printf("Closes: %s\n", strings.Join(e.Closes, ", "))
	}
	if len(e.Supersedes) > 0 {
		fmt.Printf("Supersedes: %s\n", strings.Join(e.Supersedes, ", "))
	}
	if len(e.Attachments) > 0 {
		fmt.Printf("Attach: %s\n", strings.Join(e.Attachments, ", "))
	}
	fmt.Printf("Time:   %s\n", e.Time.Format("2006-01-02 15:04:05"))
	fmt.Println()
	fmt.Println(e.Content)
	fmt.Println()
}

func groupByLayer(entries []*sdd.Entry) map[sdd.Layer][]*sdd.Entry {
	m := make(map[sdd.Layer][]*sdd.Entry)
	for _, e := range entries {
		m[e.Layer] = append(m[e.Layer], e)
	}
	return m
}

func layerOrder() []sdd.Layer {
	return []sdd.Layer{
		sdd.LayerStrategic,
		sdd.LayerConceptual,
		sdd.LayerTactical,
		sdd.LayerOperational,
		sdd.LayerProcess,
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

func randomSuffix(n int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b), nil
}
