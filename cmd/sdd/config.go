package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/query"
	"github.com/urfave/cli/v3"
)

// configCmd is the CLI surface over the config overlay: the bare command
// prints the effective merged config with per-value provenance, `get` reads
// one effective value, and `set` writes a key to the global or local layer
// with a comment-preserving upsert.
func configCmd() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Show and edit SDD configuration (global-first overlay with per-repo override)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "format",
				Usage: "Output format: text (default) or json",
				Value: "text",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runEffectiveConfig(cmd, "")
		},
		Commands: []*cli.Command{
			{
				Name:      "get",
				Usage:     "Print one effective config value (e.g. sdd config get llm.model)",
				ArgsUsage: "<key>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() != 1 {
						return fmt.Errorf("usage: sdd config get <key>")
					}
					return runConfigGet(cmd.Args().First())
				},
			},
			{
				Name:      "set",
				Usage:     "Set a config key (user-global by default; --local for .sdd/config.local.yaml)",
				ArgsUsage: "<key> <value>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "local",
						Usage: "Write to .sdd/config.local.yaml instead of the user-global config",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() != 2 {
						return fmt.Errorf("usage: sdd config set [--local] <key> <value>")
					}
					target := "global"
					if cmd.Bool("local") {
						target = "local"
					}
					return runConfigSet(ctx, target, cmd.Args().Get(0), cmd.Args().Get(1))
				},
			},
		},
	}
}

// effectiveConfigResult builds the provenance view shared by the bare
// command and `get`. Outside an sdd repo the per-repo layers are simply
// absent — global settings and defaults still resolve.
func effectiveConfigResult(key string) (*query.EffectiveConfigResult, error) {
	f, err := newReadFinder()
	if err != nil {
		return nil, err
	}
	sddDir, err := resolveSDDDir()
	if err != nil {
		sddDir = ""
	}
	return f.EffectiveConfig(query.EffectiveConfigQuery{SDDDir: sddDir, Key: key})
}

func runEffectiveConfig(cmd *cli.Command, key string) error {
	result, err := effectiveConfigResult(key)
	if err != nil {
		return err
	}
	if cmd.String("format") == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Entries)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for _, e := range result.Entries {
		fmt.Fprintf(w, "%s\t%s\t(%s)\n", e.Key, e.Value, e.Source)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	// The effective table answers "what will happen", and a setting that does
	// nothing is part of that answer.
	if key == "" {
		return printUnknownConfigKeys()
	}
	return nil
}

func printUnknownConfigKeys() error {
	f, err := newReadFinder()
	if err != nil {
		return err
	}
	sddDir, err := resolveSDDDir()
	if err != nil {
		sddDir = ""
	}
	result, err := f.UnknownConfigKeys(query.UnknownConfigKeysQuery{SDDDir: sddDir})
	if err != nil {
		return err
	}
	if len(result.Keys) == 0 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "\nignored — this sdd does not know these keys:\n")
	for _, k := range result.Keys {
		fmt.Fprintf(os.Stderr, "  %s  (%s)\n", k.Key, k.File)
	}
	return nil
}

func runConfigGet(key string) error {
	result, err := effectiveConfigResult(key)
	if err != nil {
		return err
	}
	if len(result.Entries) == 0 {
		return fmt.Errorf("no value for %q", key)
	}
	for _, e := range result.Entries {
		fmt.Println(e.Value)
	}
	return nil
}

func runConfigSet(ctx context.Context, target, key, value string) error {
	// sddDir is optional for global writes — `sdd config set` must work
	// outside any repo (that is the point of the global layer).
	sddDir, err := resolveSDDDir()
	if err != nil {
		sddDir = ""
	}
	_, mgr, err := defaultRepos()
	if err != nil {
		return err
	}
	h := handlers.New(handlers.Options{
		SDDDir: sddDir,
		Repos:  mgr,
	})
	if err := h.ConfigSet(ctx, &command.ConfigSetCmd{Target: target, Key: key, Value: value}); err != nil {
		return err
	}
	fmt.Printf("%s = %s (%s)\n", key, value, target)
	return nil
}
