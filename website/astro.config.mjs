import starlight from "@astrojs/starlight";
import starlightLlmTools from "@wave-rf/starlight-llm-tools";
import { defineConfig } from "astro/config";
import starlightThemeNova from "starlight-theme-nova";
import annotations from "./integrations/annotations/index.mjs";

export default defineConfig({
  site: "https://networkteam.github.io",
  base: "/sdd",
  integrations: [
    annotations(),
    starlight({
      title: "SDD",
      description:
        "Signal, Dialogue, Decision — grow a durable decision graph through dialogue.",
      plugins: [starlightThemeNova(), starlightLlmTools()],
      sidebar: [
        { slug: "what-is-sdd" },
        { slug: "getting-started" },
        { slug: "working-modes" },
        {
          label: "Guides",
          items: [
            { slug: "guides/working-with-coding-agents" },
            { slug: "guides/teams-and-processes" },
            { slug: "guides/connected-repositories" },
            { slug: "guides/browsing-the-graph" },
            { slug: "guides/teams-and-processes" },
          ],
        },
        {
          label: "Reference",
          items: [
            { slug: "reference/entry-kinds" },
            { slug: "reference/configuration" },
          ],
        },
        {
          label: "Vision",
          items: [{ slug: "vision/landscape" }, { slug: "vision/scenes" }],
        },
        { slug: "about" },
      ],
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/networkteam/sdd",
        },
      ],
    }),
  ],
});
