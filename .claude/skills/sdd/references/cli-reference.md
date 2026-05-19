---
sdd-content-hash: c3115b188f45e46e2dac1787fa80710c31a0b229f5466b1fbf82965521c8752f
sdd-version: dev
---
# SDD CLI Reference

## Commands

- `sdd info` — session framing only: `Local participant: ...`, `Language: ...` (when configured), `Search: ...`. Stable surface for skill `!`sdd ...`` injections that need the agent to see who's local and which retrieval modes are available without the rest of `sdd status`.
- `sdd status` — overview grouped by decision kind (Aspirations, Contracts, Plans, Activities, Directives), plus Gaps and Questions, Recent Insights, and Recent Done Signals (uses summaries). Header lines match `sdd info` byte-for-byte.
- `sdd view --layout=<spec>` — composable pipeline of primitives (source, filter, transform, aggregate, rank, page, render) with named macros as sugar. Mechanical catch-up at scale; bare `sdd view` prints help with vocabulary tables. See "`sdd view` pipeline" below.
- `sdd show <id>` — full entry with upstream summary chain (depth-limited)
- `sdd show <id> --downstream` — include downstream entries (refd-by, closed-by, superseded-by)
- `sdd show <id> --max-depth N` — set upstream/downstream expansion depth (default 4, 0 = primary only)
- `sdd list [--type d|s|a] [--layer stg|cpt|tac|ops|prc] [--kind <kind>] [--topic <label>]` — filtered listing. `--kind` accepts any signal kind (gap, fact, question, insight, done, actor, annotation) or decision kind (directive, activity, plan, contract, aspiration, role, focus); the two sets are disjoint. `--topic` filters to entries whose effective topic set (inline `topics:` ∪ topics declared by annotations whose refs include the entry) has any label with the given path as a component-wise, case-insensitive prefix. Uses summaries.
- `sdd new <type> <layer> [flags] <description>` — create entries (output prints the new entry ID, file path, and the LLM-generated summary so the agent can verify fidelity)
- `sdd summarize [<id> | --all]` — regenerate entry summaries
- `sdd summarize <id> --text "<summary>"` — write a user-supplied summary directly, bypassing the LLM. Use `--text -` to read from stdin. Single entry only; rejected with `--all` or multiple IDs. The hash is recomputed from the current prompt so subsequent automatic regenerations skip-by-hash unless `--force` is passed.
- `sdd lint` — check graph integrity (dangling refs, type mismatches, broken attachment links, stale summaries). When an embedding provider is configured, also reports the search index's fingerprint and drift count.
- `sdd index` — warm up the per-participant search index at `.sdd/index/` over every entry on disk. Skips entries whose stored hash and embedder fingerprint match the manifest; `--force` re-embeds everything regardless. Required only on a fresh clone or after deliberate full rebuilds — `sdd search` lazy-fills missing/stale entries on demand.
- `sdd search [--term <regex>...] [--query <phrase>] [--type d|s] [--layer ...] [--kind ...] [--include-superseded] [--limit N]` — three-mode retrieval. `--term` runs text mode (live grep with multi-term AND), `--query` runs vector mode, both flags together run hybrid (RRF fusion). At least one of `--term` / `--query` is required; vector and hybrid require `Search: vector,text` in `sdd status`. See [search reference](search.md) for citation reading and mode selection guidance.
- `sdd wip start <entry-id> --exclusive --participant <name> <description>` — create WIP marker
- `sdd wip start <entry-id> --branch --exclusive --participant <name> <description>` — create WIP marker, create git branch and check out to it
- `sdd wip done <marker-id>` — remove WIP marker (deletes branch if merged)
- `sdd wip done <marker-id> --force` — remove WIP marker and force-delete unmerged branch (discard flow)
- `sdd wip list` — list active WIP markers

## Entry IDs

Every argument that takes an entry ID — positional args on `sdd show`, `sdd summarize`, and the `--refs`, `--closes`, `--supersedes` flags on `sdd new` — accepts both:

- **Full ID** (e.g. `20260408-104102-d-prc-oka`) — deterministic and collision-proof as the graph grows. **Agents always use full IDs when invoking the CLI.**
- **Short ID** (e.g. `d-prc-oka`, shape `{type}-{layer}-{suffix}`) — human convenience. Resolves to the full ID when the suffix uniquely identifies an entry. Ambiguous short IDs exit non-zero and list all matching full IDs.

