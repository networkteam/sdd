# SDD CLI Reference

## Commands

- `sdd status` — overview grouped by decision kind (Aspirations, Contracts, Plans, Activities, Directives), plus Gaps and Questions, Recent Insights, and Recent Done Signals (uses summaries)
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

## `sdd show` output format

- **Depth 0** (target entry): metadata block, then a `Summary:` section (omitted when no summary is stored), then the full body
- **Depth 1+** (upstream/downstream): summary lines with relation labels, kind, and entry ID
- **Dedup**: each entry shown at shallowest occurrence; later encounters show `(see above)`
- **Truncation**: at max-depth boundary, hidden entries listed as `[truncated: refs <id>, ...]`

Summary line format: `{indent}- {relations} {full-id} ({kind}): "{summary}"`

The depth-0 `Summary:` section renders the same text shown in `sdd list` and `sdd status` — surfacing it inline gives readers a quick orientation when looking up an ID and makes summary-body drift visible during normal review.

## `sdd new` flags

- `--refs id1,id2` — referenced entry IDs (context/foundation)
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
  --refs 20260505-215340-s-cpt-rwd,20260505-215333-s-cpt-jq7,20260504-100323-s-cpt-8tu \
  --topics catch-up-scaling \
  "These three entries cluster around the catch-up-scaling concern that drove Plan 1 / Plan 2."
```

**Annotation with multiple topics and a sub-selection**:

```bash
sdd new s cpt --kind annotation --confidence medium \
  --refs 20260423-203503-d-cpt-ygn,20260506-151849-d-tac-gvn \
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
  --refs <id> --participants "Name,Claude" \
  --attach -:plan.md \
  "$DESC"
```

Use quoted `'EOF'` so markdown content with `$`, backticks, or backslashes is preserved verbatim. For scratch files you do want on disk, `.sdd/tmp/` is gitignored.
