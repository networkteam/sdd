# sdd search — design

## Rationale chain

The semantic-search gap was first identified at ~80 entries (s-tac-n2v) and deferred via an LLM-as-semantic-matcher workaround (s-tac-2y8) with the explicit condition "revisit when the graph grows by a magnitude." At ~399 entries (5x growth), friction is empirically visible:

- Agents grep for keywords and miss conceptually related entries when wording differs.
- The LLM-as-semantic-matcher workaround inside sub-skills is expensive and unreliable as content diversifies.
- Multi-participant work would sharpen the friction further: parallel sessions can capture signals on the same concept without sharing refs, with no current mechanism to detect silent divergences.

The reopened gap signal (s-tac-ixt) framed the proposal as "embeddings narrow, the graph's typed edges enrich" — search returns ranked top-N matches that the agent expands via existing graph traversal to surface ranked clusters. This plan operationalizes that framing.

## Alternatives considered

### Storage: in-frontmatter vs local index

**In-frontmatter (vectors as base64 in entry YAML, committed):**

- Pros: single source of truth, no parallel artifacts (aligns with d-stg-3k0), git-portable, branch reconciliation free, immutability matches embedding lifecycle, embed-once-per-graph cost shared across participants.
- Cons: frontmatter bulk; coordination cost (every participant must agree on the model); model-swap = whole-graph commit; capture-time API dependency; per-participant experimentation requires coordination.

**Local index (parallel, gitignored, per-participant):** *chosen.*

- Pros: per-participant model experimentation, opt-in to vector search, fast capture path, model-swap is a local rebuild, no frontmatter bulk.
- Cons: cold-start cost on clone or branch switch; reconciliation logic at search time; tension with d-stg-3k0 (parallel artifact).

The deciding factor was *opt-in*: the user can choose to enable or skip vector search, and individual participants can experiment with different embedding models without coordinating. Branch reconciliation resolves through entry-id intersection at search time + lazy-fill on misses — straightforward, no need to commit binary churn.

### Vector library: chromem-go vs coder/hnsw vs hand-rolled

**chromem-go** (`github.com/philippgille/chromem-go`, MPL-2.0): chosen.

- Pure Go, zero third-party dependencies.
- Chroma-shaped API — sensible standard if backends are ever swapped.
- Built-in metadata filtering and persistence (gob).
- Flat scan is adequate at our scale (≤25k × 1024 floats = 100MB raw, sub-100ms cosine sweep on a laptop).
- License: MPL-2.0 file-level copyleft is compatible with SDD's MIT distribution — modifications to chromem-go's own files stay MPL-2.0; SDD's code stays MIT.

**coder/hnsw** (CC0-1.0): credible alternative if scale ever moves past flat-scan territory. HNSW's complexity is unjustified at current and projected scale, and would require a DIY metadata layer.

**Hand-rolled flat scan**: credible third option (~150 lines), maximum control. Not chosen because chromem-go's built-in metadata filtering and Chroma-compatible surface outweigh the dependency cost at v1.

### Retrieval patterns from GraphRAG and adjacent work

**Borrowed:**

- *Local-search shape* (vector hit → graph expand) — the seed-and-expand pattern matches s-tac-ixt's framing. Skill-side expansion via existing `sdd show --max-depth`.
- *Edge-typed expansion* — refs / closes / supersedes get different weights at expansion time (skill-side, not CLI; CLI returns flat seeds with citations).
- *Hybrid retrieval via RRF* (k=60) — Cormack et al.'s parameter-light fusion outperforms weighted sums without per-corpus tuning.
- *Dual-level embedding* (summary + body sections) — matches the two-tier approach planned here.
- *Filter superseded-by from default seeds* — they pollute clusters with historical near-duplicates.

**Skipped:**

- *Community detection (Leiden) and community summaries* — SDD's typed graph already encodes structure; rediscovering it via clustering wastes work and risks contradicting the hand-curated structure.
- *Global search / map-reduce over summaries* — that's `sdd-catchup`'s job, and it draws on the typed structure directly.
- *LLM-driven entity & relation extraction* — entries are already typed and dialogue-shaped.
- *Multi-hop reasoning chains* (3+ hops with intermediate LLM calls) — the graph is small enough that 1–2 hop expansion plus skill-side judgment suffices.
- *HyDE* (hypothetical document embeddings) — risky at our scale where users often type specific terminology that already matches the corpus; could wash out exact-match signal. Worth a future flag for vague queries.

