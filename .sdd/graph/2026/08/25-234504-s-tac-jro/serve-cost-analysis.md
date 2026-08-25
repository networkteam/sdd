# Engine-mode capture at scale — full serve-cost analysis

Outer evaluation of SDD engine mode used for real work on a client project.
The project, its domain, its graph content and its topic vocabulary are
deliberately absent from this record; only the framework's own behaviour is
reported. Placeholders stand in wherever client vocabulary would otherwise
appear.

## Method

Two records were read for one session held on 2026-08-24:

- the engine session event log (append-only JSONL, one snapshot per version),
  giving instance lifecycle, step transitions, chooser answers and timestamps;
- the host agent's conversation transcript (Claude Code JSONL), giving
  per-request context size, per-tool payload bytes, and message timing.

Byte figures are measured on the exact rendered payloads that reached the
model — tool-call inputs and tool-result texts as stored in the transcript.
Records the transcript keeps but does not send to the model (a duplicated
`toolUseResult` field) were excluded. Context figures are the host's own
per-request accounting: uncached input plus cache-read plus cache-creation
tokens.

Byte-to-token ratios are not asserted. The content is non-English and
JSON-escaped, and reasoning tokens are retained by the host but not stored in
the transcript, so a stable ratio could not be derived. Bytes and token
figures are therefore reported separately, never converted into one another.

## Session shape

One dialogue session, ~6 hours wall clock, two distinct phases:

| phase                         | span          | context at end |
|-------------------------------|---------------|----------------|
| reading and analysing a source document | 11:34 – 15:20 | 277k tokens |
| capturing entries             | 15:20 – 17:28 | 843k tokens |

Thirteen capture instances ran in the second phase: ten completed, three
abandoned. The host session ended at roughly 85% of a 1M-token context window.

The capture phase alone cost **566k tokens — 67% of the whole session — for 13
entries, ~44k tokens per captured entry.**

## Context accounting

Total tool traffic (inputs sent plus results served): **1.36 MB**, of which
**84% is the engine's own MCP surface** (1.14 MB).

| surface                                  | bytes   | share |
|------------------------------------------|---------|-------|
| `next` → playback serves (30 calls)      | 429,704 | 31.6% |
| `start_procedure` opening serves (14)    | 224,464 | 16.5% |
| host shell commands                      | 151,523 | 11.1% |
| `next` → guideReview serves (11)         | 137,395 | 10.1% |
| `next`, other steps (22)                 | 117,132 |  8.6% |
| `show` (11)                              | 112,755 |  8.3% |
| `search` (5)                             |  83,738 |  6.2% |
| door serve (`start_session`)             |  30,923 |  2.3% |
| everything else                          |  71,492 |  5.3% |

`show` and `search` are the agent's own reads — unique content, legitimately
paid for. The serve surfaces above them are where repetition lives.

## Duplication accounting

Across all 98 engine serves, by field:

| field           | serves | total bytes | distinct values | byte-identical repeat |
|-----------------|--------|-------------|-----------------|-----------------------|
| `instructions`  | 66     | 342,796     | 66              | 0                     |
| `report_schema` | 66     | 311,650     | **5**           | **293,301**           |
| `entries`       | 11     | 110,491     | 11              | 0                     |
| `results`       | 5      | 83,363      | 5               | 0                     |
| `framing`       | 18     | 59,263      | 18              | 0                     |
| `pending_chooser` | 52   | 13,514      | 4               | 12,633                |
| `base_junction` | 13     | 7,306       | 1               | 6,744                 |
| `goal`          | 76     | 4,168       | 5               | 3,908                 |
|                 |        | **958,814** |                 | **316,586 (33%)**     |

`report_schema` is the single largest waste in the session: 311,650 bytes
carrying five distinct values, 94% of it byte-identical repetition. That is
~23% of all tool traffic.

The schema is generated, not authored — `ReportSchemaForStep` builds it from
the step's collect list plus *every* declared state field, which is why a
whole procedure yields so few distinct values. It is assigned directly onto
the tool result and never passed through the served-once memory, so it is
outside deduplication entirely.

### Why `instructions` shows zero repeats but is still repetitive

The instruction unit is hashed whole, post-injection, as a single sha256. Any
byte that differs re-serves the entire unit. Diffing two steady-state capture
opening serves, the *complete* difference was the injected topics table:

```
- <label-a>   53   heat 42.297      + <label-a>   55   heat 50.262
- <label-b>   38   heat 33.103      + <label-b>   40   heat 39.075
- <label-c>    5   heat  1.723      + <label-c>    5   heat  1.722
- <label-d>    1   heat  0.089      + <label-d>    1   heat  0.088
```

Both columns drift: counts move as entries land, heat moves in the third
decimal place. Stripping heat alone would not restore byte identity.

The table measures 1,735 bytes of an 8,076-byte unit — it is not the bulk, it
is the hostage-taker: 1.7 KB of volatile content holds 6.3 KB of static prose
out of dedup. Rendered as bare labels, sorted, the same content is 715 bytes.

## The capture opening serve

Fourteen `start_procedure` serves, field composition:

| field           | avg bytes/serve |
|-----------------|-----------------|
| `instructions`  | 8,596           |
| `report_schema` | 7,469           |
| all others      | ~110            |
| **total**       | **16,221**      |

No injected entry lists, no brief lines — nothing that scales with the
adopting project's graph. The serve is fixed text, repeated per capture.