Short IDs are fine in user-facing narrative (catch-up tables, grooming summaries, dialogue). Never substitute them for full IDs in CLI calls you construct — a suffix collision would break the call later when the graph grows.

## `sdd view` pipeline

`sdd view` runs a layout pipeline over the graph. Each section is a colon-chained sequence of function calls; multiple sections separate with commas. Render is always the section's terminator. Filters intersect cumulatively; non-filter modifiers (rank, page, name, render) apply last-write-wins per kind.

```
layout    := entry ("," entry)*
entry     := func (":" func)*
func      := name ("(" arg-list? ")")?
```

Args use parens: `kind(plan)`, `n(10)`. Multi-arg disjunction: `kind(plan,directive)`. Nested calls let algorithms carry decay: `rank(heat(exp-14d))`. Strings use quotes: `topic("infrastructure/cli")`. No whitespace anywhere except inside quoted strings.

### Sources

| Function | Semantics |
|---|---|
| `source(graph)` | All graph entries (default if omitted) |
| `source(wip)` | Active WIP markers from `.sdd/graph/wip/`. Disjoint vocabulary — only `name()` and `as-wip-list` compose; graph-side primitives error |

### Filters (graph source)

| Function | Semantics |
|---|---|
| `active` | Entries not closed and not superseded |
| `kind(K[, K2, ...])` | Disjunction within one filter; multiple `kind()` calls intersect |
| `layer(L)` | Entries at layer L (`stg`, `cpt`, `tac`, `ops`, `prc`; full names also accepted) |
| `since(spec)` | ISO date `YYYY-MM-DD` or duration `Nd|Nw|Nm|Ny`. Quoted string. m/y use calendar arithmetic; d/w use 24h offsets |
| `topic(L)` | Entries whose effective topic set has L as a component-wise prefix (case-insensitive) |
| `not(<filter>)` | Excludes entries matched by the inner filter. Supported inner: `kind`, `layer`, `topic` |

### Rank, page, output, transforms

| Function | Semantics |
|---|---|
| `rank(<algorithm>)` | Sort by computed score, descending. Adds `{score: X.XXX}` to rendered entries |
| `n(N)` | Take first N entries (after filtering and ranking) |
| `name(<string>)` | Final section header — overrides any prefix and any rank-based auto-derive. Last call wins; `name("")` clears any prior name |
| `name-prefix(<string>)` | Prefix the auto-derive composer extends with the rank suffix. Macros bake this so `top(N)` reads "Top by heat (exp-14d)" by default and "Top by in-degree" after `:rank(in-degree)` — the prefix stays, the suffix tracks rank |
| `expand(involvement)` | Per row, explode involvement triples into focus-block sub-rows (focus-block only) |
| `group(by(<field>))` | Bucket entries by field; produces grouped shape (consume with `as-grouped`) |
| `stalled(<value>)` | Threshold below which a focus target with assigned actors is "stalled" (default 1.0) |

**Section header resolution:** the executor picks the header in this order:
1. Explicit `name(...)` → final, no auto-append.
2. `name-prefix(...)` set + `rank(...)` set → `"<prefix> by <algorithm>"` (e.g. "Top by heat (exp-14d)", "Done by date").
3. `name-prefix(...)` set, no rank → just the prefix (e.g. "Focus", "Participants").
4. No prefix, rank set → `"Top by <algorithm>"` (covers raw `rank(heat):as-list` without a macro).
5. Neither → headerless section.

### Algorithms (used inside `rank(...)`)

| Algorithm | Formula |
|---|---|
| `heat(decay)` | Σ over incoming refs of decay(age). Default decay: `exp-14d` |
| `in-degree` | Raw count of incoming refs (decay arg ignored) |
| `mult(decay)` | `heat(decay) × in-degree` |
| `add(decay)` | `heat(decay) + in-degree` |
| `log(decay)` | `heat(decay) × log(1 + in-degree)` |
| `by(date)` | Sort by entry creation timestamp (no scores) |

### Decay names (used inside algorithm calls)

| Name | Formula |
|---|---|
| `exp-{7,14,30}d` | `2^(-age_days/N)` — half-life every N days |
| `linear-{7,14,30}d` | `max(0, 1 - age_days/N)` — zero past N days |
| `none` | `1` (no age effect) |

### Render terminators

