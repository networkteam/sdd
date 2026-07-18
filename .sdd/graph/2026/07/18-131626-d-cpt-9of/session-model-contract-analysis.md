# Engine session model — observation-first analysis and behavioral contract

Date: 2026-07-18
Dialogue: Christopher + Claude, anchored on the two live failure traces of 2026-07-18.

## 1. Scope and evidence base

This record reconstructs the engine's current session behavior from captured traces, source, and tests; separates established behavior from hypothesis; identifies which mechanisms are essential and which complexity causes regressions; and derives the behavioral contract (invariants + user stories) that the committed redesign realizes.

Evidence read in full:

- s-tac-eqv (engine session continuity and reorientation trace) with attachment
- s-tac-mwd (unconsented resume of a semantically matching parked dialogue) with attachment
- s-tac-2sp (branched conversation silently locked out), s-tac-y2z (live/parked ambiguity in open-threads)
- d-tac-bfc, d-tac-dbk, d-tac-zm2, d-tac-wjq, d-tac-nqo, d-prc-dlg
- Source: `mcpapp/tools.go`, `mcpapp/server.go`, `mcpapp/session.go`, `application/session_runtime.go`, `application/workflow.go`, `application/transition.go`, `application/revalidation.go`, `application/write_api.go`, `application/graphstore.go`
- Tests: `mcpapp/server_test.go` (unbound rejection is pinned by `TestResumeNoSessionUnboundListsParked`, `TestUnboundRejectionInlinesParkedSessions`), `application/session_runtime_test.go`

## 2. MCP identity facts

MCP offers no stable identity for an agent conversation:

- stdio: no session ID; identity is the server process, which dies with the client. Client restart, `/branch`, or a new tab each spawn a fresh `sdd serve`.
- Streamable HTTP: server-assigned `Mcp-Session-Id`, echoed per request; survives network blips (stream resumability via Last-Event-ID) but not client re-initialization; per client process, not per conversation.

Of the three states in play — agent context (LLM conversation), SDD session, MCP session — the MCP session is the only one that cannot carry dialogue continuity, and the current design keys ownership to it. The agent context is the only stable carrier of "which dialogue am I in", because it is the dialogue continuity. The SDD session handle is the cookie: minted by the engine (server-authoritative, subject-scoped when hosted), adopted and carried by the agent conversation, presented back explicitly. It needs no secrecy: locally the filesystem is the trust boundary; hosted, authorization comes from the authenticated subject. Agent session and SDD session are 1:1 at any instant but not identical: lifetimes diverge (parked sessions resume from new conversations; one conversation touches several sessions), the server can only validate one of them, and branching makes "agent session identity" ambiguous by construction — a distinct SDD session ID is what gives the branch case a principled answer.

## 3. Current-state model

Four overlapping identity/ownership layers on one dialogue:

1. **Durable session** — JSONL event log + metadata, `s_…` handle, versioned; every append is CAS-guarded (`Sessions.Append(id, expectedVersion, …)`, typed `ErrorSessionConflict` on a lost race). The dialogue's real identity and only true integrity mechanism.
2. **Holder lease** — in metadata: subject, MCP session ID, client info, generation, LastActivity, ExpiresAt with TTL by chooser kind (gate 2m / agent 5m / user 30m). Refreshed by `begin()` before every operation via a full `BindSession` — itself a load + CAS append, so every `next` costs two loads and two CAS writes. Expired holders are silently replaced (`expired_takeover`); live foreign holders require explicit takeover.
3. **In-memory connection binding** — `map[*mcp.ServerSession]*shellSession`; gates every stateful tool including `list_sessions` and `resume_session` even when a session handle is explicitly named. Lost on any disconnect or server restart.
4. **Served-once memory** — per-connection content-hash dedup of rendered blocks; same-session `resume_session` deliberately clears it (`forgetConnection`) before replaying (built for the Codex compaction recovery, d-tac-zm2).

