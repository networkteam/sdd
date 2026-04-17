# Fork resonance into a dedicated SDD repo

## Scope

- New repo focused exclusively on SDD (Signal → Dialogue → Decision)
- Top-level layout designed for an external reader, not a contributor to both resonance and SDD
- README positioning SDD for external evaluation: what it is, what it's for, how to try it
- Move SDD-specific content (framework/, sdd skills, docs/framework/) out of resonance

## What moves to the new sdd repo

- `framework/` — Go CLI source and `bin/` (gitignored)
- `.claude/skills/sdd*` — the SDD skill suite (sdd, sdd-catchup, sdd-groom, sdd-explore)
- `docs/framework/` — graph entries and attachments
- SDD-focused CLAUDE.md guidance (extracted from the current mixed-purpose file)

## What stays in resonance

- Retro cycle skills (create-spec, spec-task-breakdown, task-retro, spec-retro, meta-retro, meta-retro-followup)
- `docs/spec/`, `docs/tasks/`, `docs/retro/` for resonance's own work
- Anything tied to the retro-harness identity

## Why now

- SDD's trajectory has diverged from resonance's retro-harness scope — d-cpt-omm (two-type redesign) is clearly its own direction
- External evaluation (s-stg-qg0) needs a clean, focused repo to land into
- Current setup creates cognitive overhead for anyone trying to understand what either project is

## Open questions

- Exact folder layout at the new repo root — does `framework/` stay named or become something like `cli/` + `graph/`?
- Does the SDD graph (current `docs/framework/graph/`) migrate as-is with full history, or is it re-seeded fresh from the new repo's inception?
- How to preserve git history for moved files — `git subtree split` or `git filter-repo`?
- Does the current `future-framework` branch need to merge back to resonance `main` first, or do we fork directly off the branch?
- Where does the binary-distribution story sit — in the same repo (`release/` workflow), or a separate release repo?

## What this unlocks

- Clean onboarding story for external users
- A README that doesn't hedge between "this is about retros AND graphs"
- Unblocks the distribution tooling work (separate signal) which needs a concrete artifact to ship

## Out of scope for this decision

- The distribution mechanism itself (captured as a separate signal)
- Whether resonance continues to be actively developed (no change — it continues as-is with its retro cycle)
