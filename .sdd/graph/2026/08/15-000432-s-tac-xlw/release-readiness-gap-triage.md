# Release-readiness gap triage — engine-first SDD (2026-08-14)

Full inventory behind the insight. Every open gap signal in the graph (~110) was read
in summary; the ones below were read in full or verified against code. Severity is
assigned against one question: **what does a person hit if we tell them the engine is
how SDD works?**

State at triage: last tag `v0.17.0-alpha.1` (`s-prc-2hj`), 119 commits behind HEAD.

## The two releases

`d-cpt-p3k` commits two different things — the engine as primary working mode, *and*
the /sdd flow to deprecation — and explicitly leaves the deprecation mechanics
unsettled ("what retires when, what remains as a pointer skill, how installed renders
and `sdd init` behave, and the communication to existing users").

- **Release A — "the engine is how SDD works."** Engine recommended, skills still
  installed. This is the next release.
- **Release B — skills actually retired.** Gated by the bootstrap parity verdict
  (`d-tac-fim`), which is gated by `d-tac-9be` completing.

Severities below are for A. B-specific items are marked.

---

## BLOCKERS

### B1 · Silent loss on the capture path — a cluster, not four bugs

| Entry | Failure |
| --- | --- |
| `20260814-233547-s-tac-bqt` | engage→capture drops the `anchor` seed, no error |
| `20260811-233331-s-tac-bjn` | declared field reported at a non-collecting step is silently discarded |
| `20260707-175502-s-prc-lgu` | staged attachments silently orphaned; body asserts a file that isn't there |
| `20260811-232108-s-tac-kus` | playback collects only 6 fields — the fault-catching step is partly blind |

In engine-first mode capture is the **only** write path, which is what promotes this
from annoyance to structural. `d-tac-ymk` (pending) already commits to eliminating the
class from the entry side via declared bindings.

**Minimum to ship:** make undeliverable and discarded reports *loud*. That converts all
four from silent corruption into a visible error without waiting on the binding design.

### B2 · Search over MCP returns no evidence — `20260814-160042-s-tac-rst`

Verified in code during this triage:

- `internal/query/search.go:25` — `DefaultMaxCitationsPerEntry = 3`
- `cmd/sdd/search.go:441` — CLI substitutes the default when the flag is unset
- MCP passes an omitted `max_citations` through as the Go zero value;
  `EffectiveMaxCitations() == 0` suppresses citations entirely

The agent-facing surface — the one engine-first makes primary — is the blind one. It
already produced a false conclusion about attachment indexing in a live run. The change
surface is one default value; what it lands on is the grounding discipline everything
else rests on.

### B3 · The binary ships wrong configuration instructions — `20260710-144212-s-tac-z7j`

Error/help strings in `internal/llm/embed`, `internal/llm/factory`, `internal/llm/gollm`,
`internal/finders/search`, plus the bundled `SKILL.md.tmpl`, still direct users to
`.sdd/config.local.yaml`, contradicting the placement directive `d-cpt-6cq`. A newcomer
following the tool's own message configures somewhere that does not apply.

### B4 · Catch-up breaks Codex at turn one — `20260719-122103-s-tac-jom`

~50KB / 13,749 tokens, middle-truncated by Codex's ~10K tool-output budget, and
unrecoverable because compose instructions forbid re-fetching. Catch-up is the natural
opener. Root cause is the uncapped, un-`brief`'d lane spec in `d-prc-cat`, contrasting
with the byte-capped discipline the user-dialogue shell already applies.

### B5 · The deprecation call itself is unmade — `20260715-175731-d-cpt-p3k`

No code surface at all — it gates the release notes. The smallest item on the list and
the one nothing else can route around.

### B6 · No learning path — `20260715-172002-s-cpt-i5a`

Docs plan `d-tac-c7b` is active but unshipped. "Use the engine now" currently points
people at a positioning README, insider narrative docs, and the skills being
deprecated. Blocker for the *announcement*, not the binary — descopable to a README
quickstart if the site isn't close.

---

## MAJOR

### M1 · Engine can't reach what the CLI can — host-neutrality breaches (`d-cpt-476`)

- `20260731-083311-s-tac-4vx` — multi-target supersede rejected; forces a CLI fallback
- `20260722-141909-s-tac-6rd` — no annotation capture branch (also blocks the bootstrap
  topic-founding fix `s-cpt-ksz`)
- `20260707-134223-s-tac-n5a` — groom fork-detection has no engine equivalent

These look survivable only because every graph today is local. Each is a hard stop for a
hosted or remote user — the exact scenario engine-first is meant to open.

### M2 · Write-gate wedge — `20260707-170235-s-prc-0p1`

`validation failed: N issue(s)` names neither rule nor field, journals nothing, doesn't
distinguish mechanical validation from LLM pre-flight, and `closes` is a frozen param the
adjust chooser can't reach. Diagnosis required reading Go source — which an outside
engine user does not have.

### M3 · Assemble gate illegible — `20260722-141852-s-tac-dhn`

All branch predicates served flat regardless of drafted kind; agents fabricate refs on
near-root entries. Being worked by the per-kind fact line (`d-tac-9be`, driving).

### M4 · Bootstrap parity unverified — `s-prc-7ku` + `d-tac-fim` + `s-tac-tom`

Procedure shipped and passed inner verification (`s-tac-3gl`); the outer eval found
actor/role capture the dominant gap (`s-tac-lgc`); `fim`'s live parity evaluation — the
thing that *authorizes* retirement — hasn't run, and its first AC waits on `d-tac-9be`.
`s-tac-tom` (two validator bugs forcing overrides during actor capture) sits inside it.
**Not a blocker for A. It is the gate for B.**

### M5 · `sdd init --scope user` writes no MCP registration — `20260706-182028-s-tac-qxp`

Project scope shipped (`s-tac-vhh`) so the main path works. But under engine-first a
user-scope init installs deprecated skills and no server — effectively a no-op.

### M6 · Worktree sessions silently unbound — `20260728-004132-s-tac-cjt`, `20260713-121507-s-prc-akz`

Entering a worktree outside the implementation procedure never triggers the
branch-binding prompt; reads resolve against the default branch and worktree entries
look *absent*, indistinguishable from a correct binding.

### M7 · Base facts pollute project search — `20260716-144113-s-cpt-7ch`

Every new adopter's first searches mix SDD's own shipped facts with theirs, foreign
timestamps and all. Much more visible now that base facts ship broadly.

---

## MINOR

| Entry | Note |
| --- | --- |
| `20260811-164958-s-tac-9l8` | serve duplication escaping dedup — token waste |
| `20260811-234304-s-tac-ual` | capture-time LLM completions record no usage/cache metrics |
| `20260707-170902-s-cpt-ev3` | session log omits outbound data; silent on failure |
| `20260718-092125-s-tac-9cq`, `20260718-091255-s-prc-e2v` | view-fact formatting and first-call discovery |
| `20260712-191729-s-tac-703` | cross-repo `read_attachment` fails |
| `20260710-144207-s-tac-v1y`, `20260709-161317-s-tac-ol7`, `20260709-161217-s-tac-dt0`, `20260709-161133-s-cpt-3wt` | config and repo-lifecycle polish |
| `20260612-001543-s-tac-gtq` | Codex sandbox approvals on first run |
| `20260721-151122-s-tac-spe` | duplicated CLI/engine write path — invisible now, expensive later |
| `20260810-123346-s-tac-aw9` | in-degree semantics; only gates weighted in-degree shipping |

## Not release-shaped

`20260726-195739-s-cpt-hcp` (authoring-model ceiling) · `20260715-173711-s-cpt-e8x` and
`20260715-113417-s-cpt-vxd` (remote non-Git lifecycle) · `20260611-092625-s-cpt-4v3`
(non-developer MCP onboarding — that is the `d-tac-hza` focus) ·
`20260707-143547-s-prc-tso` (release-workflow port).

---

## Release B only — and invisible from the engine-gap side

**`20260507-174656-s-tac-zaz`** — `sdd init` does not prune installed skill files whose
bundle source was removed. The moment the skill renders are dropped, every existing user
keeps a stale `.claude/skills/sdd/` forever, reproducing exactly the dual-flow ambiguity
(`s-tac-53t`) that motivated `d-cpt-p3k` in the first place. The entry already sketches
the fix: delete only when the content hash matches the last stamp; warn on modified
orphans.

Filed May 2026 under `cli/init`. Nothing about it reads as engine work, which is why a
triage scoped to engine gaps would miss it.

## Resolved during the triage

**`20260814-135103-s-tac-4hs`** (show heading hierarchy) read as fixed-but-open when this
sweep ran — commit `6675bad3` had already demoted embedded bodies via `mdcompose` on both
outlets while the signal stayed open. It was closed the same day by `s-tac-th4`, so it is
neither a blocker nor a grooming candidate. Recorded because the fixed-but-open state is
the pattern worth watching, not this instance.

---

## Minimum gate for Release A

| # | Item | Change surface |
| --- | --- | --- |
| 1 | `s-tac-rst` — MCP search citation default | one default value at the MCP query boundary |
| 2 | `s-tac-z7j` — config strings | a string sweep across four packages plus the skill template |
| 3 | Loud failure for the B1 class | bounded, at the report and seed boundaries; no new subsystem |
| 4 | `s-tac-jom` — catch-up serve size | a lane-spec pass on one procedure (cap + `brief`) |
| 5 | `d-cpt-p3k` deprecation mechanics | no code; a decision |

None of these opens a new dependency or a surface the project does not already own.
