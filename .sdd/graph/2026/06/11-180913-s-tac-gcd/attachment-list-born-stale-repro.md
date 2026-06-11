# Attachment-list born-stale — reproduction

## Mechanism
- Create (`internal/command/new_entry.go`, `BuildEntry`): `entry.Attachments` = the `--attach`
  targets, joined as `<attachRel>/<target>`, **in CLI order**.
- The skip-hash = `ComputePromptHash(RenderSummaryPrompt(entry).Combined())`, and
  `formatEntryForPrompt` (`internal/llm/summary.go`) renders an `Attachments: <list>` line.
- Read (`internal/finders/graph.go`, `LoadGraph`): ignores any stored value and re-derives
  `entry.Attachments` by scanning the attachment directory — **every file, `os.ReadDir`
  (alphabetical) order**.
- When the two lists differ (order or set), the `Attachments:` line differs → create-time
  stored hash ≠ read-time recompute → "born stale" on first `sdd lint`, no edit involved.

## Reproduction matrix (real `BuildEntry` create vs real `LoadGraph` read)
| case                                   | result |
|----------------------------------------|--------|
| single file, dir matches               | MATCH  |
| single file, no body link              | MATCH  |
| multi-file, CLI order == alphabetical  | MATCH  |
| multi-file, CLI order != alphabetical  | STALE  |
| dir has a file not in `--attach`       | STALE  |

The resolved `{{attachments}}` body link is identical on both sides (resolved in `BuildEntry`
before hashing, never re-resolved on read) — it is NOT a contributor. Only the rendered
attachment LIST diverges.

## Scope & non-causes
- Affects only entries whose attachment-dir contents/order at read differ from the `--attach`
  args at create. Single-file / order-matching entries are unaffected.
- Version-independent: the summary-rendering code is unchanged across recent releases; ruled
  out a binary-version cause by rendering the same entry under multiple tags (byte-identical).

## Fix direction
Canonicalize the attachment input to the hash (e.g. sort + dedupe to match the dir scan on
both sides), or — preferred, unifying with the neighbor-prose facet — take the skip-hash over
stable, durable semantic inputs rather than the full rendered prompt.
