# Outer validation — session-store relocation and acknowledged migration

Anchor: `20260722-112853-d-tac-ln1` (session branch-binding and store-locality plan)
Lens: outer (validation) only. Inner lens covered separately by `20260726-195621-s-tac-0hg`.
Date: 2026-07-26. Participant: Christopher. Run on `main` after the merge of `worktree-session-branch-binding`.

All paths below are repo-relative.

---

## 1. Scope of this run

Outer validation asks whether the shipped work is the right thing, working in use, serving the
person it is for. For this work type — a CLI-driven offline migration plus its interactive
acknowledgement — that resolves to three concrete checks, run against a real long-lived store
rather than a fixture:

1. Does the migration perform its one job on real pre-existing data — the accumulated session
   material of a repository that has been running sdd across the format changes the work itself
   introduced?
2. Is the acknowledgement gate the explicit, comprehensible y/n decision that `20260723-192701-d-tac-g7r`
   commits to, *as experienced on a real terminal* — not merely present in the code path?
3. Does the surrounding CLI output serve a human reader per `20260628-131545-d-cpt-dgk`?

Not covered by this run: the branch-binding half of the plan. No worktree session was driven end
to end. Coverage claimed is outer-lens on the store-relocation and acknowledged-migration slice only.

---

## 2. Evidence

### Run 1 — `sdd init`

Output, verbatim:

```
  145 in-tree session or staged-blob payload file(s) need relocation to the machine-global store; 1 identity-less global session or staged-blob payload file(s) need rekeying to the repository-ID store; left unchanged. Stop `sdd serve`, restart agent sessions, then rerun with --migrate-sessions
skills: 17 file(s) at <repo>/.claude/skills
  up to date
skills: 17 file(s) at <repo>/.agents/skills
  up to date
```

The in-tree count (145) was stable across runs; the identity-less global count was reported as 1 in
this run and 2 in a later one, so no fixed total is asserted here.

Screenshot evidence: the reported line is rendered **unwrapped and truncated at terminal width**,
cut mid-word at `payload file(`. Nothing after the cut reaches the screen.

Human attestation (Christopher): he saw no prompt and no indication that a keypress was expected;
he read the output as a status line. His words on the CLI UX: "awful (long line, no hint that the
user should press any key). My explicit wish from the design was a y/n choose in an interactive
run."

### Run 2 — `sdd init --migrate-sessions`

Hard failure. Nothing migrated:

```
  ✕ relocating session store: digesting relocation payload .sdd/sessions/s_20260714-095459-e9ecfc96.jsonl: decoding current session line 1: json: unknown field "Holder"
```

### Code paths read

- `local/local_sessionstore.go:552` — `classifySessionHandle` buckets a session log **solely** on
  presence of a top-level `version` key. Absent → `sessionFormatLegacy`. Present → `sessionFormatCurrent`.
- `local/local_sessionrelocate.go:1649` — `copySessionLog`: legacy-format logs are copied
  byte-for-byte via `io.Copy` and never decoded. Current-format logs are decoded line by line.
- `local/local_sessionrelocate.go:1667` — each current-format line goes through `decodeStrictJSON`.
- `local/local_sessionrelocate.go:1732` — `decodeStrictJSON` sets `DisallowUnknownFields()`.
- `local/local_sessionrelocate.go:1606` — `payloadDigest`, where the failure surfaced: this is
  **manifest planning**, before any payload is moved.
- `cmd/sdd/main.go:1264` — `chooseSessionStoreRelocation(pending, explicit, interactive, prompt)`:
  gates correctly, calling the prompt when pending, not explicit, and stdin is a TTY.
- `cmd/sdd/main.go:1305` — `promptSessionStoreRelocation` composes the prompt string.
- `cmd/sdd/main.go:1622` — the declined-branch message printed after the prompt returns false.
- `internal/cliout/tui/prompt.go:121` — `confirmPromptModel.View` renders `"%s [y/N]: %s"`.
- `internal/cliout/tui/prompt.go:135` — `RunConfirm`.

### Failing payload

The failing session log was written 2026-07-14. Its first line carries:

```json
{"version":1,"metadata":{"CodecVersion":1,"ID":"…","Subject":"local","Project":"github.com/networkteam/sdd","Participant":"Christopher","Label":"","Holder":{"Subject":"local","MCPSessionID":"…","ClientName":"codex-mcp-client","ClientVersion":"0.144.3","Generation":1,"LastActivity":"…","ExpiresAt":"…"}}}
```

`Holder` was deleted from the session model on 2026-07-18 by commit `c80af3a`
("delete holder lease, attachment stamp (session-model slice 3)"). The codec version was **not**
bumped alongside the removal.

