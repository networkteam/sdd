# Entry Summary Plan

## Overview

Embed a `summary` field in every entry's YAML frontmatter — 1-2 sentences (~50 words) describing the entry and its direct relationships. LLM-generated via `claude -p` prompt template (same integration pattern as pre-flight). Replaces recursive full-content ref expansion in `sdd list`, `sdd show`, `sdd status`, and all consuming skills.

## Design

### Summary scope

Each summary describes:
- **What this entry is** (observation, commitment, thing done)
- **How it relates to its direct refs/closes/supersedes** — using entry IDs as parenthetical handles

Summaries do NOT include transitive upstream context. Transitive context emerges when the CLI stitches a chain of summaries at read time — each hop adds one entry's perspective, and the consuming LLM decides what to emphasize.

### Examples

**Signal** (s-prc-okf, refs d-prc-31v):
> Pre-flight over-applies the regression test contract (d-prc-31v) to prompt template refinements and definition improvements, which have no code path to regression-test. The contract needs scoping to distinguish bug fixes from refinements.

**Decision** (d-cpt-uu1, refs d-stg-574, a-cpt-xbv, d-cpt-dlw, s-stg-qg0, closes s-cpt-4gj):
> Distribute SDD via GoReleaser → GitHub Releases → Homebrew tap + binstaller install.sh, with skills embedded in the binary via go:embed. Agent-neutral per d-stg-574, sequenced after repo fork (d-cpt-dlw), grounded in distribution research (a-cpt-xbv), resolves the distribution tooling gap (s-cpt-4gj).

**Action** (a-tac-e21, closes d-tac-q5p):
> Implemented stdin persistence and `--dry-run` flag for `sdd new`, fulfilling d-tac-q5p. Stdin saved to `.sdd-tmp/` with hash-based paths; dry-run composes with skip-preflight and inherits existing error-exit paths.

### Pattern

- First sentence: what the entry *is*
- Second sentence (if needed): how it relates to direct refs/closes — entry ID as parenthetical handle
- Entries with many refs get a compressed clause per relationship, not a sentence each
- Target: ~50 words, max 2 sentences

## Frontmatter field

```yaml
---
type: signal
layer: process
summary: "Pre-flight over-applies the regression test contract (d-prc-31v) to prompt template refinements..."
---
```

Stored as `summary` in the YAML frontmatter, alongside type, layer, confidence.

## Prompt template

New template: `templates/summary.tmpl`

**Input**:
- Entry type, layer, confidence, kind
- Entry description (full text)
- For each direct ref/closes/supersedes: its ID + its existing summary (or first sentence of description as fallback during migration)

**Output**: The summary string (1-2 sentences, ~50 words)

**Constraints in the template prompt**:
- Describe this entry and its direct relationships only
- Do not include transitive upstream context
- Use entry IDs as parenthetical handles when mentioning refs
- First sentence: what the entry is. Second sentence: how it relates to direct refs/closes.
- Max 2 sentences

## CLI changes

### `sdd new`

After creating the entry file in memory and before committing:
1. Read summaries of all direct refs/closes/supersedes entries from the in-memory graph
2. Run the summary prompt template via `claude -p`
3. Write the `summary` field into the entry's frontmatter
4. Commit the entry (with summary already present)

The summary is computed from the in-memory graph — no need for the file to exist on disk first.

### `sdd summarize`

New command:
- `sdd summarize <id>` — regenerate summary for a single entry
- `sdd summarize --all` — regenerate all summaries (migration and bulk maintenance)

Processes entries in DAG depth order (entries with no refs first, then entries whose refs are all summarized, etc.) so that ref summaries are available as input for downstream entries.

Uses the same prompt template as `sdd new`.

### `sdd list`

Shows truncated summary instead of truncated first sentence of description. This reduces the importance of the first-sentence pre-flight rule (though good first sentences are still good practice).

### `sdd show`

For the target entry: full content as today.

For refs/closes/supersedes entries: show summary + ID only (not recursive full-content expansion). Example:

```
refs:
  - d-prc-31v: "Standing contract: every bug fix requires a regression test covering the fixed path."
  - s-prc-okf: "Pre-flight over-applies regression test contract to template refinements, not just code bugs."
```

No recursive expansion beyond direct refs — each ref's summary already encodes its own direct relationships.

### `sdd status`

Uses summaries for all entries. Open entries include summary; active decisions include summary. No full-content rendering in status output.

## Skill changes

### `/sdd-catchup`

- Receives summaries for all entries (compact graph scan)
- Receives full details only for open signals and active decisions (the entries the user needs to reason about)
- Major token reduction: closed entries (majority of graph) contribute only their summary line

### `/sdd-explore`

- Target entry: full detail
- Upstream chain (refs): summary + ID per hop (flat list)
- Downstream entries: summary + ID per entry
- Related entries: summary + ID per entry
- The outer agent gets a compact briefing and can request full detail for specific entries if needed

## Staleness detection (deferred)

Not in first implementation. Future: store a hash of inputs (entry content hash + ref summary hashes) in frontmatter as `summary_hash`. `sdd lint` flags entries where the stored hash doesn't match the computed hash. `sdd summarize` regenerates flagged entries.

## Migration

Run `sdd summarize --all` against the existing 209 entries. DAG depth ordering ensures ref summaries exist before dependent entries are processed. No manual intervention needed — the prompt template handles all entry types uniformly.

Optional: a second pass after initial migration to improve quality, since first-pass entries near the roots used description fallbacks instead of summaries for their refs.

## Open questions

- Exact length constraint: "~50 words" as a guideline in the prompt, or a hard truncation?
- Should `sdd show` offer a `--full` flag to get the old recursive expansion when needed?
- Summary language: always English, or follow the entry's language?
