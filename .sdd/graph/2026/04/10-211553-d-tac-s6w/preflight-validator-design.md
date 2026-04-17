# Pre-flight Validator Design

## Core Architecture

`sdd new` invokes `claude -p` (Claude Code programmatic mode) as an external validator before creating graph entries. The validator is a separate model instance with no session context — it sees only the proposed entry and relevant graph context. This implements the "proposal-execution split" pattern: the session agent proposes, an external agent validates.

## Why not Claude Code hooks?

Hooks are global — they fire on patterns across all tool calls, inject output into the conversation context, and need complex matchers to scope them to SDD operations. A validator inside `sdd new` is perfectly scoped: fires only on entry creation, has full graph context (the CLI already loads the graph via LoadGraph), and gates the entry without polluting the session.

## Why this works (research basis)

Research on agent process reliability identifies four control layers: prompt, tool, workflow, validation. SDD currently relies almost entirely on the prompt layer (SKILL.md instructions, playbook pre-flight checklists). The research finding: "prompts specify intent, the harness specifies law" — instructional guidance is soft because the model can always ignore or reinterpret it. Structural guidance is harder to evade because the agent cannot advance unless an external mechanism validates the required state transition.

Key principle: "Make the model responsible for generating candidates, not for certifying its own compliance." When the same model both acts and decides whether it followed process, drift is common. An external validator breaks this self-certification loop.

## Operation flow

1. Session agent + human dialogue → agree on an entry
2. Agent calls `sdd new a --closes <plan-id> "description"`
3. `sdd new` performs structural validation (refs exist, types match, IDs well-formed — already implemented)
4. `sdd new` selects the appropriate pre-flight check based on operation type and flags
5. `sdd new` invokes `claude -p` with: the proposed entry, referenced/closed/superseded entries (full text), active contracts, and the pre-flight prompt template
6. Validator returns structured result: pass/fail + list of specific gaps
7. **Fail** → entry creation is rejected, gaps printed to stderr. Session agent and human see the gaps, adjust, retry.
8. **Pass** → entry is created normally

## Pre-flight checks by operation type

### Action closing a plan/decision (`--closes` pointing at a decision/plan)

Validator receives: proposed action description, the decision/plan being closed, plan attachment items (if any), parent decision requirements.

Checks:
- Every plan item / decision requirement has corresponding coverage in the action description
- No silent omissions — gaps are listed explicitly
- No overclaiming — action doesn't claim to have done things not reflected in its description
- If plan items remain unaddressed: fail with "these items are not covered: [list]"

### Decision closing signals (`--closes` pointing at signals)

Validator receives: proposed decision, the signals being closed.

Checks:
- Each closed signal is genuinely addressed by the decision, not just referenced as context
- The decision doesn't introduce commitments not grounded in the referenced signals or current dialogue (silent additions)
- Layer is appropriate for the scope of commitment

### Decision with refs (no closes)

Validator receives: proposed decision, referenced entries, list of open signals in the graph.

Checks:
- Refs are complete — are there open signals in the graph clearly related to this decision but not referenced?
- Decision content is grounded in the referenced entries
- No undiscussed scope smuggled in

### Action with `--closes` on signals (direct closure)

Validator receives: proposed action, the signals being closed.

Checks:
- Evidence in the action actually supports closure
- No open sub-threads or unresolved tensions related to those signals
- Closure isn't premature

### Signal capture

Lighter validation — mostly classification checks.

Checks:
- Is this actually a signal (observation/fact) or does it smuggle an undiscussed decision?
- Is the layer appropriate? (e.g., a specific implementation detail shouldn't be strategic)
- Confidence level is honest given the content

### Supersedes operations

Validator receives: proposed entry, the entry being superseded (full text).

Checks:
- The new entry actually covers the ground of what it supersedes
- No silent scope narrowing — requirements or constraints from the superseded entry aren't dropped without acknowledgment

### Contract enforcement (cross-cutting)

For ALL operation types, the validator also checks:
- Active contracts (`kind: contract`) in the graph that might be relevant to the proposed entry
- If the proposed entry appears to violate a contract, fail with the specific contract and the apparent conflict

## Implementation details

### Pre-flight prompt templates

Templates are embedded into the `sdd` binary via `go:embed`. Each template is a focused prompt for one pre-flight scenario. Templates use placeholder variables filled by the CLI:

- `{{.ProposedEntry}}` — the entry being created (type, layer, description, refs, closes, etc.)
- `{{.ReferencedEntries}}` — full text of all entries in refs
- `{{.ClosedEntries}}` — full text of entries being closed
- `{{.SupersededEntries}}` — full text of entries being superseded
- `{{.ActiveContracts}}` — full text of all active contracts
- `{{.PlanItems}}` — plan attachment content (if closing a plan)
- `{{.ParentDecision}}` — the decision a plan implements (if applicable)

### Invocation

```go
// Pseudocode
cmd := exec.Command("claude", "-p", "--model", "haiku", preflightPrompt)
cmd.Stdin = strings.NewReader(contextPayload)
output, err := cmd.Output()
```

Model choice: Haiku for speed/cost (bounded task, structured input). Could be configurable via `--preflight-model` flag or graph config.

### Output format

The validator returns structured output (JSON or a simple pass/fail + gaps format):

```
PASS
```

or

```
FAIL
- Plan item "broken attachment references" has no coverage in the action description
- Active contract d-prc-xyz requires X, but this entry appears to Y
```

### CLI flags

- `--skip-preflight` — escape hatch for when the validator is wrong or unavailable. Creates the entry with a warning annotation in the frontmatter (`preflight: skipped`). Should be rare and visible.
- `--preflight-model <model>` — override the default model for validation

### Error handling

- If `claude` CLI is not available: warn and create entry (preflight is an enhancement, not a hard dependency for basic operation)
- If `claude -p` times out or errors: warn and create entry with `preflight: error` annotation
- If validation fails: reject entry, print gaps, exit non-zero

## Relationship to existing decisions

- **`d-prc-4du` (plan requirements)**: The pre-flight checklists defined there move from instructional text in the /sdd skill into structural enforcement via the validator. The checklists become the basis for the prompt templates.
- **`s-stg-3vr` (instructions unreliable)**: This is a direct response — moving critical checks from the prompt layer to the tool+validation layer.
- **`s-prc-868` (agent fails to evaluate)**: The validator catches evaluation failures that the session agent misses, because it's a separate instance doing a bounded task.

## Design principles

- **Localized, not global**: Fires only on `sdd new`, not on all tool calls
- **Graph-aware**: The CLI already loads the full graph — the validator gets rich context
- **Bounded task**: Each pre-flight is a specific, focused check — much more reliable than asking an agent mid-session to self-evaluate
- **Additive**: Structural validation doesn't replace dialogue — the human and session agent still discuss entries before capture. The validator is a safety net, not a substitute for the process.
- **Extensible**: New pre-flight checks are new embedded templates. No hook wiring, no global configuration.
