# Spec: catch-up "open-loops" lane (`coldness` rank)

Grounding: insight `20260625-115201-s-cpt-6h6` (two-questions / inverse-heat reframe + design note `design.md`), gap `20260625-115123-s-prc-6cw` (the blind spot it resolves).

## 1. Goals & Requirements

The catch-up briefing answers only one of the two questions a returning reader has. Heat (Σ over incoming refs) answers **"what is the project converging on."** It is structurally blind to **"what have I committed to but not yet acted on"** — an unacted commitment has, by definition, no incoming refs, so it scores ~0 and falls off the result cap. A freshly captured plan is invisible at exactly the moment it most needs to be seen.

Build a second catch-up data lane that surfaces unacted-on commitments, ranked by the inverse of heat.

Requirements:

- A new `sdd view` rank algorithm `coldness(e) = decay(age_of_entry) / (1 + in_degree(e))`, where decay is applied to the **entry's own creation age** (not, as in heat, to each incoming ref's age).
- A new injected data block in the catch-up template that fetches the coldest active commitments, carrying each entry's **upstream** refs so the briefing agent can thread it.
- Sub-skill guidance so open-loop entries are **woven into the existing story-arc threads** (marked as unattended/new), not rendered as a flat block — and a genuinely disconnected one becomes a short "new direction" beat.
- Documentation surfaces (`sdd view` help text, `cli-reference` skill doc) updated in the same change — this is the cross-surface change pattern the untraced-impact gap (`20260607-125045-s-prc-guf`) flags; the spec lists every surface so none drift.

Out of scope (deferred, named so they are explicit):

