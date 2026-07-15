# Docs-site tooling research (2026-07-15)

Research for the SDD user-documentation site: pick an open-source, agent-native documentation stack, publishable to GitHub Pages from markdown/MDX sources in this repo, agent-writable, with modern looks. Conducted via web research in-session; Christopher judged appearance from demo links and made the final pick.

## Criteria

1. Open-source, self-publishable (no hosted lock-in)
2. Agent-native: llms.txt manifests, per-page raw markdown, copy-page-as-markdown affordances
3. Lightweight GitHub Pages publishing (static output, official CI path)
4. Markdown/MDX sources in-repo, agent-writable
5. Modern layout and looks
6. Maintenance surface and dependency weight/reversibility (per repo cost-framing rule)

## Field surveyed

| Candidate | Stack | Agent-native | GitHub Pages | Authoring |
|---|---|---|---|---|
| Astro Starlight | Astro, static-first, framework-agnostic | via plugins (incl. one by a core maintainer) | official `withastro/action` | MD + MDX |
| VitePress | Vue/Vite | `vitepress-plugin-llms` (used by Vue/Vite/Vitest docs) | official guide | MD only, no MDX |
| Fumadocs | Next.js + React | best built-in (llms.txt, copy-markdown, open-in-AI) | static export needs extra wiring for .md endpoints | MDX |
| Docusaurus | React (Meta) | community plugin | easy | MD + MDX |
| MkDocs Material | Python | partial | easy | MD only |

## Assessment

- **Starlight** (https://starlight.astro.build): static-first and framework-agnostic — fits a Go repo with no JS-framework allegiance. Plain MD/MDX content files with simple frontmatter, ideal for agent authoring. Agent-native via `starlight-llms-txt` (by Starlight core maintainer delucis; llms.txt/llms-full.txt/llms-small.txt) and `@wave-rf/starlight-llm-tools`. Modern default theme, built-in Pagefind search, i18n. Real-world anchor: Cloudflare's docs (developers.cloudflare.com) run on Starlight.
- **VitePress** (https://vitepress.dev): lightest surface, fastest dev loop; plugin proven on Vue/Vite/Vitest's own docs. Rejected for v1: no MDX, and the stock look is instantly recognizable as Vue-docs with less theme flexibility.
- **Fumadocs** (https://fumadocs.dev): most modern aesthetic, only candidate with built-in AI features (https://www.fumadocs.dev/docs/integrations/llms). Rejected: Next.js/React is the heaviest dependency surface for this repo, and per-page markdown endpoints need extra wiring under static export for GitHub Pages.
- **Docusaurus** (https://docusaurus.io): versioning and plugin depth are not v1 needs; look dated next to the others.
- **MkDocs Material**: Python toolchain; MkDocs core maintenance question marks reported in 2026 (1.x unmaintained, 2.0 pre-release).

## Pick (Christopher, by stack fit + appearance)

- **Generator: Astro Starlight**
- **Theme: starlight-theme-nova** (https://github.com/ocavue/starlight-theme-nova, demo https://starlight-theme-nova.pages.dev/) — community theme by ocavue; Tailwind-aware, cascade layers for predictable style overrides.
- **Agent layer: @wave-rf/starlight-llm-tools** (https://github.com/Wave-RF/starlight-llm-tools) — per-page `.md` twins with navigation headers, llms.txt/llms-full.txt/llms-small.txt per llmstxt.org, Copy-Markdown button + AI dropdown (Claude/ChatGPT/Cursor). MIT. Requires Astro ≥5.0, Starlight ≥0.36.
- **Publishing: GitHub Pages** via the official Astro action.

## Known risk

`@wave-rf/starlight-llm-tools` is young (v0.3.1, June 2026, minimal adoption). Accepted because the plugin is thin and replaceable: fallback is `starlight-llms-txt` (core-maintainer-owned) plus `starlight-copy-button` covering the same ground with two dependencies instead of one.

## Sources

- https://www.pkgpulse.com/guides/best-documentation-frameworks-2026
- https://www.pkgpulse.com/guides/fumadocs-vs-nextra-v4-vs-starlight-documentation-sites-2026
- https://www.fumadocs.dev/docs/comparisons
- https://www.fumadocs.dev/docs/integrations/llms
- https://github.com/delucis/starlight-llms-txt
- https://github.com/okineadev/vitepress-plugin-llms
- https://github.com/Wave-RF/starlight-llm-tools
- https://github.com/ocavue/starlight-theme-nova
- https://starlight.astro.build/resources/plugins/
- https://www.jekyllpad.com/blog/open-source-documentation-tools
