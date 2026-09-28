package cliapp

import (
	"context"
	"fmt"
	"time"

	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
	"github.com/urfave/cli/v3"
)

func exportCmd() *cli.Command {
	return &cli.Command{
		Name:  "export",
		Usage: "EXPERIMENTAL: export the whole graph with derived status, heat and git arrival times as one JSON document (unstable shape, for UI prototyping)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "format",
				Value: "json",
				Usage: "Output format: json (the only one)",
			},
			&cli.StringSliceFlag{
				Name:  "repo",
				Usage: "Also export a connected repo's graph by repo-id (repeatable, additive to the local graph)",
			},
			&cli.BoolFlag{
				Name:  "all-repos",
				Usage: "Also export every connected repo",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if format := cmd.String("format"); format != "json" {
				return fmt.Errorf("--format: unsupported value %q (only json)", format)
			}
			reg, _, err := defaultRepos()
			if err != nil {
				return err
			}
			repoIDs, err := reg.SelectRepoIDs(cmd.StringSlice("repo"), cmd.Bool("all-repos"))
			if err != nil {
				return err
			}
			if err := freshenRepoCaches(ctx, cmd.ErrWriter, repoIDs); err != nil {
				return err
			}

			dir, err := resolveGraphDir(cmd)
			if err != nil {
				return err
			}
			f, err := newReadFinder()
			if err != nil {
				return err
			}
			g, err := f.CurrentGraph(dir)
			if err != nil {
				return err
			}
			result, err := f.OnGraph(g).Export(ctx, query.ExportQuery{RepoIDs: repoIDs, Now: time.Now()})
			if err != nil {
				return err
			}
			return presenters.RenderExportJSON(cmd.Writer, result)
		},
	}
}
