# Evaluation run 1 — Claude Opus 4.8 (high) via Claude Desktop

Workflow MCP server experiment (20260609-234656-d-cpt-afn), slice-1 server (20260610-225912-s-tac-n9a).

## Setup

- Server: `sdd serve --transport http --addr 127.0.0.1:8765` with bearer token, run from the evaluation worktree against its graph.
- Tunnel: `ngrok http 8765 --request-header-add "Authorization: Bearer <token>"` — the tunnel injects the token because the desktop connector UI offers no custom-header auth; the public URL alone gets 401.
- Client: Claude Desktop, model Opus 4.8 (effort: high), custom connector pointed at the ngrok URL, no auth configured client-side. No system prompt, no skill content.
- Required client setting: tool access mode "tools preloaded" ("Tools bereits geladen"). Under dynamic discovery ("Tools bei Bedarf laden") the agent did not act on the connector tools at all.
- Mid-run server fix: the MCP SDK's DNS-rebinding protection 403'd all tunnelled requests (public Host header arriving via localhost); disabled on the bearer-guarded HTTP transport (commit 9425827) before the run proceeded.

## Scorecard

| # | Observed behavior | Verdict |
|---|---|---|
| 1 | Briefing composed (not dumped), session token held | hold — clustered by focus, prioritized next steps; all cited entry IDs verified real |
| 2 | Grounds before drafting, sensible refs | hold — grounded unprompted; challenged fact-vs-done framing before drafting |
| 3 | Waits for explicit confirmation, folds dialogue into description | hold — multiple correction rounds, re-played, captured only on literal confirmation |
| 4 | Revises substantively on blocking finding | hold — supplied missing acceptance criteria and corrected a ref kind in one revision; marked ACs as own proposal needing review; no thrash, no skip |
| 5 | Verifies summary, reports ID, suggests next steps | hold — checked summary fidelity on every capture; took reasoned positions on low advisories |

**Run verdict: full compliance — all five behaviors hold with zero skill content.**

## Key observations

- **Gate as teacher**: the just-in-time instructions deliberately never mention acceptance criteria. The agent learned the plan-AC convention entirely from the blocking finding text and applied it correctly in one step.
- **Dialogue-first beyond the bar**: before drafting the plan it challenged whether committing a plan ahead of the pending experiment verdict was right, recommended a gap signal instead, and yielded only on explicit user direction — then encoded the sequencing concern into the plan's ACs and a depends-on ref.
- **Side findings for iteration**:
  - Captured entries list only the human participant; instructions say nothing about participant inference for the contributing agent.
  - Pre-flight ref-kind advisories repeatedly suggested sharper kinds than the agent's `related`-floor choices (defensible both ways; agent reasoned its choice each time).
  - One minor mechanism imprecision (ngrok "rewrites" vs preserves the Host header) passed into a confirmed fact body — content-drift class, below supersedure threshold.

## Entries produced by the run

- 20260610-231642-s-ops-nfn — fact: DNS-rebinding protection disabled for tunnelled evaluation, bearer token as guard, production caveat
- 20260610-232018-s-ops-mgg — fact: Claude Desktop tools-preloaded requirement
- 20260610-232719-d-tac-47d — plan: production auth on the HTTP transport, gated behind the experiment verdict

## Transcript (verbatim, in order)

### Turn 1 — "Where are we with this project?"

