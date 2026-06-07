# sdd stats — v1 design

## Purpose

`sdd stats` is the analytics surface: how the graph is being worked, and what its
LLM operations cost. It reads two sources — the per-call stats sink
(`.sdd/stats/llm.jsonl`) for **usage**, and the graph itself for **activity** — and
presents them as separate sections.

It is distinct from `sdd view`, whose purpose is surfacing *live graph entries* for
working and catch-up. Stats never lands in `view`, and `view`'s graph vocabulary
never lands in `stats`. The two answer different questions from different sources.

Grounded in the terminal-experience architecture (d-cpt-mvb): TTY humans get a
lipgloss-styled view; agents and pipes get clean structured output via
`--format json` (the convention d-cpt-5f4 / d-cpt-rkj already set).

## v1 scope

v1 covers **usage only** (the sink). Activity (graph-derived) is a follow-up.

**In scope**
- **U1** — usage rolled up by model/provider over a date range.
- **U2** — per-op rollups with throughput (the ollama-vs-omlx comparison need).
- **X1** — `--format json` emits the same aggregates for agents.
- **X2** — date range selectable via `--since`.

**Out of v1 (roadmap, recorded so nothing is lost)**
- **U3** — live `--watch` monitor. Depends on the interactive output-coordinator
  that d-cpt-mvb *defers*; ships when that lands. (This is why U3 is out of v1 — a
  dependency boundary, not just priority.)
- **A1** — activity overview (entries over time, totals by type/layer/kind, active
  days, streak). **Heatmap postponed**: Git/forge (GitHub/GitLab) contribution
  graphs already serve the visual-activity purpose, so an SDD-native heatmap is low
  priority.
- **A2** — activity by participant.

## CLI surface

```
sdd stats [--since <spec>] [--op <op>] [--provider <p>] [--model <m>] [--format json]
```

- `--since` — `all-time` (default) | `7d` | `30d` | `YYYY-MM-DD`. Reuses `sdd view`'s
  `since()` parser.
