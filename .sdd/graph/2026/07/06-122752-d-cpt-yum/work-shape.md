# Work-shape on done signals — design note

## Decision
On done signals, inline `topics:` denote work-shape (the kind of work that concluded), reusing the topic mechanism. Subject-matter for a done is recovered from its refs. The evaluation lens rides this as `evaluation/<lens>[/specifics]`.

## Why reuse topics rather than a new field
Work-shape is hierarchical, multivalued, extensible, and wants stable canonical labels — which is exactly what the topic mechanism already is. Options weighed:

- **Dedicated `work:` frontmatter field** (reusing the topic-path machinery). Cleanest axis separation, but a parallel label mechanism duplicating topics for no real gain, plus a type-system surface change (frontmatter, view/show, derivation).
- **Ref-based sibling-linking** (relate the per-lens reviews via refs). More machinery than tagging; better suited to relating reviews than classifying work.
- **Topics-reuse on dones (chosen).** No new field; works with existing `sdd view`/filter today.

## The conflation objection, resolved
Reusing `topics:` looked like it would conflate "what it's about" with "what kind of work." It doesn't, because a done is semantically distinct: it records concluded work, and its subject is derivable from the entries its refs point at. The inline slot was never needed for subject on a done — so it is free for the work axis.

## Existing dones
Some existing dones carry inline subject/area topics under the old reading (e.g. s-tac-evy: `skill/evaluation, portability/mcp`). Left as-is under immutability; a future de-annotation method could clean them up. Not a blocker.

## Complementarity with d-cpt-5nn + the deferred edge
d-cpt-5nn governs *derived* done-topics — subject transferred from the `closes` edge. This convention governs *inline* done-topics (work-shape). Complementary. Deferred edge: a done that closes nothing (an evaluation-done builds-on the work it judges) has no `closes` edge, so its subject must be derived by **following the references of the appropriate kind** — for whenever done-subject derivation is built.

## The lens as a work-shape sub-path
The evaluation lens is a branch of the evaluation work-shape: `evaluation/inner` (verification — sound under the project's guidelines), `evaluation/outer` (validation — the right thing, works in use), with an optional specifics tail (`evaluation/outer/user-feedback`). No separate lens attribute. Coverage — "has this work been outer-evaluated, by anyone?" — is a component-prefix match over the dones referencing the work plus a ref walk; multi-participant and accumulating across the graph, rather than one both-lenses-at-once pass.

## Feeds Slice B
Both procedures realize this convention: evaluate proposes `evaluation/<lens>` on the evaluation-done; implementation proposes `implementation/<...>` on its closing done.