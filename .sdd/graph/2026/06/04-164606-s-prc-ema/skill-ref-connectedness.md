# Skill refinement — ref/edge connectedness and body consistency

Commits: 4782778 (bundle source), bb58f9a (installed copy via sdd init)

## What changed

The "Refs matter" guidance in /sdd SKILL.md was rewritten from a narrow provenance opener to a principle plus a rule.

**Before:** "Always link to the signals/decisions that led to this entry."

**After — principle (connectedness):** connect the entry's reasoning to the graph. Wherever the body draws on, responds to, contrasts with, or leads to other entries, capture those as refs. What the connections are depends on the entry's type and scenario — not a fixed checklist (a done signal wires to what it closes; a plan to what it addresses and depends on; an annotation to its members; an actor signal maybe to little).

**After — rule (consistency):** the entry is self-contained — every entry-ID typed in the body is one of the entry's edges, and every edge is reflected in the body's narrative. No dangling prose mention; no edge the narrative doesn't support.

## Asymmetries and carve-outs

- **Direction is asymmetric.** body → edge is strict and mechanically checkable (any ID typed in the body must be an edge). edge → body is by relationship, not literal ID — a ref reflected as "the Hermes/Honcho analysis" without typing the ID still passes. The per-ref `desc` adds detail but does not substitute for the body showing the connection.
- **closes / supersedes are not refs.** They carry status effects, live in their own fields, are explained in the body as the retirement rationale, and are never repeated as a ref. So "edges" = refs plus closes plus supersedes.
- **Graph-mechanics carve-out (existing).** Universal contracts (immutability, canonical-only, ref semantics) named in passing stay unref'd.

## Why

Surfaced during the s-cpt-dnh capture this session: the body named an aspiration it serves and a drift-evidence signal in prose but did not ref them, and nothing in the skill caught it. A prose mention without an edge is invisible to graph traversal, heat/ranking, and the LLM ref-meta consistency check — the reasoning is there for a human re-reading the body but lost to everything else. Formalizing also forces naming the ref kind (the discipline the rubric already wants) and discourages gratuitous name-drops: if you mention an entry, you owe it a kind.

This is a text-instruction refinement; the LLM ref-meta consistency check remains the backstop. The body↔edge rule governs the entry description, not attachments (which may name IDs freely as supplementary record).