| Render | Input shape |
|---|---|
| `as-list` | Flat list — one line per entry. Auto-derives header from rank when no `name()` (e.g. "Top by heat (exp-14d)", "Most recent" for by(date)) |
| `as-grouped` | Grouped result (requires `group(by(<field>))`); one `### <bucket>` per group |
| `as-focus-block` | Focus-block result (requires `kind(focus):active:expand(involvement)`); per-focus header + per-target lines tagged `{state: pull-available|stalled|driving}` |
| `as-participants-block` | Active actor heads + bound active roles (requires `kind(actor):active`); one `### <canonical>` per actor with bound role entries underneath |
| `as-wip-list` | WIP marker rows (requires `source(wip)`) |

### Macros

Each macro bakes a `name-prefix(...)` so the rendered section gets a header automatically; for ranked macros the resolver appends the rank suffix.

| Macro | Expansion | Default header |
|---|---|---|
| `top(N)` | `active:n(N):rank(heat(exp-14d)):name-prefix("Top"):as-list` | `## Top by heat (exp-14d)` |
| `topic(L)` | `topic(L):rank(heat(exp-14d)):name-prefix("Topic: <L>"):as-list` | `## Topic: <L> by heat (exp-14d)` |
| `focus` | `kind(focus):active:expand(involvement):name-prefix("Focus"):as-focus-block` | `## Focus` |
| `decisions` | `active:kind(plan,directive,activity,contract,aspiration):group(by(kind)):name-prefix("Decisions"):as-grouped` | `## Decisions` |
| `signals` | `active:kind(gap,question):group(by(kind)):name-prefix("Signals"):as-grouped` | `## Signals` |
| `insights` | `active:kind(insight):since("30d"):rank(by(date)):name-prefix("Insights"):as-list` | `## Insights by date` |
| `done` | `kind(done):since("30d"):rank(by(date)):name-prefix("Done"):as-list` | `## Done by date` |
| `aspirations` | `active:kind(aspiration):name-prefix("Aspirations"):as-list` | `## Aspirations` |
| `contracts` | `active:kind(contract):name-prefix("Contracts"):as-list` | `## Contracts` |
| `participants` | `active:kind(actor):name-prefix("Participants"):as-participants-block` | `## Participants` |
| `wip` | `source(wip):name-prefix("WIP"):as-wip-list` | `## WIP` |

User modifiers append after macro expansion and resolve via last-write-wins. Examples:

- `top(20):rank(in-degree)` → `## Top by in-degree` (prefix "Top" stays; suffix tracks the override)
- `top(20):name("My summary")` → `## My summary` (explicit `name(...)` is final, no auto-append)
- `focus:name("Active focuses")` → `## Active focuses` (final override)
- `focus:name-prefix("Active")` → `## Active` (prefix override; no rank to append)

### Worked examples

```bash
sdd view --layout='top(20)'
# Twenty most-warm active entries (catch-up "what's hot")

sdd view --layout='focus,top(15)'
# Active focuses with state-derived involvement, then top-15 by heat.
# Sections render independently — an entry in both the focus block and
# the top list appears in both (cross-section dedup is an open design
# question; sections currently render fully on their own information).

sdd view --layout='decisions,signals,participants'
# Three sections: kind-grouped decisions, kind-grouped signals,
# active actor canonicals with bound roles

sdd view --layout='topic(catch-up-scaling):rank(heat):n(10):as-list'
# Top 10 entries clustered under catch-up-scaling, ranked by heat

sdd view --layout='wip'
# Active WIP markers (just source(wip):as-wip-list under the hood)

sdd view --layout='top(10):rank(heat(exp-7d)):name("Hot last week"),top(10):rank(heat(exp-30d)):name("Hot last month")'
# Two side-by-side top-10s with explicit headers

sdd view --layout='top(20):not(kind(contract,aspiration))'
# Top 20 by heat, excluding standing entries that anchor by structure
```

## `sdd show` output format

- **Depth 0** (target entry): metadata block, then a `Summary:` section (omitted when no summary is stored), then the full body
- **Depth 1+** (upstream/downstream): summary lines with relation labels, kind, and entry ID
- **Dedup**: each entry shown at shallowest occurrence; later encounters show `(see above)`
- **Truncation**: at max-depth boundary, hidden entries listed as `[truncated: refs <id>, ...]`

Summary line format: `{indent}- {relations} {full-id} ({kind}): "{summary}"`

