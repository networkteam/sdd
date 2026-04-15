# Confidence-scored pre-flight verdicts and acceptance criteria in plans

## Overview

Replace pre-flight's binary PASS/FAIL verdict with confidence-scored findings (high/medium/low severity) and introduce an `## Acceptance criteria` section in plan decisions. Together these address find-something bias (s-prc-316), plan-prose validation overreach (s-prc-wi7), and a portion of the workflow disruption (s-prc-6hw). Empirical grounding for the calibration is drawn from a-prc-dix (mined 37 actual FAILs from 14 days of sessions; 35% outright FP rate, 9 of 12 affected sessions resorted to `--skip-preflight`).

This plan supersedes d-prc-90w — d-prc-90w's text expresses AC validation in binary FAIL terms, which doesn't fit the new verdict format. Combining the two avoids touching every template twice.

## Verdict format

### Finding structure

Each finding carries:
- **Severity**: `high` | `medium` | `low` (string enum; not numeric)
- **Category**: short tag (e.g. `type-mismatch`, `missing-ref`, `plan-coverage-ambiguity`, `ac-unaddressed`)
- **Observation**: 1-3 sentences describing what was noticed

### Output format (what the LLM emits)

One finding per line:

```
- [high] category: observation text
- [medium] category: observation text
- [low] category: observation text
```

When there are no findings:

```
No findings.
```

### Tooling behavior (hidden from the template)

- `high` findings block entry creation (exit non-zero)
- `medium` and `low` findings are displayed but do not block
- All findings shown by default
- `--skip-preflight` remains the override
- Single source of truth for consequences: `handler_new_entry.go`

**Template prompts describe severity in purely semantic terms.** No mention of threshold, blocking, or display behavior in any template. This decouples the prompt from tooling consequences — if thresholds evolve, no prompt re-calibration is needed.

## Calibration philosophy

### Severity semantics (template-facing)

- **`high`** — Clear wrong usage, direct contradiction, or structural type error
- **`medium`** — Observation worth naming for dialogue: deviation, specific proposal, partial coverage, ambiguity that could be intentional
- **`low`** — Stylistic, editorial, or suggestion

All three levels are valid outputs. Reporting no findings is also valid.

### Unifying principle

Match severity to what the finding warrants. Flag freely across all three levels; don't force `high` to name a minor observation, and don't suppress observations to avoid commitment.

### Calibrating against active contracts

A finding against an active contract is `high` only when the entry **directly contradicts** the contract's core constraint — the spirit, not the letter. Subtle deviations, missing ritual markers, or wording that doesn't precisely echo the contract are `medium` or `low`. Contracts express intent; treating them as literal rules invites find-something behavior. If the entry's reasoning shows the agent considered the contract and made a deliberate choice, that is `low` or no finding.

Test: would a reasonable reader say "this entry breaks the rule" or "this entry is in the spirit of the rule but worded differently"? Only the first is `high`.

### Immutability as calibration context

Per d-cpt-e1i, entry type/layer/kind are immutable after capture (baked into the ID). Pre-flight is the last gate before the ID is locked. This creates both urgency (catch clear misclassification at capture time) and permissiveness (supersede remains the remedy, so ambiguity need not block). Don't expand the blocking surface because correction paths exist downstream.

### Trust the structural layer

Pre-flight does not enforce what the type system, CLI, or `sdd lint` can enforce structurally. `signal_capture` in particular stays lean — when kinds arrive (per d-cpt-omm, pending), kind/content alignment becomes structural and prose-level type discipline is delegated to the kind layer.

## Acceptance criteria structure

Plan decisions (`kind: plan`) include an `## Acceptance criteria` section in the plan attachment:

```markdown
## Acceptance criteria
- [ ] Observable, verifiable outcome 1
- [ ] Observable, verifiable outcome 2
```

Each item is a single verifiable outcome — not an implementation detail. ACs are the contract between plan author, implementing agent, and pre-flight validator for closing actions.

