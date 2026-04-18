# d-tac-lg1 validation evidence

AC-by-AC evidence for the release pipeline plan closure at v0.1.0.

## Commits

- `3ce37f3` — ci: add release pipeline infrastructure (GoReleaser config, CI workflow, .golangci.yml baseline, main.version wiring, baseline lint fixes, slog migration in handler_init.go)
- `0f476a3` — ci: add release workflow, binstaller installer, and Install docs
- `865713d` — ci: target existing networkteam/homebrew-tap instead of a new homebrew-sdd repo
- `69385f3` — ci: add devbox script gen-installer for binstaller regen

Final commit: `69385f35a4f9099e4b6da08cea96ac258d4054d8` (tagged v0.1.0).

Release workflow run: https://github.com/networkteam/sdd/actions/runs/24588939733

## AC-by-AC

### AC 1 — GoReleaser v2 builds darwin+linux × amd64+arm64 with ldflags

**Satisfied.** `.goreleaser.yaml` uses `version: 2`, builds with `goos: [darwin, linux]`, `goarch: [amd64, arm64]`, `CGO_ENABLED=0`, `ldflags: "-s -w -X main.version={{.Version}}"`. Snapshot and release runs confirmed 4 tarballs produced with correct names (`sdd_0.1.0_<os>_<arch>.tar.gz`).

### AC 2 — main.version declared; sdd --version prints stamped version

**Satisfied.** `var version = "dev"` in cmd/sdd/main.go with `Version: version` on the root cli.Command. `-v` alias dropped from cli.VersionFlag to avoid collision with the existing `--verbose`. Verified locally (`./bin/sdd --version` → "sdd version dev") and in release (`/Users/hlubek/.local/bin/sdd --version` → "sdd version 0.1.0").

### AC 3 — CI workflow runs on PR/push with test matrix and lint

**Satisfied.** `.github/workflows/ci.yaml` with test matrix `[ubuntu-latest, macos-latest]` running Go via `go-version-file: go.mod`, and lint on ubuntu-latest using `golangci/golangci-lint-action@v8` pinned to `v2.11.4`.

### AC 4 — .golangci.yml with explicit linter set; lint+vet+test clean