- `--op` / `--provider` / `--model` — filter the dataset before rollup (composable,
  echoing `view`'s filter philosophy). `sdd stats --op embed-documents` *is* the
  provider-comparison view.
- `--format json` — structured aggregates on stdout, no TUI chrome.
- Absent / empty sink → clean `no stats recorded yet` message, exit 0 (not an error).

## Views (TTY)

> **These mockups are a starting point, not a frozen spec.** The concrete visual
> design — layout, headings, colors, styling, spacing — is evaluated and refined
> *live with the user* during implementation (this is exactly the per-command UX
> design d-cpt-mvb defers). The acceptance criteria fix the **data and structure**;
> the **look** is a collaborative implementation-time activity, not dictated here.

### Default — `sdd stats --since 30d`

```
 sdd stats — last 30 days
 source: .sdd/stats/llm.jsonl · 412 calls

 Totals
   tokens in   1.84M      cache read   980k       calls   412
   tokens out  168k       cache write   42k        time    2h 14m

 By model
   MODEL                PROVIDER     CALLS      IN     OUT   CACHE R   CACHE W       TIME
   claude-sonnet-4-6    anthropic      178   1.20M    160k      938k       42k    36m 12s
   qwen3-embedding:8b   ollama         210    642k      —         —         —    1h 37m
   claude-haiku-4-5     anthropic       24    2.1k     8k          0         0        18s

 By operation
   OP                 CALLS   ITEMS      IN    TOK/S   MS/CALL   ITEMS/S
   embed-documents      186    3.1k    638k    1.39k       720       9.2
   embed-queries         24      24    4.0k      870       184       5.4
   summarize            146       —   1.02M    2.10k       310         —
   preflight             56       —    178k    1.80k       540         —
```

(Numbers illustrative. Embeds have no OUT; chat ops have no ITEMS — shown as `—`.)

### Comparison — `sdd stats --op embed-documents`

The U2 payoff: the omlx-vs-ollama question settled on one screen.

```
 sdd stats — embed-documents · all time
 source: .sdd/stats/llm.jsonl · 372 calls

 By model
   MODEL                        PROVIDER   CALLS   ITEMS      IN    TOK/S   MS/CALL   ITEMS/S
   qwen3-embedding:8b           ollama       186    3.1k    638k    1.39k       720       9.2
   Qwen3-Embedding-8B-4bit-DWQ  openai       186    3.1k    638k    2.65k       377      17.6
```

(Illustrative — would show whether omlx actually wins. Note the `openai` provider
label: any OpenAI-compatible endpoint records as provider `openai`, so omlx and real
OpenAI are told apart by **model name**, not provider. See open questions.)

## `--format json`

Same aggregates, no chrome — the agent path.

```json
{
  "range": { "since": "2026-05-08", "until": "2026-06-07" },
  "source": ".sdd/stats/llm.jsonl",
  "totals": { "calls": 412, "tokens_in": 1840000, "tokens_out": 168000,
              "cache_read": 980000, "cache_create": 42000, "duration_ms": 8040000 },
  "by_model": [
    { "model": "claude-sonnet-4-6", "provider": "anthropic", "calls": 178,
      "tokens_in": 1200000, "tokens_out": 160000, "cache_read": 938000,
      "cache_create": 42000, "duration_ms": 2172000 }
  ],
  "by_op": [
    { "op": "embed-documents", "calls": 186, "items": 3100, "tokens_in": 638000,
      "tokens_per_s": 1390, "ms_per_call": 720, "items_per_s": 9.2,
      "duration_ms": 459000 }
  ]
}
```

## Throughput definitions

Computed over each grouped row:
- `tok/s`   = Σ input_tokens / Σ duration_seconds
- `ms/call` = Σ duration_ms / calls
- `items/s` = Σ items / Σ duration_seconds (embedding ops; blank when items = 0)

## Implementation placement (CQRS — contract d-cpt-ah1)

- `query/` — `StatsQuery` (range + op/provider/model filters).
- `finders/` — `StatsFinder` reads `.sdd/stats/llm.jsonl` and delegates aggregation
  to `model`. Pure read.
- `model/` — pure aggregation: rollup by model and by op, throughput math. No I/O.
- `presenters/` — TTY renderer uses **`github.com/charmbracelet/lipgloss/table`**
  (the `table` subpackage of the already-vendored lipgloss v1.1.0): declarative
  `table.New().Headers(...).Rows(...).StyleFunc(...)` renders to a string, printed
  one-shot with no event loop. Agent path is plain JSON. Renderer selected by TTY
  detection + `--format` (per d-cpt-mvb / d-cpt-5f4). No new dependency.
- `llmstats/` — add a **reader** alongside the existing writer, sharing the one
  record shape (single-path; no duplicate schema).

## Rendering libraries (build on what's vendored)

- **v1** — `lipgloss/table` (v1.1.0, already in go.mod). Styled tables → string →
  print. Nothing else needed for one-shot output.
- **U3 (`--watch`)** — `bubbles/table` (interactive table, bubbles v1.0.0 already
  vendored) inside a bubbletea program. Still no new dependency.
- **A1 heatmap** — would need `github.com/NimbleMarkets/ntcharts` (community charts
  on bubbletea+lipgloss; bar/line/sparkline/heatmap). A **new third-party dep** —
  only justified if we ever build native charts, which Git/forge activity graphs
  already make low-priority. Noted, not committed.

## Inspiration / non-goals

Modeled on Claude Code's Usage/Stats screens (date toggle, by-model rollup, per-op
breakdown). **Dropped**: dollar cost and limit/quota bars — SDD tracks tokens, not
dollars (no pricing table, per d-tac-zis), and has no quota.

## Open questions (resolve at implementation, not blocking capture)

1. **Default `--since`** — all-time vs 30d.
2. **Narrow terminals** — truncate long model names (as in the comparison mockup) or
   wrap? Column set at < 80 cols.
3. **Non-TTY, non-json default** — plain aligned table, or fall through to json?
4. **omlx vs OpenAI disambiguation** — record the endpoint host so same-model,
   different-server rows are distinguishable? (v1: model name suffices.)
5. **lipgloss v1 → v2** — charm's docs now show v2 (`charm.land/lipgloss/v2`);
   go.mod pins v1.1.0. Stats v1 uses the v1 `table` package and does **not** couple
   to a v2 upgrade — that's a separate, repo-wide decision.
