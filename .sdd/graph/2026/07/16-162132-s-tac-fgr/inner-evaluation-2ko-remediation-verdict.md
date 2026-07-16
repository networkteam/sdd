# Inner re-evaluation: mutation-target recovery remediation (s-tac-spx / commit f13a059)

Second inner-lens run over branch `codex/2ko-local-mutation-recovery`, evaluated 2026-07-16 after remediation commit `f13a059` ("fix: close mutation recovery evaluation findings", 21 files, +851/−45) landed to clear the gap 20260716-150424-s-tac-idr raised by the first inner evaluation 20260716-145957-s-tac-5ua. Method: mechanical gates re-run directly; Opus re-review of the remediation commit; both defect fixes spot-checked by direct inspection.

## Mechanical gates

gofmt clean · go vet clean · go test ./... green · golangci-lint 0 issues — matching the closing done's claims.

## Prior findings — verdicts

### A) Fail-loud release errors — FIXED + PROVEN
Every recovery release path now propagates via named return + deferred `errors.Join`: `RecoverMutation` defer (application/recovery.go:327-333, covering discard/abandon-unknown/finalize-retry), `ReconcileMutation` defer (recovery.go:230-234), apply handing off with `release = false` to `applyOnAcquired`'s joining defer, and acquire-failure early returns joining both errors. No bare `_ = acquired.Release()` remains. Proven by `TestRecoveryNonApplyPathsSurfaceTargetReleaseErrors` — injects release failures per verb, asserts terminal state and the surfaced error.

### B) Recover CLI dead-end — FIXED + PROVEN (primary scenario)
New `ReconcileMutation` (recovery.go:173): authorized through the recovery resolver with the distinct `RecoveryReconcile` verb, nonterminal (reconcile read + audited `recovery_attempt`; never applies, finalizes, discards, abandons, binds, or releases blobs), exactly one caller (`cmd/sdd/recover.go:53`), absent from every read/startup/resume surface — the no-automatic-replay rule holds. The CLI reconciles first for interactive, non-legacy, unknown items and re-derives the verb menu from refreshed evidence, so apply/discard are offered on the first run for the crash-mid-write case. Proven by `TestReconcileMutationRefreshesIntentOnlyProjectionBeforeVerbSelection` (unknown → reconcile → not-applied-awaiting-decision → apply → applied/recovered, reconcile attempt asserted in the log) plus `cmd/sdd/recover_test.go` predicate tests (`TestRecoveryVerbMenusCoverEveryActionableProjection`, `TestRecoveryInteractiveSelectionReconcilesUnknownBeforeOfferingVerbs`, `TestParseRecoveryVerbCoversEveryPublicVerb`).

### C) Projection gap — FIXED + PROVEN
`renderRecoveryNotices` emits host-neutral text ("a pending write awaits explicit recovery: <id> · <state> · <target>", no command named), wired into `Info`, `View`, and the `sessionInfo` query consumed by the dialogue and catch-up procedures. Asserted by `TestReadSurfacesNeverReplayPendingMutation` (which also re-proves read surfaces reconcile 0 times and release 0 blobs), `TestCatchup_HappyPath`, `TestUserDialogue_OpeningServeAndQuietConclude`.

### D) Revalidation fragility — FIXED + PROVEN
`equalEntryDocuments` (revalidation.go:56-66) replaces `reflect.DeepEqual` with canonical-JSON byte comparison, collapsing YAML-vs-JSON scalar type differences (time.Time vs string, int vs float64) while rejecting genuine divergence; marshal errors propagate. Empirically confirmed that `TestPreparedRevalidationToleratesJSONRoundTripScalarTypes` would have failed under the old code — a genuine proof, not adjacent coverage.

### E) Verification matrix — all 12 previously-unproven rows now genuinely proven
1. apply verb end-to-end — `TestReconcileMutationRefreshesIntentOnlyProjectionBeforeVerbSelection`
2. legacy-v1 bind-target authorized/audited — `TestLegacyIntentRequiresAuthorizedAuditedTargetBinding`
3. negative verb gates (discard-after-applied; discard/apply-while-unknown; abandon-after-definitive) — `TestPreparedTransitionRecoversUnknownApplyAndFinalizer`, `TestReadSurfacesNeverReplayPendingMutation`, `TestPreparedTransitionPersistsNotAppliedAndRejectsStaleBinding`
4. legacy-v1 never infers target — `TestLegacyIntentRequiresAuthorizedAuditedTargetBinding`
5. attachment bytes home staging → target graph — `TestPreparedAttachmentCrossesHomeStagingIntoTargetGraph`
6. concrete default target independent of cwd — `TestCreateEntryResolvesConcreteDefaultWithoutCWDAndReleasesAroundLLM` (t.Chdir)
7. acquisition never spans preflight/LLM — same test (LLM callback fails if target active; acquisitions/releases balanced)
8. zero/multiple/detached/changed-HEAD acquisition failures — `TestMatchingWorktreesExactBranchAndDetached` + `TestGitWorktreeAcquirerRejectsMultipleMatchesAndChangedHEAD`
9. all three implementation modes — `TestImplementation_RoutesBaseAndWorkBranchesInEveryMode`
10. CLI verb selection — `cmd/sdd/recover_test.go` (predicates; the Action closure itself proven across two layers, not one driving test)
11. actionable + terminal projections incl. unknown and applied-finalization-pending — reconcile/finalizer/read-surface tests above
12. foreign-project target rejection — `TestPreparedTransitionRejectsEmptyTargetAndStructuredDivergence` (ErrorWriteDenied, 0 events persisted)

## Residual findings (minor, non-blocking)

- **F-1 — reconcile-first soft dead-end on an edge path**: an unknown item whose target acquisition fails, or whose reconciliation stays non-definitive, now surfaces `ReconcileMutation`'s error and the interactive flow stops before the verb menu — where abandon-unknown was previously offered directly. Recoverable: the error names abandon-unknown, and explicit `--verb abandon-unknown` bypasses the refresh. The primary scenario (reachable target, definitive not-applied) is strictly improved.
- **F-2 — untested error branches**: `ReconcileMutation`'s acquire-failure path (recovery.go:213-229) and non-definitive-reconcile return (recovery.go:255-257) have no coverage.
- **F-3 — low risk**: `local/git_target.go` switched worktree-list/symbolic-ref to `CombinedOutput`, merging stderr into parsed output; benign for these porcelain/quiet commands, and stray stderr cannot forge a branch match.

## Judgment

Sound and landable. The closing done's claims held under adversarial re-review with no overreach; closing 20260715-170623-d-tac-2ko and 20260716-150424-s-tac-idr is justified. The residuals are follow-up material, not landing conditions. Outer validation remains uncovered by scope; branch landing stays with the host lifecycle.