The depth-0 `Summary:` section renders the same text shown in `sdd list` and `sdd status` — surfacing it inline gives readers a quick orientation when looking up an ID and makes summary-body drift visible during normal review.

## `sdd new` flags

- `--refs '{json}'` — referenced entry with semantic kind (repeatable). JSON object `{"id":"<id>","kind":"<kind>","desc":"<optional>"}`. Kind comes from the closed set below; the legacy CSV form `--refs id1,id2` was dropped because each ref now carries its own kind. `desc` is optional, used to explain *why* this ref exists in this entry's narrative.
- `--supersedes id` — entry ID this replaces
- `--closes id1,id2` — entry IDs this resolves/fulfills
- `--participants p1,p2` — participant names
- `--confidence high|medium|low` — confidence level
- `--kind <kind>` — signals: gap (default), fact, question, insight, done, actor, annotation; decisions: directive (default), activity, plan, contract, aspiration, role, focus
- `--canonical name` — frontmatter `canonical` (kind: actor only)
- `--aliases a,b` — frontmatter `aliases` (kind: actor only)
- `--actor canonical` — frontmatter `actor` (kind: role only)
- `--topics LABEL[,LABEL...]` — inline `topics:` labels (any kind). CSV form. Each label is a topic-path string (`/`-joined components, e.g. `infrastructure/cli`).
- `--topic '{json}'` — annotation topic cluster (kind: annotation only). Repeatable. Either a JSON object `{"label":"path","members":["id",...]}` (sub-selection of refs) or a bare label like `--topic catch-up-scaling` (applies to all refs).
- `--actors NAME[,NAME...]` — focus-level default actor canonicals (kind: focus only). CSV form.
- `--when '{json}'` — focus-level default temporal scope (kind: focus only). JSON object `{"from":"YYYY-MM-DD","to":"YYYY-MM-DD"}`; at least one of `from` / `to` is required when present.
- `--involvement '{json}'` — focus involvement triple (kind: focus only). Repeatable. JSON object `{"target":"<id>","actors":["..."],"when":{"from":"...","to":"..."}}`. Omitting `actors` inherits the focus-level default; explicit `"actors":[]` declares pull-available involvement (deliberately unattributed).
- `--attach spec` — file to attach (repeatable, see below)
- `--skip-preflight` — skip pre-flight validation (entry is annotated with `preflight: skipped`)
- `--dry-run` — run validation and pre-flight only, without writing or committing the entry
- `--preflight-timeout` — timeout for pre-flight validation (default `2m`)

See the Entry IDs section above for how ID arguments are resolved across all commands.

## Annotation and focus capture examples

**Annotation — tag a cluster of entries with a topic** (one entry, all refs are members):

```bash
sdd new s cpt --kind annotation --confidence medium \
  --refs '{"id":"20260505-215340-s-cpt-rwd","kind":"related"}' \
  --refs '{"id":"20260505-215333-s-cpt-jq7","kind":"related"}' \
  --refs '{"id":"20260504-100323-s-cpt-8tu","kind":"related"}' \
  --topics catch-up-scaling \
  "These three entries cluster around the catch-up-scaling concern that drove Plan 1 / Plan 2."
```

**Annotation with multiple topics and a sub-selection**:

```bash
sdd new s cpt --kind annotation --confidence medium \
  --refs '{"id":"20260423-203503-d-cpt-ygn","kind":"related"}' \
  --refs '{"id":"20260506-151849-d-tac-gvn","kind":"related"}' \
  --topic catch-up-scaling \
  --topic '{"label":"type-system/kinds","members":["20260423-203503-d-cpt-ygn"]}' \
  "Both refs are catch-up related; only ygn is also a type-system/kinds entry."
```

The first `--topic` is bare-label form (applies to all refs); the second restricts to one member.

**Focus — declare an involvement period** (uses outer single quotes for JSON to preserve `$`/backticks; inner double quotes for JSON keys):

```bash
sdd new d tac --kind focus --confidence medium \
  --participants Christopher,Claude \
  --actors Christopher,Claude \
  --when '{"from":"2026-05-06","to":"2026-05-20"}' \
  --involvement '{"target":"20260506-151849-d-tac-gvn"}' \
  --involvement '{"target":"20260506-151345-d-tac-uww","actors":[]}' \
  --involvement '{"target":"20260506-152044-d-tac-1du","actors":["Claude"],"when":{"from":"2026-05-13","to":"2026-05-20"}}' \
  "Drive type-system 7+7 + sdd view + playbook rewrite over the next two weeks."
```