---

## 3. Defect analysis

### 3a. Strict decoding rejects the format's own history

The tolerance boundary for old session logs was drawn exactly once, at the pre-0.16 legacy line, and
then treated as permanent. Everything on the "current" side of that line is assumed to be
structurally identical forever.

That assumption broke the moment `Holder` was removed without a codec bump:

- A session written between the current-format landing and `c80af3a` carries `"version":1`, so
  `classifySessionHandle` files it as **current** — the byte-for-byte legacy escape does not apply.
- It also carries `Holder`, which no longer exists on the struct, so `decodeStrictJSON` rejects it.
- `CodecVersion` is still `1`, so the codec-version gate immediately below the decode cannot
  distinguish it either.

The relocator therefore cannot read exactly the data it exists to relocate. Worse, the failure is
fatal for the entire sweep: one such file aborted the run before any of the 145 in-tree payloads
moved.

This regresses against the active contract `20260602-203349-d-cpt-i2x` — a current binary must read
every older on-disk form it encounters, read-compatibility is permanent and never silently dropped,
and any format-breaking change must ship a conversion path. It is also a step back from
`20260714-140424-s-tac-fco`, the earlier legacy-session migration, which tolerated pre-0.16 logs and
was proven by an isolated end-to-end run over 51 real logs and 14 staged files.

### 3b. The acknowledgement gate is invisible

`20260723-192701-d-tac-g7r` commits relocation to running "only after user acknowledgement", with
the acknowledgement stating the real precondition.

The mechanism is implemented: `chooseSessionStoreRelocation` calls the confirm on an interactive
stdin and `RunConfirm` renders `[y/N]:` with a focused text input.

The experience is not. `promptSessionStoreRelocation` concatenates into a single ~330-character
sentence: the full two-clause payload census, the instruction to stop every `sdd serve` process and
restart its agent sessions, and the question "Relocate now?". Bubbletea's inline renderer truncates
that at terminal width, so the `[y/N]` affordance and the input cursor are never on screen. The user
sees a status line, presses Enter or Escape, and the prompt — which defaults to N — resolves to the
declined branch. The bubbletea view is then cleared on exit, so nothing of the prompt survives in
scrollback either.

Net effect: the committed acknowledgement path is effectively unreachable interactively. The user is
silently routed to decline every time. This also breaches `20260628-131545-d-cpt-dgk`, which commits
a per-command UX design and a coordinator-owned terminal for TTY humans.

The ~330-character machine-shaped prompt text is folded into this defect rather than tracked apart:
it is the same surface and the same fix. Even correctly wrapped, a sentence fusing a payload census,
a precondition instruction, and a question would not read as a decision to make.

---

## 4. What held

Recorded deliberately, because the safety design is vindicated:

- The failure was **loud**, not silent.
- It occurred during manifest planning (`payloadDigest`), **before any payload moved**. The store was
  left intact. The slice is unusable, not unsafe.
- Detection and counting worked — the 145 in-tree payloads and the identity-less global payloads
  were identified.
- `sdd serve` starts over the leftovers rather than refusing, as committed.
- The standing relocation notice is present in every serve response of this session's framing.
- The non-interactive fallback line does name the recovery command.

---

## 5. What the run teaches

The inner evaluation `20260726-195621-s-tac-0hg` judged this same work merge-ready with **zero
blockers** across four lenses, one of which was a data-safety code review. The first outer run hit a
hard failure on the primary path within seconds.

The gap between those verdicts is not a lens run badly. It is a lens never run: no execution against
a real, historically accumulated store. Christopher's attestation states it directly — a smoke test
on a copy of this repository would have shown both defects immediately.

The graph already held this lesson twice, and neither instance was applied:

- `20260423-145830-s-prc-lgz` concluded that end-to-end smoke tests against real external tools are
  load-bearing **precisely when** domain logic is well covered by unit tests. The migration was
  well unit-tested and failed on the first real store.
- `20260716-155256-s-prc-686` concluded that validation driven by the implementer's own simulation
  inherits the implementer's assumptions. A synthetic fixture is the implementer's assumption about
  what a session log looks like; the real store was the only thing that knew otherwise.

This compounds `20260726-200257-s-prc-cbl`. The review regime generated the strict decoding as
defensive machinery and then verified that machinery as sound — with the strictness itself being the
defect. Review depth substituted for a single real run.

---

## 6. Judgment

The store-relocation and acknowledged-migration slice **does not work in use**. It fails at its
primary purpose on the first real store it met, and its one human decision point is invisible on a
real terminal. Recorded as a single folded finding at Christopher's direction, with fixing treated as
urgent.
