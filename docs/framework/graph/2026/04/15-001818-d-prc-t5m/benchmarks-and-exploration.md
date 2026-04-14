# Catch-up Performance: Benchmarks and Exploration

## Problem

Session startup catch-up takes ~2 minutes despite summary and depth-limited show optimizations landing the same day. Target: ≤30s.

## Root cause investigation

**Initial hypothesis**: I/O or per-entry content size.
**Finding**: Tool calls finish fast (~24s for all CLI calls in a full catch-up). The bottleneck is model synthesis — the LLM inference pass that produces the narrative briefing.

## Benchmark: entry count vs. synthesis time (Sonnet)

Single-pass `claude --model sonnet -p` with the catch-up prompt and varying slices of `sdd status` output. All entries use full descriptions from the actual status output.

| Variant | Entries | Input words | Wall-clock |
|---------|---------|-------------|------------|
| Small slice | 5 | 540 | **27s** |
| Medium slice | 12 | 968 | **47s** |
| Large slice | 24 | 1737 | **1m48s** |
| Full status | 33 | 2256 | **1m51s** |

**Key observation**: 24 and 33 entries take nearly the same time (~1m50s), suggesting an output-bound plateau. The model writes ~1800 words of narrative regardless of input size beyond ~20 entries.

## Approaches explored and rejected

### 1. Tags / pre-clustering (inline tags on entries)

Added `[tags: topic1, topic2]` to every entry in the full status output to eliminate thread-inference work.

| Variant | Entries | Wall-clock |
|---------|---------|------------|
| Full (no tags) | 33 | 1m51s |
| Full (with tags) | 33 | **2m08s** |

**Result**: Slower. More input tokens, same output work. Thread inference is not the bottleneck.

### 2. Snapshot + delta (cached narrative + new entries)

Used a prior catch-up result as a "snapshot" and asked the model to integrate 3 new entries.

| Variant | New entries | Input words | Wall-clock |
|---------|-------------|-------------|------------|
| Snapshot + 3 delta | 3 | 2166 | **2m23s** |

**Result**: Worst of all — slower than full synthesis from scratch. The model reads the entire snapshot AND reasons about what changed AND rewrites the full narrative. More work, not less.

### 3. Tiered catch-up (fast default + full on demand)

Conceptually sound — only synthesize recent/active entries by default. But:
- 5 entries at 27s hits the target, yet is too few for a useful narrative
- 12 entries at 47s exceeds the 30s target
- The "fast" tier would need aggressive filtering that risks missing important context

**Result**: Deferred. The threshold between "useful" and "fast" is too narrow.

### 4. Caching strategies

Multiple caching approaches discussed:
- **Narrative cache with timestamp**: Only useful when nothing changed between sessions. Cache invalidates on every `sdd new` call during sessions (5+ per session).
- **Cache as CLI-generated side effect**: Deterministic, but adds cost to every `sdd new`.
- **Narrative + entry ID footer for diffing**: Adds metadata overhead; entries are immutable so "status changes" are actually new entries with closes/supersedes — simpler than expected but still requires re-synthesis.

**Result**: Deferred. Caching only helps the narrow case of "nothing happened since last session."

### 5. Pre-clustered threads file (committed metadata)

A `threads.yaml` mapping entry IDs to named threads, maintained through dialogue. Eliminates thread inference, enables CLI-level grouping.

**Result**: Rejected after tag benchmark showed thread inference isn't the bottleneck. Also adds curation overhead — another file to maintain.

## Chosen approach

1. **Switch catch-up sub-skill to Haiku** — faster inference, acceptable quality
2. **Reduce sub-skill fetch scope** — fewer `sdd show` calls for detailed entries
3. **Regular grooming** — keep open entry count manageable
4. **Accept ~1m for full catch-up** — the output-bound constraint means sub-30s requires cutting narrative quality, which isn't worth it

## Haiku timing

Full catch-up with Haiku and leaner fetching: ~1m for the sub-skill (vs ~1m40s+ with Sonnet).
