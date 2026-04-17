# Acceptance Criteria in Plan Decisions

## Overview

Add a structured `## Acceptance criteria` section to plan decision attachments. This section becomes the contract between plan author, implementing agent, and pre-flight validator — replacing the current behavior where pre-flight validates against the full plan prose.

## Plan attachment format

Plans include an `## Acceptance criteria` section with Markdown checklist items:

```markdown
## Acceptance criteria
- [ ] sdd show renders depth 0 full, depth 1+ as summary lines
- [ ] --max-depth flag with default 4
- [ ] Dedup with cross-reference markers
- [ ] Truncation markers at boundary with entry IDs
```

Each item is one verifiable, observable outcome — not an implementation detail.

## Pre-flight changes

### New plan decisions (`decision_refs.tmpl`)

When the decision has `kind: plan`, check that the plan attachment contains an `## Acceptance criteria` section with at least one checklist item.

### Closing actions (`closing_action.tmpl`)

Replace full-plan-prose validation with AC-focused validation:

1. Extract the `## Acceptance criteria` section from the referenced plan attachment
2. For each AC item, check whether the action description addresses it:
   - **Confirmed done**: the action describes fulfilling this criterion
   - **Deviation explained**: the action notes the deviation with dialogue reasoning (e.g. "Deviation: single max-depth instead of per-direction — decided during dialogue that independent depths add complexity without benefit")
   - **Not addressed**: gap — flag as FAIL
3. The full plan text is still available as context for the LLM to understand intent, but validation focuses on ACs only

### Go code changes

In `assembleContext` (`preflight.go`), when reading plan attachments for closing-action checks:
- Parse the attachment to extract the `## Acceptance criteria` section
- Pass it as a new `AcceptanceCriteria` field in the template context
- Keep `PlanItems` with the full plan for context, but the template validates against `AcceptanceCriteria`

For decision-refs checks on plan decisions:
- Read the plan attachment and check for `## Acceptance criteria` section presence

## Skill changes

### /sdd SKILL.md — Plan capture

When capturing a plan decision, guide the agent to include ACs:
- "Every plan needs an `## Acceptance criteria` section with checkable items"
- "ACs describe observable outcomes, not implementation details"

### /sdd SKILL.md — Implementation mode

When transitioning to implementation:
- "Review the plan's acceptance criteria before starting"
- "Use ACs as your work checklist — confirm each one before writing the closing action"
- "If you deviate from an AC, note the deviation and the dialogue reasoning in the closing action"

## Acceptance criteria

- [ ] Plan decisions include an `## Acceptance criteria` section with `- [ ]` checklist items
- [ ] Pre-flight for new plan decisions checks that ACs section is present
- [ ] Pre-flight for closing actions checks that each AC from the referenced plan is addressed — either confirmed done or deviation explained with dialogue reasoning
- [ ] /sdd skill guides the agent to include ACs when capturing plan decisions
- [ ] /sdd skill implementation mode instructs the agent to use ACs as a work checklist before writing the closing action
