# Inner evaluation: derived base-fact index implementation (s-tac-31o)

Anchor: 20260721-152753-s-tac-31o — commits 60adf47, b7a31d8, c5718d6.
Scope: inner lens (verification) only. Outer lens (fresh-agent discovery benchmark) deliberately uncovered — prescribed as its own run anchored on the closing done.

## Method

Four independent Opus review agents, each with a distinct perspective, plus an independent verification run; disagreements between agents resolved by direct code inspection before synthesis.

1. **Architecture & public API** — CQRS layering (d-cpt-ah1/AGENTS.md), push-logic-down, single-path, fail-loud, logging; local/remote split soundness of the application/mcpapp boundary.
2. **Plan fidelity** — each acceptance criterion of d-tac-ptn against actual code, with the binary rebuilt and smokes re-run (`view --help`, `active:indexed:as-list`, `show` of the indexed fact); design contract of d-cpt-qhn.
3. **Adversarial bug hunt** — line-by-line read of the three diffs, scratchpad repros, `go test -race` on engine/model/finders (clean).
4. **Testing strategy & complexity** — test pyramid, brittleness, missing edges; proportionality of the store hardening and the viewlayout refactor.

Independent run: `go vet ./...` clean, `go test ./...` all pass, `golangci-lint run ./...` 0 issues (fresh run in the worktree).

## Verdict

Sound with qualifications: faithful to the plan and architecturally disciplined; two confirmed medium-severity defects, one over-engineering hotspot, one latent API-boundary leak, minor test gaps.

## Plan fidelity (agent 2)

All 10 in-scope ACs **MET** (AC11, fresh-agent outer evaluation, deferred by design). Highlights:

- AC1–AC2: nested `index:{title,topic}` round-trips (`fact_index.go` UnmarshalYAML/MarshalYAML), fact-only + trimmed + topic-membership validation centralized in `Validate`/`ValidateForEntry`; all four input paths (YAML, engine capture, `EntryDraft`, `sdd new --index` with `DisallowUnknownFields`) funnel to it; `show` exposes the block verbatim.
- AC3: `Graph.IndexedFacts` gates on `DerivedStatus`, excludes dependency graphs (deps stay `MultiGraph` members, never merged into local entries), no supersession inheritance, deterministic topic → case-folded title → full-ID sort.
- AC4: `indexed` parses zero-arg only, rejects `source(wip)`, applied as a pure structural filter; smoke `active:indexed:as-list` returns the view-layout fact.
- AC5: `factIndex` inject renders `- {ID} — {Title}` guarded by `{{if .factIndex}}`; no hard-coded fact IDs in procedure text.
- AC6–AC8: exact topic/title enrollment; host-neutral Markdown body (no CLI/MCP names, no "debugging" framing); terminal help and Markdown fact render from one `buildReference` model over the live vocabulary, both covering `indexed`.
- Design contract (d-cpt-qhn): enrollment by presence, derivation after active/supersession resolution, no special rendering snuck into view/catch-up/skills (grep-confirmed).
- Deviations (benign): title hardening beyond spec (control chars, U+2028/9 rejected; `topicRaw` preserved for unparseable topics); `StatusActive` branch in `IndexedFacts` unreachable for facts; commit c5718d6 bundles engine-wide atomic-state work beyond the plan's slices; b7a31d8 bundles the (clean, orthogonal) viewlayout refactor.

## Confirmed findings

### F1 (medium, confirmed by agents 1 & 3 independently; severity verified by synthesizer): one malformed indexed fact prevents session opening

Load treats a malformed `index` (e.g. `index.topic` ∉ `topics`) as a soft `Warning` and admits the entry (`graph.go` `validateFactIndex`); `Graph.IndexedFacts` (fact_index.go:138) re-runs `ValidateForEntry` and hard-fails the whole read on the first offender. `factIndex` is an inject on the user-dialogue `junction` (opening) step (d-prc-dlg.md:23), and inject errors abort the render (instance.go renderUnit → serve → Start) — verified directly: the opening serve fails, so the session cannot open. Exposure: hand-edited project entries (write paths already hard-reject; base facts are build-validated). Also breaks AC4's population-parity promise on such graphs: `FilterIndexed` (structural, `Index != nil`) includes what `IndexedFacts` errors on.

**Agreed remediation: validity is part of enrollment.** One rule everywhere — a fact is indexed only if its `index` block is valid. Both `FilterIndexed` and `IndexedFacts` select on valid enrollment (shared `HasValidIndex()`); malformed = load warning + quiet exclusion, never an error, never inclusion; hard rejection stays at write paths. Restores population parity, keeps opening robust, single strictness policy.

