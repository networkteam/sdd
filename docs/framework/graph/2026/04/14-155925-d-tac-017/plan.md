# Depth-limited BFS for `sdd show`

## Overview

Replace unbounded recursive expansion in `sdd show` with BFS traversal that renders summaries at depth 1+. Applies independently to both upstream and downstream directions. Depends on summaries from d-tac-h93 being present in entry frontmatter.

## Traversal design

- **Depth 0** (target): full content (description, frontmatter, everything as today)
- **Depth 1+** (all directions): summary + ID + kind only
- BFS from target through refs/closes/supersedes (upstream) and refd-by/closed-by/superseded-by (downstream)
- Per-direction dedup: each entry shown at shallowest depth within its direction. Later encounters get a cross-reference marker (e.g. `(see above)`)
- Rare duplication across directions is accepted — upstream and downstream maintain separate visited sets
- Hard stop at `--max-depth` (default 4), independent per direction. Entries at the boundary with further refs get a truncation marker (e.g. `[3 more ancestors truncated]`)

## Output format

Each summary line follows this format:

```
{indent} - {relations} {full-id} ({kind}): "{summary}"
```

- **relations**: comma-joined edge types when multiple exist (e.g. `refs,closes`). Upstream: `refs`, `closes`, `supersedes`. Downstream: `refd-by`, `closed-by`, `superseded-by`.
- **full-id**: always the full entry ID (e.g. `20260414-152337-d-tac-h93`)
- **kind**: always shown in parentheses when present. Currently decisions only (directive/contract). Post two-type redesign (d-cpt-omm): every entry has a kind — no format change needed.
- **summary**: the entry's generated summary from frontmatter (~50 words)

### Example output

```
# 20260414-000402-s-ops-dd8

ID:     20260414-000402-s-ops-dd8
Type:   signal
Layer:  operational
Conf:   medium
Who:    Christopher Hlubek, Claude
Time:   2026-04-14 00:04:02

[full description content]

## upstream:
  - refs 20260414-152337-d-tac-h93 (directive): "Embed LLM-generated summaries in entry frontmatter..."
    - refs 20260414-152107-s-ops-d1c: "SDD session entry points consume excessive tokens..."
      - refs 20260414-000402-s-ops-dd8: (this entry)
    - refs 20260414-000446-s-prc-6hk: "/sdd-explore returns raw sdd show output..."
      - refs 20260407-214235-d-tac-vd8 (directive): "Build sdd-explore with --downstream flag..."
        - refs 20260407-213209-s-cpt-ez3: "Explore mode handles open entries through dialogue..."
          [2 more ancestors truncated]
      - refs 20260407-003418-d-ops-258 (directive): "Every open signal must appear in catchup output..."
    - closes 20260414-000402-s-ops-dd8: (this entry)

## downstream:
  - refd-by 20260414-152107-s-ops-d1c: "SDD session entry points consume excessive tokens..."
  - refd-by 20260414-000446-s-prc-6hk: "/sdd-explore returns raw sdd show output..."
```

## CLI changes

- Add `--max-depth N` flag to `sdd show` (default 4)
- Refactor current recursive renderer into BFS with depth tracking
- At depth 0: render as today (full content)
- At depth 1+: render summary line per the format above
- Upstream and downstream each run their own BFS with their own visited set
- `--downstream` flag continues to work, now also depth-limited

## Dependencies

- Requires entry summaries from d-tac-h93 to be populated in frontmatter
- Expects `Entry.Summary` to be present — no fallback for missing summaries
- Sequenced after `sdd summarize --all` migration

## No fan-out cap

With ~50-word summaries, even worst-case expansion at depth 4 (~30 deduped entries) stays under ~2000 tokens. No separate breadth limit needed.