The first triple inherits both top-level defaults; the second overrides actors with explicit empty (pull-available); the third overrides both actors and when.

## Quoting convention for JSON flags

- Outer single quotes preserve `$`, backticks, and backslashes verbatim
- Inner double quotes for JSON keys and string values
- The CLI accepts JSON parse errors gracefully — failures cite the offending field path

## Attachments

The `--attach` flag is repeatable and supports filename mapping:

- `--attach path/to/file.md` — attach with original filename
- `--attach path/to/file.md:renamed.md` — attach with a custom target filename
- `--attach -:plan.md` — read stdin and save as `plan.md` (at most one `-` per invocation)

Use `{{attachments}}/filename` in the entry description to link to attachments. The CLI resolves these to relative paths. Example:

```
sdd new d tac --attach /tmp/design.md:plan.md "See [plan]({{attachments}}/plan.md) for details."
```

## Long descriptions

Descriptions are positional arguments. For multi-line markdown (plan decisions with `## Acceptance criteria`, or decisions with rationale paragraphs), use quoted heredocs assigned to variables — no temp files needed. Pipe the attachment content via stdin with `--attach -:plan.md`:

```
DESC=$(cat <<'EOF'
Fork SDD into a standalone repo...

## Acceptance criteria

- [ ] Repository exists with main branch pushed
- [ ] ...
EOF
)

PLAN=$(cat <<'EOF'
# Fork plan

## Alternatives considered
...
EOF
)

echo "$PLAN" | sdd new d tac --kind plan --confidence high \
  --refs '{"id":"<id>","kind":"addresses"}' \
  --participants "Name,Claude" \
  --attach -:plan.md \
  "$DESC"
```

Use quoted `'EOF'` so markdown content with `$`, backticks, or backslashes is preserved verbatim. For scratch files you do want on disk, `.sdd/tmp/` is gitignored.

## Ref kinds

Every reference (the `refs:` field) carries a semantic kind from a closed vocabulary, capturing *why* this entry points at the referenced one. Pre-flight rejects any new entry whose refs lack a kind from the set below.

| Kind | When to use |
|---|---|
| `grounds` | Anchors to standing structure — a contract, aspiration, or active standing directive that the entry leans on |
| `builds-on` | Extends prior lineage — a previous decision or plan that this entry continues *after* the target (the target is closed, or the new entry adds work in a forward "next step" sense rather than refining in place) |
| `refines` | Sharpens, narrows, or clarifies an **active** target's commitments **in place** — the augmenting-directive pattern. Target stays active; lifecycle is split (the refining entry closes alongside the target via the target's done signal) |
| `addresses` | Responds to a gap, question, or insight signal — the entry's purpose is to act on it |
| `surfaces` | Created or discovered the referenced gap during this work — used when capture surfaces both the signal and the decision in one pass |
| `evidence` | Empirical observation supporting the claim — a fact or done signal whose data the entry cites |
| `depends-on` | Functional prerequisite — the referenced entry must land before this one is meaningful |
| `related` | Parallel sibling, no other axis fits — neighborly context that doesn't ground, build on, address, or depend |

**Distinguishing `refines` from `builds-on`.** Both name a forward relationship to a prior entry, so the test is: *is the target still active, and does the new entry sharpen its commitments in place, or does it continue the chain in time?* If the target is active and the body narrows / clarifies / qualifies the target's existing commitments without superseding, use `refines`. If the target is closed (or the new entry stands alongside in a "next step" sense rather than refining in place), use `builds-on`. The augmenting-directive pattern always uses `refines`.

`closes` and `supersedes` stay bare-string ID lists — those relationships carry uniform mechanical meaning and don't need per-edge metadata.

**Legacy entries** with bare-string refs continue to parse for traversal (mapped to `kind: unknown` internally), but `sdd new` always writes object form with explicit kind. Once a binary on this codebase writes a new entry, *older binaries cannot read it* — keep binary and skill upgrades in sync across the team.

**On-disk shape**:

```yaml
refs:
  - id: 20260101-000000-s-cpt-aaa
    kind: addresses
    desc: resolves the gap surfaced in the bootstrap session
  - id: 20260102-000000-d-cpt-bbb
    kind: grounds
```

The optional `desc` is rendered on `sdd show` as a sub-line beneath the ref, giving readers per-ref rationale without parsing the body.