### When ACs apply

- **New plan decisions** (`kind: plan`): pre-flight checks that `## Acceptance criteria` section exists with ≥1 checklist item (flag as `high` if absent)
- **Closing actions** (action closing a decision where the decision is `kind: plan`): pre-flight validates each AC item — confirmed done with evidence, deviation explained with dialogue reasoning, or gap
- **Other decision kinds** do not require ACs
- **Existing plan decisions** without ACs are grandfathered (not retroactive)

## Pre-flight changes per template

### Shared `verdict.tmpl` partial (new)

Embedded in all six check-type templates. Contains:
- Severity level definitions (high/medium/low semantics)
- Output format spec (`- [severity] category: observation`)
- Contract calibration principle (spirit not letter)
- Immutability note (type/layer/kind immutable; supersede is the remedy)
- Unifying principle (flag freely at honest severity)

### signal_capture.tmpl

- Replace binary output with structured findings
- Calibration block: permissive type-discipline check; imperative-commitment-without-observation = `high`; most prescription in signals = no finding
- Trust-the-structural-layer note (kinds forthcoming via d-cpt-omm)

### closing_action.tmpl

- Replace binary output with structured findings
- General plan-coverage calibration (contradiction vs. prose-completeness)
- AC-specific calibration block (rendered only when closed entry is `kind: plan`):
  - `high`: AC unaddressed, no deviation reasoning
  - `medium`: AC named but resolution not evident from action text
  - `low`: AC addressed but phrasing thin
  - No finding: AC confirmed done with specific evidence OR deviation explained with dialogue reasoning

### closing_decision.tmpl

- Replace binary output with structured findings
- Calibration: concrete signal-target miss = `high`; partial coverage = `medium`; architectural-ritual demands = no finding (apply contract calibration)

### decision_refs.tmpl

- Replace binary output with structured findings
- Calibration: missing ref to directly-answered signal = `high`; plausibly-relevant ref = `medium`; style = `low`
- When entry is `kind: plan`: check `## Acceptance criteria` section presence in attachment (`high` if absent)

### action_closes_signals.tmpl

- Replace binary output with structured findings
- Calibration: citation-contradicts-claim = `high`; thin coverage of secondary signal = `medium`; contract over-application = no finding

### supersedes.tmpl

- Replace binary output with structured findings
- Minimal calibration adjustment (this check had 0% FP in empirical mining)
- Keep `high` for kind mismatches and silent information loss

## Code changes

### `PreflightResult` type (breaking)

Replace:
```go
type PreflightResult struct {
    Pass bool
    Gaps []string
}
```

With:
```go
type Severity string

const (
    SeverityHigh   Severity = "high"
    SeverityMedium Severity = "medium"
    SeverityLow    Severity = "low"
)

type Finding struct {
    Severity    Severity
    Category    string
    Observation string
}

type PreflightResult struct {
    Findings []Finding
}

func (r *PreflightResult) HasBlocking() bool {
    for _, f := range r.Findings {
        if f.Severity == SeverityHigh {
            return true
        }
    }
    return false
}
```

Mirror the type in `framework/sdd/query/preflight.go`.

### Parser

`parsePreflightResult` reads lines matching `- [<severity>] <category>: <observation>`:
- Extract severity (reject unknown values with error)
- Extract category (token up to colon)
- Extract observation (rest of line)

Accept `No findings.` (case-insensitive, trimmed) as an empty result.

### `assembleContext`

- When the closed entry is `kind: plan`: extract the `## Acceptance criteria` section from each plan attachment and concatenate into a new `AcceptanceCriteria` field in `preflightContext`. Full plan text remains in `PlanItems` for context.
- When the proposed entry is a `kind: plan` decision: read the proposed plan's own attachments and extract the AC section for presence-of-section check.

### `handler_new_entry.go`

- Replace `if !result.Pass` with `if result.HasBlocking()`
- Display all findings with severity prefix regardless of count
- Block only when `HasBlocking()` is true
- Non-zero exit only when blocking