### Re-ranking: cross-encoder vs agent dialogue

A cross-encoder re-rank (BGE-reranker, Cohere Rerank, etc.) on the top-30 union before final results would tighten precision materially. Skipped at the CLI layer — the agent applies semantic judgment on returned results within explore/groom dialogue, which fits the dialogue-first commitment (d-stg-beb) better than another opaque ranking layer.

## Markdown chunking detail

### Strategy

- **Two-tier embedding**: summary as one vector per entry; description body split by `##`/`###` boundaries, leaf-scoped.
- **Boundary-respecting splits** beat fixed-size on structured docs — the explicit motivation behind LangChain's `MarkdownHeaderTextSplitter` and llamaindex's `MarkdownNodeParser`.
- **Subsection text excludes parent prose**: a chunk for `## Approach > ### Storage` contains only the storage section body. Parent intro becomes its own chunk.
- **Short sections** (< ~200 tokens): keep as own chunk; embeddings handle short text fine and the breadcrumb keeps it grounded. Avoid sibling-merging.
- **Long sections** (> ~800 tokens): recursive split with paragraph/sentence preference, ~10–15% overlap between sub-chunks of the same section, no overlap across heading boundaries.
- **Token budget**: target 300–600 per chunk, hard cap ~800.
- **Heading-only sections** (no body): skipped, not embedded as empty/title-only chunks.

### Breadcrumb representation

Stored as both:

- **Metadata**: array form `["Approach", "Storage"]` — clean for filtering and display.
- **Injected into chunk text**: prepended as a single context line (`Breadcrumb: Approach > Storage`) before the section body, then embedded.

The injection follows Anthropic's contextual-retrieval principle (Sept 2024) of prepending a brief context string to disambiguate. Heading breadcrumbs are the cheap deterministic version — no LLM needed to generate them.

### Frontmatter and metadata

Frontmatter values stay as filterable metadata, not embedded text. Mixing them in pollutes the semantic space; using as post-filters preserves it.

### Library: goldmark

`github.com/yuin/goldmark` is the recommended Go markdown library: CommonMark-compliant, clean AST via `ast.Walk`, heading nodes carry `Level` directly. Frontmatter via `goldmark-meta` after stripping the `---` block.

## Manual eval test outline

Build-tagged Go tests under `internal/finders/search_eval_test.go` (or an equivalent location). Skeleton:

```go
//go:build eval

package finders_test

func TestEval_OllamaRecall(t *testing.T) {
    // Configure Ollama embedder via env (SDD_TEST_OLLAMA_ENDPOINT, model)
    // Build index over a fixture or this repo's .sdd/graph
    // Iterate hand-curated eval pairs from a CSV / table
    // Assert each pair's related entry surfaces in top-N=10
    // Log per-mode recall, average rank, and divergence cases
}

func TestEval_OpenAIRecall(t *testing.T) {
    // Same shape; env: SDD_TEST_OPENAI_API_KEY, embedding model name
}
```

Manual run: `go test -tags=eval -run TestEval ./internal/finders/...`.

The eval set (`testdata/eval_pairs.csv` or similar) lists rows of `(source_entry_id, related_entry_id, expected_under_modes, note)` so the same dataset is comparable across providers and over time.

## Open implementation questions (deferred to implementation)

These don't change the plan's shape:

- **Snippet generation**: which spans to highlight in the citation (matched terms in text mode, the embedded chunk verbatim in vector mode, both in hybrid)? Default: matching span ±50 chars, with the breadcrumb prefixed.
- **AND-semantics across chunk boundaries**: does multi-term AND require all terms in the same chunk, or just the same entry? Default: same entry (chunk-level grep + per-entry roll-up).
- **Result count default**: top-10? Configurable via `--limit`?
- **Index format upgrade path**: chromem-go's gob persistence — versioned file with migration on schema change, or full rebuild? Default: full rebuild on schema change; the index is rebuildable.

## Out of scope (cross-reference)

See the plan body's "Out of scope" section for the explicit non-goals list.
