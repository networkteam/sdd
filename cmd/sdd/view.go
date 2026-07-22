package main

import (
	"context"
	"fmt"
	"os"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/viewlayout"
	"github.com/urfave/cli/v3"
)

func viewCmd() *cli.Command {
	return &cli.Command{
		Name:               "view",
		Usage:              "Compose custom graph views with filters, ranking, transforms, and renderers",
		CustomHelpTemplate: cli.CommandHelpTemplate + "\nLAYOUT REFERENCE:\n\n" + viewlayout.Reference(finders.LiveViewVocabulary()),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "layout",
				Usage: "Pipeline spec: section[:func]*[,section]*. Render terminates each section.",
			},
			&cli.StringSliceFlag{
				Name:  "repo",
				Usage: "Also render the layout over a connected repo's graph by repo-id (repeatable, additive to the local graph)",
			},
			&cli.BoolFlag{
				Name:  "all-repos",
				Usage: "Also render the layout over every connected repo",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// A layout is always explicit. The full grammar and vocabulary live
			// on the conventional --help surface so a missing layout remains an
			// actionable error rather than a second help entry point.
			if !cmd.IsSet("layout") {
				return fmt.Errorf("--layout is required; run `sdd view --help` for the layout reference")
			}

			spec := cmd.String("layout")
			if spec == "" {
				return fmt.Errorf("--layout: empty value; run `sdd view --help` for the layout reference")
			}

			layout, err := query.ParseLayout(spec)
			if err != nil {
				return err
			}
			// Macro expansion is a separate query-layer pass: the parser
			// only deals with grammar, the expander substitutes named
			// pipelines like `top(N)` and `decisions` with their canonical
			// primitive sequences. User-supplied modifiers append.
			layout, err = query.ExpandMacros(layout)
			if err != nil {
				return err
			}

			reg, _, err := defaultRepos()
			if err != nil {
				return err
			}
			repoIDs, err := reg.SelectRepoIDs(cmd.StringSlice("repo"), cmd.Bool("all-repos"))
			if err != nil {
				return err
			}
			if err := freshenRepoCaches(ctx, repoIDs); err != nil {
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

			result, err := f.OnGraph(g).View(query.ViewQuery{Layout: layout})
			if err != nil {
				return err
			}
			presenters.RenderView(os.Stdout, result)

			// Selected repos render the same layout over their own graphs,
			// each under a repo heading — entry IDs inside a repo section
			// are scoped by that heading.
			for _, repoID := range repoIDs {
				member, err := g.MemberGraph(repoID)
				if err != nil {
					return fmt.Errorf("loading graph for %s: %w", repoID, err)
				}
				if member == nil {
					fmt.Fprintf(os.Stdout, "\n── repo: %s (unavailable) ──\n", repoID)
					continue
				}
				// The member graph records its own directory (loaded via
				// LoadGraph), so the finder resolves that repo's WIP markers
				// lazily when the layout needs them.
				mresult, err := f.OnGraph(member).View(query.ViewQuery{Layout: layout})
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stdout, "\n── repo: %s ──\n", repoID)
				presenters.RenderView(os.Stdout, mresult)
			}
			return nil
		},
	}
}