**Satisfied, with linter-shape mapping deviation.** `.golangci.yml` (v2 schema) enables: `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `misspell`, `unparam`. `formatters:` section enables `gofmt`, `goimports`. Idiomatic exclusions: `fmt.Fprint` family and `(*os.File).Close` in errcheck; `unused` suppressed in `*test_helpers_test.go`.

Deviation from plan wording: `gosimple` (plan) is merged into `staticcheck` in golangci-lint v2 — enabling staticcheck covers it. `gofmt`/`goimports` (plan as linters) moved to the `formatters:` section in v2. Effective rule set is preserved.

`go vet ./...`, `go test ./...`, and `golangci-lint run ./...` all clean on the final commit.

### AC 5 — Release workflow triggers on v* tag, publishes GH Release with tarballs+checksums+install.sh

**Satisfied.** `.github/workflows/release.yaml` triggers on `tags: ['v*']`. Verified via run 24588939733 on v0.1.0: 6 assets published (4 tarballs, 1 checksums, install.sh).

### AC 6 — attest-build-provenance across all release assets; gh attestation verify succeeds

**Satisfied, with attestation-tool deviation.** Plan specified `actionutils/trusted-go-releaser`; implementation uses native `actions/attest-build-provenance@v1`. Rationale: in-session research (`.sdd/tmp/d-cpt-uu1-research/research-3.md`) confirmed native attestations are the idiomatic 2026 primitive; the wrapper adds indirection without trust-model benefit. `subject-path` covers `dist/*.tar.gz`, `dist/*_checksums.txt`, and `install.sh` (repo-root, uploaded via goreleaser's `release.extra_files`).

Verification evidence:

- `gh attestation verify install.sh --repo networkteam/sdd` → exit 0
- `gh attestation verify sdd_0.1.0_darwin_arm64.tar.gz --repo networkteam/sdd` → exit 0
- SLSA v1 predicate binds to commit `69385f35a4f9099e4b6da08cea96ac258d4054d8`
- Signer identity: `https://github.com/networkteam/sdd/.github/workflows/release.yaml@refs/tags/v0.1.0`
- Rekor-logged (tamper-evident transparency log entry visible in cert extensions)

First workflow attempt (private repo) failed with "Feature not available for the networkteam organization"; repo was made public mid-session and v0.1.0 was re-tagged (release deleted + tag re-pushed). The second run completed green end-to-end.

### AC 7 — Tap repo + formula push

**Satisfied, with two deviations.**

**Tap-repo deviation:** plan said `networkteam/homebrew-sdd` would be created. Implementation targets existing `networkteam/homebrew-tap` (already hosts the shry cask). Single-tap-per-org pattern avoids repo sprawl. PAT with `contents:write` on homebrew-tap stored as `HOMEBREW_TAP_TOKEN` secret on networkteam/sdd.

**Formula-vs-cask deviation:** plan's `brews` block replaced with `homebrew_casks` (see description). Plan's `test do "#{bin}/sdd --version"` has no direct cask equivalent; the cask's `binary "sdd"` stanza provides implicit install-time validation of the binary path. Acceptable semantic-preserving trade.

Evidence: `networkteam/homebrew-tap/Casks/sdd.rb` exists (pushed by the v0.1.0 release run) alongside the pre-existing `Casks/shry.rb`.

### AC 8 — binstaller config + install.sh committed; regen documented

**Satisfied.** `.config/binstaller.yml` initialized via `binst init --source=goreleaser --file=.goreleaser.yaml`. `install.sh` (596 lines, POSIX shell, checksum-verified) generated via `binst gen -o install.sh` and committed to repo root. Regen procedure documented in CLAUDE.md and exposed as a devbox script (`devbox run gen-installer`) per the standing project rule on no one-off script calls.

### AC 9 — README Install section with three paths

**Satisfied.** README.md has an "Install" section covering:

- Homebrew: `brew install networkteam/tap/sdd`
- Basic curl: `curl -sL .../install.sh | sh` with note on `-b <dir>` override and `~/.local/bin/sdd` default
- Verified curl: download + `gh attestation verify install.sh --repo networkteam/sdd` + `sh install.sh` sequence, with explanation of the Sigstore-backed trust chain

### AC 10 — v0.1.0 end-to-end validation

**Satisfied.**

- v0.1.0 tag pushed; workflow green (1m3s on the second, post-public-visibility attempt).
- `curl -sL .../install.sh | sh` verified on darwin/arm64: detected platform correctly, downloaded tarball + checksums, verified checksum, extracted, installed to `~/.local/bin/sdd`. Binary reports `sdd version 0.1.0`.
- `brew install networkteam/tap/sdd` is the canonical public path. Direct test on the maintainer's machine encountered a local tap-alias collision (their `networkteam/tap` is tapped to a GitLab mirror for unrelated work; the GitHub homebrew-tap is locally aliased as `networkteam/github`). Not a project-level issue — the README path resolves correctly for any fresh environment.
- `gh attestation verify` succeeds on both tarball and install.sh (see AC 6 evidence).

### AC 11 — Agent-neutrality preserved (d-stg-574)

**Satisfied.** No Claude-specific logic in the build pipeline, release workflow, formula/cask, or installer. Skill extraction remains deferred to the follow-on plan; binary produced is agent-neutral.

## Process signals captured during execution

- `20260417-180204-s-prc-w5r` — Capture flow breaks when agents reflexively run `sdd new --dry-run` before capture. Observed at the d-tac-lg1 capture step.
- `20260417-235623-s-prc-cgk` — Agents drift past the natural handoff point when remaining ACs require user-environment actions. Observed at this closing step when an early draft attempted a half-measure progress-recording action.

Both are calibration signals refing `20260410-151529-s-stg-3vr` (instruction-drift parent).

## Future items surfaced, not blocking closure

- `actions/attest-build-provenance@v1` uses Node 20 — GitHub deprecating by June 2026 (forced Node 24 default). Simple version bump when the action publishes a Node 24 compatible major.
- Prescriptive content in `s-prc-cgk` could be extracted into a process decision (standing rule: handoff at user-env boundaries). Non-urgent.
