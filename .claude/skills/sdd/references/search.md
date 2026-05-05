---
sdd-content-hash: 56ad1eea8b1cb69f93b82cea0a358593cfd7cc5de5e384895850c64675a46ae8
sdd-version: dev
---
# sdd search

Three retrieval modes, one CLI command. Use search to **surface candidates** during exploration and grooming — never as a substitute for the dialogue or for reading a candidate's full chain via `sdd show`.

## When to use each mode

- **`--term <regex>`** — keyword / token / ID lookup. Always available. Use for:
  - Finding entries that mention a specific identifier (`--term "d-tac-lqr"`)
  - Tracking down a precise phrase (`--term "augment plan"`)
  - Filtering by a known token someone just mentioned in dialogue
  - Repeatable: `--term apple --term harvest` requires both terms in the entry (AND); use `--term "(apple|orange)"` for OR via regex alternation.

- **`--query <phrase>`** — semantic / conceptual lookup. Requires `Search: vector,text` in the `sdd status` header (an embedding provider is configured). Use for:
  - Surfacing entries that talk about the same concept with different wording
  - Finding "anything related to X" when you don't know the exact terminology
  - Bridging across language differences in an evolving graph

- **Both flags together** — hybrid mode (RRF fusion). Use for important explorations where token specificity AND semantic breadth both matter. Don't combine reflexively for routine queries.

**Misuse to avoid:** `--query` with a single keyword (e.g. `--query "auth"`) is wasted spend — use `--term auth` for that. `--query` should carry phrase-level intent (e.g. `--query "how do we handle authentication boundaries"`).

## Reading citations

Each result row is the entry header line (matching `sdd list` shape) plus an indented citation:

```
20260504-235258-d-tac-lqr tactical plan decision [confidence: medium] (Christopher, Claude) {status: active} <summary>
    ↳ Approach > Storage  ·  <~150-char snippet>
```

The breadcrumb (`Approach > Storage`) is the heading-chain inside the entry's body or attachment that motivated the hit. `Summary` means the hit came from the entry's summary chunk; `Body` means body text without a heading. Attachments render as `[attachment: <path>]`.

The citation tells you **where in the entry** the match landed. Don't treat the snippet as the answer — it's a pointer. Once you've found a likely candidate, run `sdd show <full-id>` for the full chain.

## Filters

Same as `sdd list`: `--type`, `--layer`, `--kind`. Compose freely with `--term` / `--query`. Default behavior excludes `{status: superseded-by}` entries from results because superseded near-duplicates pollute clusters; `--include-superseded` overrides when you specifically want history.

`--limit N` caps the result count (default 10).

## Lifecycle

Vector mode reads from a per-participant local index at `.sdd/index/` (gitignored). Two ways the index gets populated:

- **`sdd index`** — explicit warm-up: builds chunks for every entry on disk. Run once on a fresh clone, after a major batch of new entries, or after changing the embedding model. `--force` re-embeds everything regardless.
- **Lazy fill** — `sdd search` automatically chunks and embeds entries that are present on disk but missing from (or stale against) the index before the query runs. The first search after a branch switch or new captures may emit a few `lazy-indexed` lines; subsequent searches are fast.

`sdd lint` reports `Index:` health when an embedding provider is configured: total entries indexed and the count of entries indexed under a different fingerprint (drift). Drift converges as entries are re-embedded by `sdd search` or `sdd index --force`.

## Use in playbooks

**Explore (`/sdd-explore` and the Explore Playbook)** — when the briefing question is "what's related to this concept?" or "what entries already touch this area?", `sdd search --query "<concept phrase>"` surfaces seed candidates. Combine with `--term` when you have a known identifier or canonical name.

**Groom (`/sdd-groom` and the Grooming Playbook)** — when checking for **Pattern B** (superseded in practice but no explicit `--supersedes` link), `sdd search --query` over the candidate's summary phrase often surfaces the newer entry. Same for hunting siblings before capturing a fresh signal that might re-frame existing ground.

**Capture-time discovery** — before drafting a new entry, `sdd search` for the topic to check whether you'd be re-stating something already in the graph. Refs are cheaper than supersedes; finding the existing entry up front lets you ref it instead.

Search returns ranked candidates — never a single answer. The agent applies semantic judgment via dialogue on the results, consistent with the dialogue-first commitment (d-stg-beb).
