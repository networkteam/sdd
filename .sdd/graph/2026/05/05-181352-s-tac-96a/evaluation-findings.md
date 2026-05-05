# sdd search — evaluation findings

## What shipped

`sdd search` with three retrieval modes:

- **Text** (`--term <regex>`, repeatable): live grep over `.sdd/graph/`, ranked by match count + position; multi-term AND.
- **Vector** (`--query <phrase>`): chromem-go cosine search over chunks; per-entry score = max chunk score with depth-aware adjustments.
- **Hybrid** (both flags): RRF (k=60) fusion of text and vector ranked lists.

Plus:

- **`sdd index`** — explicit warm-up command building the index from scratch. Bucket-by-batch with per-bucket manifest checkpointing for resumability.
- **`sdd search` lazy-fill** — entries on disk but missing/stale in the manifest are re-embedded before query.
- **`sdd lint`** — reports embedder fingerprint + entry count + drift count when an embedding provider is configured.
- **`Search:` capability line** in `sdd status` header.

## AC coverage

### Search shape and CLI surface (6/6)

- ✓ `--term` / `--query` / both = text / vector / hybrid; at least one required, no positional argument.
- ✓ `--term` repeatable, AND combine; OR via regex alternation.
- ✓ Citation: breadcrumb + ~150-char snippet per chunk; bumped to ~300-char with word-boundary trim and `[...]` truncation markers after eval feedback.
- ✓ `--type` / `--layer` / `--kind` filter flags compose, matching `sdd list` semantics.
- ✓ Default excludes `{status: superseded-by}`; `--include-superseded` overrides.
- ✓ `Search: text` / `Search: vector,text` in `sdd status` header.

### Embedding provider abstraction (4/4 functionally)

- ◐ Embedder interface: split into `EmbedDocuments` + `EmbedQueries` (not single `Embed`) per the instruction-template request — instruction-tuned encoders need different prefixes on indexed passages vs retrieval queries (Qwen3, E5, Nomic, BGE). Plus `BatchSize()` so the indexer packs work without a hidden constant.
- ✓ Embedder factory parallels chat-runner factory; rate.Limiter wraps remote providers with provider-specific RPS defaults.
- ◐ Adapters ship for OpenAI-compatible (`/v1/embeddings`) and Ollama (`/api/embed` — newer batch endpoint, not deprecated `/api/embeddings`).
- ✓ Independent `embedding.*` config keys in `.sdd/config.local.yaml`.

### Chunking and embedding (8/8 functionally)

- ✓ Summary as one chunk per entry, tagged `is_summary`.
- ◐ Body parsing uses vendored langchaingo `textsplitter` on `gitlab.com/golang-commonmark/markdown` (per d-tac-85r), not goldmark. Behavior matches: leaf-scoped, parent intro own chunk, h1–h6 splits per d-tac-jvd.
- ✓ Each chunk carries breadcrumb metadata (`[]string`) plus an `Entry: <summary>` + `Breadcrumb: <chain>` preamble prepended to embedded text per d-tac-jvd.
- ✓ Heading-only sections produce no chunk.
- ✓ Sections > 800 tokens recursively split with ~10% overlap; rune-based calibration (chunkSize=3200, overlap=320).
- ✓ Frontmatter values stripped at package entry, returned as `Frontmatter map[string]any` for the indexer's filterable metadata layer.
- ✓ Entries without `##` produce summary + single body chunk; `TestMarkdownTextSplitter_NoHeadings` pins this.
- ✓ Attachments chunked under parent entry's ID with `IsAttachment=true` and `SourceAttachmentPath` recorded.

### Indexing lifecycle (5.5/6)

- ✓ Index at `.sdd/index/` (gitignored), chromem-go persistent DB.
- ✓ Each row: entry_id, chunk_path, depth, content_hash, model_fingerprint, is_summary, is_attachment.
- ✓ `sdd index` warm-up; `--force` re-embeds regardless of manifest state.
- ✓ `sdd search` lazy-fills missing/stale entries before query.
- ✓ Branch reconciliation: `SearchFinder.candidates()` intersects index hits with the loaded graph; `TestE2E_BranchReconciliation` pins this.
- ◐ Model drift: tracked per-entry in `.sdd/index/manifest.json`, not per-row. `sdd lint` reports the count of entries indexed under a different fingerprint. Functionally equivalent — when an entry's fingerprint differs, lazy-fill re-embeds the whole entry's chunks.