Per-serve sizes confirm the once-per-session type-system delivery works as
designed: serve #1 carries 14,175 bytes of instructions, every serve after it
8,076 (±18 bytes, all of it topic-table drift).

Steady-state opening serve: **15,545 bytes, of which the schema is 48%.**

If the topics table stabilised, the instruction unit would collapse to a
one-line stub (~200 bytes) for serves 2..14 — roughly 100 KB recovered. If the
schema also entered dedup, the serve would collapse to ~300 bytes — roughly
200 KB recovered.

### The general case is worse than this session's

The assemble instruction unit carries per-instance fragments beyond the topics
table — an anchor line when the capture is anchored, plus a branch on
inherited grounding for seeded captures. Every capture in this session was
fresh and unanchored, which is exactly why the topics table was the sole diff.
A grooming or closure session, where captures carry anchors or supersedes,
would break the hash on every serve regardless of any topics fix.

## The playback re-serve loop

Thirty playback serves for thirteen capture instances — 2.3 revision rounds
per entry, seven in the worst case. Each round costs twice: the agent resends
the full revised draft in its report (112,790 bytes sent across 30 calls) and
the engine echoes the full description back inside the playback instructions
(316,914 bytes served).

Diffing each re-playback against the previous serve in the same instance:

| instance | round | bytes served | bytes changed |
|----------|-------|--------------|---------------|
| A        | 2     | 5,456        | 0             |
| A        | 3     | 6,537        | 1,682         |
| A        | 4     | 6,536        | **1**         |
| A        | 5     | 7,818        | 1,593         |
| A        | 6     | 8,082        | 376           |
| A        | 7     | 8,102        | 34            |
| B        | 2     | 6,232        | 435           |
| C        | 2     | 6,040        | 594           |
| D        | 2     | 6,193        | 1,240         |
| D        | 3     | 6,467        | 372           |
| E        | 4     | 4,450        | 0             |
| F        | 2     | 4,687        | 148           |

Across all rounds 2..n: **91,473 of 165,790 playback instruction bytes are
unchanged text re-served — 55%.** Typical edits are a few hundred bytes
against a 5–6 KB re-serve, and they sit inside the description body, so
field-level granularity would not help — a one-byte fix still re-serves the
whole body.

Two rounds served with zero bytes changed, meaning a revision round was spent
without the engine recording any edit at all.

## Time accounting

Capture phase, 128 minutes, split by what the gap between consecutive
transcript records represents:

| what                        | minutes | share | events | avg   |
|-----------------------------|---------|-------|--------|-------|
| user reading and replying   | 72.2    | 56%   | 26     | 2m47s |
| model generating            | 34.5    | 27%   | 131    | 15.8s |
| engine / tool latency       | 6.2     | 5%    | 104    | 3.6s  |
| other                       | 15.0    | 12%   | 112    | 8.1s  |

**The engine is not the bottleneck — it accounts for 5% of wall clock.** The
dominant term is human review, and the number of review turns is set by
playback round count. Cutting rounds cuts both the context axis and the time
axis at once.

## Abandons

Three of thirteen capture instances were abandoned:

| instance | duration | bytes | cause                                              |
|----------|----------|-------|----------------------------------------------------|
| i_4      | 6m42s    | ~24 KB| wrong entry kind chosen; kind not correctable in-place |
| i_12     | 6m51s    | ~23 KB| an attachment was needed; attachments not correctable in-place |
| i_10     | 0m32s    | ~1 KB | deliberate defer — working as intended             |

The i_12 abandon forced a full redo as a fresh instance, which took 20m35s and
~53 KB. Two abandons therefore cost roughly 35 minutes and ~100 KB including
the redo.

Both correctable-field abandons trace to the same mechanism: `entryKind`,
`layer` and `attachments` are declared as procedure *state* and collected at
`assemble`, but none of the three revision paths (`guideReview: revise`,
`guideReview: recheck`, `playback: adjust`, `reviseOrOverride: revise`)
collects them. `assemble` collects 20 fields; the revision paths collect 10,
10 and 6. So the values are durable but unreachable once the first assemble
report has landed.

The decisive point for the context axis: **an abandon does not free the
context it consumed.** The serves paid for an abandoned instance remain in the
window for the rest of the session.

## Findings

1. Capture-heavy engine sessions cost ~44k tokens per entry, and the capture
   phase dominated a six-hour session at 67% of total context.
2. `report_schema` is served outside deduplication entirely: 66 serves, five
   distinct values, 293 KB of byte-identical repetition (~23% of tool traffic).
3. The instruction unit is hashed whole, so a volatile fragment of 1.7 KB
   holds 6.3 KB of static prose out of dedup. In this session that fragment
   was the topics table; in anchored captures it is additionally the anchor
   line.
4. Topic table counts *and* heat both drift; only bare labels are stable.
   Bare labels are 715 bytes — small enough to serve every time, which avoids
   a stranding failure that would be silent and permanent, since topic labels
   are write-once with no alias mechanism.
5. Playback re-serves the whole draft each revision round; 55% of what it
   sends in rounds 2..n is text the agent already holds. Deduplication cannot
   catch this — the bytes legitimately change — so the repair shape is a delta
   anchored on what the engine last served, not on what the agent last
   reported.
6. The engine contributes 5% of wall clock. Human review time dominates, and
   is driven by round count.
7. Correcting entry kind, layer or attachments mid-capture is impossible, so
   it costs an abandon and a full re-pay of both time and context.
