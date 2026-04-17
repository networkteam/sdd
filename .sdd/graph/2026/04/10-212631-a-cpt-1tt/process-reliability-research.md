# Process Reliability Research for AI Agents

Synthesized findings from three research prompts on getting AI agents to reliably follow structured processes, with specific focus on Claude Code's extension model (skills, hooks, MCP).

## Research questions

1. Structural enforcement mechanisms vs instructional approaches in Claude Code's extension model
2. The instruction-following gap in agentic coding tools — causes and mitigations
3. Process enforcement patterns for human-AI collaborative development

## Core finding

All three prompts converge on the same conclusion from different angles: **external enforcement dramatically outperforms instructional guidance for agent process compliance.**

> "Prompts specify intent, the harness specifies law." — The harness should own sequencing, permissioning, validation, retries, termination, and audit logs; the LLM should mainly fill in bounded decisions inside that scaffold.

## Why agents fail to follow process

- **Training mismatch.** Models are trained to continue text plausibly, not to execute durable policies across long agent loops. They sound compliant because sounding compliant is what continuation training rewards. Actually being compliant across 50 tool calls is a different capability.
- **No persistent control state.** "Stop and reflect" is not a native capability; it must be induced by prompts, memory, or controller logic. It drifts or gets skipped when the task gets complex.
- **Self-assessment bias.** Models over-trust their own outputs. Internal critique misses its own mistakes unless an external signal is present. The agent that acts cannot reliably certify its own compliance.
- **Compounding error.** Once an agent takes a wrong step, later steps amplify it. Three evaluation failures in one session aren't independent failures — they're a cascade. Catching the first deviation is disproportionately valuable.
- **Decomposed requirement failures.** Simple pass/fail hides partial compliance. Models satisfy 80% of instructions while silently dropping specific items (evaluation steps, scope checking, meta-cognitive behaviors). Decomposed metrics reveal these hidden failures.

## Four control layers

A robust agent stack separates four layers instead of relying on one "be careful" system prompt:

| Layer | What it enforces | Mechanism | SDD mapping |
|---|---|---|---|
| **Prompt** | Style, role, heuristics | System prompt, SKILL.md | /sdd skill instructions, playbooks |
| **Tool** | What actions are possible, with what constraints | Schemas, wrappers, policy checks | `sdd new` with pre-flight validator |
| **Workflow** | What order is legal | State machine, graph, supervisors | Graph semantics (types, refs, closes, contracts), WIP markers, plan requirements |
| **Validation** | Whether output/action is acceptable | Rule checks, tests, LLM judge, human approval | Pre-flight validator (claude -p), human confirmation |

Meta-cognitive habits are weak at the prompt layer but strong at the workflow and validation layers. "Reflect after every step" works poorly as text, works well as a mandatory callback before the next state transition.

## The three-layer enforcement model

From the human-AI collaboration research, the cleanest framing:

- **Hooks/tools enforce rules** — structural checks, policy gates, pre-flight validation
- **The graph enforces process** — entry types, ref chains, closes/supersedes semantics, contracts as standing constraints
- **Humans enforce authority** — dialogue before capture, strategic decisions, override capability

## Key patterns

### Generation-validation separation

The highest-leverage pattern. The model that generates must not be the model that validates. A separate instance with no task investment, no context pressure, doing a bounded specific check, reliably catches what the session agent misses.

### Completion as gated action

Don't let the model "just finish." Make completion a privileged transition requiring: requirement coverage report, evidence list, validator pass, no unresolved blockers. This one change eliminates a large fraction of silent requirement dropping.

### Requirement ledger

Extract every requirement into a structured list with IDs at the start. Require all actions and outputs to reference those IDs. Validator rejects if any remain unaddressed. Simple but one of the strongest antidotes to requirement drift and context-window loss.

### Short-step loops

Agents drift more in long unbroken trajectories. Prefer short cycles: read state → propose one action → execute → record observation → validate → replan. Basically OODA with an externalized state ledger and validator.

### Proposal-execution split

Force the model to propose an action in structured form, then let an external policy decide if it may execute. Especially useful for authority-sensitive actions.

### Hooks as enforcement (not logging)

Hooks are most useful when they intercept lifecycle events and mutate control. Pre-tool hooks can reject calls lacking required fields. Post-tool hooks can force structured reflection. Pre-completion hooks can block finalization until checks pass.

## Framework comparison

- **LangGraph**: Most explicit about workflow control. Conditional edges, checkpointed state, interrupts for human review. Makes control logic visible in the graph.
- **CrewAI**: Process modes (hierarchical management), step_callback for inspection, respect_context_window to prevent silent compression. Softer enforcement unless augmented with custom callbacks.
- **AutoGen**: Conversation orchestration with role separation. Risk of "socially plausible drift" where agents reassure each other. Needs authoritative critic and structured inter-agent messages.
- **DSPy**: Turns fuzzy instructions into learnable, optimizable typed signatures. Good for compiling process discipline into the program. Still needs runtime harness for online enforcement.

## Emerging norms

No universal standard yet, but convergence on: least-privilege tool access, policy-enforced approvals, auditable action logs, explicit interruption points, separation between reasoning/orchestration/execution. The strongest emerging norm: "human control at meaningful boundaries" — agent assists aggressively, humans/policy retain veto over changes, external side effects, and authority escalation.

## What this means for SDD

1. **The pre-flight validator (d-tac-s6w) is well-validated.** External validation via claude -p in the CLI is exactly the generation-validation separation the research recommends.
2. **The graph is already a process enforcement mechanism.** Types, refs, closes, supersedes, contracts — these constrain what's legal. Push more checks into graph semantics.
3. **Agent unreliability is a permanent design constraint.** Not a gap to close with better prompting. Every critical process step needs external verification.
4. **Self-monitoring is advisory, never the sole gate.** Self-reflection can improve candidate quality but should feed into external validation, not replace it.
5. **Compounding error means early detection matters most.** Pre-flight firing on every `sdd new` (not just closing actions) catches deviations before they cascade.
6. **MCP governance will matter as SDD grows capabilities.** Tool provisioning and access control become part of the governance boundary.

## Sources

Research draws on: arxiv papers on agent evaluation and instruction-following (DRFR metrics, Reflexion, self-correction analysis), framework documentation (LangGraph, CrewAI, AutoGen, NeMo Guardrails), Anthropic's trustworthy agents research, Claude Code hooks documentation, and practitioner experience reports on agent reliability in production.
