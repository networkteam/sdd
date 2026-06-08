# Process rules: discovery, capture, and indexing — design synthesis

A followable write-up of how rule discovery could work, grounded in three research literatures (LLM agent memory, IR retrieval, pre-LLM rule systems) and our own dialogue. This is candidate design feeding the deferred follow-up plan, not a commitment — experimental parts are flagged, and we build to learn.

## The problem
We're accumulating conditional lessons — "when situation X holds, consider doing Y." We need the few that apply to surface at the moment they apply, without flooding the agent with low-relevance rules, which dilutes the instructions that matter. Plain vector search over the whole corpus degrades as it grows: similarity goes shallow and the relevant few get crowded out.

## Two halves: representation and activation
Every rule system — production engines, clinical decision support, policy-as-code — separates how a rule is *stored* from how it *fires*. We do too: the **rule entry** is representation; the **discovery move** is activation. Almost every choice belongs cleanly to one side.

## The rule entry (representation)
A `rule` decision kind with an `enforcement` attribute — `binding` (blocks) or `advisory` (surfaces) — folding today's `contract` in as `binding`. Each rule self-describes in three sections:
- **When to apply** — the activation: the situation that should trigger it.
- **What to consider** — the substance.
- **How to calibrate** — how to weigh it.

The activation section earns its structure: because our splitter chunks on headings, *the activation becomes its own embedding chunk*. That lets us match situation-against-situation (a query describing "what I'm doing now" against the activation section) instead of situation-against-whole-rule — much higher precision.

## Discovery (activation) — the corrected retrieval model
Three independent literatures converge on: never similarity alone — pair a broad recall pass with a precise ranking/sift pass. But we correct the naive "filter-by-structure-first" version, because a hard topic/kind/layer gate would drop applicable-but-mistagged rules, and our topics are imperfect.

- **Semantic search over the full corpus drives recall** (no exclusionary gate), matched against activation chunks. Lexical search adds exact-identifier recall (file names, `sdd view`, kind names).
- **Structure (topic/kind/layer) is a ranking boost, not a recall angle.** Unioning in "every rule with topic X" would just add noise — most topic-siblings won't apply. So structure reorders the semantically-found candidates; it doesn't widen the set. How much it helps is an empirical knob, and at our scale it may be minor — we tune it against the real corpus.

## The bootstrapping problem, and the activation index
The research never addresses this: the agent has to know *what to search for*. If it doesn't know which kinds of activations exist, it can't phrase the current situation in terms the search will match.

The candidate fix is an **auto-generated activation index** — a compact section, compressed from every rule's activation, regenerated whenever rules change, and injected into the skill so it's always loaded. It's a small taxonomy of "the situations that have rules," not the rules themselves. This is the "tiny always-loaded top-level index, details on demand" pattern the memory literature recommends (MemGPT core memory, H-MEM top-level routing, Claude Code's own `MEMORY.md`), and a concrete instance of the graph-resident-rules seam (s-cpt-k8i): the CLI emits a section the skill pulls in at session start — the same dynamic injection the catch-up sub-skill already uses (d-tac-k4l).

Two tiers result: an always-loaded *compressed activation index* (orientation — "here's the vocabulary of when rules fire") and *on-demand retrieval* of the full rules (detail).

## The division of labor
- The **outer agent** has full context *and* the activation index. It compresses the current situation into a short list of activations — the queries.
- The **sub-agent** takes those queries, searches activation chunks from each angle, reranks, and returns the top-n rules *with bodies*, absorbing the noise of poor candidates.
- The **outer agent** decides which to actually read and apply, with full context.

This dissolves "how much context does the sub-agent need" — almost none, because query-composition and final judgment both stay in the outer agent. The index is what makes the outer agent's query-composition possible in the first place.

## Usage, capture, consolidation — the lifecycle
- **Usage (positive only):** a new entry refs (`grounded-in`) the rules that shaped it. Each application is an incoming ref → heat. `rank(heat)` = recently useful, `rank(in-degree)` = most applied. That is the importance signal (Generative Agents) for free via existing heat. **Demotion is passive** — a rule that stops being applied stops earning refs and decays. No negative signals, no dismissal tracking.
- **Capture (provenance):** rules are born through the existing dialogue-first capture discipline, prompted at the evaluate/close juncture (the "after every completion" move): "did we learn something worth a rule?" — check for similar or supersedable rules, play back, capture. Rule-worthiness = recurring or verified lessons. It mirrors discovery: discover at plan/implement, capture at evaluate/close.
- **Consolidation:** dedup/merge/abstract is grooming pointed at the rule corpus, connecting to the topic-grooming direction (d-tac-dxa). Auto-clustering (RAPTOR/IVF) stays a later accelerator only if scale forces it — topics are already the curated buckets.

## Enforcement
Matching is uniform — semantic on the activation chunk. The binding/advisory tier is only the *consequence*: binding always-loaded and enforced (where contracts live today, eventually hooks), advisory retrieved and surfaced. Machine-checkable structural triggers (e.g. "diff adds a `.sdd/` path") survive only as a later optimization for the few binding rules where they're cheap — not the v1 mechanism, and not what the tier selects.

## Grounded vs. experimental
- **Grounded (exists today):** hybrid search (text/vector/RRF), topics, heat/decay, typed refs, sub-agents, capture discipline, CLI-driven skill injection.
- **Experimental (we learn by building):** the auto-generated activation index (regeneration trigger + compression prompt), how precise activation-chunk matching is in practice, whether structure adds much in ranking, and the exact outer/sub context split.

## Candidate v1 build
The `rule` kind + enforcement + activation section; the activation-index generation and injection; the discovery move in the playbooks (index-guided query composition → sub-agent search/rerank → outer-agent judgment); the ref-on-apply convention; and capture-rules prompts at evaluate/close. Most rides on primitives we already have — the genuinely new build would be the activation index and the discovery/capture wiring.

## Sources
Agent-memory systems: Generative Agents (memory stream + reflection); MemGPT/Letta (core/recall/archival tiers); A-MEM, arxiv 2502.12110 (interconnected memory notes); H-MEM, arxiv 2507.22925 (hierarchical index, ~O(log N) routing); MIRIX, arxiv 2507.07957 (multi-type memory incl. procedural); MemoryBank / Mem0 (consolidation, decay); NirDiamant/Agent_Memory_Techniques (survey).
IR / retrieval: hybrid sparse+dense (BM25, SPLADE) + RRF; cross-encoder reranking; HyDE / query decomposition; RAPTOR, arxiv 2401.18059 (recursive cluster+summarize tree); GraphRAG, arxiv 2404.16130 + microsoft.github.io/graphrag; IVF coarse-to-fine ANN; metadata pre-filtering.
Pre-LLM rule systems: production rules / RETE (Drools docs); case-based reasoning (sciencedirect CBR topics); clinical decision support + alert fatigue (jmir.org/2020/10/e22013, mq.edu.au CDSS-alert-reduction); faceted classification / controlled vocabularies / ontologies; recommender & event-trigger systems; policy-as-code OPA/Rego (openpolicyagent.org/docs/policy-language).
