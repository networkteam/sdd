# Release pipeline infrastructure — plan

First sub-plan of d-cpt-uu1 (SDD distribution). Establishes the end-to-end release pipeline for SDD via Homebrew + a curl installer, publishing v0.1.0 as the validation milestone. Skill extraction (`sdd install`), graph schema metadata, and `sdd migrate` are carved out to a follow-on plan.

## Scope

**In scope:**
- GoReleaser v2 config (`.goreleaser.yaml`) — builds, archives, changelog, tap publish
- Version stamping via `-ldflags` → `main.version`; `sdd --version` surface
- CI workflow: test matrix (ubuntu-latest + macos-latest) + lint (ubuntu-latest)
- `.golangci.yml` with explicit linter set
- Release workflow: GoReleaser + native build provenance attestations
- `networkteam/homebrew-sdd` tap repo + GoReleaser `brews` cross-repo push
- binstaller config + committed `install.sh` + CLAUDE.md regen procedure
- README acquisition documentation (three paths)
- v0.1.0 end-to-end validation on fresh darwin + linux machines

**Out of scope (follow-on plan):**
- `sdd install` subcommand
- Skill `go:embed` and extraction to `~/.claude/skills/`
- Graph metadata file (`.sdd-meta.json`) and startup schema check
- `sdd migrate` subcommand
- `sdd self upgrade`
- Claude Code plugin marketplace channel (remains deferred per d-cpt-uu1)

## Tool versions locked in

Pinned from 2026 landscape research (see `.sdd/tmp/d-cpt-uu1-research/`):

- GoReleaser v2 via `goreleaser/goreleaser-action@v7` with `version: "~> v2"`
- `actions/checkout@v5`
- `actions/setup-go@v6` — built-in module + build cache, no extra `actions/cache` needed
- `golangci/golangci-lint-action@v8`, lint tool version `v2.1`
- `actions/attest-build-provenance@v1` — native GitHub attestations
- `binstaller` (`github.com/binary-install/binstaller`) — config initialized from `.goreleaser.yaml`

## Deviations from d-cpt-uu1's attachment

**Attestations — native instead of wrapper.** d-cpt-uu1 specified `actionutils/trusted-go-releaser` for attestation chain-of-trust. 2026 research (`research-3.md`) shows GitHub's native `actions/attest-build-provenance` has become the idiomatic primitive; the wrapper is a convenience layer around the same underlying mechanism but adds indirection without trust-model benefit. `gh attestation verify` consumers use `--signer-workflow` pointing at our own `release.yaml` rather than a wrapper's workflow path.

**Homebrew field — `brews` retained.** GoReleaser deprecated `brews` in v2.10 in favor of `homebrew_casks`. Casks are macOS-only; our CLI runs on macOS + Linux (including Linuxbrew users). Formula via `brews` is the cross-platform-correct choice for a CLI; we accept the deprecation warning and revisit if the field is removed.

**`eget` considered and rejected.** Briefly considered as a lightweight alternative to binstaller. Rejected: offloads bootstrap ("install eget first") rather than solving first-time install for users without brew. binstaller-generated `install.sh` remains the right fallback — it's derived from `.goreleaser.yaml`, so the release surface stays in sync automatically when regenerated.

## Resolved choices

1. **CI test matrix**: ubuntu-latest + macos-latest. Mirrors release targets (darwin + linux); windows is neither built nor tested. Saves CI minutes on a platform we don't ship.

2. **Lint ruleset**: explicit set — `errcheck, gosimple, govet, ineffassign, staticcheck, unused, gofmt, goimports, misspell, unparam`. Greenfield repo — set the floor now rather than retrofit later. Extend if things slip through.

3. **`install.sh` regeneration**: committed to repo. Regenerated manually when `.goreleaser.yaml` changes. Avoids CI complexity; produces reviewable diffs when the release surface shifts. CLAUDE.md documents the regen command.

4. **Attestation scope**: all release assets — tarballs, checksums, and `install.sh`. Natural default with native attestations; one `subject-path` glob covers everything.

5. **Initial tag**: v0.1.0. Matches d-cpt-uu1's wording; pre-1.0 already signals "anything can break" without needing a `v0.0.1` alpha framing.

## Config scaffolds (reference)

The implementing agent should adapt these to the project layout, but the shape is settled.

### `.goreleaser.yaml`

```yaml
version: 2
project_name: sdd

before:
  hooks:
    - go mod tidy

builds:
  - id: sdd
    main: ./cmd/sdd
    binary: sdd
    env:
      - CGO_ENABLED=0
    goos: [darwin, linux]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w -X main.version={{ .Version }}

archives:
  - ids: [sdd]

brews:
  - name: sdd
    repository:
      owner: networkteam
      name: homebrew-sdd
      branch: main
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    test: |
      system "#{bin}/sdd --version"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^chore:"

release:
  github:
    owner: networkteam
    name: sdd
```

### `.github/workflows/ci.yaml`

```yaml
name: ci
on:
  pull_request:
  push:
    branches: [main]
permissions:
  contents: read
  pull-requests: read
jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest]
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version: '1.26'
      - run: go test ./...
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version: '1.26'
      - uses: golangci/golangci-lint-action@v8
        with:
          version: v2.1
```

### `.github/workflows/release.yaml`

```yaml
name: release
on:
  push:
    tags: ['v*']
permissions:
  contents: write
  id-token: write
  attestations: write
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v6
        with:
          go-version: '1.26'
      - uses: goreleaser/goreleaser-action@v7
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
      - uses: actions/attest-build-provenance@v1
        with:
          subject-path: |
            dist/*.tar.gz
            dist/*.txt
            dist/install.sh
```

### `.golangci.yml`

```yaml
version: "2"
linters:
  default: none
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gofmt
    - goimports
    - misspell
    - unparam
```

## Rough sequencing

1. Wire `main.version` in `cmd/sdd/main.go` + `--version` flag; confirm `go test ./...` still passes.
2. Write `.goreleaser.yaml` and run `goreleaser check` locally.
3. Create `.golangci.yml`; run `golangci-lint run` locally and fix any baseline issues surfaced.
4. Add `.github/workflows/ci.yaml`; open a PR to verify it runs green on ubuntu + macos.
5. Create `networkteam/homebrew-sdd` repo (empty `main` branch); generate fine-grained PAT scoped to that repo with contents:write; store as `HOMEBREW_TAP_TOKEN` secret on `networkteam/sdd`.
6. Add `.github/workflows/release.yaml` with attestation step.
7. `binst init --source=goreleaser --file=.goreleaser.yaml`; commit `.config/binstaller.yml` and the generated `install.sh`; document regen command in CLAUDE.md.
8. Update README with "Install" section covering all three paths.
9. Tag v0.1.0, push, validate end-to-end on fresh darwin + linux machines; verify `gh attestation verify` succeeds on both tarball and `install.sh` asset types.

## Open sub-questions (deferred)

- Graph metadata file location post-fork — d-cpt-uu1 said `docs/framework/graph/.sdd-meta.json`; repo layout is now `.sdd/graph/`, so likely `.sdd/meta.json` or similar. Decide in the follow-on plan where this surfaces in code.
- Dependabot / renovate for automated dep updates — convention but not MVP.
- Coverage reporting (codecov or similar) — nice-to-have, not MVP.
- `sdd version --check` network nudge — explicitly deferred in d-cpt-uu1.
- Pre-release tag shape (v0.1.0-beta.1 etc.) — not needed at MVP.
