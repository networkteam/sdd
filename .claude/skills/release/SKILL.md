---
name: release
description: Cut a new release of the SDD CLI: survey changes against the graph, propose the version bump, draft curated notes, and ship after explicit user confirmation. Invoke when the user wants to create a release or bump the version.
---

# Release the SDD CLI

A release = a new git tag (`vX.Y.Z`) on `main`. The tag push triggers `.github/workflows/release.yaml`, which runs GoReleaser to publish binaries, build-provenance attestations, `install.sh`, and a Homebrew cask update at `networkteam/homebrew-tap`. The GitHub release body carries **hand-written notes**, not GoReleaser's auto-changelog.

## Workflow

### 1. Survey what's changed

```bash
git log $(git describe --tags --abbrev=0)..HEAD --oneline
```

The git log is the authoritative scope of the release. Commit subjects in this project often carry a graph entry ID like `(d-tac-e1s)` or `(s-tac-2b0)`. Collect all relevant IDs from the log, then pull them in one batch call:

```bash
sdd show <id1> <id2> <id3> --up 0 --down 0
```

Full bodies plus the envelope's immediate closes/refs context, no chain traversal, which is the right level for highlight prose. You see what plan closed what without walking the whole upstream history. Skip housekeeping commits (`sdd: refresh installed skills`, `sdd: signal/decision/wip` graph entries, pure refactors with no user-visible effect).

### 2. Propose the version bump

Pre-1.0 convention in this project:

- **Minor** (`0.X+1.0`): net-new commands, contract supersession, broad feature deltas (e.g. v0.5.0 = `sdd view` + 7+7 type system + engage refactor)
- **Patch** (`0.X.Y+1`): incremental polish, additive primitives, UX work within existing surfaces (e.g. v0.5.1 = `not(<filter>)` + `sdd init` readiness-check)

Surface the proposed level with reasoning before drafting notes. The user decides; ambiguous scope deserves dialogue.

### 3. Draft the release notes

Format:

```markdown
## Breaking

- **Short name of the break.** What changed, and what a consumer has to do about it.

## Highlights

- **Short name of the change.** One-paragraph narrative covering what shipped and the key surface details.

- **Second highlight.** ...

## Other changes

- One-line bullets for smaller items.

**Full Changelog**: https://github.com/networkteam/sdd/compare/vPREV...vNEW
```

Omit `## Breaking` when nothing broke. A break announced in an earlier prerelease of the same version is repeated in every later prerelease of that version, under a line saying it is unchanged and which release it came from, because a reader installing the later tag arrives from the last stable and never saw the earlier notes. A stable release carries the complete span since the previous stable, not just the delta since its last prerelease.

Highlights = paragraph-worthy items. Other changes = one-liners. Skip housekeeping commits. **Skip README and docs-only changes**, since the notes describe what shipped in the CLI binary, not documentation edits to the repo. Save the draft to `.sdd/tmp/vX.Y.Z-notes.md` (gitignored).

**Write plainly, and only as much as needed.** These notes are read by people outside this repo, so:

- **No em dashes.** Many readers take them as the first sign of AI-generated text. Use commas, colons, periods, or parentheses. Each bullet leads with its bolded name closed by a period, then a plain sentence.
- **No invented vocabulary.** Use the project's words or ordinary ones. If a term appears nowhere in the graph, the docs, or the code, do not introduce it here: "an untested build", not "a soak build".
- **Say the thing once.** State what shipped and what a reader must do about it. Do not explain the mechanism behind a command, justify a choice, or add the caveat that occurred to you while writing. A reader who needs that will ask.
- **Name the user-visible change, not the internal one.** Package and symbol names belong in notes only when a reader has to type them or implement against them.

A prerelease additionally opens with a blockquote saying what it is and the exact command to install it, because every safeguard that keeps it away from stable users also keeps it out of reach of anyone who wants it. One line on what the release is for, one on the safeguard, then:

    curl -sL https://github.com/networkteam/sdd/releases/latest/download/install.sh | sh -s -- vX.Y.Z-alpha.N

The `latest` in that URL fetches the installer script, not the version. The tag argument decides what gets installed. Do not explain that in the notes.

### 4. Play back and gate on explicit confirmation

Show the user the proposed version, the reasoning, and the full draft. **Wait for an explicit "yes / proceed / ship it"** before running anything. Edits accepted in dialogue are not consent. Ask if unclear.

### 5. Ship, in one command

```bash
gh release create vX.Y.Z --title vX.Y.Z --notes-file .sdd/tmp/vX.Y.Z-notes.md --target main
```

For a prerelease tag (`vX.Y.Z-alpha.N`, `-rc.N`), add `--prerelease`.

This single call creates the tag, pushes it, publishes the release with the curated body, and triggers the workflow. GoReleaser sees the existing release at the tag and attaches assets without overwriting the body, since `release.mode` defaults to `keep-existing`.

**Do not pass `--draft`.** GoReleaser does not honor a third-party draft; it creates a competing published release at the same tag, leaving the draft orphaned. (Learned via the v0.5.1 mishap.)

**Do not split into `git tag` + `git push origin vX.Y.Z` + `gh release edit`.** The single-command form rolls all three into one and is the proven minimal flow.

### 6. Verify

```bash
gh run list --workflow=release.yaml --limit 1
gh run watch <run-id>
gh release view vX.Y.Z          # body intact, assets attached
```

The workflow run typically completes in ~2 minutes. Sanity-check that the release body still shows the curated notes (not auto-generated) and that the assets list includes binaries, checksums, and `install.sh`.

For a prerelease, check the two things a prerelease is for, namely that stable consumers were left alone:

```bash
gh release view vX.Y.Z --json isPrerelease --jq .isPrerelease   # true
gh api repos/networkteam/sdd/releases/latest --jq .tag_name     # still the previous stable
gh api repos/networkteam/homebrew-tap/contents/Casks/sdd.rb --jq .content | base64 -d | grep version
```

`install.sh` resolves through `releases/latest`, which excludes prereleases, and `skip_upload: "auto"` keeps the cask off a prerelease tag, so both should be unmoved. Verify rather than assume: on v0.17.0-alpha.1 GoReleaser overwrote the prerelease flag that `gh release create --prerelease` had set, because `release.prerelease` then defaulted to `false`. It is now `auto` in `.goreleaser.yaml`, but the flag is worth reading back. If it is wrong, `gh release edit vX.Y.Z --prerelease` fixes it in place. Never re-tag.

## Recovery

If the workflow fails or completes partially: `gh run rerun <run-id>`. Re-runs are idempotent: GoReleaser attaches assets to the existing release without duplicating tap commits.

**Do not delete the release to redo it.** Deleting after the Homebrew tap was already pushed leaves the tap ahead of GitHub; recovery is messier than the original problem.

**Do not re-tag.** If the tag points at the right commit, rerun the workflow. Wrong commit is a separate problem worth capturing as a signal first.
