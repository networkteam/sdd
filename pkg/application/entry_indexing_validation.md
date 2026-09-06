# Entry indexing validation

Evidence for decision `20260906-121218-d-tac-ccm`, collected on 2026-09-06 on an Apple M3 Max. The implementation extends checkpoint `0af4e1d7`. It is delivered on `feat/entry-indexing-batches`; no release, merge or push is part of this delivery.

The application and MCP composition tests exercise local synchronous preparation and an external consumer that schedules work and returns normally at its wait budget. They cover fixed home/dependency snapshots, concurrent writes, causal freshness, callback errors, lazy iterator errors, expired targets, incomplete zero-match and complete no-match results. Compiled Go examples document the same single callback.

Entry tests cover early stop, cancellation, source errors, stable cursor resume without earlier attachment reads, exact retained sources, changed hashes/configuration, concurrent retries, zero-chunk publication, full-hash identity and failed disk publication remaining invisible after reopen. Batcher tests cover oversized incremental admission, bounded dispatch, cross-caller vector distribution, payload and provider-unit limits, oldest-item window flushing, cancellation isolation, shutdown, provider failures, vector validation and routing/deadline composition.

The persistent interruption exercise uses 17 synthetic entries, including one larger than 32 chunks, eight entry workers, a four-item admission buffer and an injected failing entry. It kills a child process after two durable publications, starts a new process over the same disk index, then removes the failure and retries. Both tested concurrency settings preserved the first two entries, completed 14 healthy siblings on resume, published the remaining failed entry on the next attempt, and skipped all 17 on a final attempt. That final attempt made only the explicit query call, with no document embedding.

Real provider exercise, Ollama `snowflake-arctic-embed2:latest`, batch size 32 and window 10 ms:

| Document concurrency | Resumed indexing | Concurrent query | Provider calls on resume | Shared document batches |
| --- | ---: | ---: | ---: | ---: |
| 1 | 8.606 s | 691 ms | 42 | 16 |
| 2 | 6.433 s | 704 ms | 28 | 9 |

Each resume recorded 41,387 input tokens through the inner observation decorator. Peak simultaneous provider calls were two and three respectively, including the separately routed query. An earlier run measured 5.919 s / 84 ms at concurrency one and 4.894 s / 992 ms at concurrency two. These are individual local exercises, not latency distributions. Start conservatively at one document call for this provider and measure the consumer's actual workload before increasing it. All limits remain explicit and provisional.

Cost benchmarks over a 10,000-entry manifest measured approximately 30.45 ms and 14.8 MB allocated per repeated decode, versus 9.0 microseconds and 536 bytes per cached presence check. A behavior test verifies that 10,000 unchanged checks decode once and a new publication refreshes the cache. Cached nearest retrieval over 1,000 published and 1,000 unpublished rows measured approximately 339 microseconds and 24 KB per top-ten query. The published-only view is built once per loaded store snapshot; these timings exclude its initial construction. Manifest writes and full local-store reloads remain per-entry costs.

Reproduction:

```sh
devbox run test
go vet ./...
devbox run lint
go test -race ./pkg/application ./pkg/local ./pkg/llm/embed -run 'Test(Batcher|Search|LocalSearchPreparation|ExternalConsumerPreparation|Preparation|Entry|Discovery|Persistent)'
go test -count=1 -v ./pkg/application -run '^TestEntryIndexingInterruptionExercise$'
SDD_LIVE_EMBED_MODEL=snowflake-arctic-embed2:latest go test -count=1 -v ./pkg/application -run '^TestEntryIndexingInterruptionExercise$'
go test ./internal/index ./pkg/local -run '^$' -bench 'Benchmark(ManifestRead|PublishedRetrieval)' -benchmem
devbox run build
sdd view --layout 'rank(by(date)):n(3):brief:as-list'
```

The live command requires a running local Ollama service with that model. The default interruption test uses a deterministic local provider. Review the [adoption checklist](entry_indexing.md) before enabling asynchronous consumer preparation.
