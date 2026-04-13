Yes — there are several real repositories that match your pattern of “Claude Code plugin content + extra distribution surface,” though I found more repos that ship **skills/hooks/commands/rules** than ones that also bundle a **separate compiled binary**. The most common distribution model is Claude Code’s **/plugin marketplace** flow, often combined with a fallback **git clone + copy files** or a custom installer script. [github](https://github.com/jeremylongshore/claude-code-plugins-plus-skills)

## Best matching repos

| Repo                                | What it ships                                                                                           | Distribution/UX shown in README                                                                                                                                                                                                                                                                                              |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `affaan-m/everything-claude-code`   | Skills, hooks, commands, rules, MCP configs; also mentions an external npm package for OpenCode support | Primary UX is `/plugin marketplace add affaan-m/everything-claude-code` then `/plugin install everything-claude-code@everything-claude-code`; it also shows manual `git clone` plus `./install.sh` for rules and copying skills/commands/hooks. [github](https://github.com/jeremylongshore/claude-code-plugins-plus-skills) |
| `levnikolaevich/claude-code-skills` | 84 Claude Code skills                                                                                   | README shows `/plugin marketplace add levnikolaevich/claude-code-skills`, then `/plugin install full-development-workflow-skills@levnikolaevich-skills-marketplace`; it also includes a direct `git clone ... ~/.claude/skills` path. [github](https://github.com/levnikolaevich/claude-code-skills)                         |
| `zilliztech/memsearch`              | Claude Code plugin with marketplace install                                                             | README snippet shows the marketplace flow: add marketplace, then install the plugin with `/plugin`. [github](https://github.com/zilliztech/memsearch/blob/main/plugins/claude-code/README.md)                                                                                                                                |
| `browserbase/skills`                | Skill/plugin marketplace content                                                                        | README snippet shows install through Claude Code’s skills marketplace UI: enter marketplace source, choose plugin, install, restart Claude Code. [github](https://github.com/browserbase/skills/blob/main/README.md)                                                                                                         |
| `affaan-m/everything-claude-code`   | Plugin content plus manual installs                                                                     | README also notes that some content cannot be distributed by plugin alone, so it uses manual copy steps for rules and a custom installer script for stack-specific setup. [github](https://github.com/jeremylongshore/claude-code-plugins-plus-skills)                                                                       |

## Distribution patterns I found

### 1) Claude Code /plugin marketplace is the dominant pattern

Several repos expose a marketplace manifest and tell users to install via `/plugin marketplace add ...` followed by `/plugin install ...`. `everything-claude-code` is the clearest example, and the README explicitly documents that as the recommended path. `levnikolaevich/claude-code-skills` does the same, with the marketplace install framed as the recommended method. [github](https://github.com/levnikolaevich/claude-code-skills)

### 2) Many repos fall back to git clone + copy

When a repo wants to ship skills or config files that don’t map cleanly into plugin distribution, the README often gives a manual option. `everything-claude-code` tells users to `git clone` and then copy `skills/`, `commands/`, and `hooks/` into Claude config directories, and it also has `./install.sh` for language-specific rules. `levnikolaevich/claude-code-skills` similarly documents a direct clone into `~/.claude/skills`. [github](https://github.com/jeremylongshore/claude-code-plugins-plus-skills)

### 3) I did not find a strong “Go binary + skills” pair in the same repo

In this pass, I found lots of Claude Code plugin/skills repositories, but not a clearly documented open-source repo that pairs a **compiled Go CLI** with **Claude Code skills** in one install path. The closest “hybrid” pattern is `everything-claude-code`, which ships plugin content and a separate installer/packaging surface, but its main runtime is still content files rather than a compiled binary. [github](https://github.com/jeremylongshore/claude-code-plugins-plus-skills)

## What the install UX looks like

The README UX usually follows one of three shapes:

- **Marketplace-first:** “Add marketplace” → “Install plugin” → optionally “list/verify installed content.” `everything-claude-code` and `levnikolaevich/claude-code-skills` both use this. [github](https://github.com/levnikolaevich/claude-code-skills)
- **Manual fallback:** `git clone` → copy markdown/config files into `~/.claude/...` or project `.claude/...` paths. `everything-claude-code` is explicit about this for rules, skills, commands, and hooks. [github](https://github.com/jeremylongshore/claude-code-plugins-plus-skills)
- **Mixed install:** plugin for the discoverable Claude Code pieces, plus a custom script for the rest. `everything-claude-code` uses `./install.sh` for rules and the plugin system for the rest. [github](https://github.com/jeremylongshore/claude-code-plugins-plus-skills)

## Repos to inspect next

The most promising repos for your exact “binary + Claude Code content” packaging question are the ones that already have a marketplace manifest and mention external tooling, especially `everything-claude-code`, `claude-code-plugins-plus-skills`, and `opsmill/claude-marketplace`. Those are the ones most likely to show a combined story of packaged tooling plus Claude-discoverable skills/hooks. [github](https://github.com/opsmill/claude-marketplace)

Would you like me to do a second pass focused specifically on **repos with a real compiled binary** plus Claude Code plugin content, and narrow it to Go-based projects only?
