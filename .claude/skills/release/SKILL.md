---
name: release
description: Cut a new release of the SDD CLI — survey changes against the graph, propose the version bump, draft curated notes, and ship after explicit user confirmation. Invoke when the user wants to create a release or bump the version.
---

# Release the SDD CLI

A release = a new git tag (`vX.Y.Z`) on `main`. The tag push triggers `.github/workflows/release.yaml`, which runs GoReleaser to publish binaries, build-provenance attestations, `install.sh`, and a Homebrew cask update at `networkteam/homebrew-tap`. The GitHub release body carries **hand-written notes** — not GoReleaser's auto-changelog.

## Workflow

### 1. Survey what's changed

```bash
git log $(git describe --tags --abbrev=0)..HEAD --oneline
```

The git log is the authoritative scope of the release. Commit subjects in this project often carry a graph entry ID like `(d-tac-e1s)` or `(s-tac-2b0)` — collect all relevant IDs from the log, then pull them in one batch call:

```bash
sdd show <id1> <id2> <id3> --max-depth 0
```

Full bodies plus immediate closes/refs context, no deep chain traversal — the right level for highlight prose. You see what plan closed what without walking the whole upstream history. Skip housekeeping commits (`sdd: refresh installed skills`, `sdd: signal/decision/wip` graph entries, pure refactors with no user-visible effect).

### 2. Propose the version bump

Pre-1.0 convention in this project:

- **Minor** (`0.X+1.0`) — net-new commands, contract supersession, broad feature deltas (e.g. v0.5.0 = `sdd view` + 7+7 type system + engage refactor)
- **Patch** (`0.X.Y+1`) — incremental polish, additive primitives, UX work within existing surfaces (e.g. v0.5.1 = `not(<filter>)` + `sdd init` readiness-check)

Surface the proposed level with reasoning before drafting notes. The user decides; ambiguous scope deserves dialogue.

### 3. Draft the release notes

Format follows v0.5.0 / v0.5.1:

```markdown
## Highlights

- **`feature` — short technical name** — one-paragraph narrative covering what shipped and the key surface details.

- **Second highlight** — ...

## Other changes

- One-line bullets for smaller items.

**Full Changelog**: https://github.com/networkteam/sdd/compare/vPREV...vNEW
```

Highlights = paragraph-worthy items. Other changes = one-liners. Skip housekeeping commits. Save the draft to `.sdd/tmp/vX.Y.Z-notes.md` (gitignored).

### 4. Play back and gate on explicit confirmation

Show the user the proposed version, the reasoning, and the full draft. **Wait for an explicit "yes / proceed / ship it"** before running anything. Edits accepted in dialogue ≠ consent — ask if unclear.

### 5. Ship — one command

```bash
gh release create vX.Y.Z --title vX.Y.Z --notes-file .sdd/tmp/vX.Y.Z-notes.md --target main
```

This single call creates the tag, pushes it, publishes the release with the curated body, and triggers the workflow. GoReleaser sees the existing release at the tag and attaches assets without overwriting the body — `release.mode` defaults to `keep-existing`.

**Do not pass `--draft`.** GoReleaser does not honor a third-party draft; it creates a competing published release at the same tag, leaving the draft orphaned. (Learned via the v0.5.1 mishap.)

**Do not split into `git tag` + `git push origin vX.Y.Z` + `gh release edit`.** The single-command form rolls all three into one and is the proven minimal flow.

### 6. Verify

```bash
gh run list --workflow=release.yaml --limit 1
gh run watch <run-id>
gh release view vX.Y.Z          # body intact, assets attached
```

The workflow run typically completes in ~2 minutes. Sanity-check that the release body still shows the curated notes (not auto-generated) and that the assets list includes binaries, checksums, and `install.sh`.

## Recovery

If the workflow fails or completes partially: `gh run rerun <run-id>`. Re-runs are idempotent — GoReleaser attaches assets to the existing release without duplicating tap commits.

**Do not delete the release to redo it.** Deleting after the Homebrew tap was already pushed leaves the tap ahead of GitHub; recovery is messier than the original problem.

**Do not re-tag.** If the tag points at the right commit, rerun the workflow. Wrong commit is a separate problem worth capturing as a signal first.
