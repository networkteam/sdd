# Multilingual SDD — plan

## Direction

Per-graph configured language written to `.sdd/config.yaml` as a top-level `language` key. One value governs three things:

1. The language captured entries are authored in (graph storage)
2. The vocabulary the `/sdd` skill translates into for user-facing rendering
3. The default language the agent leans toward in conversation

Default: English when unset. Existing graphs unaffected.

## Why English-canonical storage tokens

The type-system contract (`d-cpt-ydf`) specifies exactly two entry types with explicit kinds (gap, fact, question, insight, done; directive, activity, plan, contract, aspiration) and layers (stg, cpt, tac, ops, prc). Mechanical validation — frontmatter parsing, kind-matching in pre-flight templates, CLI flag values (`--kind`, `sdd new d cpt`) — keys on these exact English tokens. Localizing the tokens themselves would fracture the contract and fork the CLI surface per locale, directly violating the agent-neutrality directive (`d-stg-574`).

English tokens stay in YAML frontmatter, in CLI invocation, and in the CLI's own output (status headers, list rows, show body). Translation happens one layer up, in the skill.

## Why skill-side translation (not CLI-level)

The `/sdd` skill is where dialogue happens. Translating there concentrates the localization surface in the place where user-facing rendering is generated (catch-up narration, playback, grooming tables, status summaries composed by the agent). The CLI stays the canonical, agent-neutral surface — its output can be read by any tool, skill, or participant without locale negotiation.

CLI-level translation was considered (`sdd status` rendering *Direktive* directly) and rejected for now: it doubles the translation surface, requires runtime i18n in the Go codebase, and offers limited additional value when the primary interaction happens through the skill. If users later browse the graph heavily outside `/sdd`, revisit.

## Why configured-language capture + free dialogue

Dialogue is where thinking happens; forcing participants into a single language breaks the dialogue-first aspiration (`d-stg-beb`) for mixed-language teams. But the graph is the durable asset, and a graph with entries in several languages fractures traversal, summarization, and catch-up coherence.

Resolution: dialogue is free, capture is canonical. The skill canonicalizes at capture time — dialogue content is translated into the configured language before `sdd new` runs. Participants converse as they prefer; the graph stays coherent.

## Mechanics

### Config

```yaml
# .sdd/config.yaml
language: de    # locale code; omit for English default
```

### Status surface

`sdd status` emits the configured language in its header, modeled on the existing `Local participant:` line:

```
Local participant: Christopher
Language: de
Graph: 347 entries (88 decisions, 259 signals)
```

Omit the line when unset (defaults stay invisible). The agent sees it on every session start without an extra call. `sdd meta` was considered as a separate config-exposure command but deferred as YAGNI — revisit when the header grows or additional agent-accessible config surfaces appear.

### Vocabulary references

Each supported locale gets a reference file:

```
internal/bundledskills/claude/sdd/references/vocabulary-<locale>.md
```

Contents (minimum): translations for the 2 types, 10 kinds, 5 layers, plus common terms used in rendering (signal, decision, gap, status, open, closed, superseded, etc.). German is the first case.

The skill instructs the agent to read the locale file on demand via the Read tool when rendering user-facing output in non-English sessions — not preloaded, not baked into SKILL.md. Keeps the skill bundle lean and localization additive.

### Pre-flight drift check

Parallel to the participant-drift check introduced in `s-tac-7ab`: an LLM-judged finding that inspects the language of the entry description against the configured language. Binary: matches or doesn't. Severity calibration follows participant drift — high enough to block when the mismatch is clear, with `--skip-preflight` available for edge cases.

## Alternatives considered

- **Full token localization** — translate kind/layer/type tokens in frontmatter and CLI flags. Rejected: breaks the type-system contract (`d-cpt-ydf`), forks the CLI surface, violates agent-neutrality (`d-stg-574`).
- **Session-level language** — agent infers from dialogue rather than config. Rejected: an empty `/sdd` invocation has no signal at session start; catch-up narration has to pick a language before the user speaks.
- **Dialogue-driven capture language** — entries use whichever language dialogue happened in. Rejected: fractures graph coherence across sessions and participants.
- **`sdd meta` command** for config exposure instead of status header. Deferred as YAGNI; revisit when agent-accessible config grows beyond one field.

## Out of scope

- Empirical skill robustness evaluation (strand 2 of s-cpt-y33) — requires the infrastructure from this plan to evaluate against. Capture as a followup evaluation when the smoke-test AC passes.
- Localization of CLI output itself (status headers, list rows, show body). English-only until demand surfaces.
- Participant-specific language preferences distinct from graph language (cf. `s-cpt-5jk`, `s-cpt-6ct`). Graph-level language is the first increment; per-participant localization is a separable later concern.
