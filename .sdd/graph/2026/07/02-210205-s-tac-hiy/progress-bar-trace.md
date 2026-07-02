# Progress bar decoupled from indexing completion — code-path trace

## Symptom
`sdd index` (and `sdd search` lazy-fill): the interactive progress bar advances to 100%, then the process appears to hang before exiting. Confirmed by code trace — no actual deadlock; the stall is the real embedding/upsert/save time of the final (or only) batch, during which the bar already reads 100%.

## Root cause: the counter advances on batch start, before the work
- The bar is percentage-driven: `model.go` `progressMsg` → `progress.SetPercent(lastProg.Ratio())`; `Ratio() = Done/Total` clamped to [0,1] (`reporter.go:15-28`).
- The counter is moved only by `reporter.Add` (`reporter.go:69-75`).
- `sdd index` wiring (`cmd/sdd/search.go`): `reporter.SetTotal(total)` at `:209`; `OnBatchStart: reporter.Add(len(batchIDs))` at `:215`. Lazy-fill is identical: `:404` / `:410`.
- `OnEntryIndexed` is NOT wired to the reporter — nothing advances the bar on actual completion.
- `handler_index.go:234-242`: `onBatchStart(bucketIDs)` fires before the round-trip; the heavy work follows synchronously — `EmbedDocuments` (`:288`), `UpsertEntry` (`:317`), `manifest.Save` (`:196-212`).

## Single-batch amplifier
`indexEntries` packs entries into buckets of the embedder's `BatchSize` (`:187-214`). If every pending entry's chunks fit one bucket, there is a single `indexBucket` call → one `OnBatchStart` with all IDs → `Add(total)` at the very start → the bar jumps to 100% immediately, then sits for the entire single `EmbedDocuments` call.

## Teardown is correct (no deadlock)
- Quit fires on the LOG stream ending: `model.go:126-128` `logDoneMsg` → `tea.Quit`. `progressDoneMsg` is a no-op (`:138-139`) — the bar hitting 100% never quits by itself.
- `interactive.go:89-100`: the work goroutine calls `consumer.Close()` after `work()` returns, then sends to the buffered `outCh`; the main path runs `run(m)` then `<-outCh`, so it correctly waits for the real work to finish.

## Secondary: the total can be wrong
- `total` = `manifest.PendingCount` (entries absent or fingerprint-mismatched; `manifest.go:122-131`).
- But `indexEntries`' skip check also considers the content hash (`handler_index.go:161`: `state.Hash == hash && state.Fingerprint == fingerprint`).
- Divergence: a hash-failure entry is `continue`d without being added to work or counted as skipped (`:156-159`) → increments fall short → bar never reaches 100%. Conversely, if content changes under a matching fingerprint, work exceeds `PendingCount` → `Add` overshoots → `Ratio()` clamps to 1 early.

## Fix direction (from dialogue — not committed)
- Advance the bar on batch *completion*, not start.
- Show a footer label naming the entries being embedded in the in-flight batch (batch granularity; per-entry mid-call is not available without one-at-a-time embedding, which loses throughput).
- Reconcile the total with the hash-aware skip so it reflects the work actually performed.

_Line numbers as of the trace on 2026-07-02; verify against current source._
