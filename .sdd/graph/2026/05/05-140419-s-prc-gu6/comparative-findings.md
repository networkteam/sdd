# Skill bundle examination — comparative findings

Comparative read of three popular Claude Code skill bundles (GSD, gstack, beads) examined locally at `/Users/hlubek/Dev/AI/Claude/references/` on 2026-05-05. Each bundle was researched in parallel by a dedicated agent under an identical four-dimension rubric (routing/triggering, per-move documentation shape, modularization mechanics, observed pitfalls), with beads carrying two extended dimensions (skill-vs-storage interface, persistent-memory mechanics).

## Bundle positions

### GSD — two-stage hierarchical router with state-machine dispatch

- ~67 slash-command stubs, 33 sub-agents, 88 workflow files, 50+ references, 38 templates. ~3.1 MB user-facing surface.
- **Routing**: PR #2792 introduced six namespace meta-skills (`gsd-workflow`, `gsd-project`, `gsd-review`, `gsd-context`, `gsd-manage`, `gsd-ideate`) with pipe-separated keyword-tag descriptions and an explicit "User wants → Invoke" routing table. Cold-start surface: ~120 tokens (down from ~2,150 with a flat 86-skill listing). Routing *into* GSD via the namespaces; routing *within* via deterministic JSON state from an SDK plus `Skill(skill="...", args="...")` invocations from inside workflows. A soft `gsd-workflow-guard.js` PreToolUse hook injects an advisory when Write/Edit happens outside any workflow. Description budget mechanically enforced (`scripts/lint-descriptions.cjs`, MAX_LENGTH = 100).
- **Per-move shape**: numbered `<step>` state machine with `name`, `required`, entry conditions (config gates), action (`Skill(...)` invocation or shell), result-checking, error fallback, and explicit transitions ("proceed to close_parent_artifacts → regression_gate → verify_phase_goal"). Most rigorous of the three.
- **Modularization**: stub → workflow → references → mode-overlays. Lazy-loading is explicit via `<progressive_disclosure>` blocks with quantified wins (`discuss-phase` entry-point load cut from ~13K to ~0 tokens by gating Read() on flag/condition, per CHANGELOG #2606). Sub-agents (33 in `agents/`, 4–46 KB each) spawn only via `Task(subagent_type=...)`.
- **Pitfalls**: stale slash-command references after consolidations are the dominant recurring class — 8+ followup fixes after the 86→59 consolidation in #2790 (#2954, #2950, #2949, #2948, #3010, #3029, #3042, #3044), addressed reactively with regression tests (`tests/bug-3010-reapply-patches-references.test.cjs`) rather than a single source of truth. Dynamic-routing for failure escalation (`.changeset/dynamic-routing.md`) is a deliberate first-class concern.

### gstack — flat siblings with trigger-engineered descriptions

- 51 SKILL.md files, all peers at the root; ~640K tokens of skill prose; `ship/SKILL.md` 2,944 lines, `plan-ceo-review` 122K, `qa` 1,917 lines.
- **Routing**: hand-tuned `description:` prose with required "Use when..." and "Proactively invoke..." phrases (test-enforced in `test/skill-validation.test.ts`), plus a separate `triggers:` array powering an external GBrain router and voice-triggers appended to descriptions. The root skill auto-injects a `## Skill routing` table into the project's CLAUDE.md as a fallback dispatch mechanism. No parent dispatcher beyond LLM inference. CHANGELOG 0.6.4.0: "the description field is not a summary. it's when to trigger."
- **Per-move shape**: numbered `### Phase N` prose with sub-phases (8a, 8b, 8c). Linear flow; no formal entry/exit conditions; mode routing within a skill via flag-detection (`if --power then…`). Sub-skill hand-off when needed via direct Read-tooling of child SKILL.md with explicit "skip these sections" lists.
- **Modularization**: full SKILL.md is always-loaded once active; minimal lazy reading. Auto-generated from `SKILL.md.tmpl` with a 4-tier preamble system that *duplicates* boilerplate inline rather than referencing it (the cause of 1–3K line SKILL.md inflation). Of 51 skills, only `qa/`, `cso/`, `browse/` ship sibling reference files.
- **Pitfalls**: trigger collisions with host built-ins (`/debug` and `/checkpoint` both renamed because Claude Code shadows them; collision-sentinel test as insurance); voice/STT mangling routes to wrong skills (CSO → CEO); skills initially invisible without explicit trigger phrases (12 skills retroactively gained "Use when" prose); section-ordering inside the always-loaded preamble has subtle behavioral effects (a plan-review cadence regression on Opus 4.7 traced to overlay rendering above the AskUserQuestion format). Removed an unfixable routing test (`journey-think-bigger`) rather than chase ambiguous signals.

### beads — single skill with hook-injected live tool reference

- 1 skill (`skills/beads/SKILL.md`), 32 slash commands, 1 sub-agent. SKILL.md is 110 lines / ~605 words (cut from 3,306 in commit `025142d5`). Underlying Go binary `bd` with embedded Dolt (MySQL-compatible versioned RDBMS).
- **Routing**: no internal sub-skill routing — single skill with trigger phrases packed into the description ("Trigger with 'create task'…"); explicit "undertriggering guard" sentence ("Make sure to use this skill whenever managing multi-session work…") added in commit `cddbe39e`. `bd prime` runs on every SessionStart and PreCompact via plugin hooks, injecting CLI guidance into the agent's context independent of skill activation. Decision-test heuristic in SKILL.md: "Will I need this context in 2 weeks? YES = bd, NO = TodoWrite."
- **Per-move shape**: man-page-style recipes with frontmatter (`description`, `argument-hint`) and a numbered procedure. No formal entry/exit conditions; transitions implicit in prose.
- **Modularization**: SKILL.md is the only always-loaded file; `resources/*.md` files load on demand (linked from SKILL.md tables; biggest are `DEPENDENCIES.md` 747 lines, `CLI_REFERENCE.md` 642). ADR-0001 captures the decision: "Use `bd prime` as the single source of truth for CLI commands; SKILL.md contains only value-add content."
- **Pitfalls**: under-triggering activation (three explicit revisions of trigger language); documentation drift drove the ADR-0001 refactor; the CLI accidentally torpedoed its own skill (`bd sync` deleted skill files, GH#738); LLM temporal-reasoning trap on dependency direction had to be enforced at the storage layer because skill prose alone wasn't reliable.

## Cross-cutting positions

| | Routing model | Always-loaded | Hardest-recurring pitfall |
|---|---|---|---|
| GSD | Two-stage namespace router → SDK-driven state machine | ~120 tok meta + 50–100 line stub on invocation | Stale slash-command refs after consolidations |
| gstack | Description prose + voice-triggers + injected CLAUDE.md table | The whole SKILL.md (1–3K lines) per active skill | Trigger collisions with host built-ins |
| beads | Single skill; `bd prime` hook injection | ~605 words SKILL.md | Under-triggering activation |

## Implications for /sdd

`/sdd` currently mixes two of these patterns: gstack-shaped *internally* (one always-loaded SKILL.md with hand-tuned descriptions and embedded playbooks) but GSD-shaped *externally* (sub-skills `/sdd-bootstrap`, `/sdd-explore`, `/sdd-groom`, `/sdd-catchup` as namespace routes). The modularization gap (s-prc-nxy) names two candidate directions: extracting playbook content into on-demand references (Pattern A) or creating sub-skills with independent triggers (Pattern B).

The comparative material favors Pattern A as the strongest first move:

1. **Quantified token win precedent**: GSD's `<progressive_disclosure>` cut entry-point load from ~13K → ~0 for `discuss-phase` by gating Read() on flag/condition. /sdd's playbooks (catch-up, augment-plan, transition-to-implementation, grooming, explore) are conditionally relevant and 50–100 lines each — direct analog.
2. **Trigger-collision avoidance**: gstack's experience shows that sub-skill proliferation increases collision risk with host built-ins and demands ongoing trigger-engineering investment. Pattern A sidesteps this entirely by keeping playbook content inside `/sdd`.
3. **Reuse of existing infrastructure**: `references/framework-concepts.md`, `meta-process.md`, `cli-reference.md` already exist as the always-loaded supplement pattern. Extracting playbooks extends rather than reinvents.
4. **Reversibility**: if routing reliability drops, specific playbooks can be promoted to sub-skills later. The reverse direction (collapsing sub-skills back into references) is harder once trigger metadata is established and external callers depend on it.

The open question that needs branch-based scenario evaluation (per s-prc-nxy's resolution path): does the parent skill reliably gate the reference reads when each playbook is needed? GSD's explicit `<progressive_disclosure>` table format is the candidate mechanism — name each reference, name the trigger condition, give the agent an instruction it can lint against itself.

## Refinement to the service-vs-mode test

Earlier framing distinguished sub-skills (services with discrete I/O contracts) from playbook moves (dialogue stances). The catch-up case sharpens the test:

> A service'"'"'s output is a discrete artifact the caller consumes and moves past. A mode'"'"'s "output" is the caller'"'"'s continuing context.

Under the refined test:

- `/sdd-explore` (returns context for one entry; outer agent drills only into entries dialogue surfaces) → service stands.
- `/sdd-groom` (returns a candidate table; outer agent walks one item at a time) → service stands.
- `/sdd-bootstrap` (initialization walkthrough with hand-back; the now-set-up graph is the artifact) → service stands.
- `/sdd-catchup` (its "output" *is* the agent'"'"'s continuing context; the briefing doesn'"'"'t substitute for entry knowledge) → fails the test; mode-shaped.

This is empirically grounded: an earlier forked-`/sdd-catchup` session produced double-loading because the outer agent had to re-load entries the sub-skill already loaded. At 1M context this is wasted tokens; at 200K it would actively hurt. The current `/sdd` SKILL.md tells the agent to run `sdd status` and cluster directly anyway — `/sdd-catchup` is largely vestigial, used only by `/sdd-bootstrap`'s hand-back.

## Recommended scope for the upcoming plan

Bundle two complementary moves:

1. **Pattern A** — extract heavy playbook prose into on-demand references with GSD-style explicit load-gates in SKILL.md. Keep invariants (capture discipline, type/layer rules, canonical names, sync-check guidance) always-loaded.
2. **Demote `/sdd-catchup`** from sub-skill to playbook moves (quick / full / topic-drill differentiation). Update `/sdd-bootstrap`'s hand-back to land in `/sdd` and trigger the full-catch-up move.

Branch-based scenario evaluation against the s-tac-n8u baseline; routing-reliability rubric scoring whether the parent skill loads the right references for each scenario; decision criterion specifies under what reading the change lands vs reverts.

## Out of scope (separate insight)

The beads-as-foil reading (graph-as-record vs DB-as-record positioning) is not a modularization finding — captured separately as a conceptual insight signal.

## Sources

Per-bundle research reports were produced in parallel with file-path-cited evidence. This synthesis quotes the load-bearing observations; full per-bundle reports are available in the agent transcripts from the 2026-05-05 examination session.
