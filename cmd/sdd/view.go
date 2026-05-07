package main

import (
	"context"
	"fmt"
	"os"

	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
	"github.com/urfave/cli/v3"
)

// viewHelpText is shown when `sdd view` runs without `--layout`. It
// documents only what is currently implemented; later slices expand the
// vocabulary and bring this text up to the full d-tac-uww grammar.
const viewHelpText = `Usage: sdd view --layout=<spec>

Compose a pipeline of primitives separated by colons; multiple sections
are separated by commas. Render is always the terminus of each section.

Args use parens: kind(plan), n(10). Multi-arg disjunction: kind(plan,directive).
Strings use quotes: topic("infrastructure/cli"). No whitespace anywhere
except inside quoted strings.

Currently implemented vocabulary (more arrives in later slices):

  Filters:
    active                 Entries not closed and not superseded
    kind(K[, K2, ...])     Entries whose kind matches any of the listed kinds

  Page:
    n(N)                   Take first N entries (after filtering)

  Render:
    as-list                One line per entry (terminator)

Examples:

  sdd view --layout=active:as-list
    Show all active and open entries.

  sdd view --layout=active:kind(plan):n(10):as-list
    First 10 active plans.

  sdd view --layout=kind(plan,directive,activity):as-list
    All plans, directives, and activities (any status).

  sdd view --layout=active:kind(plan):n(5):as-list,active:kind(directive):n(5):as-list
    Two sections: top 5 active plans, then top 5 active directives.

See the d-tac-uww plan for the full grammar; primitives not yet implemented
return a clear "unknown function" error listing what is available.
`

func viewCmd() *cli.Command {
	return &cli.Command{
		Name:  "view",
		Usage: "Compose a pipeline of primitives over the graph (mechanical catch-up)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "layout",
				Usage: "Pipeline spec: section[:func]*[,section]*. Render terminates each section.",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// Bare `sdd view` (no --layout): print help text. Distinct
			// from urfave's --help; this is the AC's "no-presets" surface
			// — there's no implicit default layout.
			if !cmd.IsSet("layout") {
				fmt.Fprint(os.Stdout, viewHelpText)
				return nil
			}

			spec := cmd.String("layout")
			if spec == "" {
				// Empty `--layout=` is a grammar error, not a fall-through to help.
				return fmt.Errorf("--layout: empty value (expected pipeline spec; bare `sdd view` for help)")
			}

			layout, err := query.ParseLayout(spec)
			if err != nil {
				return err
			}

			g, err := loadGraph(cmd)
			if err != nil {
				return err
			}

			f, err := newReadFinder()
			if err != nil {
				return err
			}

			result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
			if err != nil {
				return err
			}

			presenters.RenderView(os.Stdout, result)
			return nil
		},
	}
}
