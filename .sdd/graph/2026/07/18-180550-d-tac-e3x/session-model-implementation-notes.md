# Session-model realization — code-grounded implementation notes

Companion to the plan realizing the session-model contract (d-cpt-9of). Maps each slice to concrete files, functions, and tests, and records every decision settled in the planning dialogue so implementing sessions inherit decisions, not guesses. Only code is left to implementation.

## Settled decision register

1. **Apply retry bound: 3 attempts** (read fresh → revalidate → apply), engine-internal and invisible to the agent — a capture that lost a race simply succeeds milliseconds later. Only three consecutive lost races surface, as `ErrorGraphConflict` (existing code, no new one) inviting an ordinary re-try of the write — never recovery.
2. **Both write paths fixed**: `CreateEntry` and `ReplaceSummary` (both pin at prepare time); recovery replay verbs apply fresh-revision the same way; `Reconcile` untouched (it resolves unknown outcomes, not conflicts).
3. **`ExpectedGraphRevision` stays in `PreparedTransition` as provenance** — field kept, CAS operand changed to the revalidated fresh revision.
4. **`session` is REQUIRED** on `next`, `start_procedure`, `park`, `stage_attachment`, and abandon-by-instance — the agent carries the handle always, identical over stdio and remote; no connection-default resolution exists. The single exception: `resume_session`'s handle stays optional because its no-args form is the compaction escape (see 15).
5. **Connection state shrinks to two caches**: served-block content-hash dedup, and consent memory (which session this connection attached). Neither is authority; both are reconstructible.
6. **One attached session per connection**; attaching to another applies the leave rule to the previous (park if moves open, conclude if quiescent) with cause `switch`. Multi-window concurrency is multi-connection.
7. **The attach funnel exists from Slice 2**: a stateful call naming a session this connection has not attached fails typed — "attach with resume_session". Consent fields slot into that same single point in Slice 4.
8. **Codec version stays 1, tolerant load both directions** — legacy holder JSON ignored on read; old binaries see nil holder and still function. In-place store evolution, same policy as the vector index.
9. **Cause enum closed at six**: `disconnect`, `switch`, `shutdown`, `claim`, `conclude`, `abandon`.
10. **The attachment stamp rides every session append including read-log events**; serves become pure reads (`Reopen`, `ServeShell`, `ServeAll` no longer write). Foreign connections' free reads never touch a session, so recency stays truthful.
11. **`claim` history records carry the consenting `userWords`** — consent is auditable in the session log.
12. **New error code `ErrorSessionDisplaced`** carrying the current attachment and ending cause; message varies (taken over / abandoned / concluded). The version-race `ErrorSessionConflict` stays internal: current attachment still mine → reload, retry once, invisible.
13. **Recency threshold: 15 minutes, one constant** in the application package (not config), used for both the takeover requirement and the active/idle listing label. Rationale: misclassifying active-as-idle is low-harm (attach still needs the user's words; the displaced side is notified at its next write); misclassifying idle-as-active costs one extra takeover question — so err long.
14. **Cross-device hot handoff mid-move is a non-goal.** What survives an attach is the session log only: step position, collected state fields (draft bodies, interview transcript, widen reports), staged blobs, label — never the other agent's conversational context. Attach-from-elsewhere targets idle or parked sessions (junction-parked work resumes with near-full fidelity — proven when a parked capture survived a client restart to completion). `takeover` is a sovereignty escape hatch (hung client, runaway autonomous session, dead branch with a fresh stamp), not a continuity feature; the takeover response states the fidelity limit. Resume-fidelity improvements stay with s-cpt-m6m.
15. **`resume_session` keeps an optional handle + gains `userWords`, `takeover`, `fullReplay`.** No-args on an attached connection re-serves its own session (post-compaction reorientation, the d-tac-zm2 capability); no-args unbound returns the session list to re-establish from. `fullReplay: true` clears served memory once for agent-declared context loss; repeated `start_session` remains the second full-reorientation escape.
16. **The shell owns framing composition.** The engine always supplies the info block (participant, language, search modes) — the part every shell needs. All other lanes — aspirations, guiding directives, focus, participants, the new recent-moves lane, the open-work count — are declared by the shell procedure's spec and rendered server-side with the existing injection mechanism (the one catch-up's lanes use), deduped as today. The hardcoded `workflowFramingLayout` constant moves out of `workflow.go` into the `user-dialogue` procedure entry. Consequences: an autonomous shell can declare a minimal set; projects can reshape framing without touching Go (aligned with graph-local procedures); the ≤25KB door assertion tests the default shell's declared composition.
17. **Own threads only, everywhere.** Junction serves (move-end, resume, conclude) list this dialogue's own open threads only; other dialogues are never pushed on any serve — session entry carries a one-line open-work count ("N open dialogues — list on request"), and the full labeled listing exists solely on demand via `list_sessions`. Today's `openThreadsRoot` merges both; it splits. The conclude threads-walk keeps its structural list (own threads).

## Slice 1 — Merge-clean graph applies (I8)

