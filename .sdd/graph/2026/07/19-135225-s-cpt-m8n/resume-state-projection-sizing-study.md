# Resume state projection — sizing and design study

Read-only design study (Opus subagent, 2026-07-19) sizing the realization of the handover-fidelity directive (d-cpt-0tm) against the observed state–serve asymmetry (s-cpt-hkx). Basis for the accepted split: contained honest-surfacing slice on the session-model worktree now, full projection-boundary design as its own plan. Also the concrete evidence for the serve-composition genericity question this file is attached to.

## 1. Where collected state lives, and where serves are built

Collected state lives in the instance Store, keyed by provenance. Each running `engine.Instance` holds a `*Store` (`internal/engine/instance.go:24-28`) whose `values map[string]storeValue` carries every param, state field, and engine write with a `Provenance` tag (`internal/engine/store.go:25-41`). `anchor`, `plan`, `widenReport` are declared `state` in the evaluate spec (`internal/baseprocedures/entries/20260703-200000-d-prc-evl.md:16-19`), so they land as `ProvenanceState` via `WriteState` (`internal/engine/store.go:122-141`). They survive handover because replay re-applies report/chooser events through `WriteState`/`importValue` (`internal/engine/session.go:758-779`, `store.go:249-277`) — exactly the "survived replay internally" the signal describes.

The store already exposes read paths that would project them: `Store.Export()` (all values with provenance, `store.go:231-237`), `Store.Get(name)` (`store.go:166-172`), `Store.TemplateContext()` (`store.go:196-202`).

Serves are built in three stacked mappers, and none projects running-instance state:

- **Engine `serve()`** (`internal/engine/instance.go:377-468`): for a running instance sets Step, Goal, ReportSchema, Missing (names only), Instructions (rendered unit). The only way a collected value reaches the agent today is if the current step's unit template happens to reference it (`renderUnit`, `instance.go:527-569`). `Produced` is set only on the terminal branch (`instance.go:384-388`) and `produced()` filters to `ProvenanceEngine` only (`instance.go:358-373`).
- **`WorkflowSession.publicServe()`** (`application/workflow.go:876-901`): maps `Serve` → `WorkflowServe`; no store material added.
- **`toRootServeResult()`** (`mcpapp/tools.go:1020-1063`): maps `WorkflowServe` → `ServeResult`; feeds every serve — advance, resume (`mapRootResume`, `tools.go:1088-1107`), nested base junctions.

The resume path: `ResumeWorkflow` (`workflow.go:222-284`) replays, `resumeResult()` (`workflow.go:523-536`) iterates running instances → `Serve(inst.ID)` → `publicServe`. The replayed store is fully in memory at `workflow.go:529` and its values are simply never copied into the outbound struct. That is the entire asymmetry.

Why the existing resume test passes despite the gap: `TestSessionResumeAcrossServers` (`mcpapp/server_test.go:1070-1178`) asserts collected evidence is visible at `playback` — but only because that unit template renders the collected summary; incidental to the step's prose, exactly the fragility the signal names.

## 2. Design options

**Option A — Collected-state field threaded through all three serve structs (full typed state, every serve).** Payload risk on the hot loop (plan/widenReport are free text, multi-KB); needs truncation caps and dedup-block plumbing to compose with served-once memory; fullReplay composes cleanly.

**Option B — Project collected state only on resume-type serves (contained, honest-surfacing).** Attach in `resumeResult()` / surface through `mapRootResume`, leaving the advance loop untouched. Growth confined to resume responses — rare, and where the agent genuinely lacks context; matches the signal (the failure was a *resumed* response). Foreign attach has empty served memory so full projection is honest. The minimal faithful-projection slice.

**Option C — Rendered "collected so far" digest block (filtered trajectory).** Labeled digest of set state fields with truncated previews plus significant chooser answers/transitions folded from the event log (`deriveWorkflowSummary`, `workflow.go:1142-1188`, extendable). Matches d-cpt-0tm's "filtered trajectory" language and its compression-labeling requirement — but the "semantically significant" boundary is exactly the design work the directive defers. Not a pre-merge slice.

## 3. Recommendation (accepted)

Ship **Option B** as the contained honest-surfacing slice; route Option A/C shaping to its own plan.

- Files: `internal/engine/instance.go` (+`Serve` field, populate in running branch; provenance-filtered accessor in `internal/engine/store.go`), `application/workflow.go` (`WorkflowServe`/`publicServe` or `resumeResult`), `mcpapp/tools.go` (resume mapping).
- Size: **S** (≤~100 lines) with a truncation cap; M if per-value dedup blocks are added.
- Test surface: `mcpapp/server_test.go` resume/reorientation suite (`TestSessionResumeAcrossServers`, `TestResumeReorientsCurrentSession`, `TestConvergingReorientation`, `TestFullReplayReservesOnce`, `TestNamedResumeOnUnboundConnection`, `TestReorientCarriesCompactionBreadcrumb`) plus a new regression: resume at a step whose unit does NOT render earlier-collected fields, assert they appear.
- Risks: tool-contract snapshot hash regenerates (certain, expected); resume payloads need a truncate-with-notice cap (mirror `guardViewSize`, `tools.go:906-915`); trust-machinery fields (`fieldPlaybackConfirmation`, `fieldPreflightOverride`) must stay excluded as `produced()` already does.

## 4. Contained slice or own plan?

Split: the minimal honest-surfacing half is genuinely contained and consistent with d-tac-e3x's scope ("honest surfacing" was in-bounds; fuller resume-fidelity was deliberately out). The full d-cpt-0tm invariant — projection boundary, filtered trajectory, completed ancestors, compression labeling — is unsettled conceptual scope that earns its own tactical plan; do not satisfy all of d-cpt-0tm inside the pre-merge worktree.