### F2 (medium, confirmed with repro): `cloneStoreValue` panics on `time.Time`

store.go:445-457/494-512: the unexported-mutable-field guard trips on `time.Time`'s `loc *time.Location`. Repro: `cloneStoreValue(time.Now())` and `map[string]any{"ts": time.Now()}` both panic. Contradicts the function's own JSON-persistable contract (time.Time marshals fine). Latent — no store value carries a timestamp today — but the panic sits on hot paths (`Get`, `TemplateContext`, `Collected`, `Export`, `commit`, `Clone`, `WriteEngine`) with no recover: the first op that stores a timestamp crashes the engine on the next read/commit.

### F3 (medium, over-engineering): ~150 of the hardening commit's ~270 store.go lines are speculative

`StoreValueCloner` extension point (zero production implementers), concrete-type-preservation panics, unexported-field reflection, cycle detection, five test-only types and their tests — defending value shapes that cannot occur (production store values are entirely JSON-shaped: scalars, `map[string]any`, `[]any`, flat two-string structs). Plus: `commit` deep-clones the whole candidate a second time on every write, and reads deep-clone on every access. The **atomicity half is correct and proportionate** and was verified behaviorally (rejected batches leave store and log byte-identical; the `Start()` reorder fixes a real half-update; `Answer`'s transactional collect-validate preserves semantics).

**Agreed remediation: normalize at the boundary.** Keep transact/validate-all-then-commit. Store contract becomes "values are JSON documents": writes clone by `json.Marshal`→`Unmarshal` (~5 lines) at the write sites (`writeState`/`setParams`, matching `WriteEngine`); `commit` becomes a plain swap. Self-enforcing (non-serializable fails loudly at write time), normalizes values to their persisted form so pre/post-restart behavior cannot diverge, and dissolves F2 (time.Time → RFC3339 string, same as after any restart; `asConfirmation` already handles the map form). Delete the cloner interface, reflection guards, panics, and test-only types.

### F4 (low): `factIndex` query leaks a model type through the application boundary

workflow_registry.go:60 returns `[]model.FactIndexRow` whose `Topic` is an untagged `model.TopicPath` (`{Components []string}` — would serialize as capitalized `Components`, not the `"cli/view"` string form). Latent: templates touch only `.ID`/`.Title` today. Every other public-API surface uses application-owned DTOs. Fix: map to an application DTO, or give `TopicPath` MarshalJSON/YAML emitting the joined string. The one unclean spot in an otherwise transport-agnostic change (base facts route through the existing `BaseEntries`/`MergeEmbedded` disk-wins path; `ctx.Graph` stays behind the `engine.Graphs` provider).

### F5 (low): test gaps and prose coupling

- `IndexedFacts` checks status *before* validating — malformed index on an inactive fact is silently skipped; no test pins this ordering (a refactor swapping it would hard-fail on retired malformed facts unseen).
- The intentional structural-vs-validated divergence between `FilterIndexed` and `IndexedFacts` has no test documenting it — a well-meaning "consistency fix" to either side passes CI. (Subsumed by F1's remediation, which removes the divergence.)
- Exact error prose ("control or line-separator") asserted in five files across model/engine/application/mcpapp — every wording tweak is a five-file edit; upper layers should assert rejection, not phrasing.
- `Answer`'s optional-collect-field commit path unexercised; `topicRaw` round-trip branch untested.

## Cleared under adversarial scrutiny

Store transaction atomicity and clone symmetry; supersession/override selection (non-fact/non-indexed successors, override with/without index); ordering comparator (fold-class canonical, total-order tiebreak); YAML/JSON boundary convergence (`!!str` enforcement rejects `title: 2026`, duplicate-key and unknown-key rejection, adjacent frontmatter preserved via the `plain` alias); `indexed` filter mechanics; `Start()`/`Answer()` atomic-update reorders; parent→child seeding alias-severing. Race tests clean. Logging, push-logic-down, and CQRS placement conform throughout; `IndexedFacts` and `FilterIndexed` live in `internal/model` with thin delegations above.

## Test strategy judgment

Sound and behaviorally anchored at the right layers — the invariant tests compare `Export()`/`StateSnapshot()` before/after rejected batches rather than restating implementation; model-layer negative coverage is thorough (lifecycle exclusion, no-inheritance, value isolation, YAML rejections, Unicode titles). Golden assertions are substring-based (contained brittleness), with two tight spots: mcpapp pins the full base-fact title line verbatim; one viewlayout test recomputes expectations from the renderer's own model builder (mild tautology).
