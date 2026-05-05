# `kind: annotation` signal + topic system — design synthesis

## The gap

The existing 6 signal kinds (gap, fact, question, insight, done, actor) represent things noticed about the world or the project. None represents a structural annotation about the graph itself — grouping entries into topics, assigning labels, recording cross-cutting relationships.

Using `kind: insight` for topic clustering was considered but rejected: insights appear in the catch-up narrative as actionable items. Structural annotations should be invisible to the catch-up — present only for querying and rendering.

## Proposed: `kind: annotation`

A 7th signal kind — purely mechanical, excluded from catch-up narrative items. Never appears as a numbered item in the catch-up. Queryable for clustering, ranking, and rendering.

### Frontmatter: typed edge bundles

Each annotation carries a bundle of typed edges. Each edge has its own type and payload; multiple edge types can coexist in one bundle:

```yaml
edges:
  - type: topic
    label: "catch-up/performance"
    members: [20260504-100323-s-cpt-8tu, 20260505-153647-d-tac-kv5, 20260505-153654-d-tac-qom]
  - type: topic
    label: "CLI/UX"
    members: [20260428-125737-d-tac-kud, 20260505-163757-d-tac-07q, 20260505-162047-d-tac-y9q]
```

Current known edge type: `topic`. Future types (e.g. `priority`, `external-reference`) can be added without schema changes. The body carries narrative: why this grouping was made, what the topic means, what session produced it.

## Two-level topic assignment

**Primary path (capture time):** Entries carry an inline `topics:` field in frontmatter, assigned at creation. The agent suggests matching labels from existing annotations in the graph. No extra entry needed — zero ceremony for the common case.

```yaml
topics: ["catch-up/performance", "CLI/UX"]
```

**Secondary path (post-capture):** Annotation signals handle entries that predate the topic system, bulk reorganizations, and corrections. One annotation signal can assign topics across many entries at once.

The in-memory model builds the topic index as a union of both sources.

## Hierarchical topics via path labels

Labels follow a path convention: `"UX/CLI"` implies membership in `"UX"` by prefix. The renderer does a prefix match — no extra entries needed for parent topics. Inspired by s-cpt-s43's Zettelkasten hierarchical tags proposal.

## Label stability

Topic labels are stable identifiers, not display names — same principle as canonical participant names. Once used, a label is permanent. To restructure: add new annotation signals with correct labels, supersede old ones. Active cluster membership migrates forward; historical entries retain old labels. This avoids the "rename all backlinks" mess of tools like Logseq.

## Edge mutation: additive union model

- **Adding members**: new annotation signal with same label — no supersession needed
- **Removing members**: supersede the specific annotation signal that added them

Topic membership = union of all non-superseded annotation signals sharing that label + inline `topics:` fields on entries. No negative edges needed; no ordering dependencies.

## Connection to lightweight catch-up

With annotation signals providing pre-computed topic clusters and in-degree + heat-decay providing ranking (s-cpt-rwd), the lightweight catch-up can select and present top-N entries under topic headers using verbatim summaries — no synthesis required.

## Type system impact

Extends signal kinds from 6 to 7, touching the type system contract (d-cpt-ygn). Requires a conceptual decision before implementation. Companion to the `kind: focus` decision proposal (s-cpt-ke6).