Plus the leave rule (disconnect/switch → park if moves open, auto-conclude if quiescent; holder history records only `Reason:"released"`) and the listing filter (`HolderLive || no open moves` → hidden from listings and open-threads).

## 4. Established behaviors (trace + code + tests)

- **E1 — The unbound trap.** An unbound connection is rejected by `list_sessions` and by named `resume_session` (`boundSession` is checked before `args.Session` is read; `tools.go`); the rejection text claims "resume_session picks one up", which is false. The only unbound path forward is `start_session`, which always mints a new durable session. Pinned by tests — enshrined design, not a slip. (Trace eqv steps 8–11.)
- **E2 — Replay amplification.** Same-session resume clears served memory then re-serves in full: catch-up compose ~43,086 bytes + framing ~12,697 bytes ≥ 55.8KB per reorientation, repeatable without bound. (Trace eqv payload reconstruction.)
- **E3 — Wrong lease kind.** `Advance` selects the lease TTL from the pre-transition step's chooser (`currentChooser`), so landing on a user junction from a report/gate step leaves a 2-minute lease guarding a step where the user may think for 30. (Trace eqv lease observation.)
- **E4 — Divergent views of one session.** After lease expiry the incumbent keeps working (same-holder refresh succeeds), but every other connection sees the session as parked — advertised as claimable in open-threads while actively driven. Expiry does not evict the incumbent; it advertises their session. If a third party claims during that window, the incumbent is displaced silently mid-thought.
- **E5 — Cause-blind lifecycle records.** "released" covers disconnect, switch, and shutdown alike; eqv's central "why did the holder go away" is unanswerable from persisted state by construction.
- **E6 — No structural consent for switching.** One `resume_session(session)` call from a bound fresh shell switches, auto-concludes the shell, and replays the old dialogue. "Recommend, never auto-run" lives in prose only; the mwd agent walked through it on a label match.
- **E7 — Live sessions are invisible.** The HolderLive filter hides a held session entirely: a branched or second client cannot see it, resume it, or detect the divergence (2sp). This resolved y2z's live/parked ambiguity by deleting information instead of adding it.
- **E8 — Concurrent graph appends are punished.** `CreateEntry` pins the target snapshot revision, then runs pre-flight (≤2 min LLM) and summary (≤1 min LLM); `ApplyPrepared` revalidates the batch against a fresh snapshot (which passes for concurrent unrelated appends — the graph is append-only, so ref resolution is monotone) and then CASes the adapter apply on the minutes-old pinned revision. Revision moved → `MutationNotApplied` → no retry anywhere → `ErrorRecoveryRequired`: the write is filed into the explicit-recovery queue. Any capture in any other session (or a plain git commit touching the graph) during another session's LLM window sends that session's write to manual recovery. (`write_api.go:214`, `transition.go:112–179`.)

Hypotheses, deliberately not asserted: why the eqv agent called resume twice (client-side truncation of the 43KB serve is plausible); what released the holder at 09:00:05 (E5 makes it unknowable); whether the 2-minute lease contributed to the lockout. E3→E4→E6 compose into a plausible system-level narrative (wrong TTL advertises an active session as parked; open-threads offers it; an agent takes the offer) — each link individually established, the composition unproven.

## 5. Diagnosis — guarantees vs. machinery

Recurring disease, three occurrences: a guarantee already supplied by a simpler mechanism, duplicated by a stricter one whose false positives become the regression.

| Mechanism | Intended guarantee | Actually supplied by | Regression caused |
|---|---|---|---|
| Connection binding as gate | session context for tools | needed only as default handle | E1 lockout, E7 |
| Holder lease TTL per chooser kind | no concurrent hijack | session-log CAS fencing | E3, E4, silent takeover window |
| HolderLive listing filter | don't offer live sessions | labeling (information) | E7 invisibility |
| forgetConnection on resume | fresh orientation after context loss | content-hash dedup re-serves changed bytes | E2 replay loops |
| Generic "released" reason | — | — | E5 undiagnosability |
| Per-op BindSession | lease freshness | fold stamp into event append | 2× store traffic per call |
| Graph revision CAS against prepare-time pin | write validity | revalidation under the acquired-target lock | E8 false conflicts into recovery |

