For a small open-source Go CLI in 2026, the most practical default is: publish versioned binaries to **GitHub Releases** with **GoReleaser**, add a **Homebrew formula/tap** for macOS/Linux users who already use Brew, and optionally provide a lightweight installer script or package-manager entry for power users and automation. That combination gives the best mix of discoverability, easy updates, and low maintenance for a ~10 MB binary across macOS arm64/x86_64 and Linux x86_64/arm64. [goreleaser](https://goreleaser.com/customization/publish/homebrew_formulas/)

## What each channel does

- **GitHub Releases** is the canonical distribution layer for small CLI tools: it is simple, versioned, easy to automate with GoReleaser, and supports multiple OS/arch assets directly from tagged releases. [goreleaser](https://goreleaser.com/customization/ci/actions/)
- **Homebrew tap/formula** gives the smoothest install experience on macOS and Linux for developer audiences because users can type `brew install yourtool` and get integrated upgrades through Brew. [formulae.brew](https://formulae.brew.sh/formula/starship)
- **`go install ...@version`** is good for Go developers, but it is not great for general end users because it requires a working Go toolchain and usually does not feel like a “real app install” for non-Go users. [github](https://github.com/ajeetdsouza/zoxide?tab=readme-ov-file)
- **`curl | bash` installers** maximize reach and minimize friction for users who do not want a package manager, but they are harder to trust, less transparent, and require you to maintain installer logic and platform-specific branching. [starship](https://starship.rs/faq/)
- **aqua / mise / similar tool registries** are best when your audience already uses developer environment managers and wants declarative installs in CI or dotfiles, not as the primary route for ordinary users. [aquaproj.github](https://aquaproj.github.io/docs/tutorial/)
- **Docker images** are useful when the CLI is primarily used in CI or ephemeral environments, but they are usually a poor fit for local interactive use because they add container overhead and do not feel native. [goreleaser](https://goreleaser.com/blog/rust-zig/)

## Best-practice stack

For a Go CLI like the one you described, the converged pattern is: ship GitHub Release assets for every supported OS/arch, automate release creation with GoReleaser, and expose a Homebrew path for convenience. GoReleaser’s documented flow is built around tagged releases and GitHub Actions, and it can publish artifacts plus keep Brew distribution in sync. [goreleaser](https://goreleaser.com/customization/publish/homebrew_formulas/)

A sensible setup in 2026 looks like this:

1. Build and sign release binaries for macOS arm64/x86_64 and Linux arm64/x86_64.
2. Upload them to GitHub Releases as the primary distribution source.
3. Generate or maintain a Brew formula/tap so macOS users can do `brew install yourtool`.
4. Optionally publish an install script for users outside Brew.
5. Optionally add entries to aqua/mise for teams that manage tools declaratively. [mise.jdx](https://mise.jdx.dev/dev-tools/backends/aqua.html)

## Trade-offs by channel

| Channel         | Discoverability                                                                                                                    | Update path                                                                                                                                            | Platform coverage                                                                                     | User effort                                               |
| --------------- | ---------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------- | --------------------------------------------------------- | ---------------------------------------------------- |
| GitHub Releases | Medium; users must know the repo or be linked there. [goreleaser](https://goreleaser.com/customization/publish/homebrew_formulas/) | Manual download or scripted upgrade; good when paired with an installer. [goreleaser](https://goreleaser.com/customization/publish/homebrew_formulas/) | Excellent if you publish all assets. [goreleaser](https://goreleaser.com/customization/ci/actions/)   | Moderate.                                                 |
| Homebrew tap    | High among devs on macOS/Linux. [formulae.brew](https://formulae.brew.sh/formula/goreleaser)                                       | Excellent via `brew upgrade`. [formulae.brew](https://formulae.brew.sh/formula/goreleaser)                                                             | Good for macOS and Linux, but not Windows. [formulae.brew](https://formulae.brew.sh/formula/starship) | Low.                                                      |
| `go install`    | High only for Go users. [github](https://github.com/direnv/direnv/issues/318)                                                      | Reinstall by version tag. [github](https://github.com/direnv/direnv/issues/318)                                                                        | Any platform Go supports. [github](https://github.com/direnv/direnv/issues/318)                       | Medium to high, because Go is required.                   |
| `curl           | bash`                                                                                                                              | Low to medium; depends on docs and trust. [starship](https://starship.rs/faq/)                                                                         | Can self-update if you implement it, but that adds maintenance.                                       | Potentially broad, but you must handle each OS carefully. | Very low to start, higher for trust and maintenance. |
| aqua / mise     | Low for casual users, high for infra-heavy teams. [aquaproj.github](https://aquaproj.github.io/docs/tutorial/)                     | Good for pinned versions and reproducible toolsets. [aquaproj.github](https://aquaproj.github.io/docs/tutorial/)                                       | Depends on the upstream release assets. [aquaproj.github](https://aquaproj.github.io/docs/tutorial/)  | Low once adopted, but only by the right audience.         |
| Docker          | Low for end users, high for CI. [goreleaser](https://goreleaser.com/blog/rust-zig/)                                                | Pull image tags to update.                                                                                                                             | Broad in containerized environments.                                                                  | Higher for local use; moderate for CI.                    |

## What popular tools converged on

These tools mostly converged on the same pattern: **GitHub Releases as the source of truth, plus a package-manager convenience layer**. `zoxide` documents installs via crates.io, Homebrew, and many distro/package-manager paths, with a shell installer as an extra option for the long tail. `starship` does the same, offering crates.io plus Homebrew and a shell installer for users who want a direct binary bootstrap. `direnv` also offers a shell installer, package-manager installs, and direct release binaries, which is a strong signal that maintainers treat GitHub Releases as the canonical artifact source while using other channels for convenience. [oppositeofnorth](https://oppositeofnorth.com/?_=%2Fajeetdsouza%2Fzoxide%23NpZ%2FcdvQaILgLgJ6rorzPjGP)

`gh` and similar tools in the GitHub ecosystem generally follow the same broad philosophy: release binaries publicly, then let users install through package managers or a simple download/install path rather than forcing source builds. GoReleaser itself exemplifies this model by publishing releases and supporting Brew publication from the same pipeline. [formulae.brew](https://formulae.brew.sh/formula/goreleaser)

## Recommended choice for your case

For a ~10 MB Go CLI targeting macOS arm64/x86_64 and Linux x86_64/arm64, I would recommend this order:

- **Primary:** GitHub Releases with GoReleaser. [goreleaser](https://goreleaser.com/customization/ci/actions/)
- **Secondary:** Homebrew tap or formula for macOS/Linux dev users. [formulae.brew](https://formulae.brew.sh/formula/starship)
- **Tertiary:** `go install @version` only if your audience is Go-native. [github](https://github.com/direnv/direnv/issues/318)
- **Optional:** one-line installer for users who refuse package managers. [direnv](https://direnv.net/docs/github-actions.html)
- **Optional:** aqua/mise for teams that pin tooling in repo config and CI. [aquaproj.github](https://aquaproj.github.io/docs/tutorial/)
- **Optional:** Docker image only if the CLI is commonly used in CI or as part of pipelines. [goreleaser](https://goreleaser.com/blog/rust-zig/)

## Practical rule of thumb

If your goal is broad adoption with low maintenance, make GitHub Releases the canonical source, use GoReleaser to automate everything, and add Homebrew for the “just works” path on developer machines. Add `go install` and `curl | bash` only as convenience layers, and treat aqua/mise and Docker as niche distribution paths for specific workflows rather than your main channel. [goreleaser](https://goreleaser.com/customization/ci/actions/)

Would you like a concrete release checklist and a sample GoReleaser config for this setup?
