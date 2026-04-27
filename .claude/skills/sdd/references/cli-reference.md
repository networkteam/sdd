---
sdd-content-hash: 7c5b468c0fec405135f0ce2e05031bc42e58fd642631639102d260ec853d3d93
sdd-version: dev
---
# SDD CLI Reference

## Commands

- `sdd status` — overview grouped by decision kind (Aspirations, Contracts, Plans, Activities, Directives), plus Gaps and Questions, Recent Insights, and Recent Done Signals (uses summaries)
- `sdd show <id>` — full entry with upstream summary chain (depth-limited)
- `sdd show <id> --downstream` — include downstream entries (refd-by, closed-by, superseded-by)
- `sdd show <id> --max-depth N` — set upstream/downstream expansion depth (default 4, 0 = primary only)
- `sdd list [--type d|s|a] [--layer stg|cpt|tac|ops|prc] [--kind <kind>]` — filtered listing. `--kind` accepts any signal kind (gap, fact, question, insight, done, actor) or decision kind (directive, activity, plan, contract, aspiration, role); the two sets are disjoint. Uses summaries.
- `sdd new <type> <layer> [flags] <description>` — create entries (output prints the new entry ID, file path, and the LLM-generated summary so the agent can verify fidelity)
- `sdd summarize [<id> | --all]` — regenerate entry summaries
- `sdd summarize <id> --text "<summary>"` — write a user-supplied summary directly, bypassing the LLM. Use `--text -` to read from stdin. Single entry only; rejected with `--all` or multiple IDs. The hash is recomputed from the current prompt so subsequent automatic regenerations skip-by-hash unless `--force` is passed.
- `sdd lint` — check graph integrity (dangling refs, type mismatches, broken attachment links, stale summaries)
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
- `--kind <kind>` — signals: gap (default), fact, question, insight, done, actor; decisions: directive (default), activity, plan, contract, aspiration, role
- `--canonical name` — frontmatter `canonical` (kind: actor only)
- `--aliases a,b` — frontmatter `aliases` (kind: actor only)
- `--actor canonical` — frontmatter `actor` (kind: role only)
- `--attach spec` — file to attach (repeatable, see below)
- `--skip-preflight` — skip pre-flight validation (entry is annotated with `preflight: skipped`)
- `--dry-run` — run validation and pre-flight only, without writing or committing the entry
- `--preflight-timeout` — timeout for pre-flight validation (default `2m`)

See the Entry IDs section above for how ID arguments are resolved across all commands.

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
