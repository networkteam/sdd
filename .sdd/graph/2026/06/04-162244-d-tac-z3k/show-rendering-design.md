# sdd show rendering design

Operationalizes the CLI output policy (d-cpt-rkj) and the three-renderer refinement (d-cpt-5f4) for `sdd show`. Settled empirically by rendering a real entry's tree (the MCP plan d-tac-0v4) both ways and reading them as an agent consumer.

## What changes

The bespoke block — padded `ID:` labels, inline `{status: …}`, `→ refs (kind)` arrows, `[truncated: …]` — is replaced by:

- **Entry**: YAML frontmatter envelope (metadata, refs, closes, supersedes, attachments, derived status/topics) + raw markdown body. Mirrors on-disk storage; no escaping of the body.
- **Neighborhood**: a compact markdown tree (below).
- **Summary**: omitted on the primary by default (the body renders right after it); `--with-summary` brings it back for human drift-review.

## Tree form (chosen): compact markdown

One line per node, indentation = depth:

```
## upstream
- grounded-in 20260525-204836-d-cpt-t3j (directive, active) — Commits to Mastra as the SDD agent runtime, wired to React via AG-UI and the Go binary via MCP.
  desc: Mastra + AG-UI + MCP architecture this plan begins implementing
  - grounded-in 20260523-143653-d-stg-6za (directive, active) — Build a self-hostable, agent-agnostic web UI sandbox for SDD; dogfood the tech team first.
    - grounded-in 20260422-122523-d-stg-x0l (aspiration, active) — Reach non-developer participants directly via chat/voice/GUI.
      - grounded-in 20260419-121939-s-cpt-wiv (gap, closed) — Four candidate aspirations for SDD's direction; judged on gradient alignment, not binary.
        +11 more refs truncated (depth 4): s-cpt-7we, d-cpt-omm, s-cpt-5hn, …
    - addresses 20260407-215428-s-stg-qg0 (gap, open) — Existing projects need low-friction entry points without rebuilding all history.
```

- Line shape: `<ref-kind> <full-id> (<entry-kind>, <status>) — <first-sentence>`.
- `desc:` rides an indented sub-line when the ref carries one (edge-perspective; complements the node-perspective micro-summary).
- Truncation sits at the child indent level (where the unexpanded children would be), as `+N more refs truncated (depth N): <ids>` — not inline on the parent. Keeps "indent = depth" intact.

## Tree form (rejected): nested YAML

```yaml
upstream:
  - ref: grounded-in
    id: 20260525-204836-d-cpt-t3j
    kind: directive
    status: active
    summary: Commits to Mastra as the SDD agent runtime, wired to React via AG-UI and the Go binary via MCP.
    upstream:
      - ref: grounded-in
        id: 20260523-143653-d-stg-6za
        ...
```

Rejected because, for the same slice, it ran ~3–4× the lines (repeating `ref:/id:/kind:/status:/summary:` per node, worse as depth grows), forced a mode-switch back into YAML after the markdown body, and bought nothing for reading — parsing goes through `--format json`, not this view. Its only edge (parseability, plus a small-model comprehension bump) doesn't apply to a frontier reader consuming a reading view.

## Micro-summary at depth

Depth (>0) lines carry the entry's **first sentence**, derived at render time (no stored change — sidesteps any migration of immutable entries; revisable). Rationale: the relation-kind + `{status}` + `desc` + first sentence orient fully; full summaries rebuild the multi-KB wall. Full summary stays primary-only / `--with-summary`.

Known soft spot: a legacy entry with a weak first sentence **and** no `desc` (old `unknown` refs) can under-inform. Bounded — `--with-summary` and drilling cover it; render-time trim means we can revisit.

## Renderers and selection

One entry+tree data model (from finders) feeds two reading renderers (rendering isolated in presenters, per CQRS):

- **Styled (human, TTY)** — lipgloss: indent guides, relation-kind color, dimmed status, over the same structure.
- **Plain markdown (agent / pipe / `--format text`)** — the forms above.

Selection: TTY detection + `--format` + `NO_COLOR`, precedence flags > env > config > defaults. lipgloss is in-family (charmbracelet/bubbles already used by `sdd init`).

## Deferred

`--format json` for `sdd show` waits to share its schema with MCP `structuredContent` (d-tac-0v4), so the JSON contract is defined once rather than twice.