Log integrity under concurrent writers is supplied completely by CAS alone: a stale writer's append fails typed; no interleaving is possible. The holder adds attribution to the failure, not safety. Dialogue coherence (two agents driving one dialogue) is a consent/awareness problem, not an integrity problem — the lease treats it as enforcement, keyed to the wrong identity, deriving liveness from time.

## 6. Invariants (the behavioral contract)

1. **I1 — A dialogue's identity is its durable session handle.** Never a transport connection. The handle is presented explicitly on every stateful call; the agent conversation carries it (compaction-durable channels instruct retention; every serve response repeats it). Any client presenting the handle can address the session, bound or not, on stdio and remote alike.
2. **I2 — Log integrity comes from CAS, and only CAS.** Any number of concurrent clients; a raced append fails typed and the loser re-reads. No ownership machinery participates in integrity.
3. **I3 — Time passing never takes a session from the client driving it.** Staleness may change what another client is offered (a claimable session), never what the incumbent can do.
4. **I4 — Discovery is a free read.** `list_sessions` and attach-by-handle work unbound. Every session with open work is listed — active ones labeled (client, last activity recency), never hidden.
5. **I5 — Crossing into another dialogue requires explicit user intent, structurally.** Attaching to a session this conversation did not open requires the user's verbatim ask (the user-chooser convention); attaching to one that is actively driven additionally requires the explicit takeover flag, with the response naming the current attachment. A fresh user request is never consent. Takeover resumes from the recorded state only — the working conversation in the other agent's context stays behind (resume-fidelity caveat, s-cpt-m6m).
6. **I6 — Reorientation converges.** Already-served identical blocks stub; a deliberate full replay is an explicit, one-shot agent-declared request (the compaction story), never inferred from a repeated call. No sequence of calls replays unboundedly.
7. **I7 — Every lifecycle change records its specific cause** (disconnect, switch-to, shutdown, explicit claim, user conclude, abandon-by-whom). Session status (parked/active/concluded) is derivable from the store alone — a session whose server died without a goodbye is still correctly classified.
8. **I8 — Concurrent graph appends merge cleanly.** Entries are immutable; a write from session B is never failed, delayed, or sent to recovery because session A appended meanwhile. A write conflicts only on a genuine logical-path collision. The recovery queue is for unknown outcomes (crash mid-apply), never for lost races. Realization shape: revalidate against the fresh snapshot at revision R′ under the acquired-target lock — as today — then apply with expected revision R′, not the prepare-time pin (which stays recorded in the durable intent as provenance).

Governing distinction: **the graph is a shared ledger — concurrent appends are the normal case and must merge silently; the session log is one dialogue's spine — concurrent appends are the anomaly and must surface as dialogue.** Same CAS primitive, opposite correct responses. The current code has it backwards on the graph side (false conflicts) and too silent on the session side (lease takeover).

## 7. User stories (with dialogue refinements)

### A. Starting and continuing

