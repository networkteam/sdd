package main

import (
	"context"
	"fmt"
	"os"

	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
	"github.com/urfave/cli/v3"
)

// viewHelpText is shown when `sdd view` runs without `--layout`. It
// enumerates the implemented pipeline vocabulary; keep it in sync with
// the finder's knownFunctions and the cli-reference skill doc whenever a
// primitive is added — this text has drifted from the implementation
// before.
const viewHelpText = `Usage: sdd view --layout=<spec>

Compose a pipeline of primitives separated by colons; multiple sections
are separated by commas. Render is always the terminus of each section.
Pipeline applies in canonical filter → rank → page → render order
regardless of source ordering.

Args use parens: kind(plan), n(10), rank(heat). Multi-arg disjunction:
kind(plan,directive). Nested calls let algorithms carry decay:
rank(heat(exp-14d)). Strings use quotes: topic("infrastructure/cli").
No whitespace anywhere except inside quoted strings.

Implemented pipeline vocabulary:

  Sources:
    source(graph)          All graph entries (default if omitted)
    source(wip)            Active WIP markers. Disjoint vocabulary — only
                           name() and as-wip-list compose with it.

  Filters (intersect cumulatively):
    active                 Entries not closed and not superseded
    kind(K[, K2, ...])     Entries whose kind matches any of the listed kinds
    intent(I[, I2, ...])   Directives whose intent matches any listed value
                           (pending, guiding, settled). Directive-only.
    type(T)                Entries of the given type — d/s or decision/signal
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
    participant(P[, ...])  Entries listing any of the named canonical
                           participants. Bare idents for single-word names
                           (Christopher); quoted strings for names with spaces
                           ("Jonathan Philipp"). Multiple calls intersect.
    untagged               Entries whose effective topic set is empty — the
                           topic backfill worklist.
    id(ID[, ID2, ...])     Keep only the listed entries. Short IDs bare
                           (d-tac-6tz); full IDs quoted ("20260520-131326-d-tac-6tz").
    not(<filter>)          Exclude entries matched by the inner filter.
                           Inner filters: kind, intent, layer, topic.

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
      coldness(decay)      decay(entry age) / (1 + in-degree) — heat's
                           inverse: fresh, un-referenced entries rank
                           highest, surfacing unacted-on commitments.
                           Default decay: exp-30d (slower than heat).
      by(date)             Sort by entry creation timestamp; no scores.

    Decay names (used inside algorithm calls):
      exp-{7,14,30}d       2^(-age_days/N) — half-life every N days
      linear-{7,14,30}d    max(0, 1 - age_days/N) — zero past N days
      none                 No age effect (heat(none) == weighted in-degree)

  Page:
    n(N)                   Take first N entries (after filtering and ranking)

  Aggregate:
    group(by(<field>))     Bucket entries by a field — one of kind, layer,
                           type, participant. Buckets emit alphabetically.
                           participant is multi-valued: a co-authored entry
                           lands in each author's bucket. Requires as-grouped;
                           mutually exclusive with rank() and n().

  Transform:
    expand(involvement)    Explode each focus's involvement triples into
                           per-target sub-rows (as-focus-block only).
    expand(refs)           Render each entry's outgoing refs as indented
                           sub-lines (as-list only). expand(refs(inactive))
                           narrows to refs whose target is now inactive.

  Output:
    name(<string>)         Final section header '## <string>'. Last call wins;
                           suppresses the rank-derived auto-suffix. Without
                           name()/name-prefix(), a bare section emits no header.
    name-prefix(<string>)  Header prefix the auto-deriver extends with the rank
                           suffix (e.g. "Top by heat (exp-14d)").
    stalled(<value>)       Configure the heat-score threshold below which a
                           focus target with assigned actors is classified
                           "stalled". Default 1.0; applies only to
                           as-focus-block sections.
    brief                  Compact entry lines: identity plus the first
                           summary sentence, no attribute segments. Composes
                           with the entry-line renders; rejected for
                           as-counts.

  Render (terminator — required):
    as-list                One line per entry
    as-grouped             One ### header per group, then entry lines
                           (requires a preceding group(by(<field>)))
    as-counts              Per-topic count + heat rows over the filtered set
                           (no rank / n / group / expand)
    as-focus-block         Per-focus header + per-target lines with state
                           (pull-available / stalled / driving) and score
                           (requires expand(involvement))
    as-participants-block  Active actor heads + their bound active roles
                           (requires kind(actor):active)
    as-wip-list            WIP marker rows (requires source(wip))

  Macros (named pipelines, recognised at section start; user modifiers append):
    top(N)                 active:n(N):rank(heat(exp-14d)):as-list
    topic(L)               topic(L):rank(heat(exp-14d)):as-list
    focus                  kind(focus):active:expand(involvement):as-focus-block
    decisions              active:kind(plan,directive,activity,contract,aspiration)
                           :group(by(kind)):as-grouped
    signals                active:kind(gap,question):group(by(kind)):as-grouped
    insights               active:kind(insight):since("30d"):rank(by(date)):as-list
    done                   kind(done):since("30d"):rank(by(date)):as-list
    aspirations            active:kind(aspiration):as-list
    contracts              active:kind(contract):as-list
    participants           active:kind(actor):as-participants-block
    wip                    source(wip):as-wip-list

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

  sdd view --layout=active:participant("Jonathan Philipp"):rank(by(date)):as-list
    Everything Jonathan authored or co-authored, most recent first.

  sdd view --layout=active:kind(gap,question):group(by(participant)):as-grouped
    Open gaps and questions bucketed by who raised them.

  sdd view --layout=top(20):not(kind(contract,aspiration))
    Warmest active entries excluding standing structural anchors.

  sdd view --layout=active:untagged:n(20):as-list
    Active entries carrying no topic — the topic backfill worklist.

  sdd view --layout=active:as-counts
    Per-topic counts and heat across the active set.

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

Unknown primitives return a clear "unknown function" error listing the
available vocabulary. See the cli-reference skill doc for the full grammar.
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

			repoIDs, err := repos.SelectRepoIDs(cmd.StringSlice("repo"), cmd.Bool("all-repos"))
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

			result, err := f.View(query.ViewQuery{Graph: g, Layout: layout, GraphDir: dir})
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
				cacheDir, err := repos.CacheDir(repoID)
				if err != nil {
					return err
				}
				memberDir, err := repos.GraphDir(cacheDir)
				if err != nil {
					return err
				}
				mresult, err := f.View(query.ViewQuery{Graph: member, Layout: layout, GraphDir: memberDir})
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