**Files:** `application/transition.go`, `application/write_api.go`, `application/recovery.go`, `local/local_graphstore.go` (tests), `sddtest` graph-store conformance.

Current defect chain: `CreateEntry` pins `targetSnapshot.Revision()` into `PreparedTransition.ExpectedGraphRevision` before pre-flight (≤2 min LLM) and summary (≤1 min LLM) run (`write_api.go:214`). `applyOnAcquired` re-reads a fresh snapshot, runs `revalidatePreparedTransition` against it (structural: refs re-resolved, graph rebuilt — passes for concurrent unrelated appends since the graph is append-only and resolution is monotone), then calls `acquired.Graph.Apply(ctx, prepared.ExpectedGraphRevision, …)` — the stale pin (`transition.go:115`). A moved revision returns `MutationNotApplied` and `transition.go:178` routes it to `ErrorRecoveryRequired` with no retry anywhere.

Changes: decisions 1–3. The bounded retry exists because the filesystem adapter's lock is per-call (`local_graphstore.go:108` takes its own mutex + file lock), so a concurrent *process* can still write in the milliseconds between `Current()` and `Apply()`. The retry is mechanical only — revalidation has no LLM component; pre-flight and summary are never re-run. The recovery queue becomes reachable only for `MutationUnknown` outcomes and non-conflict failures.

Tests: interleaved captures from two sessions with an artificially slow LLM stage — both land, zero recovery entries, pre-flight runs once per capture; genuine logical-path collision (two writers, same WIP marker path) still fails typed; conformance suite gains the merge-under-append case.

## Slice 2 — Handle-addressed tools and free discovery (I1, I4)

**Files:** `mcpapp/tools.go`, `mcpapp/session.go`, `mcpapp/instructions.go`, `application/workflow.go`.

- Decisions 4–7: required `session` on work tools; two-cache connection state; one attached session per connection with leave-on-switch; the funnel from this slice (interim: `resume_session` by handle behaves as today — bind + leave rule; consent fields arrive in Slice 4 at the same point). Loading a handle not in server memory reuses the existing replay path (`ResumeWorkflow`) — server memory is a cache, not an authority.
- A stateful call with no attached session and no valid handle fails typed naming both doors — `start_session` for fresh, `resume_session` to attach — with the parked list inlined (the current rejection shape, now actionable because attach is unbound-capable).
- `boundSession` stops being a gate: `list_sessions` and named `resume_session` work on a fresh unbound connection (`tools.go:584`, `tools.go:609` — `args.Session` honored before any binding check).
- Listings include every session with open work: participant, label, client name, last activity, derived active/idle tag — the HolderLive-hide filter is removed from `listSessions`, `openThreadsRoot`, and the inline rejection list.
- `serverInstructions` and all tool descriptions rewritten to the handle-carried model, host-neutral: the handle is the dialogue identity, retain it across compaction, recovery = free list + re-establish with the user. Descriptions must match implemented behavior exactly (the description/implementation mismatch class from the continuity trace), pinned by the tool-contract snapshot test.

Tests: named attach on fresh unbound connection (stdio and in-memory HTTP transports); unbound `list_sessions`; handle-required rejection; funnel error naming the attach path; switch applies leave rule with cause; tool-contract snapshot.

## Slice 3 — Delete holder machinery, attachment stamp (I2, I3, I7)

**Files:** `application/session_runtime.go` (major deletion), `application/workflow.go`, `application/sessionstore.go`, `local/local_sessionstore.go`, `sddtest` session-store conformance, `mcpapp/server.go`.

- Delete: `chooserHolderTTL`, the `ChooserKind` plumbing through `begin()`/`currentChooser` lease selection, `SessionHolder.ExpiresAt`/`Generation`, `HolderLive`, the expired-vs-explicit takeover distinction, and the per-operation `BindSession` load+append (two loads + two CAS appends per operation drops to one).
- Replace `Holder`/`HolderHistory` in `SessionMetadata` with `Attachment{Subject, ClientName, ClientVersion, MCPSessionID, LastActivity}` and `AttachmentHistory []AttachmentRecord{Attachment, EndedAt, Cause, UserWords?}` per decisions 9–11. The stamp updates inside `appendStoredEvent`'s metadata write.
- `verifyBinding` reduces to subject/project immutability plus version CAS; attribution for conflict messages comes from the stored attachment, not a generation fence.
- Compatibility per decision 8. Session status becomes derived-only: open moves + attachment recency classify active/parked; a server killed without a leave event classifies correctly from the store alone.
- `leaveSession`/`handleDisconnect`/`Shutdown` in `mcpapp` pass their specific cause down; `Release` records it.

Tests: `TestSessionHoldersUseChooserTTLTakeoverAndCASFencing` replaced by clock-free equivalents (injected `Now` only for recency labels); incumbent-continuity (arbitrary elapsed time, no competing claim → next operation succeeds); one-append-per-operation; serves-don't-write; legacy-metadata load; kill-without-goodbye classification.

## Slice 4 — Structural consent and informative conflicts (I5)

