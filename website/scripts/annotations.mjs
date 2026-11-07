#!/usr/bin/env node
import { existsSync, readFileSync } from "node:fs";
import { parseArgs } from "node:util";
import { locate } from "../integrations/annotations/anchor.js";
import { load, remove, update } from "../integrations/annotations/store.mjs";

const usage = `Usage:
  node website/scripts/annotations.mjs list [--page <path>] [--all] [--json] [--server <url>]
  node website/scripts/annotations.mjs resolve <id>
  node website/scripts/annotations.mjs reopen <id>
  node website/scripts/annotations.mjs delete <id>

list prints open comments; --all includes resolved ones. Anchor status comes
from the page as the running dev server renders it (found via
website/.astro/dev.json, or --server).`;

const website = new URL("../", import.meta.url);

const named = { amp: "&", lt: "<", gt: ">", quot: '"', apos: "'", nbsp: " " };

// Mirrors textIndex in integrations/annotations/client.js.
function mainText(html) {
  const start = html.search(/<main[\s>]/);
  const end = html.lastIndexOf("</main>");
  if (start === -1 || end === -1) return "";
  return html
    .slice(start, end)
    .replace(/<!--[\s\S]*?-->/g, "")
    .replace(/<(script|style|noscript|template)\b[\s\S]*?<\/\1>/gi, "")
    .replace(/<(?:[^>"']|"[^"]*"|'[^']*')*>/g, "")
    .replace(/&(#x[\da-f]+|#\d+|[a-z]+);/gi, (entity, code) => {
      if (code[0] !== "#") return named[code.toLowerCase()] ?? entity;
      return String.fromCodePoint(code[1].toLowerCase() === "x" ? parseInt(code.slice(2), 16) : parseInt(code.slice(1), 10));
    });
}

function devServer() {
  try {
    const { pid, url } = JSON.parse(readFileSync(new URL(".astro/dev.json", website), "utf8"));
    process.kill(pid, 0);
    return url;
  } catch {
    return null;
  }
}

function sourceFile(page) {
  const slug = page.replace(/^\/+|\/+$/g, "");
  const candidates = slug ? [`${slug}.md`, `${slug}.mdx`, `${slug}/index.md`, `${slug}/index.mdx`] : ["index.md", "index.mdx"];
  const found = candidates.find((c) => existsSync(new URL(`src/content/docs/${c}`, website)));
  return found && `website/src/content/docs/${found}`;
}

const trim = (path) => path.replace(/^\/+|\/+$/g, "");

async function list({ page, all, json, server }) {
  const origin = server ?? devServer();
  const annotations = (await load()).filter(
    (a) =>
      (all || a.status === "open") &&
      (!page || [a.page, a.url].some((p) => p && (trim(p) === trim(page) || trim(p).endsWith(`/${trim(page)}`)))),
  );

  const texts = new Map();
  if (origin) {
    for (const url of new Set(annotations.map((a) => a.url ?? a.page))) {
      const res = await fetch(new URL(url, origin));
      texts.set(url, res.ok ? mainText(await res.text()) : "");
    }
  }
  const rows = annotations.map((a) => ({
    ...a,
    anchor: origin ? (locate(texts.get(a.url ?? a.page), a.quote) ? "anchored" : "orphaned") : "unchecked",
    source: sourceFile(a.page),
  }));

  if (json) {
    console.log(JSON.stringify(rows, null, 2));
    return;
  }
  console.log(
    origin
      ? `Anchors checked against the dev server at ${origin}.`
      : "Anchors unchecked: no running dev server in website/.astro/dev.json. Start one or pass --server <url>.",
  );
  if (!rows.length) console.log(all ? "\nNo comments." : "\nNo open comments.");
  const indent = (text) => text.replace(/\n/g, "\n           ");
  for (const r of rows) {
    console.log(`
${r.id}  ${r.status}, ${r.anchor}
  page:    ${r.page}${r.source ? `  (${r.source})` : ""}
  heading: ${r.heading ? `${r.heading.text} (#${r.heading.id})` : "-"}
  quote:   "${indent(r.quote.exact)}"
  comment: ${indent(r.comment)}`);
  }
}

const { positionals, values } = parseArgs({
  allowPositionals: true,
  options: {
    page: { type: "string" },
    all: { type: "boolean" },
    json: { type: "boolean" },
    server: { type: "string" },
    help: { type: "boolean", short: "h" },
  },
});
const [command, id] = positionals;

try {
  if (values.help || !command) {
    console.log(usage);
  } else if (command === "list") {
    await list(values);
  } else if ((command === "resolve" || command === "reopen") && id) {
    await update(id, { status: command === "resolve" ? "resolved" : "open" });
    console.log(`${command === "resolve" ? "Resolved" : "Reopened"} ${id}.`);
  } else if (command === "delete" && id) {
    await remove(id);
    console.log(`Deleted ${id}.`);
  } else {
    console.error(usage);
    process.exitCode = 2;
  }
} catch (err) {
  console.error(`annotations: ${err.message}`);
  process.exitCode = 1;
}