> I need to check the project state before I can answer.Three focuses are active in parallel:
>
> **MCP / external-agent (newest, hottest).** As of last night you shipped slice 1 of the workflow MCP server (commit f92fed5): `sdd serve` with per-session state machine, four dialogue-loop tools, a structurally enforced grounding gate, bearer-token transports, and a full integration suite — verified live. That closes the build activity and unblocks the two-client evaluation runs the workflow-MCP experiment was waiting on. The CLI-parity MCP server plan (d-tac-0v4) remains the high-scoring driver here.
>
> **Type-system / refs + topics.** Largely steady. Recent CLI work landed: `sdd show` restructure (YAML frontmatter + glamour rendering), `sdd stats` palette alignment, interactive terminal output for `sdd index`/`sdd search`, supersede-chain resolution to live head, and a capture-time block on superseding non-head entries. The topics focus has a stalled directive (d-tac-nbp, retroactive grooming) waiting on Plan-1 tooling.
>
> **Open and warm — pre-flight is the recurring sore spot.** Multiple open signals point at the same class of defect: pre-flight verdict oscillation and severity/conclusion divergence on ref-kind findings (s-prc-pex, s-prc-lbv, s-prc-vvd), all leaking the "applicable ref-kinds never reach high" guarantee. There's also a caching gap (s-tac-osb) where volatile graph content in the system block defeats prefix reuse.
>
> Concrete next steps, in priority order:
> 1. **Run the two-client evaluation** now unblocked by the slice-1 ship — that's the verdict the whole MCP focus is gated on.
> 2. **Converge the pre-flight oscillation cluster** — three open signals describe one leaking guarantee; they likely want a single tie-break/convergence criterion rather than three separate fixes.
> 3. **Decide the stalled topic-grooming directive** — it's been waiting; either ship Plan-1's tooling or explicitly defer.
>
> Which one do you want to pick up?

### Turn 2 — observation given: DNS-rebinding 403 / disabled with bearer-token rationale; "Record that."

> Before I propose an entry, let me ground this against what's already in the graph.One thing to challenge before I draft: you said "we disabled it." Is that an implemented-and-committed change (a `done` signal), or a noticed fact about the transport's current configuration (a `fact` signal)? It reads to me like the latter — you observed the 403, made a config decision, and it's now live as part of slice 1. But there's no commit hash here, and slice 1 already shipped at f92fed5. Did this land inside that commit, or is it a separate change?
>
> I'll assume it's a **fact signal** recording the configuration decision and its rationale, since that's what "record this" most naturally means and there's a real trade-off worth capturing. If it's actually a fresh committed change you want tracked as `done`, tell me and I'll reframe with the commit pointer.
>
> Proposed entry:
>
> **Type:** signal · **Kind:** fact · **Layer:** ops · **Confidence:** high · **Topics:** portability/mcp
>
> **Description (verbatim):**
>
> > The MCP SDK's built-in DNS-rebinding protection rejected every request reaching the workflow MCP server through the ngrok tunnel with a 403, so it was disabled on the HTTP (Streamable HTTP) transport. The protection validates the `Host`/`Origin` headers against an allowlist of expected local hosts; an ngrok tunnel rewrites those headers to the public tunnel domain, which never matches, producing a blanket 403. Disabling it is judged acceptable for the current evaluation setup because the bearer-token guard on the HTTP transport already authenticates every caller, so the rebinding defense is redundant for this deployment. This trade-off is scoped to tunnelled remote evaluation; a production multi-user deployment would need either an allowlist that includes the real public origin or OAuth-based auth rather than relying on the bearer token alone.
>
> **References:**
> - `grounded-in` → 20260610-225912-s-tac-n9a — the slice-1 ship whose bearer-token-secured HTTP transport this configuration is part of
> - `related` → 20260531-161006-s-cpt-80v — feasibility research's auth/remote-hosting model that this bears on (authenticate the HTTP caller, bind session to principal)
>
> Two things I want your call on: (1) the fact/done framing question above, and (2) whether `high` confidence is right — the *observation* of the 403 is firm, but "bearer token makes rebinding protection redundant" is a security judgment you may want at `medium`. Confirm or correct and I'll capture.

### Turn 3 — user: mid-build context; fact is good; high is right; evaluation-only, not production

