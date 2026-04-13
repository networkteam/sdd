The Claude Code plugin ecosystem is **real but still niche-to-growing**, with the official marketplace plus a few community marketplaces and large curated repos, not a huge long-tail directory yet. The strongest signals of growth are the official docs, the Anthropic-managed marketplace, and community repos advertising hundreds of plugins or active release streams. [github](https://github.com/anthropics/claude-plugins-official/blob/main/.claude-plugin/marketplace.json)

## What exists on GitHub

A few GitHub repos clearly participate in the current marketplace workflow via `.claude-plugin/marketplace.json` or `/plugin marketplace add`:

- `anthropics/claude-code` hosts the docs and a marketplace file for the core repo. [github](https://github.com/anthropics/claude-code/blob/main/.claude-plugin/marketplace.json)
- `anthropics/claude-plugins-official` is the official Anthropic-managed marketplace, installable with `/plugin install <name>@claude-plugins-official`. [github](https://github.com/anthropics/claude-plugins-official)
- `jeremylongshore/claude-code-plugins-plus-skills` is a large community ecosystem claim: “270+ Claude Code plugins with 739 agent skills”. [github](https://github.com/jeremylongshore/claude-code-plugins-plus-skills)
- `b-open-io/claude-plugins`, `GLINCKER/claude-code-marketplace`, `ananddtyagi/cc-marketplace`, `Dev-GOM/claude-code-marketplace`, and `sgaunet/claude-plugins` all present as marketplace repos or plugin catalogs in GitHub snippets. [github](https://github.com/Dev-GOM/claude-code-marketplace)

The docs also confirm that marketplaces can be added from GitHub repos, git URLs, local paths, or direct JSON URLs, which is why you’re seeing both “marketplace repo” and “single-file catalog” patterns. [code.claude](https://code.claude.com/docs/en/plugin-marketplaces)

## Markdown skills vs binaries

Most of the visible ecosystem is still **content-first**: skills, commands, agents, and hooks packaged as markdown and JSON, not native compiled software. The official docs emphasize that plugins can bundle commands, agents, skills, hooks, MCP servers, and LSP servers, but the publish/install path itself is repo-based and schema-driven rather than binary-centric. [angelo-lima](https://angelo-lima.fr/en/claude-code-plugins-marketplace/)

That said, the ecosystem already shows a split:

- Pure markdown/JSON skill packs and instructional repos dominate the community-facing catalogs. [github](https://github.com/sgaunet/claude-plugins)
- Some repos appear to ship runnable tooling and package-manager style orchestration around plugins, which implies code beyond markdown, but the public snippets I found do not consistently expose whether those are compiled binaries or script-based tooling. [github](https://github.com/serpro69/claude-toolbox)

So the best current summary is: **markdown skills are the default; compiled binaries are still the exception rather than the norm**. [code.claude](https://code.claude.com/docs/en/plugin-marketplaces)

## SessionStart binary loaders

There are clear examples of `SessionStart` hooks in the wild, and bug reports confirm they are used to bootstrap plugin behavior at session start. I also found evidence that some plugins use `SessionStart` hooks with platform-specific shell wrappers, including a Windows `.cmd` workaround for a `.sh`-based hook path. [github](https://github.com/thedotmack/claude-mem/issues/519)

However, I did **not** find a clearly documented, public GitHub example in the search results that explicitly says: “download platform-specific binaries into `${CLAUDE_PLUGIN_DATA}` during SessionStart.” The docs do establish plugin cache/state locations and environment variables such as `CLAUDE_PLUGIN_DATA`, `CLAUDE_PLUGIN_ROOT`, and versioned plugin cache behavior, so that pattern is technically aligned with the platform model. In practice, the emerging pattern seems to be: [github](https://github.com/anthropics/claude-code/issues/38699)

- use `SessionStart` to initialize plugin state or wrapper scripts,
- detect OS/arch,
- fetch or unpack a local asset into plugin data/cache,
- then run the platform-specific executable from there. [github](https://github.com/anthropics/claude-plugins-official/issues/402)

## Versioning patterns

Versioning is already following a few recognizable patterns:

- **Git refs and SHAs**: the docs say plugin sources can be pinned with `ref` or `sha`, with marketplace and plugin sources pinned independently. [code.claude](https://code.claude.com/docs/en/plugin-marketplaces)
- **Marketplace-as-code**: repos publish `marketplace.json`, then update the catalog by pushing changes to GitHub, GitLab, or another git host. [code.claude](https://code.claude.com/docs/en/plugin-marketplaces)
- **Semantic releases**: the larger community repo I found has a visible release cadence and version tags, including `v4.15.0` and 34 releases total. [github](https://github.com/jeremylongshore/claude-code-plugins-plus-skills)
- **Marketplace names as install targets**: installs often look like `<plugin>@<marketplace>`, which encourages marketplace-scoped versioning and stable names over ad hoc URLs. [code.claude](https://code.claude.com/docs/en/discover-plugins)

The update cadence looks uneven but active: the official marketplace and docs are very recent, while the biggest community repo shows repeated releases through February 2026. [github](https://github.com/anthropics/claude-plugins-official)

## Ecosystem size

This is no longer a one-off experiment, but it is still **small enough to feel curated** rather than saturated. The most concrete public count I found was `270+ Claude Code plugins with 739 agent skills` in one community repository, plus an Anthropic official marketplace and several smaller curated marketplaces. [github](https://github.com/b-open-io/claude-plugins)

So the size read is:

- **Not tiny**: there are multiple independent marketplaces and active release activity. [github](https://github.com/anthropics/claude-plugins-official/blob/main/.claude-plugin/marketplace.json)
- **Not huge yet**: the ecosystem is still concentrated in a handful of high-visibility repos and official docs. [code.claude](https://code.claude.com/docs/en/discover-plugins)
- **Growing**: the official marketplace, discover/install workflow, and community cataloging are all recent and expanding. [github](https://github.com/anthropics/claude-plugins-official)

If you want, I can turn this into a **repo-by-repo matrix** with columns for marketplace source, plugin type, binaries/scripts, hook usage, and release cadence.
