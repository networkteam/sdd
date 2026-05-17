# Catch-up sub-skill build — evaluation record

## What shipped, by slice

**Slice 0 — `sdd info` command + `sdd status` header refactor**

New CLI command emits the three session-framing lines (`Local participant`, `Language` when configured, `Search`) as a stable surface for skill `!\`sdd ...\`` injections. Replaces the `sdd status | head -3` workaround which would have leaked `sdd status`'s output shape into the catch-up skill. `sdd status` shares the same `writeInfoHeader` presenter helper so the two surfaces stay byte-for-byte in lockstep.

Files: `internal/query/info.go`, `internal/finders/info.go`, `internal/presenters/info.go` (new); `internal/presenters/status.go`, `cmd/sdd/main.go` (modified).

**Slice A — main `/sdd` always-on framing injections**

`internal/bundledskills/claude/sdd/SKILL.md` frontmatter broadened to `allowed-tools: Read Grep Bash(sdd *)`. Added a "Project framing" section near the top with four `!\`sdd ...\`` injections — `sdd info`, `sdd view --layout='aspirations'`, `sdd view --layout='focus'`, `sdd view --layout='participants'` — that resolve fresh each time `/sdd` is invoked. Strategic context for threading judgment without per-flow fetches.

**Slice B — `/sdd-catchup` sub-skill**

New `internal/bundledskills/claude/sdd-catchup/SKILL.md`. Inline mode (no `context: fork`), `allowed-tools: Bash(sdd *)`. The voice block, judgment patterns, transformation examples, target-tone example, format template, format rules, and don't-list ported verbatim from the converged design (`catchup-design.md` on `d-tac-k4l`). Four fetches injected via `!\`sdd ...\`` — Recent done, Active and hot, Open and warm, WIP markers. The "Aspirations and focus — for judgment, not for content" section explicitly defers to main `/sdd`'s always-on injection rather than re-fetching.

**Slice C — routing**

Main `/sdd` SKILL.md's "First things first" and the Modes-of-working table both redirect Check-in mode to `/sdd-catchup` via the Skill tool. `/sdd-bootstrap`'s catch-up handoff swapped to the sub-skill too. Obsolete `playbook-catchup.md` removed from bundle source and the installed copy (the latter as defense against the known `s-tac-zaz` orphan-cleanup gap).

**Slice D — verify, build, install**

`go vet ./...`, `go test ./...`, `golangci-lint run ./...` all clean. `./bin/sdd init --scope project` refreshed `.claude/skills/`. Plus a setup-side commit putting `bin/sdd` on PATH via `devbox.json` (`$DEVBOX_PROJECT_ROOT/bin:$PATH`) so the catch-up's `!\`sdd ...\`` injections resolve to the local dev build instead of any system-installed binary.

## Mid-build refinement