### Retrieval and ranking (4/4)

- ✓ Text mode: grep, ranked by match count + position, multi-term AND, filter flags honored.
- ✓ Vector mode: cosine over chunks, max chunk score per entry, breadcrumb + snippet citation.
- ✓ Hybrid mode: RRF k=60 fusion; citation from whichever side ranked the entry higher; multi-term AND preserved on text arm.
- ✓ Depth-aware scoring: summary boost +10%, depth penalty up to -40% with floor.

### CQRS decomposition (5/6 — architectural deviation accepted)

- ✓ `internal/command/BuildIndexCmd` + `LazyFillIndexCmd`.
- ✓ `internal/query/SearchQuery` carries mode (derived from flags), filters, term/query, IncludeSuperseded, Limit, MaxCitationsPerEntry.
- ✓ `internal/handlers/IndexHandler` (Build + LazyFill) — only side-effecting code path.
- ✓ `internal/finders/SearchFinder` — pure reads.
- ◐ Pure types: Chunk lives in `internal/textsplitter/` (alongside the splitter that produces it); Citation/SearchResult in `internal/query/` (read-intent shape). AC literally said `internal/model/`. Reasonable architectural deviation accepted in dialogue.
- ✓ `internal/presenters/RenderSearch` with citation rendering.

### Skill bundle and apply (3/3)

- ✓ `internal/bundledskills/claude/sdd/references/search.md` covers when to use each mode, citation reading, instruction templates with known-good values for Qwen3 / Nomic / multilingual-e5 / OpenAI.
- ✓ Hooks in `SKILL.md` Explore Playbook briefing and Grooming Playbook (Pattern B) point at search; cli-reference.md updated.
- ✓ `.claude/skills/sdd/` regenerated via `./bin/sdd init --scope project` and committed alongside the bundled source.

### Smoke testing (3/3)

- ✓ Unit tests cover chunker (heading splits, leaf scoping, heading-only, oversized splits, attachments), scoring (depth penalty, summary boost, RRF), and finders (text grep with multi-term AND, vector lookup, hybrid).
- ✓ E2E tests with mocked embedder validate index build, lazy-fill, branch reconciliation, citation accuracy, mode dispatch.
- ✓ Build-tagged `search_eval_test.go` against real providers (Ollama + OpenAI) — excluded from CI, runnable on demand.

### Empirical evaluation (3/3)

- ✓ 12 hand-curated should-be-related entry pairs in `internal/finders/testdata/eval_pairs.csv` covering chunker chain, CQRS planning, modularization thread, augment-plan pattern, structural-separation contract, and the dialogue-first / parallel-coherence twin aspirations.
- ✓ Recall benchmark: `go test -tags=search_eval -run TestEvalRecall ./internal/finders/...` runs all pairs through every applicable mode; results logged with rank-of-related-entry per mode.
- ✓ Recall numbers folded into this document.

## Recall benchmark (Qwen3-Embedding 8b on local Ollama)

Run: `SDD_EVAL_PROVIDER=ollama SDD_EVAL_OLLAMA_MODEL=qwen3-embedding:8b go test -tags=search_eval -run TestEvalRecall -v ./internal/finders/...`

12 hand-curated should-be-related pairs from `internal/finders/testdata/eval_pairs.csv`. Index: 417 entries / 1822 chunks indexed in 12m27s. Test wall time: 752s (~13m, dominated by the index build).

**Recall@10 by mode**:

| Mode | Hits / Pairs | Recall |
|---|---|---|
| Vector | 10 / 12 | **83%** |
| Hybrid (RRF k=60) | 8 / 12 | 67% |
| Text (multi-term AND) | 1 / 3 | 33% |