- **A1 — Fresh start.** Enter the engine; one step later I'm oriented. The opening serve is slim: recent graph movement in headline form, one line for open work ("3 open dialogues"), details on request. A full catch-up and the session list are choices, never front-loaded. Nothing resumes on its own, and the door does not advertise other sessions eagerly — over-offering is what armed the mwd failure. Same on stdio and remote.
- **A2 — "Let's continue where we left off."** The agent disambiguates what the user means (an SDD dialogue vs. captured entries — users won't use those words), confirms which dialogue, attaches; the exact step is restored. The user's verbatim ask is what authorizes the switch — one required field on attach, not a new gating procedure.
- **A3 — Fresh request resembling parked work** (mwd). The agent may offer "there's a parked dialogue on exactly this — continue it, or start fresh?" but it asks. Fresh is the default; resuming is structurally impossible without the user's words.

### B. Surviving the machinery

- **B1 — Client restart / crash / new tab.** The dialogue survives anything that happens to the client. Failure modes enumerated: (1) clean exit → leave recorded with cause; (2) crash → transport close path, same; (3) stdio server dies with the client → no leave event is ever written, so parked-ness must be derivable from the store (open moves + attachment recency), never dependent on a goodbye.
- **B2 — Transport blip (remote).** HTTP reconnects with a new MCP session ID are a non-event by construction — nothing durable is keyed to the connection. Critical for the hosted engine.
- **B3 — Long think at a junction.** Hours or days later, the answer still works. Normal, not an edge case.
- **B4 — Compaction mid-session.** The agent declares context loss explicitly and gets one full re-serve (position, steps, schemas); an accidental repeat costs stubs, not another 55KB. Note: id-less "resume whatever this connection was in" only ever worked on stdio; with explicit handles, recovery works identically on both transports (handle-less call → free session list → re-establish with the user).
- **B5 — Server restart.** Everything above holds because nothing durable lived in server memory.

### C. More than one client (where consent lives)

- **C1 — Two windows deliberately.** Concurrent sessions in one repo / multiple worktrees are normal and must not confuse agents; another session's existence reaches an agent only when the user asks. Each is visible in listings, labeled honestly ("attached: Claude Code, active 40s ago").
- **C2 — Takeover of my own stale session.** From another machine (hosted, same subject): session shows attached-but-idle; the user's explicit ask authorizes takeover; the displaced client's next call is told plainly who took over and when. Expectation set honestly: takeover resumes the recorded state; conversational nuance in the old agent's context does not transfer.
- **C3 — Branched conversation** (2sp). Both branches carry the same handle; the first write from the second branch hits the CAS conflict and receives an informative answer — who advanced it, when, where it stands — plus its options (follow along, take over with user consent, start fresh). Silent lockout is the forbidden outcome. No dedicated branch/fork tool for now; a deliberate dialogue-fork capability waits for a demonstrated need.
- **C4 — Same user, two agents racing.** The log never corrupts; the loser of any race is told immediately at the write and turns it into dialogue. Graph captures from other sessions never interfere (I8).

### D. Stewardship

- **D1 — Housekeeping.** All my dialogues visible with honest labels; teardown in one step without resuming. Abandon appends a marker (append-only, log stands); a still-active agent's next call on an abandoned session gets "abandoned by X at T, reason R", not a bare conflict.
- **D2 — Auditability.** Cause-specific lifecycle records are what let failures like eqv be diagnosed from persisted state.
- **D3 — Hosted, non-developer participant.** All stories hold when the client is a webapp session; nothing assumes a filesystem, a process, or a single subject per server. Sessions are subject-scoped.

## 8. Smallest viable lifecycle

Two layers instead of four:

- **Durable session** (unchanged core): log + metadata, CAS-versioned; plus an **attachment stamp** `{subject, clientName, mcpSessionID, lastActivity}` written as part of each event append (the separate per-op BindSession round trip is deleted). Information for consent decisions, never authorization.
- **Connection state, demoted to convenience**: default session handle (so tools may omit it where unambiguous) and served-block hashes for payload dedup. Losing it loses nothing durable.

Lifecycle: open fresh (door default); every operation appends via CAS and stamps attachment; disconnect parks-or-concludes with specific cause; a session with an old lastActivity is claimable — claiming is explicit and user-consented, stamps a new attachment, and the displaced client's next append fails typed naming who and when, at which point it reorients. Divergence is detected at the only moment it matters (a write), by machinery that already exists.

## 9. Deleted / simplified

Deleted: chooser-TTL table, ExpiresAt, HolderLive, expiry-takeover semantics, pre-transition chooser selection (E3 evaporates), holder generations as fence (CAS version is the fence; a claim counter may remain for messages), per-operation BindSession (halves store traffic), the boundSession gate on list_sessions / named resume / abandon-by-handle, forgetConnection-on-resume (replaced by explicit full-replay flag), the live-session listing filter (replaced by labeling).

Kept: CAS, the event log, replay, the leave rule (with specific causes), served-once dedup, session labels, the shell model, door-opens-fresh.

Maintenance surface: removes roughly half of `session_runtime.go`, the lease branch matrix in BindSession, and the class of time-dependent tests; what remains depends only on store contents, which conformance suites (`sddtest`) pin without clocks. No new dependencies.

## 10. Alternatives and failure modes

- **A. Status quo + point fixes** (fix E1 ordering, E3 chooser, unbound list). Keeps two ownership models forever; 2sp structurally unfixed; every future divergence between connection identity and dialogue identity (new harness, branch features, HTTP reconnects) produces a new lockout variant. Complexity added to patch symptoms.
- **B. Adopted: CAS-integrity + informational attachment + structural consent.** Honest failure mode: two clients that both obtained consent can interleave a dialogue; the log stays consistent, turns alternate; mitigation is detection at the write ("advanced by X, 20s ago — reorient?"), not prevention. Consent-via-required-userWords cannot verify semantics — it forces the agent through the ask, same trust level as user choosers everywhere else.
- **C. Hard single-writer lock, explicit release only.** Strongest coherence; a crashed client wedges the session until a force path exists, and the force path reintroduces exactly B's takeover UI, now mandatory. (This is where the lease came from; B keeps claimability and moves the decision to the user.)
- **D. Fully sessionless connection** (zero connection state). Purest, but served-once dedup genuinely belongs to the connection — payload memory is about this pipe's context window. B takes D's stance for identity and keeps the connection only for caching.

## 11. Test matrix

Clock-free except staleness labeling (injected Now):

1. Fresh open → attachment stamp recorded; slim door serve.
2. Disconnect with open moves → parked with cause `disconnect`; quiescent → concluded with cause.
3. Stale attachment → incumbent's next append still succeeds; other clients see claimable labeling. (TTL-expiry eviction of the incumbent must be impossible.)
4. Named attach on a fresh unbound connection → works, full serve (E1 regression test).
5. Competing clients → loser's append fails typed naming the winner; subsequent reorient succeeds (2sp regression test).
6. Attach to a not-own session without userWords → typed rejection (mwd regression test).
7. Repeated resume → payload monotonically shrinks to stubs; explicit full-replay serves once in full (E2 regression test; B4 preserved).
8. Stale transition (report against an already-advanced step) → typed conflict, no double-apply.
9. Two sessions capturing interleaved with artificially slow LLM stage → both land; no recovery entries; no pre-flight re-run (E8/I8 regression test). True collision (same WIP marker path) still conflicts typed.
10. Abandoned-while-active: abandoned session's still-attached client gets "abandoned by X at T", not a bare conflict.

## 12. Open points for the mechanics plan

- Door shape: keep `start_session` always-fresh with attach as the deliberate second verb (lean), or merge open/attach into one door tool with an optional handle. Behavioral contract is identical either way (fresh default, consent to attach).
- Slim-door composition: what exactly "recent graph movement, headline form" serves (bounded), with full catch-up as an offered move.
- Observer mode (read-only following of a live dialogue) — deliberately deferred; C3's conflict-at-write answer covers the branch case without it.
- Resume-fidelity (s-cpt-m6m) remains open and is honestly surfaced in takeover UX rather than solved here.
- Whether the ~34 pending recovery items of July 14–17 include E8-caused false conflicts (vs. the target-binding migration) — worth checking during cleanup, as validation of I8's value.