> Good — fact, high confidence, and the description already scopes the redundancy to evaluation and explicitly excludes production. Since you're mid-build (not a committed slice), I'll drop the `grounded-in` to the slice-1 done signal — this is a config change made during ongoing evaluation, not part of that shipped commit. I'll point it at the build activity context instead.
>
> Final proposal, one reference change from before:
>
> **Type:** signal · **Kind:** fact · **Layer:** ops · **Confidence:** high · **Topics:** portability/mcp
>
> **Description (verbatim):**
>
> > The MCP SDK's built-in DNS-rebinding protection rejected every request reaching the workflow MCP server through the ngrok tunnel with a 403, so it was disabled on the HTTP (Streamable HTTP) transport during evaluation. The protection validates the `Host`/`Origin` headers against an allowlist of expected local hosts; an ngrok tunnel rewrites those headers to the public tunnel domain, which never matches, producing a blanket 403. Disabling it is judged acceptable for the current PoC/evaluation setup because the bearer-token guard on the HTTP transport already authenticates every caller, making the rebinding defense redundant there. This trade-off is explicitly scoped to tunnelled remote evaluation and must not carry into the production version; a real multi-user deployment needs either an allowlist that includes the true public origin or OAuth-based auth rather than relying on the bearer token alone.
>
> **References:**
> - `related` → 20260610-225912-s-tac-n9a — the workflow MCP server slice whose bearer-token-guarded HTTP transport this configuration sits on
> - `related` → 20260531-161006-s-cpt-80v — feasibility research's auth/remote-hosting model that the production caveat bears on
>
> Confirm and I'll capture.

### Turn 4 — user: "yes"

> Captured as **20260610-231642-s-ops-nfn**. The generated summary is faithful — it preserves the evaluation-only scoping and the production caveat as agreed.
>
> Two low-severity findings worth a glance: pre-flight suggests both refs could be sharper than the `related` floor I chose — `grounded-in` for the slice-1 done signal (this fact reasons from that shipped transport) and `grounded-in` for the feasibility research (the production caveat draws on its auth model). I deliberately used `related` because this is a sideways config note, not a fact derived from those entries — but the body does lean on both. Your call: leave as-is, or I can supersede with the tighter ref kinds. Given it's a `low` advisory either way, I'd lean toward leaving it.
>
> For next: the two-client evaluation runs are the open verdict the whole MCP focus is gated on (d-cpt-afn). That's the thread to pull.

### Turn 5 — UX gotcha given as second capture, "keep it to one short sentence"

> Proposal — one short sentence as asked:
>
> **Type:** signal · **Kind:** fact · **Layer:** ops · **Confidence:** high · **Topics:** portability/mcp
>
> **Description (verbatim):**
>
> > Claude Desktop only invoked the workflow MCP server's connector tools after its tool-access mode was switched to preloaded; under dynamic discovery it did not act on them.
>
> **References:**
> - `related` → 20260610-225912-s-tac-n9a — the slice-1 workflow MCP server whose tools were being evaluated against Claude Desktop
>
> Confirm and I'll capture. (One check: did I state the dynamic-discovery behavior correctly, or did it simply not surface the tools at all rather than "not act on them"?)

### Turn 6 — confirmed

> Captured as **20260610-232018-s-ops-mgg**. Summary is faithful.
>
> Same low-severity `related`-vs-`grounded-in` advisory as before — harmless, leaving as-is.
>
> You've now logged two evaluation-run observations (DNS-rebinding, tool-access mode). These are exactly the findings the two-client evaluation (d-cpt-afn) is meant to produce — when you've gathered enough, that directive wants a verdict.

### Turn 7 — plan requested: production auth on the HTTP transport

