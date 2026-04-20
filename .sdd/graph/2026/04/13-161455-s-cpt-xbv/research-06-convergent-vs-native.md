**Bet B looks safer and more durable** for a new open-source CLI in 2026: ship through conventional package managers, then make Claude Code integration a config or adapter layer. Claude Code’s plugin system is real and usable now, but it is still a single-vendor distribution surface with policy and marketplace coupling that can change under you. [code.claude](https://code.claude.com/docs/en/discover-plugins)

## Why B wins

Claude Code’s marketplace makes discovery and installation convenient, but the docs frame it as an Anthropic-controlled catalog and managed install flow, with auto-updates, policy controls, and team settings all mediated by Claude Code itself. That is good for reach, but it also means your distribution is tied to Anthropic’s product decisions and lifecycle. [code.claude](https://code.claude.com/docs/en/plugin-marketplaces)

Conventional package managers give you a vendor-neutral distribution path that survives model churn, agent churn, and even plugin-framework churn. For a CLI, that usually matters more than first-party marketplace placement because CLIs often outlive the AI product they integrate with. Package-management ecosystems are also the default expectation for infrastructure and developer tools. [nesbitt](https://nesbitt.io/2026/01/03/the-package-management-landscape.html)

## Plugin maturity

Claude Code’s plugin system appears mature enough for real use, with plugins, skills, hooks, MCP servers, marketplaces, auto-update support, and team-managed rollout already documented. That said, the docs still read like a rapidly evolving platform rather than a long-settled standard, and even the changelog shows ongoing changes to plugin behavior and policy enforcement. [code.claude](https://code.claude.com/docs/en/changelog)

So the plugin marketplace is attractive as an **additional** channel, not as the primary bet. A good rule is: use it for reach, not for survival. [code.claude](https://code.claude.com/docs/en/discover-plugins)

## Lock-in risk

The lock-in issue is not just technical; it is distribution and governance. If your primary install path is the native marketplace, you inherit Anthropic’s marketplace rules, trust model, update model, and any future prioritization changes. That is fine if your product is intentionally Claude-native, but it is a weak default if your CLI aims to become a durable tool across agents. [code.claude](https://code.claude.com/docs/en/changelog)

There is also a practical downside: plugin ecosystems can be brittle while a platform is still evolving. The existence of reports around marketplace compatibility and the fact that organizational policy can block plugin installs suggest that the surface is still moving. [github](https://github.com/anthropics/claude-code/issues/37679)

## Cross-agent bet

The better long-term bet is that other agents will converge on a **shared skills/plugin abstraction**, but not necessarily on Claude’s marketplace. Cursor, GitHub Copilot CLI, and Gemini CLI already support skills-style concepts or open-standard-like skill packaging, which points toward portability at the skill level rather than portability at the marketplace level. [cursor](https://cursor.com/docs/skills)

That means you should design your integration artifact to be portable: a skills folder, a CLI adapter, or a config scaffold that can be installed across tools. The strongest trend is toward reusable capability packages, not toward one marketplace winning outright. [geminicli](https://geminicli.com/docs/cli/skills/)

## Ecosystem lifetime

Tooling ecosystems usually have shorter half-lives than the developer workflows they automate. Package managers, shell installs, and plain binaries tend to outlast UI surfaces and marketplace UX because they are simpler, more composable, and easier to script in CI. [blog.packagecloud](https://blog.packagecloud.io/5-best-linux-package-managers/)

For an open-source CLI launched in 2026, that argues for distribution primitives you control: `brew`, `curl | bash`, `go install`, release binaries, and maybe containers. Then let Claude Code, Cursor, Copilot, and Gemini be integration targets rather than your primary dependency. [cursor](https://cursor.com/docs/skills)

## Practical strategy

A strong strategy is:

- Primary distribution: Homebrew, `go install`, GitHub releases, and one-line installer.
- Integration layer: Claude Code config/plugin/skill support as optional setup.
- Secondary channel: Claude Code marketplace submission for discovery.
- Portability goal: keep the AI-specific bits thin and standards-based. [docs.github](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/create-skills)

That gives you the upside of Claude Code’s audience without betting the project’s survival on Anthropic’s distribution model. It also keeps your path open if the broader market settles around a shared skills format rather than a single marketplace. [serenitiesai](https://serenitiesai.com/articles/agent-skills-guide-2026)
