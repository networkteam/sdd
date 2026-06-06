# Pre-flight cache restructure — implementation record

Closes d-tac-yt1 (drop open-signal scan) and d-tac-fah (byte-stable universal preamble). Commit f7dfada.

## Before: the caching problem, measured

The stats sink (s-tac-bmw) logged 18 pre-flight/summarize calls. The pre-flight calls:

| time  | cache_create | cache_read | input |
|-------|--------------|------------|-------|
| 12:04 | 6163  | 0    | 9757  |
| 12:48 | 6674  | 0    | 8816  |
| 12:49 | 7607  | 0    | 8720  |
| 16:27 | 27417 | 0    | 32146 |
| 16:44 | 27417 | 0    | 28526 |
| 16:51 | 27417 | 0    | 29858 |
| 22:32 | 6292  | 0    | 10007 |
| 22:33 | 0     | 6292 | 10007 |
| 22:36 | 27417 | 0    | 32922 |

- Reuse was rare: 1 cache-read hit in 9 calls (the 22:32->22:33 pair — identical prefix within the 5-min TTL).
- The prefix was not byte-stable: create sizes varied (6163 / 6674 / 7607 / 6292), so call N+1 could not read call N's cache.
- The four 27417 creates are decision_refs captures embedding the full open-signal set — the largest volatile chunk.
- Net cost, not saving: ~130k cache-create tokens (billed ~1.25x) against ~6k read (~0.1x); the write premium fell mostly on caches that expired unread.
- Summaries never cache (under Anthropic's ~1024-token floor) — unchanged, out of scope.

## The fix

d-tac-yt1: dropped the open-signal completeness scan from decision_refs (OpenSignals context, template block, and field), retuned ref-completeness to judge against referenced entries only. Skill-side Widen->Inspect (s-prc-lud) covers the scan's old job. Removes the 27417-token volatile chunk.

d-tac-fah: one byte-stable universal_system preamble (role + entry_quality, unrelated_refs, durability, unusual_close, ref_meta_consistency + vocab, language, contracts, verdict) shared by the 10 substantive checks. Per-type task/rubric/calibration + the proposed entry moved to each check's _user block. annotation/focus keep a lighter bespoke system block (avoids unrelated_refs flagging membership refs, entry_quality nagging empty bodies). durability tightened to apply only to kind: done signals so it stays silent on the non-done checks now sharing the preamble.

Contracts in the preamble (they change far less often than the 5-min TTL) sorted by full ID in assembleContext — the byte-stability invariant, guarded by a render test asserting two different check types produce a byte-identical system prompt, stable across re-assembly. Byte-stability is proven by test; the realized cache-read win is measurable on future sequential captures (closes s-tac-osb).

## Eval validation (case-by-case, 23 cases)

The full eval can't finish in one `go test` run — 23 live claude calls exceed the 10-minute default timeout — so validated in chunks.

All calibration cases pass: 14 substantive non-ref-meta + 9 ref-meta (incl. RefMeta_RefinesActivePlan / RefMeta_BuildsOnActiveSharpened — the refines-vs-builds-on encoding).

Three issues surfaced and resolved:
1. DeviationWithReasoning regressed (passed on main, failed under fah). Durability gained salience in the shared preamble and correctly fired high on a fixture with no commit and no attachment — under-specified fixture; added an attachment (its FullCoverage sibling already has one). Not a fah defect: durability working better.
2. Two augmenting fixtures (CleanRefinement, TopicFilterReconstruction) failed on main too — stale: they used builds-on on active plans, which the ref-meta recalibration (s-prc-lsg) correctly wants as refines. Verified against ref-kinds.md (active + sharpened-in-place -> refines) and corrected. Pre-existing, traceable to s-prc-lsg's eval not covering these.
3. RefMeta_DescContradicts_High failed once on a transient JSON-parse error — the model emitted an unescaped quote inside an observation string while citing an excerpt. Judgment was correct; passed on re-run. Added a safe-quote instruction to the verdict output format. Mitigation only; parse-retry and tool-use structured output remain the durable guards.

## Open / follow-up

- s-tac-osb stays open until a future session's sequential captures show realized cache reads (the "after" measurement).
- Output-robustness: parse-retry (cheap) + tool-use structured output (durable) for the JSON fragility — noted, not built.
- s-prc-lsg left two stale eval fixtures (corrected here); its recalibration eval didn't cover the augmenting cases.
- Consider a longer -timeout on the eval target so the full suite runs in one pass.