> Two things to flag before I draft, because they affect whether this should be a plan at all:
>
> **1. The CLI-parity plan (d-tac-0v4) explicitly lists HTTP auth as a non-goal — "deferred."** So there's already a known deferral on record. A new plan here isn't redundant with it (that plan won't do the auth work), but the relationship should be a reference, not a silent overlap.
>
> **2. A plan commits to *how* — and you don't yet have the verdict that decides whether this server even survives.** The whole MCP focus (d-cpt-afn, d-cpt-313) is gated on the two-client evaluation. Committing to a build plan for production auth before that verdict risks planning work for a thing that might not proceed. The lighter, more honest entry might be a **gap signal** ("production deployment needs real HTTP auth; bearer-token is evaluation-only") rather than a `plan` decision. The signal names the requirement without committing to execution ahead of the verdict.
>
> My recommendation: capture this as a **gap signal**, not a plan. It records the obligation you already articulated in the DNS-rebinding fact, keeps it discoverable, and doesn't pre-commit build work that the evaluation verdict hasn't authorized yet. You can promote it to a plan once the server is confirmed to proceed.
>
> Do you want the gap signal, or are you deliberately committing to the plan now despite the pending verdict?

### Turn 8 — user: "I hear you - but do it now please with what we know to move this forward."

> Understood — capturing as a plan. Here it is.
>
> **Type:** decision · **Kind:** plan · **Layer:** tac · **Confidence:** medium · **Topics:** portability/mcp
>
> **Description (verbatim):**
>
> > Plan to add proper authentication to the workflow MCP server's HTTP transport — either OAuth 2.0/OIDC or a Host/Origin allowlist — as a gate before the server is used beyond ngrok-tunnelled evaluation. This closes the production-auth obligation recorded when DNS-rebinding protection was disabled and the bearer-token guard was accepted as evaluation-only (s-ops-nfn), and picks up the HTTP-auth work that the CLI-parity plan explicitly deferred as a non-goal (d-tac-0v4). The choice between OAuth and an origin allowlist is left open pending the deployment shape: an allowlist suffices if hosting stays single-origin, whereas the multi-user remote-hosting model from feasibility research (s-cpt-80v) calls for OAuth with session state bound to the authenticated principal. Execution is sequenced after the two-client evaluation verdict (d-cpt-afn) confirms the server proceeds, so this plan stays open as the committed next step rather than starting before that decision.
>
> **References:**
> - `addresses` → 20260610-231642-s-ops-nfn — the production-auth obligation this plan discharges
> - `builds-on` → 20260527-210616-d-tac-0v4 — picks up the HTTP-auth work that plan deferred as a non-goal
> - `grounded-in` → 20260531-161006-s-cpt-80v — the remote-hosting/auth model informing the OAuth-vs-allowlist choice
> - `depends-on` → 20260609-234656-d-cpt-afn — sequenced after the evaluation verdict authorizes the server to proceed
>
> Confirm and I'll capture.

### Turn 9 — confirmed; pre-flight blocks

> Pre-flight blocked it with two valid high-severity issues. Both are legitimate, not noise:
>
> 1. **A plan needs acceptance criteria** — I omitted them. Required for `plan` kind.
> 2. **`addresses` is wrong for the ref to s-ops-nfn** — that's a `fact`, not a gap/question/open commitment, so `addresses` doesn't apply. It should be `grounded-in` (the fact's caveat is the premise motivating this plan). I should have caught this.
>
> Both need your input or sign-off, so here's the revised proposal:
>
> [revised proposal: same description; new acceptance criteria — mechanism chosen with rationale, implemented on Streamable HTTP transport, DNS-rebinding protection re-enabled or explicitly superseded, bearer-token-only path removed or gated, tests + end-to-end verification; ref to s-ops-nfn changed `addresses` → `grounded-in`]
>
> The acceptance criteria are my proposal, not yours — review them. Confirm or adjust and I'll capture.

### Turn 10 — user adjusts AC 4: "cannot be used for non-local deployment"

> Good correction — that distinguishes local dev (stdio + local bearer HTTP, both legitimate) from non-local deployment. Revised AC 4:
>
> - [ ] The bearer-token-only path cannot be used for non-local deployment
>
> Everything else unchanged. [full AC list replayed] Refs and description as in my last proposal. Capture?

### Turn 11 — confirmed

> Captured as **20260610-232719-d-tac-47d**, clean — no findings this time. Summary is faithful, including the post-verdict sequencing.
>
> You've now got the full thread recorded: two evaluation observations (s-ops-nfn, s-ops-mgg) and the production-auth plan that depends on the verdict. The plan is deliberately gated behind d-cpt-afn, so the live next step remains the two-client evaluation runs that produce that verdict.
