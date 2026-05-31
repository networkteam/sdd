# Topic system delivery (d-tac-6tz)

Closes the four-shape topic plan and the catch-up aggregation gap (s-cpt-sy4).
Merged to main via the `sdd/6tz-operationalize-topic-system-view` branch.

## Acceptance criteria — evidence

- **AC2 — CLI surface** (commit f12915b). `sdd view` gained `as-counts`
  (per-topic count + summed heat, ordered count→heat→label), `untagged`
  (entries with an empty effective topic set), and `id(ID,...)` (select listed
  entries; short IDs bare, full IDs quoted). `sdd search` gained
  `--max-citations 0` (entry headers only). cli-reference documents all four
  plus the previously-undocumented multi-ID `sdd show` form.
- **AC3 — annotation self-membership** (commit 21521a1). `Graph.EffectiveTopics`
  now treats an annotation's own declared labels as topics it carries. Verified
  on the real graph: `topic("catch-up-scaling")` returns the annotation
  s-cpt-vpc, and `sdd show` renders `Topics:` on it.
- **AC4 — label-stability policy** (commit cd086c2). framework-concepts gained
  the policy: labels are stable identifiers, reuse before creating, prefer
  hierarchical paths for families.
- **AC1 — capture-time procedure** (commit c7f3b16). The /sdd capture playback
  gained a topic-research step (as-counts → id() on refs → max-citations 0
  search) and a Topics line. Validated by a fresh-session paste-test: a cold
  agent autonomously researched labels, saw no CLI cluster existed, and
  proposed a new `ux/cli` label (captured as s-cpt-idq).

## Decisions made in dialogue

1. **`untagged` as a dedicated primitive** rather than `not(topic())` with no
   argument — reads cleanly, parses trivially. (Plan left this open.)
2. **`--max-citations 0` means literal zero** (commit 643537e). Reworked from a
   SuppressCitations bool to the integer carrying literal intent, default (3)
   resolved at the CLI via cmd.IsSet. Three search tests that relied on
   zero-means-default now set the cap explicitly.
3. **`as-counts` aggregation on `model.Graph.TopicCounts`** — pushed down from
   the finder per push-logic-down, since it is pure computation over model types.
4. **`id()` full IDs must be quoted** — the layout grammar parses leading-digit
   tokens as numbers, so `id("20260520-...")` is required; short IDs work bare.
   Consistent with the topic("...")/since("...") convention. Candidate
   follow-up: bare full-ID support via a grammar change.

## Out of scope

Topic backfill on existing entries was left to the downstream grooming
directive (d-tac-nbp).
