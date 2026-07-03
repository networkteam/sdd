# Base procedure design record — session shell as a graph entry

Settled in dialogue (Christopher, Claude) after the second dogfooding run;
grounds the base-procedure directive.

## Why a base procedure

The goal-driving loop's promise is that the agent is never goalless, but the
loop only existed inside a procedure — the two goalless moments were exactly
where the dogfooding run failed: before the first move (the engage false
start) and between moves. A long-lived base instance closes both. The parity
inventory anticipated this as "two-tier knowledge text: base tier at session
open; deeper units served just-in-time" — previously smeared across a thin
skill, static server instructions, and a 21KB framing blob.

## Entry mechanism — one strict door

A dedicated session-start tool auto-starts the base instance and returns its
first serve. Rejected alternatives:
- "Any first tool call returns base's serve" — rejected for a single explicit
  door; implicit entry recreates ambiguity about what state the session is in.
- Skill prose naming a cold-start default — rejected: hardcodes a procedure
  name into an agent-side surface while the set is graph-resident (the
  "catchup" typo class); the recommendation belongs in the served payload.
- list_sessions as the entry point — rejected: drags a recovery scan into
  every session start, and conflates parked work with sessions live in
  another tab.

## Gating scope

Loop tools reject without a session, pointing at the door. Free reads stay
ungated — the guest-surface principle (s-cpt-1dz) gates only the write path,
and ceremony in front of grounding punishes widening. While no session runs,
read results carry a one-line breadcrumb to session start: an agent that
enters through a pasted entry ID gets its data and the trail home.

## Knowledge tiers

- Tier 0, connection handshake (server instructions): the minimum to not
  misread a read — signals/decisions, immutability, summaries are pointers,
  the door. Must survive truncation; some clients render instructions poorly.
- Tier 1, base opening serve: process core, conduct/register (compressed from
  the /sdd skill), live procedure enumeration, info header, routing guidance.
  Served once per consumer; re-servable on demand; re-served on resume.
- Tier 2, per-step units: unchanged.
Property: no path through the tool surface avoids a breadcrumb; every
breadcrumb points at the same door; the expensive text serves once.

## Resident step = free dialogue, structurally

A pending user junction that never demands an answer. Free dialogue is the
pending state (reflect mode, deliberately un-policed, per the inventory's
host-side list). Sub-moves interleave while it pends. Junction serves on
sub-move completion carry the open-threads block — relocated from the payload
fields added under d-tac-nqo to ordinary base serves.

## Graceful exit

- Conclude: a user-relayed option on the resident junction; open threads are
  surfaced for per-thread decisions (finish / abandon / park the session)
  before base completes. The session then stops listing as open work.
- Disconnect with base as the only running instance auto-concludes: un-logged
  free dialogue leaves nothing to resume, so a closed tab leaves no corpse.
  Without this, base would have made stale-session litter universal.
- Disconnect with open sub-moves parks the session — that is what resume is
  for. Bulk teardown of accumulated parked sessions stays with s-tac-j25.

## Concurrency

A session bound to a live connection is live by definition and is not offered
as resumable — the parked/live discrimination list_sessions lacked. Sub-agents
on Claude Code share the outer agent's MCP connection (verified in the
dogfooding forensics: the explore worker landed in the same engine session),
so workers pay no session ceremony on this harness.

## Rider fixes

Labels apply only after the procedure canonical resolves (a rejected
start_procedure currently still labels the session). The sdd-engine skill
shrinks to a pointer at the door; full retirement is the direction — on
skill-less clients the server instructions plus the door suffice.
