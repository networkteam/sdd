import starlight from "@astrojs/starlight";
import starlightLlmTools from "@wave-rf/starlight-llm-tools";
import { defineConfig } from "astro/config";
import starlightThemeNova from "starlight-theme-nova";

export default defineConfig({
  site: "https://networkteam.github.io",
  base: "/sdd",
  integrations: [
    starlight({
      title: "SDD",
      description:
        "Signal, Dialogue, Decision — grow a durable decision graph through dialogue.",
      plugins: [starlightThemeNova(), starlightLlmTools()],
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
