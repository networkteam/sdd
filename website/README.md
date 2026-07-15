# SDD documentation site

This directory contains the Astro Starlight site. Its content tree is intentionally minimal until the documentation information architecture is settled.

Use the repository's Devbox environment for all development commands:

```sh
devbox run docs:dev
devbox run docs:build
```

The build emits the static site to `website/dist/`. The GitHub Pages workflow publishes it at `https://networkteam.github.io/sdd/`.

## LLM tools compatibility

`@wave-rf/starlight-llm-tools` 0.3.1 omits Astro's `base` path from absolute links in its generated Markdown and LLM manifests and from the Copy Markdown fetch URL. The package's `postbuild` step runs `scripts/finalize-build.mjs` to correct and verify those generated links. Remove the compatibility rewrite once the plugin handles non-root deployments itself.
