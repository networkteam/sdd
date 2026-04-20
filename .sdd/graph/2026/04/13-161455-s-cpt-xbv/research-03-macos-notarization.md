In practice, the realistic 2026 paths are still: pay for Apple’s Developer Program and sign/notarize properly, or ship unsigned/ad hoc-signed binaries and accept a first-run friction/workaround. Apple’s notarization flow is tied to Developer ID distribution outside the Mac App Store, and Apple’s docs still frame notarization as the normal path for software distributed with Developer ID. [developer.apple](https://developer.apple.com/programs/)

## (a) Individual developers

Yes, an **individual** can enroll without a company entity, but it is still the paid Apple Developer Program, not a free account path to notarization. Free/individual Apple IDs can build and run locally, but Apple’s notarization and Developer ID distribution model is for enrolled developers with the relevant certificates and service access. So the answer is effectively: no free notarization for public distribution. [marcabien](https://marcabien.com/en/register-for-apple-developer-from-abroad-step-by-step-guide)

## (b) Homebrew and Gatekeeper

Homebrew’s behavior is a bit nuanced. Bottled formulae are generally installed without a quarantine xattr on the final Cellar files in the normal case, but Homebrew has had issues and flags around `--no-quarantine` for cases where downloaded items or postinstall steps would trigger Gatekeeper prompts. In other words, Homebrew avoids prompts by controlling quarantine handling in its install path, not because every bottle is Apple-notarized and stapled. Casks are the more obvious place where quarantine is commonly managed by Homebrew, while formula binaries often rely on how Homebrew downloads, pours, and installs them. [github](https://github.com/Homebrew/homebrew-bundle/issues/474)

## (c) What those projects do

`uv` is a useful example of the “ship it, mostly unsigned” camp: a 2025 issue reported its macOS executables were not code signed by Astral, and later discussion mentions adding ad-hoc signing in the release pipeline for macOS binary behavior. `starship` historically has discussed notarization and signing as a release-workflow improvement, but the issue trail shows that this was aspirational/iterative rather than an obvious always-on Apple Developer ID deployment model. `zoxide`’s release workflow in GitHub shows plain tar.gz/zip artifact packaging, not a visible macOS notarization pipeline in the release packaging snippet. For `ripgrep` and `bat`, the commonly seen distribution pattern is similarly simple release archives rather than a clearly public, Apple-notarized macOS app distribution flow in the release artifacts surfaced here; the broader Rust CLI ecosystem commonly ships unsigned or ad hoc-signed CLI binaries and relies on users extracting them locally or installing through package managers. [github](https://github.com/astral-sh/uv/issues/14870)

## (d) curl|bash and quarantine

Yes, that pattern exists. Homebrew’s own issue tracker explicitly references `--no-quarantine`, and the workaround people mention is to remove quarantine with tools like `xattr` or to install in a way that does not apply quarantine in the first place [github](https://github.com/Homebrew/homebrew-bundle/issues/474). For curl|bash style installers, the practical goal is usually to download, install, and then run `xattr -d com.apple.quarantine ...` or equivalent logic so Gatekeeper does not block the first execution; that is a workaround, not a notarization substitute [github](https://github.com/Homebrew/brew/issues/14531). This is common in installer scripts because the script can “bless” the installed binary by clearing the downloaded-file quarantine attribute after verifying what it just installed. [github](https://github.com/Homebrew/brew/issues/14531)

## Tradeoff

“Notarize properly” gives the smoothest first-run experience and the best Apple-native trust signal, but it costs time, money, and release-process complexity, and it generally requires Apple’s paid Developer Program infrastructure. Accepting first-run workaround friction is cheaper and simpler for an open-source CLI, especially if you distribute via GitHub Releases and your audience is technical enough to use Homebrew, `xattr`, or right-click-open when needed. The tradeoff is basically conversion vs. ops burden: proper signing/notarization reduces user friction and support tickets, while unsigned distribution keeps maintainer overhead low but makes macOS security prompts part of the onboarding. [github](https://github.com/Homebrew/brew/issues/14531)

For a Go CLI shipped on GitHub Releases, the most realistic 2026 decision is usually either:

- pay for Apple Developer Program and notarize the binaries, or
- ship plain/ad hoc-signed archives and document the first-run workaround clearly. [developer.apple](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)
