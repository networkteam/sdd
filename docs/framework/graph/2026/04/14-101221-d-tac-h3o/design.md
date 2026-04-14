# Pre-flight template alignment: dialogue drives graph evolution

## Principle

Validation checks whether entries faithfully capture dialogue reasoning, not whether they conform to structural ceremony. Dialogue is the primary mechanism through which the graph moves; the validator ensures the record is complete, not that the form is standard.

## Template changes

### 1. `supersedes.tmpl`

**Current check 3 — drop:**
> Type consistency: The superseding entry should be the same type as the superseded entry.

**Replace with:**
> Dialogue-driven replacement: The entry text must explain what happens to the superseded entry's concerns — covered, retired, relocated to tooling, or otherwise addressed. The structural shape (type, kind) of the new entry is the author's dialogue-driven choice. The validator checks that the reasoning for the replacement is present and that the superseded entry's concerns are accounted for, not that the form matches.

Checks 1 (full coverage) and 2 (no silent scope narrowing) remain — they serve the same principle (record completeness).

### 2. `decision_refs.tmpl`

**Current check 1 — narrow:**
> Ref completeness: Are there open signals in the graph that are clearly related to this decision but not referenced? If so, they may be missing refs.

**Replace with:**
> Ref completeness: Two checks: (a) Does this entry duplicate information from existing entries (signals, decisions) that it should reference instead? New entries should be minimal — reference existing information rather than restating it. (b) Does this entry logically build upon or advance existing entries without referencing them? If the entry wouldn't make sense without the context of another entry, or moves that line of thought further, it should ref that entry. Refs represent logical dependency ("builds on"), not topical association — entries that merely touch on the same topic without logical dependency do not need refs.

### 3. `closing_action.tmpl`

**Add durability section** (new check, per `d-prc-22i`):

> **Durability**: The artifacts the action references must be durable in the Git-tracked repository by the commit that records the action. Durability can arrive via any of: (a) a prior commit referenced by the action — for code or system changes; (b) attachments on this entry via `--attach` — the `sdd new` commit carries entry and attachments together; (c) attachments on a referenced upstream entry already in the graph — for split or multi-session knowledge capture. Accept any of these as evidence. If none apply, flag as a durability gap.

### 4. SKILL.md line 60

**Replace:**
> Before capturing an action: Ensure the work it describes is committed to Git first. An action records a fact of execution — the code changes it references must be durable before the action entry is. Commit implementation changes, then capture the action.

**With:**
> Before capturing an action: Ensure the artifacts it references are durable — either via a prior commit (code or system changes) or via `--attach` on the entry itself (research, synthesis, design docs; the `sdd new` commit carries entry and attachments together).

### 5. `entry_quality.tmpl`

**Extend the "Dialogue context is grounding" bullet** to cover structural departures:

Add after the existing text:
> This principle extends to structural choices. If the entry text argues for a non-standard structural move (e.g. superseding a contract with a directive for retirement, using a different kind than the entry it replaces), treat the argued reasoning as human-confirmed context. Check that the reasoning is present and coherent — do not reject solely because the structural form is non-standard.

## Not in scope

- `closing_decision.tmpl` — deferred to two-type system (`d-cpt-omm`)
- `contracts.tmpl` — tension is intended; contracts push back, which is good
- `signal_capture.tmpl` — working well (correctly caught prescriptive language in s-cpt-ix1 first attempt)
- `action_closes_signals.tmpl` — no issues surfaced
