# Slice A code review record — commit a287683

Review run 2026-07-05 by an Opus sub-agent against the seeding-contract directive (20260704-235517-d-tac-tlo) and slice A of the completion directive (20260705-010751-d-tac-dbk), verified in dialogue (Christopher, Claude). go vet, full test suite, golangci-lint: green.

## Clean areas (reviewed, no findings)

- **Seeding/replay symmetry**: live start does SetStart(params) then seedFromParent → WriteState (session.go:245–251); replay does SetStart then WriteState from the logged seed map (session.go:561–575). Parent seed and caller params disjoint by construction (Store.Has skip, session.go:313); caller override wins identically in both paths.
- **No validation bypass**: seeded values go through WriteState → ValidateValue (store.go:76–79) and face the same on-entry predicate gates; a seeded anchor still must pass anchorsResolve (predicates.go:69–77) — a bad handed-down ID stalls the resolver.
- **Provenance stable across replay**: seeds written as ProvenanceState both live and on replay; StateSnapshot byte-identical after restart — seeding cannot spuriously reopen a confirmed playback.
- **Auto-advance safety**: cascade returns at any non-gate step (instance.go:150–152) — cannot skip a user or agent chooser, cannot skip substantive work (evaluate's assess still holds on hasEvaluation).
- **Task class**: user choosers rejected at load (spec.go:597–600); explore excluded from shell enumeration (registry.go:679) and loadProcedure list (tools.go:1022).
- **Spec fidelity**: capture's inherited-evidence branch, evaluate's junction seed note, engage/evaluate resolver prose (no recency), explore class: task — all match the decided mechanics; no stale text.

## Findings

1. **[medium · design-conformance] Fixed grounding set with silent no-op** — groundingHandoffFields is global (session.go:292); differently-named parent evidence transfers nothing silently (session.go:309–320). Impacts and decided resolution recorded in the follow-up gap entry (parent-declared per-junction `seed` mapping, shipped set as no-declaration default, load-time failure on missing source field, serve note on empty default handoff). Must land before slice B specs.
2. **[medium · test-gap] Anchor handoff zero coverage** — the seeding tests' child declares only widenReport+body; the MCP e2e passes capture's anchor as explicit param. Add: child declaring anchor as state, inherit case + per-dispatch override case.
3. **[low · design-conformance] Fork hint prose-only** — no structured signal keyed off class: task. Decided: task-class start_procedure responses carry `execution: fork-preferred` in structuredContent; harness automation consuming it stays deferred.
4. **[low · test-gap] No seeding-never-skips-a-chooser test** — the invariant holds in code; lock it against slice B refactors.
5. **[noted] evaluate→capture MCP test uses a stubbed pre-flight** — encodes the ≤4-call shape; the live evidence lives in the slice A done signal. Acceptable.
6. **[informational] Anchor element of the fixed set is dormant for evaluate→capture** — capture takes anchor as a param, so only widenReport rides the handoff there; consistent with the declared deviation.

## Seed grammar sharpening (from review dialogue)

Declaration lives entirely in the parent's dispatching junction option:

    dispatch:
        procedure: capture
        seed:
            widenReport: scanReport   # child field <- parent field
            anchor: candidateId

Right side = which parent field carries the evidence; left side = which child field it lands in (the child's gate contract). Omitted seed block = shipped default {widenReport, anchor}. Declared source missing from parent = load-time error. Default handoff finding nothing = serve note.

## Related work not duplicated here

- Serve misfires (chooser identifier absent from pending_chooser; fields-nesting mismatch vs report_schema): 20260705-121536-s-tac-keb.
- Evaluation-as-done shape into slice B specs + evaluate revision: 20260705-104004-s-prc-mes.
