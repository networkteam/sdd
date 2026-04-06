package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"os"
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
						printEntry(e)
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
					printEntry(e)
				}
				fmt.Println()
			}

			// Recent actions
			recent := g.RecentActions(5)
			if len(recent) > 0 {
				fmt.Println("## Recent Actions")
				fmt.Println()
				for _, e := range recent {
					printEntry(e)
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
		Usage:     "Show an entry with its reference chain",
		ArgsUsage: "<id>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			id := cmd.Args().First()
			if id == "" {
				return fmt.Errorf("usage: sdd show <id>")
			}

			g, err := loadGraph(cmd)
			if err != nil {
				return err
			}

			chain := g.RefChain(id)
			if len(chain) == 0 {
				return fmt.Errorf("entry not found: %s", id)
			}

			for i, e := range chain {
				if i > 0 {
					fmt.Println("---")
				}
				printEntryFull(e)
			}

			return nil
		},
	}
}

func listCmd() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List entries with optional filters",
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

			entries := g.Filter(typ, layer)
			for _, e := range entries {
				printEntry(e)
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
				Usage: "Comma-separated list of decision IDs this supersedes",
			},
			&cli.StringFlag{
				Name:  "participants",
				Usage: "Comma-separated list of participants",
			},
			&cli.StringFlag{
				Name:  "confidence",
				Usage: "Confidence level (high, medium, low)",
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
			if participants := cmd.String("participants"); participants != "" {
				entry.Participants = strings.Split(participants, ",")
			}
			if confidence := cmd.String("confidence"); confidence != "" {
				entry.Confidence = confidence
			}

			// Write file
			dir := cmd.String("graph-dir")
			if !filepath.IsAbs(dir) {
				dir, _ = filepath.Abs(dir)
			}

			filename := id + ".md"
			filePath := filepath.Join(dir, filename)
			content := sdd.FormatFrontmatter(entry) + "\n" + entry.Content + "\n"

			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return fmt.Errorf("writing %s: %w", filePath, err)
			}

			fmt.Println(filename)
			fmt.Printf("  → %s\n", filePath)

			return nil
		},
	}
}

func printEntry(e *sdd.Entry) {
	conf := ""
	if e.Confidence != "" {
		conf = fmt.Sprintf(" [%s]", e.Confidence)
	}
	fmt.Printf("  %s  %-8s %-12s%s  %s\n",
		e.ID, e.TypeLabel(), e.LayerLabel(), conf, e.ShortContent(80))
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