Text mode covered only the three pairs whose `modes` column included `text` in the CSV — pairs with strong verbatim token overlap. The other nine pairs are conceptual links where the related entry uses different wording, so text mode wasn't expected to find them.

### Per-pair vector and hybrid ranks

| Source | Related | Vector rank | Hybrid rank |
|---|---|---|---|
| d-tac-lqr | d-tac-jvd | 3 | 5 |
| d-tac-lqr | d-tac-85r | 7 | **MISS** |
| d-tac-jvd | d-tac-85r | 2 | 3 |
| s-tac-ixt | d-tac-lqr | 3 | 3 |
| d-cpt-ah1 | d-cpt-l3s | 4 | 7 |
| s-prc-nxy | d-tac-ab1 | 5 | 1 |
| s-prc-gu6 | d-tac-ab1 | 8 | 4 |
| d-prc-9ti | d-tac-jvd | 3 | 5 |
| s-tac-n8u | s-prc-nxy | **MISS** | **MISS** |
| d-cpt-vt1 | d-tac-s6w | **MISS** | **MISS** |
| s-stg-3vr | d-cpt-vt1 | 5 | 3 |
| d-stg-beb | d-stg-qlt | 7 | **MISS** |

### Surprises

**Hybrid is weaker than pure vector on this corpus.** RRF rewards consensus across rankers — entries both rankers found rank higher. When text mode has very low recall (no shared verbatim tokens for most pairs), text contributes mostly noise: entries that share keywords but not concept get top text ranks, dragging the fusion away from vector's better picks.

Concrete: `d-tac-lqr → d-tac-85r` was at vector rank 7 (just inside top-10), but hybrid pushed it *out* of top-10 because text-mode noise outranked it on consensus. Same for `d-stg-beb → d-stg-qlt`: vector rank 7, hybrid MISS.

This is a known property of RRF when ranker quality is asymmetric. The implementation is correct; the finding is that hybrid as currently tuned doesn't pay off for this corpus and embedder. A follow-up signal could explore (a) RRF with confidence-weighted contributions, (b) hybrid fallback to pure vector when text recall is degenerate, or (c) accepting that hybrid is for users who already know the keyword AND want semantic breadth, not as a default.

**The two double-misses** (`s-tac-n8u → s-prc-nxy`, `d-cpt-vt1 → d-tac-s6w`) are the hardest pairs. The token-measurement signal feeding the modularization gap is a forward-looking link — Qwen3 doesn't pick up that "token cost makes modularization worth evaluating" semantically connects to "modularization is worth evaluating." The structural-separation contract → pre-flight validator pair is contract-to-implementation, abstract-to-concrete — also hard for an embedder. Both pairs are real graph relations the agent would benefit from seeing; future eval-set growth should include these as targets to track over time.

**Single-token text-mode hit landed top-1** (`s-prc-nxy → d-tac-ab1`, rank 1). When token overlap is strong and on-topic, text mode is the cheapest path to the right answer. The skill guidance correctly directs `--term` for token/ID lookup.

### Gap closure assessment

The friction claim that motivated `s-tac-ixt` was that grep-only search misses conceptual connections at 5x graph scale, and the LLM-as-semantic-matcher workaround is expensive. Vector mode at **83% recall@10** on hand-curated should-be-related pairs is a measurable improvement — for the 10/12 pairs where Qwen3 surfaces the related entry within top-10, the agent gets the right seed without invoking an LLM judgment pass. The 2 remaining pairs are at the harder end of the spectrum (forward-looking signal-to-gap, abstract-to-concrete contract-to-implementation) where future eval iterations will tell us whether better embedders, query templates, or graph-edge-aware re-ranking close the gap further.

## Deviations from spec

Three deviations, all dialogued and accepted:

1. **Embedder interface signature**: split into `EmbedDocuments` + `EmbedQueries` instead of single `Embed`, plus `BatchSize()`. Driven by instruction-template support — query-side and document-side templates need to attach asymmetrically per Qwen3 / E5 / Nomic conventions, so the call-site intent has to be explicit at the type level.

