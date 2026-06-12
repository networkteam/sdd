# Decouple the summary-skip hash from the rendered prompt — design

## Problem (two facets, one root)
The summary-skip hash is `ComputePromptHash(RenderSummaryPrompt(entry).Combined())` — a hash of the
entire LLM prompt. That prompt embeds inputs that are not stable across the create→read boundary, so
entries drift stale with no edit:

- **Neighbor-prose coupling (s-tac-pfn).** RelatedEntries embeds the *summary text* of every
  refs/closes/supersedes target. Any change to a neighbor's prose — LLM re-summarize OR a manual
  `sdd summarize --text` correction — invalidates the hash of every entry pointing at it. The edited
  neighbor's own hash stays clean, so the breakage is silent. Confirmed in the wild via a `closes`
  edge whose target's summary was hand-corrected.
- **Attachment-list born-stale (s-tac-gcd).** `formatEntryForPrompt` renders an `Attachments:` line.
  `BuildEntry` composes that list from `--attach` targets in CLI order; `LoadGraph` re-derives it by
  scanning the attachment directory (every file, alphabetical `os.ReadDir` order). When order or set
  diverges, the line differs and the entry is born stale on first read. Reproduced in a create-vs-load
  matrix.

Root: hashing the rendered prompt couples skip-detection to rendering details that aren't durable.

## Fix
Compute the skip-hash over a **canonical, durable semantic basis**, not the rendered prompt:

- Entry's own durable fields: body (resolved, persisted), type, layer, kind, refs (id + kind + desc),
  closes, supersedes, confidence.
- Neighbors: **identity only** (id, optionally kind) — never their summary prose.
- Exclude: neighbor prose, the rendered attachment list, derived status, and any prompt formatting.

The LLM prompt is untouched — neighbor summaries and attachment context still flow into generation
(quality unchanged). Only what we hash for skip-detection changes.

### Open design choices (decide in execution)
- **Attachments:** drop from the hash basis entirely, or include a *canonical* (sorted, deduped)
  attachment set so create and read agree. Dropping is simpler; canonical-set preserves
  "attachments changed → re-summarize" behaviour. Leaning drop, since the list isn't semantically
  part of the summary's skip signal.
- **Basis construction:** a dedicated `summaryHashBasis(entry, graph)` serializer vs. reusing a
  trimmed render path. Prefer the explicit serializer — decoupled from prompt formatting so future
  prompt tweaks never drift hashes again.

## Migration — accept-either-basis (honors no-forced-re-summarization)
- Staleness = stored hash matches **neither** the legacy rendered-prompt basis **nor** the new
  semantic basis. Matching either ⇒ current.
- New and regenerated summaries store the new-basis hash.
- Consequence: a settled graph regenerates nothing on upgrade; entries migrate to the new basis only
  when re-summarized for a real reason. No `--force`, no mass re-summarize, no history rewrite.
- Once repos have drifted over (observable via lint), remove the legacy-basis computation.

## Verification
- Pinned regression tests for both facets: a neighbor `--text` edit leaves referrers' hashes stable;
  the create-vs-load attachment matrix is all MATCH.
- `go vet`, `go test -race`, `golangci-lint` clean.
