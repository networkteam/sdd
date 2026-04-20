The idiomatic pattern is: install a **small bootstrap binary first**, then let that binary own all ongoing state under a user-scoped home directory and expose explicit subcommands like `install`, `toolchain install`, `use`, `extension install`, or `plugin install` for the second stage. In practice, the best tools separate **binary acquisition** from **runtime/assets/tooling acquisition**, so “zero to working” is usually 1–3 commands, not a magical one-shot install hidden behind first run. [rust-lang.github](https://rust-lang.github.io/rustup/concepts/toolchains.html)

## The common pattern

A two-stage install usually looks like this: first, a package manager, shell script, or standalone installer places the main executable on `PATH`; second, the executable populates user-owned state such as toolchains, virtualenvs, plugins, caches, completions, or extensions under directories like `~/.cargo`, `~/.local/bin`, `~/.config`, or tool-specific stores. [rust-lang.github](https://rust-lang.github.io/rustup/installation/index.html)

That split is idiomatic because it keeps the initial install small and auditable while allowing the tool itself to manage upgrades, add-ons, and per-user state afterward. It also avoids asking the OS package manager to know about highly dynamic content like language toolchains, Python interpreters, GitHub extensions, or plugin repos. [docs.github](https://docs.github.com/en/github-cli/github-cli/using-github-cli-extensions)

## What rustup does

`rustup`’s advertised first-touch install is the shell bootstrap command from Rust’s install page, and the Rust docs describe `rustup` as installing `rustc`, `cargo`, `rustup`, and other standard tools into `~/.cargo/bin` on Unix. [rust-lang](https://rust-lang.org/tools/install/)

So for Rust, the bootstrap binary and the “payload” are tightly coupled: `rustup-init` installs the manager plus a default toolchain, and that yields a working `cargo` right away for the default case. When users need something more controlled, the docs recommend a two-phase flow: install `rustup` with `--default-toolchain none`, then run `rustup toolchain install nightly --allow-downgrade --profile minimal --component clippy` or similar to fetch the actual toolchain/components. [rust-lang.github](https://rust-lang.github.io/rustup/concepts/toolchains.html)

## What uv does

`uv` advertises a standalone installer in its README and install docs: `curl -LsSf https://astral.sh/uv/install.sh | sh` on macOS/Linux, with Windows PowerShell equivalents and package-manager alternatives like `pipx install uv`, `brew install uv`, and others. [raw.githubusercontent](https://raw.githubusercontent.com/astral-sh/uv/main/README.md)

After that, `uv` handles second-stage environment creation and management through normal commands rather than a special global “self install” step. Its README example shows `uv init example`, `cd example`, then `uv add ruff`, at which point `uv` creates `.venv` automatically; its environment docs also show that it can target arbitrary interpreters or even system Python with `--python` or `--system`. [docs.astral](https://docs.astral.sh/uv/pip/environments/)

## What gh does

GitHub CLI is typically installed first with a platform package manager such as Homebrew, then extensions are added later with `gh extension install OWNER/REPO` or a full repo URL. The GitHub docs explicitly separate these phases and describe `gh extension install` as the way to pull in an extension from a GitHub or local repository. [github](https://github.com/github/gh-classroom/blob/main/README.md)

A useful detail is that local development installs are managed as symlinks to an executable in the cloned repo root, which means `gh` is really acting as an extension host and dispatcher rather than copying arbitrary files into a separate opaque bundle. That is a strong model if you want your CLI to support both “install from registry/repo” and “link local dev checkout” workflows. [cli.github](https://cli.github.com/manual/gh_extension_install)

## What mise does

`mise` installs the CLI first, then uses subcommands to acquire plugins and tools, with docs stating that it installs to `~/.local/bin` by default. Its plugin docs show both explicit plugin installation via `mise plugins install ...` and automatic plugin installation when a user installs a tool like `mise install cmake@3.30`. [mise.jdx](https://mise.jdx.dev/cli/plugins/install.html)

The walkthrough goes a step further and recommends `mise use` as the main first-touch command because it both installs the tool and writes activation into `mise.toml`; the docs warn that `mise install` alone does not make the tool available in the shell configuration for that project. That is a nice example of a second-stage command doing both **materialization** and **activation**, which is often what users actually mean by “working.” [mise.jdx](https://mise.jdx.dev/walkthrough.html)

## First-touch commands

| Tool     | README / docs first-touch                                                                                                                                                          | What it installs first                                                                                                                                                              | What user types next to be working                                                                                                                                                                                                                                |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `rustup` | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` from the Rust/rustup install flow [rust-lang.github](https://rust-lang.github.io/rustup/installation/index.html) | `rustup`, `rustc`, `cargo`, standard tools into `~/.cargo/bin` by default [rust-lang.github](https://rust-lang.github.io/rustup/installation/index.html)                            | Usually nothing beyond opening a new shell and checking `rustc --version`; advanced users may then run `rustup toolchain install ...` [rust-lang.github](https://rust-lang.github.io/rustup/installation/index.html)                                              |
| `uv`     | `curl -LsSf https://astral.sh/uv/install.sh \| sh` in README/docs [raw.githubusercontent](https://raw.githubusercontent.com/astral-sh/uv/main/README.md)                           | `uv` binary via standalone installer, typically under `~/.local/bin`; self-update is then available [rust-lang.github](https://rust-lang.github.io/rustup/concepts/toolchains.html) | `uv init <project>`, `cd <project>`, `uv add <pkg>`; `uv` creates `.venv` as needed [raw.githubusercontent](https://raw.githubusercontent.com/astral-sh/uv/main/README.md)                                                                                        |
| `gh`     | Often package-manager install first, e.g. `brew install gh` in extension READMEs [github](https://github.com/github/gh-classroom/blob/main/README.md)                              | Core `gh` binary only [github](https://github.com/github/gh-classroom/blob/main/README.md)                                                                                          | `gh extension install owner/repo` for extensions; normal `gh auth login` is usually needed before real GitHub use, though extension install syntax itself is separate [docs.github](https://docs.github.com/en/github-cli/github-cli/using-github-cli-extensions) |
| `mise`   | Install `mise` CLI first; docs say default install location is `~/.local/bin` [mise.jdx](https://mise.jdx.dev/getting-started.html)                                                | Core `mise` binary [mise.jdx](https://mise.jdx.dev/getting-started.html)                                                                                                            | `mise use <tool>@<version>` is the recommended working command because it installs and activates; `mise plugins install ...` is explicit plugin-first mode [mise.jdx](https://mise.jdx.dev/walkthrough.html)                                                      |

## Design guidance

If you want your CLI to “install itself further,” the cleanest pattern is to make the second stage an explicit noun-verb command like `tool install`, `tool use`, `tool plugin install`, or `tool extension install`, and have it write only to clearly named user-owned directories under XDG paths or a well-known tool home. That mirrors `rustup`’s split between binary and toolchains, `uv`’s split between binary and environments, `gh`’s split between host CLI and extensions, and `mise`’s split between CLI, plugins, and activated tools. [rust-lang.github](https://rust-lang.github.io/rustup/installation/index.html)

The least surprising UX is usually:

- Install binary: package manager or curl installer. [rust-lang.github](https://rust-lang.github.io/rustup/concepts/toolchains.html)
- Initialize once: `tool setup` or `tool install` for home-dir assets, completions, default config, and optional integrations. [rust-lang.github](https://rust-lang.github.io/rustup/installation/index.html)
- Activate something concrete: `tool use X`, `tool add X`, `tool extension install Y`, or `tool plugin install Z`. [raw.githubusercontent](https://raw.githubusercontent.com/astral-sh/uv/main/README.md)

## Practical recommendation

For your own CLI, I would avoid a silent “first run mutates home directory” model and instead advertise a visible second-stage command such as `tool setup` or `tool install` right after binary install. That is closer to how these tools behave in spirit: users install the host first, then run an intentional command that fetches managed state and makes the system actually useful. [docs.github](https://docs.github.com/en/github-cli/github-cli/using-github-cli-extensions)

A solid zero-to-working README pattern would be:

```bash
# 1. Get the binary onto PATH
curl -LsSf https://example.com/tool/install.sh | sh

# 2. Install user-scoped assets and defaults
tool setup

# 3. Install or activate the first real capability
tool use default
```

If you want, I can turn this into a concrete design rubric for your CLI: command naming, filesystem layout, idempotency rules, uninstall behavior, and what should happen on first run versus explicit setup.
