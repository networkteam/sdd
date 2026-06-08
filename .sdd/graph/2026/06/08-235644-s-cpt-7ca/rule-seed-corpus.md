# Rule seed corpus (candidates from done-signal mining + session lessons)

Eight candidate process rules in the **When to apply / What to consider / How to calibrate** shape, with source IDs. Seed corpus and test fixtures for the rule build — not yet capturable (the `rule` kind does not exist).

## 1. Trace a change to every dependent surface before shipping
- **When:** shipping a change to one part of the system (feature, new on-disk output, rule/vocab change, config) when other parts (code, config, docs, eval fixtures, CLI surfaces) depend on or encode it.
- **Consider:** enumerate the connected surfaces and update each, or carve them out as future work in dialogue. "What else touches this?" is a required step.
- **Calibrate:** advisory, strong (the named systemic gap; cheapest at plan/ship time). Likely too broad alone — see specializations below.
- **Source:** generalizes s-prc-guf. Recurring (≥5 instances).
- **Topic:** reliability/discipline

## 2. A new machine-local path under .sdd/ needs a gitignore entry in sdd init
- **When:** your change writes a new machine-local file/dir under `.sdd/` (sink, index, cache, stats, tmp).
- **Consider:** add the path to `sdd init`'s gitignore set and extend the init gitignore tests.
- **Calibrate:** binding (mechanical, checkable; missed at least twice).
- **Source:** s-ops-qbl / s-ops-84c (stats sink), s-ops-626 (index dir). Recurring. Specialization of #1.
- **Topic:** cli/init

## 3. A closing done signal must cite the commit hash of the code it records
- **When:** capturing a done signal that records committed code as its artifact.
- **Consider:** name the commit hash(es); commit first, then cite.
- **Calibrate:** binding (already a playbook step + pre-flight artifact-durability check).
- **Source:** s-prc-ijn. Already enforced — capture only if folding existing enforcement under rules.
- **Topic:** reliability/discipline

## 4. Body and edges must stay consistent both ways
- **When:** drafting any entry whose body names other entries by ID, or carrying refs/closes/supersedes.
- **Consider:** every ID in prose is an edge; every edge appears in the prose. No dangling mention, no silent edge.
- **Calibrate:** advisory, strong (skill-text rule, no mechanical gate — a real enforcement gap).
- **Source:** s-prc-ema. One-off, generalizes to all capture.
- **Topic:** type-system/refs

## 5. Scope programmatic commits with explicit pathspecs
- **When:** writing tooling that commits, or staging in a repo with a possibly-dirty index.
- **Consider:** pass `-- <paths>` to `git commit`; a scoped `git add` isn't enough (an unscoped commit folds in the whole index).
- **Calibrate:** binding for auto-commit tooling; advisory-strong for hand commits (the CLAUDE.md never-`add -A` rule).
- **Source:** s-tac-k5g / s-tac-tdz (index leakage across sdd operations).
- **Topic:** reliability/discipline

## 6. Flag the untested path when automated tests can't cover it
- **When:** closing work where part can't be exercised by vet/test (interactive TUI loops, live keypress, terminal rendering).
- **Consider:** state what the gate did and did not cover; name the manual verification still warranted.
- **Calibrate:** advisory (judgment on adequate coverage).
- **Source:** s-tac-tum (charm v2 upgrade). One-off, general.
- **Topic:** reliability/testing

## 7. A new or extended CLI capability must be made knowable to the skill
- **When:** you add or extend a CLI capability (a `sdd view` option, a flag, a command).
- **Consider:** update the skill surfaces that teach it — `cli-reference.md` and the relevant playbooks — so the agent knows the capability exists.
- **Calibrate:** binding-ish (checkable: did cli-reference/playbooks change when the CLI surface did?). Specialization of #1.
- **Source:** session lesson; recurring (flagged for `sdd view` options).
- **Topic:** skill/architecture

## 8. Scope skill/prompt edits tightly to the goal; don't cut load-bearing text
- **When:** editing skill or prompt text.
- **Consider:** review the change against the intended goal and verify you are not removing instructions that serve a purpose — removal degrades effectiveness invisibly.
- **Calibrate:** advisory (judgment-heavy).
- **Source:** session lesson.
- **Topic:** skill/architecture