2. **Markdown library**: vendored langchaingo `textsplitter` on `gitlab.com/golang-commonmark/markdown` instead of `github.com/yuin/goldmark` (per d-tac-85r). The vendor brought worked-through edge-case handling for heading state, recursive overlap splits, and list/table/code-block rendering.

3. **CQRS pure-types location**: Chunk in `textsplitter/`, Citation/SearchResult in `query/`. The plan AC said `internal/model/`. Each placement keeps types adjacent to the package that produces them — Chunk is splitter-internal, Citation is read-intent shape. Architectural deviation accepted in dialogue.

## Eval-driven improvements (not in original ACs)

The implementation session ran real queries against the indexed graph. Five improvements landed because the output surfaced gaps the spec didn't anticipate:

1. **Bucket-by-batch index build.** First Build did one cross-entry `EmbedDocuments` call — for ~2000 chunks at ~1.5s each on local Qwen3 8b, that's a 50-minute "is anything happening?" wait with no progress visible. Bucket entries by `embedder.BatchSize()`, save manifest after every bucket, fire `onIndexed` per entry as buckets flush. Same throughput; visible progress; resumable on crash.

2. **Stable Ollama fingerprint.** Ollama dims are discovered from the first response; including dims in the fingerprint meant the value shifted across the first call, marking every prior row as drift on the next session. Drop dims from the Ollama fingerprint (model name uniquely determines dim for Ollama; no matryoshka knob like OpenAI). Regression test pins before/after-call equality.

3. **10× chunk oversampling for vector mode.** Top-K=20 was too tight when one large entry (d-tac-lqr alone owns 24 chunks) could dominate the chunk-level top hits, collapsing the per-entry rollup to 1–2 unique entries. 10× the requested limit (with a floor of 50) keeps the rollup pool diverse without making the chromem scan noticeably more expensive.

4. **Status-aware ranking.** Closed gap on output formatting outranked the open gap directly about output ordering for a query phrased about ordering — chunk-level vector similarity was comparable, and we didn't consider entry lifecycle. Apply a small score multiplier per derived status: open / active / done stay at 1.0, closed-by gets 0.85x, superseded 0.7x, cascade-orphan 0.6x. Hybrid mode picks up the change for free since RRF fuses already-status-adjusted ranks.

5. **Multi-citation per entry with per-citation relative scores.** Single-citation rollup hid the answer to "what did we choose as the vector storage?" — the chromem-go alternatives section in design.md was the actual answer but the summary chunk won the per-entry score competition. Emit up to 3 citations per entry (gated by `≥85% of top chunk` threshold and `MaxCitationsPerEntry` config), each with its own status-adjusted score, normalized to a relative percentage at render time. Default 100% / 91% / 87% style; cross-entry comparison works because percentages share a result-set scale.

## Closing the friction claim (s-tac-ixt)

The originating gap (s-tac-ixt) framed grep-only search as missing conceptual connections at 5x graph scale, and the LLM-as-semantic-matcher workaround as expensive. Manual queries during the eval ride confirmed both directions of the resolution:

- **Concept queries surface the right framework entries.** "Retrieval versus reasoning" surfaces the dialogue-first aspiration (d-stg-beb) and the positioning gap it closed (s-stg-gtu) without sharing exact tokens. "Graph honest with parallel agents" surfaces the parallel-coherence aspiration (d-stg-qlt). "Plan refinement during implementation" surfaces the augment-plan pattern (d-prc-9ti).

- **Hybrid mode reliably finds the right chunk in the right place.** "Vector storage for semantic search" with multi-citation surfaces the chromem-go alternatives section in design.md alongside the summary, with breadcrumb `Alternatives considered > Vector library: chromem-go vs coder/hnsw vs hand-rolled` — a precise pointer back to the choice and its rationale.

- **Status-aware ranking puts open work ahead of historical record** without filtering closed context entirely, so an agent reading results sees "what's still attention-worthy" first while still having access to "what we used to think."

The tooling cost replaces the per-query LLM-as-matcher cost with a one-time embed cost (~12 minutes for 417 entries on local Qwen3 8b, amortized over many queries thereafter). Per-participant index storage (~30MB chromem gob + 192KB manifest) trades coordination cost for free per-participant model experimentation.
