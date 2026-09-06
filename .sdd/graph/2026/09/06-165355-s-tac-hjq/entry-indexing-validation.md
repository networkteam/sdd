# Entry indexing validation

Evidence for decision `20260906-121218-d-tac-ccm`, collected on 2026-09-06 on an Apple M3 Max. The implementation extends checkpoint `0af4e1d7`. Implementation commits `0af4e1d7`, `07383c65` and `ad7cacbd` were reviewed in PR #9, merged into local `main` with normal merge commit `adbd49ba`, and pushed to origin. Existing history is preserved. No release was created.

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

The live command requires a running local Ollama service with that model. The default interruption test uses a deterministic local provider. The application package Go documentation and examples carry the external consumer adoption checklist.

## Review extension at ad7cacbd

The extension adds optional full-entry-ID selection to the same discovery path. Nil selects all entries; nonnil empty or malformed selections fail. Sorted, deduplicated selections bind continuation cursors alongside source revision and index configuration. Valid absent or ineligible entries yield nothing; selected unreadable documents, attachment reads and publication errors remain failures. Tests verify that resume and explicit selection avoid earlier/unselected attachment reads.

AppliedMutation.AffectedEntryIDs maps canonical document changes, deletions and attachment owners to unique IDs. It does not establish source durability. The consumer example queues discovery only after obtaining a reproducible finalized revision, which can differ from the workspace revision. An empty affected set queues no discovery job.

Pinned search reacquisition preserves the authorized branch. A multi-branch test covers preparation, publication coverage and retrieval on the selected source, while local adapter tests reject mismatched branch scope. Batcher validation now checks every input before admitting any item, retaining measured units; invalid trailing byte/unit inputs or measurement failures cause zero provider calls. Existing incremental admission, cancellation and shutdown tests continue to pass.

Validation passed for this commit's source tree: devbox run test including the nested example module; go vet ./...; devbox run lint with only the same 50 existing testpackage warnings; focused race tests across application/local/embed/finders; build and graph-view smoke test. The full suite includes the deterministic process-interruption exercise. The prior live provider measurements above remain the live evidence; no new universal default is inferred.

The package Markdown files have been removed. Consumer adoption guidance is in Go package/symbol documentation and compiled examples. This evidence accompanies the completion record.


## Acceptance evidence

| Plan criterion | Evidence |
| --- | --- |
| 1. Incremental discovery and interruption | Application discovery tests stop after one requirement, cancel iteration and propagate attachment failures. |
| 2. Stable continuation and replay identity | Whole-project and selected discovery resume without earlier attachment reads; selection order/duplicates normalize to one cursor scope and changed scope is rejected. |
| 3. Exact retained input and full version identity | Exact-source, hash/configuration mismatch and full-hash collision tests; fixed derivation schema participates in entry hashes. |
| 4. Atomic publication and retrieval, including zero chunks | Memory/disk publication tests and forced disk publication failure followed by reopen show unpublished rows remain invisible. |
| 5. Retry and concurrent convergence | Published versions skip source/embedding work; concurrent entry attempts converge and distinct versions remain independent. |
| 6. Sibling progress and entries spanning batches | Persistent process-interruption exercise retains published siblings, isolates an injected failure and finishes an entry larger than a batch. |
| 7. Bounded shared batching and flushing | Cross-caller vector routing, concurrency, byte/unit limits and oldest-item/tail-window tests. |
| 8. Incremental admission, cancellation and shutdown | Requests larger than the buffer complete; cancellation isolation and shutdown tests release callers. |
| 9. Usage attribution and query separation | Inner observation records provider calls with batch/caller attribution; document callers receive zero duplicated usage, and the exercise routes queries outside the document backlog. |
| 10. Fixed authorized search sources and causal freshness | Local/external composition tests, write-lineage freshness test and multi-branch regression cover preparation, coverage and retrieval on the selected snapshots. |
| 11. SDD-owned coverage and explicit failures | Incomplete zero-match and complete no-match tests, ordinary consumer wait expiry, callback errors and lazy source errors. |
| 12. One supported composition point | Application/MCP local and external-consumer behavior tests plus compiled SearchTarget examples. |
| 13. Existing routing and deadlines | Batcher composition tests use EmbedderFunc routing and inner Bounded rather than batcher-owned query or timeout policy. |
| 14. Checkpoint costs | Manifest-cache behavior tests and the manifest/retrieval benchmarks recorded above. |
| 15. Persistent interruption and realistic provider measurements | Process-kill/restart exercise and real Ollama measurements recorded above; all settings remain provisional. |
| 16. Local support, checks and adoption guidance | Root/example suites, race checks, vet, lint, build/view and PR CI passed; adoption guidance lives in application Go docs/examples. |

PR CI passed Ubuntu and macOS tests and lint for `ad7cacbd`. The merge tree equals that reviewed tree. The review extension additionally covers affected-entry ownership, finalized source durability, selection errors versus absent content, pinned-branch preservation and pre-admission input validation.
