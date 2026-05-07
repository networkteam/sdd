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
Pipeline applies in canonical filter → rank → page → render order
regardless of source ordering.

Args use parens: kind(plan), n(10), rank(heat). Multi-arg disjunction:
kind(plan,directive). Nested calls let algorithms carry decay:
rank(heat(exp-14d)). Strings use quotes: topic("infrastructure/cli").
No whitespace anywhere except inside quoted strings.

Currently implemented vocabulary (more arrives in later slices):

  Filters:
    active                 Entries not closed and not superseded
    kind(K[, K2, ...])     Entries whose kind matches any of the listed kinds

  Rank:
    rank(<algorithm>)      Sort by computed score, descending. Adds
                           {score: X.XXX} to each rendered entry.

    Algorithms (used inside rank):
      heat(decay)          Σ over incoming refs of decay(age of ref source).
                           Default decay: exp-14d.
      in-degree            Raw count of incoming refs (decay arg ignored).
      mult(decay)          heat(decay) × in-degree
      add(decay)           heat(decay) + in-degree (raw sum, slice 3)
      log(decay)           heat(decay) × log(1 + in-degree)
      by(date)             Sort by entry creation timestamp; no scores.

    Decay names (used inside algorithm calls):
      exp-{7,14,30}d       2^(-age_days/N) — half-life every N days
      linear-{7,14,30}d    max(0, 1 - age_days/N) — zero past N days
      none                 No age effect (heat(none) == weighted in-degree)

  Page:
    n(N)                   Take first N entries (after filtering and ranking)

  Render:
    as-list                One line per entry (terminator)

Examples:

  sdd view --layout=active:as-list
    Show all active and open entries.

  sdd view --layout=active:n(20):rank(heat(exp-14d)):as-list
    Top 20 active entries by 14-day heat — the catch-up "what's warm" view.

  sdd view --layout=rank(in-degree):n(15):as-list
    Top 15 entries by structural in-degree, ignoring recency.

  sdd view --layout=active:kind(plan):rank(heat):n(5):as-list
    Top 5 active plans by heat with default decay.

  sdd view --layout=rank(by(date)):n(10):as-list
    Most recent 10 entries (sort by creation time, no scores).

  sdd view --layout=active:rank(heat(exp-7d)):n(10):as-list,active:rank(heat(exp-30d)):n(10):as-list
    Two sections: top 10 by short-half-life heat, then top 10 by long-half-life heat —
    useful for comparing "what's hot now" against "what's been hot for a while".

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