## Skill changes

### `/sdd` SKILL.md — plan capture guidance

Add to the "Transition to implementation" section:

> When capturing a plan decision, the plan attachment must include an `## Acceptance criteria` section with `- [ ]` checklist items. Each AC is a single verifiable outcome (not an implementation detail). ACs are the contract between plan author, implementing agent, and pre-flight validator.

### `/sdd` SKILL.md — implementation mode

Add:

> Before starting implementation, review the plan's acceptance criteria and use them as a work checklist. The closing action must address each AC — confirmed done with specific evidence, or deviation explained with dialogue reasoning.

### `/sdd` SKILL.md — responding to pre-flight findings

Add:

> Pre-flight findings are scored by severity:
> - `high` findings block entry creation. Address them by revising the entry, or by `--skip-preflight` if the finding is wrong.
> - `medium` findings are displayed but don't block. Read them, decide whether to revise or explain, continue.
> - `low` findings are informational. Read, decide, continue.
>
> Don't reflexively `--skip-preflight` on medium findings — they often surface genuine observations worth dialoguing. Only skip when confident the finding is wrong.

## Out of scope (follow-ups noted)

1. **Structural metadata checks move to CLI/lint**: presence-of-field checks (missing Kind, missing Confidence) currently in LLM pre-flight belong at CLI parse time — required fields error cleanly without LLM judgment. Follow-up signal to be captured.
2. **Contract scope review**: d-cpt-ah1 (CQRS), d-prc-31v (regression tests), d-prc-zch (commit-before-capture) generate false positives when applied broadly. Contract calibration principle mitigates this at the validator; the contracts themselves may benefit from scope clarification. Follow-up dialogue.
3. **Empirical calibration regression suite**: Beyond the eval tests in this plan, ongoing session mining could drive iterative calibration. Deferred.
4. **Retroactive AC application**: Existing plan decisions without ACs are grandfathered. Only future plans require ACs.
5. **`--preflight-strict` flag** to lower blocking threshold to `medium`: Deferred until we see whether `medium` findings become actionable in practice.

## Acceptance criteria