**Files:** `application/workflow.go` (ResumeWorkflow / claim path), `mcpapp/tools.go`, `application/errors.go`.

**Own vs. foreign.** A session is a connection's own if this connection opened it or already attached with consent. Own-session operations — including no-args post-compaction reorientation — never need consent. Consent applies exactly once, at the Slice 2 funnel point (`resume_session`).

**Decision table at attach**, driven by the target's attachment stamp and the 15-minute constant (injected clock in tests — labeling, not enforcement):

| Target attachment | attach with `userWords` | + `takeover: true` |
|---|---|---|
| none, or ended with cause | attaches; history `claim` | — |
| idle (lastActivity older than 15 min) | attaches — the expected cross-client path, targeting parked/idle work (decision 14); a 2-hour junction think means idle, never evicted | — |
| recent (within 15 min) | typed refusal naming the holder: client, last activity, "may be actively driven; pass takeover with the user's explicit ask" | attaches; history `claim` over the named holder |
| `userWords` missing (any row) | typed rejection naming what is required — the mwd regression | — |

Success responses name the previous attachment; the takeover response states the fidelity limit (decision 14). Consent is procedural, not security — same trust convention as user choosers; cross-subject access on a hosted server is blocked by subject scoping (authorization), orthogonal to consent.

**Displacement detection at the write** (decision 12): when B attaches, the stamp append bumps the session version; A's next append interprets the CAS conflict — still mine → benign retry, invisible; someone else → `ErrorSessionDisplaced` built from stamp and history ("taken over by Claude Code at 14:32 (claim); your position may be stale — reorient with resume_session(s_X) or start fresh"). This is the 2sp resolution: a branched conversation learns of divergence at its first write, with names and a next step. The stamp provides attribution; CAS provides the tripwire; no lease.

**Abandoned-while-attached** (D1): `abandon(session)` appends abandon events plus a history record with cause `abandon`, actor, and reason; the still-attached client's next call gets "abandoned by Christopher at 14:40, reason: …" through the same interpreted-conflict path.

**Client-restart (B1) falls out naturally:** new connection, agent still holds the handle, first `next` hits the funnel error, agent attaches carrying the user's ask already present in the conversation — two calls total.

Tests: consent-missing rejection; takeover-required refusal naming the holder; competing clients — loser told who won, reorient succeeds; abandoned-while-attached message content; benign same-writer retry invisible.

## Slice 5 — Converging reorientation and slim door (I6, A1)

**Files:** `mcpapp/tools.go`, `mcpapp/server.go` (servedBlocks), `mcpapp/openthreads.go`, `application/workflow.go` (framing/door composition), `internal/engine` spec surface (lane declarations), embedded base procedure text (`user-dialogue` shell units).

- Decision 15: drop `forgetConnection` from the same-session resume path (dedup makes repeats converge to stubs); `fullReplay` as the explicit one-shot; repeated `start_session` keeps full reorientation.
- Decision 16: shell-declared framing lanes. Engine supplies the info block always; the `user-dialogue` spec declares aspirations, guiding directives, focus, participants, a recent-moves lane (the most recently created entries, brief one-line rendering, small n, ~2.5KB cap), and the open-work count line. `workflowFramingLayout` deleted from Go. Dedup unchanged (content-hash over rendered lanes).
- Decision 17: `openThreadsRoot` splits — own threads at move-end/resume/conclude serves; the count line at session entry; other dialogues only via `list_sessions`. Shell routing text updates to match; text changes ride the embedded base-entry refresh path.
- Serve-size assertions: a reorientation response never exceeds the original serve; repeated resume payloads monotonically non-increasing; default-shell door payload ≤ 25KB asserted in a test.

## Ordering and execution

Slice 1 first and independent — it stops recovery-queue pollution immediately. Slice 2 → 3 → 4 sequential (3 removes what 2 routes around; 4 needs 3's stamp for attribution). Slice 5 depends on 2 only and can run parallel to 3/4. Each slice lands with `go vet`, `golangci-lint`, full tests, and a commit-citing done signal; implementation sessions run engine mode, so the work dogfoods the surface it changes.

Maintenance-surface framing: net deletion — roughly half of `session_runtime.go`, the lease branch matrix, the time-dependent test class, and one CAS append per operation. Additions: one required tool argument, one consent check with three fields, one cause enum, one lane-declaration spec surface, and a bounded retry loop. No new dependencies; port signature changes limited to `SessionMetadata`'s shape (conformance suites updated in the same slice).

## Deliberately out of scope

- Dialogue fork tool (C3 answered by conflict-at-write; capability waits for demonstrated need).
- Observer mode (read-only following of a live dialogue).
- Door verb merge: `start_session` (fresh) and `resume_session` (attach) remain two tools — semantics change, names stay.
- Resume-fidelity improvements (s-cpt-m6m) — surfaced honestly in takeover UX, not solved here; cross-device hot handoff mid-move is an explicit non-goal.
- Cleanup of the existing ~34 pending recovery items — separate grooming; worth checking then how many were I8-class false conflicts.
