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
    layer(L)               Entries at the given layer (stg, cpt, tac, ops, prc;
                           full names also accepted)
    since(spec)            Entries on/after a cutoff. Spec is ISO date
                           ("2026-04-01") or duration ("7d", "2w", "1m", "1y").
                           m and y use calendar arithmetic; d and w use 24h
                           offsets. Argument must be a quoted string.
    topic(L)               Entries whose effective topic set has L as a
                           component-wise prefix (case-insensitive). Use
                           bare identifiers for simple labels (catch-up-scaling)
                           and quoted strings for paths ("infrastructure/cli").

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

  Aggregate:
    group(by(<field>))     Bucket entries by a frontmatter field. Field is one
                           of kind, layer, type. Buckets emit alphabetically.
                           Mutually exclusive with rank() and n() in this slice.

  Render:
    as-list                One line per entry (terminator)
    as-grouped             One ### header per group, then entry lines (terminator;
                           requires a preceding group(by(<field>)))

  Macros (named pipelines, recognised at section start; user modifiers append):
    top(N)                 active:n(N):rank(heat(exp-14d)):as-list
    topic(L)               topic(L):rank(heat(exp-14d)):as-list
    decisions              active:kind(plan,directive,activity,contract,aspiration)
                           :group(by(kind)):as-grouped
    signals                active:kind(gap,question):group(by(kind)):as-grouped
    insights               active:kind(insight):since("30d"):rank(by(date)):as-list
    done                   kind(done):since("30d"):rank(by(date)):as-list
    aspirations            active:kind(aspiration):as-list
    contracts              active:kind(contract):as-list

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

  sdd view --layout=active:layer(tac):since("7d"):rank(heat):n(10):as-list
    Top 10 active tactical entries from the last week, ranked by heat.

  sdd view --layout=active:topic(catch-up-scaling):rank(heat):as-list
    Active entries clustered under the catch-up-scaling topic, ranked by heat.

  sdd view --layout=topic("infrastructure/cli"):rank(by(date)):n(20):as-list
    Most recent 20 entries tagged anywhere under infrastructure/cli.

  sdd view --layout=active:kind(plan,directive,activity,contract,aspiration):group(by(kind)):as-grouped
    Active decisions grouped by kind — close to the decisions section in the
    catch-up macros (slice 6 wraps this as the named macro).

  sdd view --layout=active:kind(gap,question):group(by(kind)):as-grouped
    Active signals grouped by kind — open gaps and questions, side by side.

  sdd view --layout=top(20)
    Twenty most-warm active entries (the catch-up "what's hot" view).

  sdd view --layout=top(20):rank(in-degree)
    Same shape as top(20) but ranked by structural in-degree — user modifier
    overrides the macro's rank() per last-write-wins.

  sdd view --layout=decisions,signals
    Active decisions grouped by kind, then active gaps and questions grouped
    by kind. Two macros, two sections, one invocation.

  sdd view --layout=top(15),decisions,insights
    Mixed macros: warm top-15, decisions block, last-30-days insights.

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
			// Macro expansion is a separate query-layer pass: the parser
			// only deals with grammar, the expander substitutes named
			// pipelines like `top(N)` and `decisions` with their canonical
			// primitive sequences. User-supplied modifiers append.
			layout, err = query.ExpandMacros(layout)
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