- [ ] `PreflightResult` holds `Findings []Finding` with `Severity` (`high`/`medium`/`low` enum), `Category` (string), `Observation` (string); `HasBlocking()` helper returns true iff any finding is `high`
- [ ] `parsePreflightResult` parses `- [severity] category: observation` lines and `No findings.`; rejects unknown severity values with an error
- [ ] Shared `verdict.tmpl` partial created and embedded in all 6 check-type templates; describes severity purely semantically (no mention of threshold/blocking/display)
- [ ] `verdict.tmpl` contains the contract-calibration principle, immutability note, and unifying principle
- [ ] Each of the 6 check-type templates has a `## Calibration` block with verbatim anti-examples from a-prc-dix corpus and high-severity anchors
- [ ] `closing_action.tmpl` has an AC-specific calibration block that renders only when the closed entry is `kind: plan`
- [ ] `assembleContext` extracts the `## Acceptance criteria` section from plan attachments and exposes it as `AcceptanceCriteria` in preflightContext
- [ ] Plan decisions (this one and all future plans) include `## Acceptance criteria` with `- [ ]` checklist items
- [ ] `decision_refs.tmpl` flags missing `## Acceptance criteria` on `kind: plan` decisions as `high`
- [ ] `closing_action.tmpl` validates each AC item from the referenced plan — `high` if unaddressed without deviation reasoning, `medium` if resolution unclear, `low` if phrasing thin, no finding if confirmed done or deviation explained
- [ ] `handler_new_entry.go` blocks only when `HasBlocking()` is true; displays all findings with severity prefix
- [ ] `/sdd` SKILL.md guides plan capture to include `## Acceptance criteria`
- [ ] `/sdd` SKILL.md implementation mode instructs reviewing ACs before start and using them as a work checklist
- [ ] `/sdd` SKILL.md guides response to findings by severity (don't reflexively skip `medium`)
- [ ] Eval tests (under `//go:build eval`) added to `preflight_eval_test.go` — one subtest per anti-pattern category from the corpus
- [ ] Strict eval test criteria: all 13 FP cases from the corpus produce no `high` finding; all 16 TP cases produce at least one `high` finding (LLM indeterminacy accepted; run manually via `go test -tags=eval`)

## Calibration reference (source for template calibration blocks)

Verbatim quotes from a-prc-dix corpus where available; illustrative examples are clearly labeled.

### Shared verdict.tmpl principles

```
Flag findings at the severity that matches what you observe:

- high   — Clear wrong usage, direct contradiction, or structural type error
- medium — Observation worth naming for dialogue: deviation, specific proposal,
           partial coverage, ambiguity that could be intentional
- low    — Stylistic, editorial, or suggestion

All three levels are valid outputs. Reporting no findings is also valid.

Calibrating against active contracts: a finding is only high when the entry
directly contradicts a contract's core constraint. Subtle deviations, inexact
wording, or missing ritual markers are medium or low. Contracts express intent,
not rigid form. If the entry's reasoning shows the agent considered the
contract and made a deliberate choice, that is low or no finding.

Type, layer, and kind are immutable after capture. Catch clear misclassification;
leave ambiguity to supersede.
```

### signal_capture

```
Signals bring observations and potential directions as dialogue material.
Focus on clear cases.

#### high — clear wrong usage

Illustrative: "We must migrate the database to PostgreSQL by next sprint and
deprecate the MongoDB adapter. The team should start immediately with the
schema migration scripts."
→ Imperative + timeline + ownership, no observational content. Reads
unambiguously as a committed decision.

#### medium — worth naming for dialogue

Illustrative: "The onboarding flow has a high drop-off rate at step 3. We
must simplify the form by removing optional fields and merging the address
section."
→ Observation present but the entry commits to a specific prescriptive
design. Worth raising whether the remedy belongs in a decision.

#### low — stylistic

Verbatim: opening "Add to sdd new" in a signal with observational body.
→ Imperative opener, observational content; readable as a signal.

#### Report no finding

Verbatim: "Entry mixes signal with decision-like prescriptive content
('The prompt should either X or Y')."
→ "Should either" offers alternatives as dialogue material — valid signal
framing.

Verbatim: "Entry contains prescriptive language ('they should explicitly
accept', 'should treat... as valid outcome')."
→ "Should" is dialogue material in a signal; proposing a direction is valid.

Verbatim: "First sentence must describe the entry's substance [...] Rewrite
the opening to: 'The claude CLI exposes...'"
→ Editorial rewrite of first-sentence wording; the body conveys substance.

Verbatim: "Confidence level 'high' does not match the evidence."
→ Confidence is the author's deliberate self-assessment.
```

### closing_action

```
Closing actions must genuinely cover the decision or plan's requirements.
The work lives in commits, attachments, and upstream entries — not
necessarily restated in action prose.

#### high — direct contradiction or concrete requirement unmet

Verbatim: "Evidence for closure contradicts a-tac-tsd: The action claims
validateAttachmentLinks was 'implemented and tested,' but a-tac-tsd
explicitly states 'Does NOT yet cover broken or missing attachment
references.'"
→ Action cites evidence that directly refutes its own claim.

Verbatim: "Tests for migrated commands and queries are not explicitly
addressed. Plan requires 'test moved or added in the correct package' for
each migration; the action states 'no new tests required' but does not
clarify whether existing tests were moved."
→ Plan has a specific test-disposition requirement; action explicitly
contradicts it without explanation.

#### medium — plan-coverage ambiguity

Illustrative: "Action mentions cleanup behavior at a high level but doesn't
address the plan's explicit 'no automatic cleanup — agent handles removal
manually' requirement. Unclear whether the manual cleanup path was
implemented or if the requirement was dropped."
→ Plan item has specific behavior; action prose leaves it ambiguous.

#### low — formatting

Verbatim: deferral acknowledged in text without specific "Deviation: ..."
marker format.
→ Deferral is present; an explicit marker would improve scannability.

#### Report no finding

Verbatim: "Per-direction dedup algorithm not mentioned."
→ Implementation details live in committed code. Pre-flight checks whether
work was done, not whether description restates it.

Verbatim: "Self-describing first sentence fails: opening 'Implemented sdd
summarize command and llm package per d-tac-h93 plan' is reference-dependent."
→ Referencing the closed plan is standard practice for closing actions.

Verbatim: "Output format specification not included."
→ Format spec is in the code and plan attachment; prose doesn't need to
restate.
```

### closing_action — Acceptance criteria coverage (renders only when closed entry is kind: plan)

```
Plan decisions include ## Acceptance criteria checklist items. Each AC must
be addressed by the action — confirmed done with specific evidence or
deviation explained with dialogue reasoning.

#### high — AC unaddressed, no deviation reasoning

Illustrative: Plan AC: "Dedup with cross-reference markers."
Action description: no mention of dedup or cross-reference markers. No
deviation explanation.
→ Silent omission of a specific AC.

#### medium — AC named but resolution not evident

Illustrative: Plan AC: "sdd show renders depth 0 full, depth 1+ as summary
lines."
Action description: "Depth-limited rendering implemented."
→ Partially acknowledged; specific depth-0-full/depth-1+-summary
distinction isn't evident. Plausibly implemented; worth clarifying.

#### low — AC addressed but phrasing thin

Illustrative: Plan AC: "--max-depth flag with default 4."
Action description: "Added --max-depth flag."
→ AC addressed; default value not stated. Implementation likely matches;
prose could confirm.

#### Report no finding

Illustrative: Plan AC: "Truncation markers at boundary with entry IDs."
Action description: "Truncation markers appear at the depth boundary,
rendered as '[truncated: refs id1, id2]' with full IDs preserved."
→ AC confirmed done with specific evidence.

Illustrative: Plan AC: "Independent per-direction depths."
Action description: "Deviation: single --max-depth instead of per-direction
— decided during dialogue that independent depths add complexity without
corresponding benefit."
→ Deviation explained with dialogue reasoning.
```

### closing_decision

```
A decision closing a signal must address what the signal identifies. Apply
contract calibration: only direct contradiction of an active contract is
high.

#### high — concrete signal requirement unmet

Verbatim: "Signal target not met: Signal requires ≤30s for default catch-up;
decision delivers ~1m. Decision does not explain why this shortfall is
acceptable or revisit the target requirement."
→ Decision claims to close a signal with a specific quantitative target
but misses it without justification.

#### medium — partial coverage, scope unclear

Illustrative: "Decision addresses two of three signal concerns; the third
('metrics dashboard visibility') is not discussed. Possible intentional
scope narrowing, but not acknowledged."
→ Decision handles most of the signal; one concern is silent.

#### low — reasoning compression

Illustrative: "Decision's rationale for deferring tiered catch-up is present
but compressed; a sentence on why output-bound is the bottleneck would help
future readers."
→ Rationale present but thin.

#### Report no finding

Verbatim: "Plan omits explicit CQRS decomposition (action/command/query/
handler/model types) required by d-cpt-ah1 contract."
→ Apply contract calibration: d-cpt-ah1 expresses intent, not rigid form.
Small tactical plans don't need decomposition tables.

Verbatim: "Decision introduces 'agent-neutral abstraction' and llm/claude
subpackage structure without grounding in signals. Signals request metadata
surfacing from Claude specifically; they do not call for provider-neutral
design."
→ Decisions synthesize design direction; the grounding test is "does the
signal motivate a response?" not "is the exact direction pre-established?"
```

### decision_refs

```
A decision must ref the signals or decisions that directly motivated it or
that it directly answers. Not every tangentially-related entry needs a ref.

#### high — missing ref to directly-answered signal

Verbatim: "Missing ref to 20260413-143855-s-cpt-4gj. Signal s-cpt-4gj
directly raises the distribution question this decision prescribes an answer
to (agent-neutral channels: Homebrew, curl|bash, GitHub Releases). s-cpt-4gj
lists three options including 'packaging as a Claude Code plugin,' which
the decision implicitly rejects."
→ Decision answers the specific question raised by an unreferenced signal.

#### medium — potentially relevant ref

Illustrative: "Decision discusses agent-neutrality; a tactical signal
(s-tac-7kh) explicitly raises non-technical user access, which may inform
the neutrality argument. Worth considering as a ref, but could be
intentional scope narrowing."
→ Ref plausibly relevant but not directly answered.

#### low — style

Illustrative: "Decision's summary is dense; splitting across 2-3 sentences
would improve scannability."
→ Prose suggestion.

#### Report no finding

Verbatim: "Agent-agnostic design as a strategic principle is not grounded
in referenced signals."
→ Strategic decisions synthesize direction. Demanding every principle be
pre-established in a signal prevents strategic thinking.

Verbatim: "Missing Kind field in header."
→ Presence-of-field is CLI/lint territory, not LLM judgment.
```

### action_closes_signals

```
An action closing signals must genuinely resolve the stated concerns.
Cross-reference claims against cited evidence.

#### high — citation contradicts claim

Verbatim: "Misinterpretation of d-cpt-vt1: The action claims d-cpt-vt1
acknowledges silent scoping as a 'permanent design constraint,' but
d-cpt-vt1 is about reliability through independent validation, not about
accepting silent scoping. It prescribes a solution (structural separation),
not an acceptance of the pattern."
→ Agent stretches meaning of a cited reference in a way the reference
directly refutes.

#### medium — thin coverage of a named signal

Illustrative: "Action closes two signals; description focuses on resolving
the first, and the second signal's concern is addressed briefly without
explaining how the action meets its specific framing."
→ Primary signal clearly resolved; secondary could be better linked.

#### low — style

Illustrative: "Action's first sentence names the package but not the
concrete outcome; a reader seeing only this line wouldn't know what was
closed."
→ Prose suggestion; body conveys substance.

#### Report no finding

Verbatim: "Signal s-prc-17y and contract d-prc-31v require regression test
in the same commit as the bug fix. Action confirms test exists but does
not confirm it was in the same commit as the original fix."
→ Test and commit are cited. Demanding same-commit forensic proof is
beyond the validator's capability to verify.

Verbatim: "Action is a bug fix and must provide evidence of regression
testing per d-prc-31v. The action description should confirm tests were
added in commit e80cc3e."
→ Apply contract calibration: d-prc-31v applies to executable code bug
fixes. Prompt template changes don't have a regression path — spirit of
the contract, not its letter.
```

### supersedes

```
Supersede is structural — kind transitions, scope retention, information
preservation. This check type is high-signal: most findings are genuine.

#### high — structural type error or silent information loss

Verbatim: "Kind mismatch: superseded entry is a 'contract' (standing rule),
proposed entry is a 'plan' (implementation plan). Superseding a standing
constraint with a plan is structurally inconsistent."
→ Supersede semantics require compatible kind transitions. Contract→plan
is a category error.

Verbatim: "The new entry silently narrows scope by dropping the definitions
of the three types (signal, decision, action) and five layers that were
part of the superseded entry's contract."
→ Silent information loss. Core framework content drops without
acknowledgment.

#### medium — scope narrowing implicit

Illustrative: "New entry's scope is narrower than the superseded entry's.
Changes are inferable from comparison but not explicitly noted."
→ Narrowing is valid; should be called out.

#### low — style

Illustrative: "Supersede explanation is brief — a sentence naming what
changed would help future readers."
→ Prose suggestion.

(No-finding anti-examples: none from corpus — supersede had 0% FP in mining.
This check type is well-calibrated.)
```
