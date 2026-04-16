# Observed failure

During a catch-up presentation, the decision \`20260416-190215-d-tac-g8n\` (a plan with 0/6 acceptance criteria closed, no closing action) was presented to the user as:

> Short-form ID acceptance (\`d-tac-g8n\`) — active decision, implementation done.

The phrase "active decision, implementation done" is self-contradictory. "Active" means no closing action exists; "implementation done" implies one does. The graph mechanically says the decision is open.

# How it happened

The \`/sdd-catchup\` sub-skill returned a structured block per entry. For this entry, the context field read:

> Recent action (a-prc-8nl) confirmed the need. Closes signal s-prc-b2v.

Two traps in that sentence:

1. "Recent action confirmed the need" — \`a-prc-8nl\` is an upstream motivating action, not a closing action. Prose phrasing blurred the direction.
2. "Closes signal s-prc-b2v" — correct (decision closes signal), but easily misread as "this entry is closed."

The outer skill then compressed the block into narrative, adding a lifecycle verdict ("implementation done") that wasn't grounded in any relation field. The sub-skill's label ("active decision") was present in the same block and was overridden by the editorialized verdict.

# Root-cause framing

The pre-flight gate enforces mechanics-over-vibes for entry **creation**. No analogous discipline exists at the reading/presentation surface. The outer skill is free to generate lifecycle claims without citing the graph; the sub-skill's prose "context" field invites it by blending relation direction into narrative.

# Directions explored in dialogue (unresolved)

- **Structured mechanical status from the sub-skill.** Replace prose "context" blurbs with explicit fields: status, kind, ac_progress, closed_by, superseded_by, upstream/downstream refs. Prose narrative scoped to value/relevance, not lifecycle.
- **Graph-generated status badge on each catch-up item.** A badge rendered directly from relations (e.g. \`plan · 0/6 ACs · active\`) sits next to the narrative. Contradictions become visible.

Neither direction was adopted. Captured as observation for later decision-making.