- **Participant scoping ("mine only").** v1 shows everyone's open loops (decision #5).
- **The "delta" lane** (what changed since I last looked, via a seen-watermark). A distinct third concern requiring session-state SDD does not keep today.
- **Cross-section dedup.** Sections render independently today; an entry appearing in two source blocks is reconciled by the agent at threading time (decision #8), consistent with current catch-up behavior.
- **Excluding `intent: guiding` directives.** A no-op today (the attribute does not exist yet); becomes required when directive-intent (`20260610-000354-d-tac-n9k`) ships. Carried as a future interaction on decision #4, not v1 work.

## 2. Architecture & Design Decisions

### 1. New `coldness` rank algorithm in the model layer

**Decision.** Add a scoring function to `internal/model/ranking.go` alongside the existing algorithm functions:

```go
// ColdnessScore ranks unacted-on entries: high when recent and un-referenced,
// pushed down by each incoming ref (the hand-off to heat) and faded by entry age.
func ColdnessScore(g *Graph, e *Entry, decay DecayFunc, now time.Time) float64 {
    if decay == nil {
        return 0
    }
    entryAgeDays := now.Sub(e.Time).Hours() / 24
    return decay(entryAgeDays) / (1 + InDegreeScore(g, e))
}
```

**Reasoning.** Mirrors the existing `HeatScore(g, e, decay, now)` signature (`internal/model/ranking.go:17`) and reuses `InDegreeScore` (`ranking.go:36`) and the `DecayFunc` contract (`internal/model/decay.go:11`, `type DecayFunc func(ageDays float64) float64`). Entry age is computed the same way heat computes ref-source age (`now.Sub(t).Hours()/24`, `ranking.go:27`), so units match the decay functions exactly. The `1/(1+in_degree)` form makes the hand-off **gradual** — every incoming ref pushes the score down (one ref halves it, two thirds it), which is the intended "each ref demotes it toward hot," not a hard cutoff at the first ref.

### 2. Decay applied to entry age, reusing the existing decay registry

**Decision.** `coldness` accepts the same optional decay argument as `heat` (`coldness(exp-30d)`, bare `coldness`), resolved through the existing `model.DecayByName` (`decay.go:26`). No new decay function is added.

**Reasoning.** The available decays (`exp-7d/14d/30d`, `linear-*`, `none`) already cover the chosen half-life (decision #3). The only conceptual difference from heat is the age *source* (entry vs ref); the decay vocabulary is identical, so it should be shared rather than duplicated.

### 3. Coldness default decay = `exp-30d` (its own default, slower than heat's)

**Decision.** Introduce `model.DefaultColdnessDecayName = "exp-30d"` and use it when `coldness` is called with no decay argument — distinct from `model.DefaultDecayName` (`"exp-14d"`, `decay.go:49`) used by heat/mult/add/log. The catch-up lane passes `coldness(exp-30d)` explicitly regardless.

**Reasoning.** "What is hot lately" is a recent-weeks question (heat → 14d); "what have I left undone" should fade slowly ("SDD is not a todo list"). Defaulting bare `coldness` to the fast 14d default would contradict its purpose. `exp-30d` already exists, so no decay-registry change is needed (the slower `exp-60d` option was considered and declined for v1 to avoid a new knob; revisit if 30d fades too fast in practice).

### 4. Lane kind set: plan, activity, directive, gap, question

**Decision.** The catch-up open-loops lane filters to `kind(plan,activity,directive,gap,question)`.

**Reasoning.** These are the action-carrying / concern-raising kinds — commitments (plan, activity, directive), unattended concerns (gap), and open questions. Observational kinds (fact, insight) and standing kinds (aspiration, contract, focus, plus structural actor/role/annotation and terminal done) are excluded — they are not commitments you act on. Including `directive` also resolves **dimension one** of the grounding gap (`s-prc-6cw`): fresh directives currently appear in *no* catch-up lane (the "active and hot" lane is `kind(plan,activity)`, "open and warm" is `kind(gap,question,insight)`). Coldness surfaces them by un-acted-ness, complementing — not conflicting with — the directive-intent pending/guiding split (`20260610-000354-d-tac-n9k`), which keys on a stored attribute rather than ranking.

**Future interaction — directive intent (`20260610-000354-d-tac-n9k`).** When the directive-`intent` attribute ships, `intent: guiding` directives are standing context, not unacted commitments — surfacing them as open loops would be the same error as including aspirations/contracts. At that point this lane must add `not(intent(guiding))` to its layout (keeping pending *and* unspecified directives, excluding only guiding), the identical exclusion `d-tac-n9k`'s AC 5 applies to the "Active and hot" lane. Today this is a **no-op**: no `intent` attribute exists yet, so every directive reads as unspecified and is correctly included — the v1 lane is correct as-specified. The coupling is the risk, not the filter: `d-tac-n9k`'s AC 5 enumerates the *existing* catch-up lanes and predates this one, so the intent implementation will not name "Open loops". The lane is placed adjacent to "Active and hot" (decision #6) so the same `not(intent(guiding))` is applied to both, and this interaction is recorded here because it is exactly the untraced-impact pattern (`20260607-125045-s-prc-guf`).

### 5. Everyone's open loops (no participant scoping) in v1

**Decision.** The lane applies no `participant()` filter.

**Reasoning.** No existing catch-up lane is author-scoped, and coldness (absence of incoming refs) is author-agnostic. The layout strings in the template are static, and the filter grammar has no local-participant token, so "mine only" would require new machinery (a `participant(self)` resolver or template-time substitution of the local participant) — deferred. In a solo+AI graph the distinction is near-empty anyway.

### 6. Catch-up lane: layout string and placement

**Decision.** Add a new injected block in `internal/bundledskills/templates/sdd-catchup/SKILL.md.tmpl` immediately after the "Active and hot" block (line 167), so the two ranking lanes (hot, cold) sit together:

```
{{ inject `sdd view --layout='kind(plan,activity,directive,gap,question):active:rank(coldness(exp-30d)):n(8):expand(refs):name("Open loops"):as-list'` }}
```

**Reasoning.** Mirrors the existing "Active and hot" injection shape (`SKILL.md.tmpl:167`) and uses the `{{ inject }}` helper (`internal/bundledskills/bundledskills.go:93`), which renders `!`cmd`` for Claude and an English "Run … and use its output" instruction for Codex. `active` excludes closed/superseded (and terminal entries like settled directives) — `internal/finders` `active` filter. `n(8)` matches the focused cap of "Active and hot" rather than the wider `n(15)` of "Open and warm." `name("Open loops")` labels the fetched data block; it does **not** imply a rendered section (decision #8).

### 7. `expand(refs)` (unfiltered) to carry upstream for threading

**Decision.** The lane uses `expand(refs)` — all outgoing refs — not `expand(refs(inactive))`.

**Reasoning.** A freshly captured entry has no *incoming* refs (that is why heat misses it) but almost always has *outgoing* refs — what it builds on / addresses / grounds in. Those are its thread. `expand(refs)` renders each as a `→ <kind> <full-id> {status} : "desc"` sub-line (`internal/finders/view.go`, verified against live `sdd view` output), giving the briefing agent the upstream — kind, target, status, and the per-ref desc — needed to weave the entry into the arc its upstream belongs to. The `(inactive)` narrowing used by "Active and hot" is the wrong filter here: we want the *whole* upstream, not only the closed parts.

### 8. Rendering: woven into threads, not a flat block

**Decision.** Open-loop entries are not rendered as their own "Open loops" section. The sub-skill prose instructs the agent to:

1. Weave each open-loop entry into the story-arc thread its **upstream refs** belong to (the `→` sub-lines), marked as unattended / not-yet-acted-on (e.g. "you captured this, still open").
2. Treat a genuinely **disconnected** open-loop entry (no upstream, or upstream not present in any other block) as a **new line of work** — its own short thread beat ("a new direction opened"), which is high-value to show.
3. Reconcile overlap: an entry already threaded from "Active and hot" or "Open and warm" is threaded once, not duplicated.

**Reasoning.** The briefing's identity is story-arc clustering, not kind/source grouping (the catch-up format rules explicitly forbid re-clustering by kind/layer; source is analogous). A flat "Open loops" block would recreate that flatness and scatter one storyline across two lists. This is the presentation conclusion reached in the grounding insight's design note: the lane is a **source guarantee** (the entry is in the candidate set), not a rendered section.

### 9. Documentation surfaces updated in the same change

**Decision.** `coldness` is documented wherever the rank vocabulary is listed, in the same change set:

- `cmd/sdd/view.go` `viewHelpText` algorithms table (around `view.go:66–78`).
- `internal/bundledskills/templates/sdd/references/cli-reference.md.tmpl` algorithms table (lines 99–106).

**Reasoning.** A graph/CLI feature shipped without its doc surface is the exact untraced-impact pattern of `s-prc-guf` (and the help-drift incident `20260619-172919-s-tac-vil`). Listing both surfaces in the spec keeps them from drifting.

## 3. Implementation Changes

### Model (`internal/model/`)

- **`ranking.go`** — add `ColdnessScore(g *Graph, e *Entry, decay DecayFunc, now time.Time) float64` (decision #1).
- **`decay.go`** — add `const DefaultColdnessDecayName = "exp-30d"` near `DefaultDecayName` (decision #3).

### Finders (`internal/finders/ranking.go`)

- Add `"coldness"` to `knownAlgorithms` (line 24).
- In `parseAlgorithm` (lines 58–103): add a `case "coldness":` handling the optional decay arg exactly like `heat/mult/add/log`, except defaulting to `model.DefaultColdnessDecayName` when no arg is given.
- In `applyRanking` (lines 158–206): add `case "coldness": scores[i] = model.ColdnessScore(g, e, decay, now)`.
- Confirm the section-header auto-derive renders `coldness(<decay>)` for bare `rank(coldness)` (the "Top by &lt;algorithm&gt;" composer). It reads the rank spec generically, so likely no change is needed — the header test row pins the expectation; if the composer special-cases algorithm names, extend it to cover `coldness`.

### CLI (`cmd/sdd/view.go`)

- Add a `coldness(decay)` row to the `viewHelpText` algorithms table with formula `decay(entry age) / (1 + in-degree)` and a one-line note that it is heat's inverse (default `exp-30d`).

### Skill templates (`internal/bundledskills/templates/`)

- **`sdd-catchup/SKILL.md.tmpl`** — add the injected "Open loops" block after line 167 (decision #6); add a short note line analogous to the existing sub-line note (line 169) explaining the `→` upstream sub-lines; add the threading-guidance prose (decision #8) in the briefing-composition section (the prose body, lines 10–158), and reflect the new lane in the "Fetched data" description so the agent knows it is a fourth narrative source.
- **`sdd/references/cli-reference.md.tmpl`** — add the `coldness(decay)` row to the algorithms table (after line 106) and a worked-example line for the open-loops lane near the other `sdd view` examples.

### Build / install (process, no source change)

After template/code edits: `devbox run build`, then `sdd init --scope project` to re-render and re-stamp the installed `.claude/skills/` and `.agents/skills/` copies (auto-committed). Run `devbox run validate-skills` to confirm the Codex render still validates.

## 4. Test Cases

### Model — `internal/model/ranking_test.go` (new `TestColdnessScore`)

Follow the existing fixture pattern: fixed clock `fixedNow` (`ranking_test.go:12`), `daysAgo(d)` (`:16`), `minimalEntry(id, time)` (`:22`), `refsOf(ids...)` (`:47`), compare with `epsilon`.

| Test | Setup | Expected |
|---|---|---|
| Fresh, unacted | entry age 0, in-degree 0, `exp-30d` | `1.0` (decay(0)=1, /(1+0)) |
| Acted once | entry age 0, in-degree 1 | `0.5` (decay(0)=1, /(1+1)) — the gradual hand-off |
| Acted twice | entry age 0, in-degree 2 | `≈0.333` |
| Aged, unacted | entry age = 30d, in-degree 0, `exp-30d` | `0.5` (decay at one half-life) |
| Ancient, unacted | entry age = 120d, in-degree 0, `exp-30d` | `≈0.0625` — faded toward the horizon |
| Nil decay | any | `0` (guard, matches `HeatScore`) |

### Finders — `internal/finders/view_test.go` (new `TestView_RankColdness`)

Follow `TestView_RankHeatDefaultDecay` (`:737`) / `TestView_RankHeatExplicitDecay` (`:760`): build a graph, `f.View(query.ViewQuery{Graph: g, Layout: layout})`, assert order and `{score}` on the `model.FlatList`.

| Test | Layout | Expected ordering |
|---|---|---|
| Fresh beats aged | `rank(coldness(exp-30d)):as-list` over a fresh 0-ref entry + an aged 0-ref entry | fresh first |
| Unacted beats acted | same, two same-age entries, one with an incoming ref | the un-referenced one first |
| Default decay | `rank(coldness):as-list` | scores computed with `exp-30d` (assert a known value), confirming the coldness-specific default |
| Header auto-derive | `rank(coldness):as-list` (no `name()`) | section header reads `Top by coldness (exp-30d)` — confirms the rank→header composer renders the new algorithm name with its default-decay suffix (resolution rule #4, "No prefix, rank set → 'Top by &lt;algorithm&gt;'") |
| Explicit decay parses | `rank(coldness(exp-7d)):as-list` | parses and ranks (no error) |
| Unknown decay errors | `rank(coldness(exp-99d)):as-list` | parse error citing the rank/decay (matches existing decay-validation behavior) |
| Composes with filters | `kind(plan,activity,directive,gap,question):active:rank(coldness(exp-30d)):n(8):expand(refs):as-list` | the catch-up lane runs end to end; expand(refs) sub-lines render |

### No decay test change

`exp-30d` already exists in `decay.go`; `decay_test.go` needs no change (the `exp-60d` option was declined, decision #3).