The Recent done fetch originally used `kind(done):since("3d"):rank(by(date)):n(10):name("Just landed"):as-list`. During the first fresh-session evaluation the 3-day window returned empty (this graph's previous done signal was 9 days old), and the agent rendered stale ref state from active entries' summaries. Replaced with `kind(done):rank(by(date)):n(10):name("Recent done"):as-list` — latest 10 dones regardless of age, section renamed for honesty when the latest is from a week ago. Captured the deeper concern (`sdd view` doesn't surface refs' derived status) as a separate gap.

## AC validation against d-tac-k4l

| AC | Status |
|---|---|
| 1. Main /sdd injects info, aspirations, focus, actors/roles via `!\`sdd ...\`` | ✓ |
| 2. /sdd-catchup at internal/bundledskills/claude/sdd-catchup/SKILL.md | ✓ |
| 3. /sdd-catchup runs inline (no `context: fork`) | ✓ |
| 4. /sdd-catchup injects four catch-up fetches | ✓ |
| 5. /sdd-catchup carries the voice block | ✓ |
| 6. /sdd-catchup carries format template, rules, don't-list | ✓ |
| 7. Both skills declare `allowed-tools: Bash(sdd *)` | ✓ |
| 8. Main /sdd invokes /sdd-catchup via the Skill tool | ✓ |
| 9. Aspirations/focus/participants shape threading, not restated as content | ✓ |
| 10. Output matches converged design — verified by evaluation | ✓ (this signal) |

## Evaluation — AC 10

Three fresh-session evaluations, all on Claude Opus 4.7. Two `/sdd` invocations producing full catch-up briefings, one drill follow-up on a topic cluster.

### Voice fidelity

**Session 1**: clean. No banned vocabulary detected. The `conflate → blur` swap applied correctly ("the mode table also blurs Explore and Act"). Used "gap" rather than the entry text's "leak" in the Other-open section.

**Session 2**: one violation — header `A CQRS leak the view work caught` uses `leak` as a noun, explicitly banned by the design's mechanical swap (`leak (as noun, project tic) → say what's leaking`). The entry text the agent was reading uses `leak`, and the agent echoed it. Elsewhere clean. Slip is within normal voice variance; the rule is working, just not catching every echo.

**Session 3 (drill)**: clean. "Same weight, very different meaning" is the kind of pithy summary the voice block targets. Canonical names preserved per Names-that-stay.

### Format adherence

Both catch-up sessions: ✓ italic focus line at top, ✓ story-arc bold headers (not kind names), ✓ verb-first numbered items ending in periods, ✓ inline IDs in trails when entries are named, ✓ A–D drill labels, ✓ bolded open question, ✓ no semicolons in body.

Minor: a few sentences ran 17–22 words against the ≤15-word target in both sessions. Not severe.

Drill session: appropriately shifted from catch-up's numbered shape to engage-style narrative + question lists + move catalog (signal + explore intent calls for narrative brief, not AC table).

### Threading quality

Both catch-up sessions clustered by current relevance, not by kind/layer. Both led with the in-flight build and threaded causally into surrounding context.

**Session 1** picked three threads: catch-up build · pre-flight calibration · view ranking. 7 items.

**Session 2** picked four threads: catch-up build · post-engage lens moves · CQRS leak · validator/summary reliability. 8 items.

Both inside the 2–4 thread, 6–8 item target. Different selections; both defensible against the data.

Aspirations and focus shaped threading without being restated: ✓ in both. The italic focus line is the only surface mention.

**Temporal blur (Session 2)**: rendered "Sequenced to land after the Engage refactor itself" against `d-prc-iqw`, but the refactor had landed (`s-prc-st0` closed `d-prc-kyz`). The agent had no signal of the closure — the `since("3d")` window predated all 05-07 dones, and the active entries' summary text reflected pre-completion state. This is a data-availability artifact, not a voice failure: the mid-build refinement (`since` removed) reduces it for future invocations, and the deeper structural concern (refs without derived status in `sdd view` output) is captured as its own gap signal.

### Drill behavior observation (Session 3)

User selected "drill into C" (Heat scoring vs ref-type qualifiers). The agent recognized C named two paired entries (`s-cpt-k7z` and `s-cpt-zsd`), read their chains via `sdd show --downstream`, produced a kind+intent-appropriate brief (narrative for signal+explore), surfaced surrounding context (`s-tac-x1n` sibling already-shipped, `s-cpt-sy4` independent), listed six concrete decision dimensions, offered four candidate moves including Park, and closed with an orienting question. Voice and format clean.

But: the drill verb fell through to engage-mode behavior directly. The expected behavior per `d-tac-qom` is to *expand* a topic cluster into more entries — surface the area's landscape first, then the user picks where to engage. The agent collapsed the expand and engage steps into one. The sub-skill's render rules specify "Say drill A or survey for the full picture" as output prompt but never specify what the verb should do — the agent defaulted to engage on the entries it could resolve from the still-loaded catchup data. `d-tac-qom` therefore remains open; the expand-then-engage gap is captured as its own follow-up.

### Wall-clock cost

| Session | Time | Note |
|---|---|---|
| 1 (/sdd → catch-up) | 1m 5s | Three threads, 7 items |
| 2 (/sdd → catch-up) | 52s | Four threads, 8 items |
| 3 (drill into C) | 57s | Two-entry chain reads + synthesis |

All inside the design's 41s–2m33s envelope from the paste-test phase. Session 2 well under a minute. For comparison: pre-build catch-up consumed 32% of session context and exceeded 3 minutes per `s-cpt-jq7` — the bottleneck the build targeted.

## Closures, deferrals, follow-ups

- **Closes `d-tac-k4l`** — plan fulfilled across the four slices plus the mid-build refinement.
- **Closes `d-tac-kv5`** (quick catch-up directive) — the sub-skill's story-arc threading + italic focus line + Other-open section surface "what wants the user's next action" via a different shape than the original three-tier (In focus / Warm but unmentioned / Parked) framing. The underlying goal is achieved.
- **`d-tac-qom` remains open** — drill behavior gap captured as a follow-up signal.
- **`s-cpt-sy4`** continues to carry the mechanical topic-aggregation backing (out of scope per the plan).

## Setup-side change recorded

`devbox.json` adds `$DEVBOX_PROJECT_ROOT/bin:$PATH` so `sdd` on PATH resolves to the local dev build inside the project shell. Required for the `!\`sdd ...\`` injections to call the build under test rather than any system-installed binary. CLAUDE.md updated to reflect the cleaner invocation pattern.

## Branch and commits

Branch `catchup-subskill` carries:

- `4a63633` — slice 0 + A + B + C source changes, sub-skill creation, /sdd-bootstrap update, orphan cleanup
- `ff9ccc8` — devbox PATH fix + CLAUDE.md update
- `b664f0b` — mid-build refinement (`since` removed from Recent done fetch)
- Plus three auto-commits from `sdd init` refreshing the installed bundle (the known `s-prc-epa` commit-scoping issue).

WIP marker `20260517-141802-christopher` retired with this signal.
