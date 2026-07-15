import { readdir, readFile, stat, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

const dist = fileURLToPath(new URL("../dist/", import.meta.url));
const pagesOrigin = "https://networkteam.github.io/";
const docsOrigin = `${pagesOrigin}sdd/`;
const unbasedDocsUrl = /https:\/\/networkteam\.github\.io\/(?!sdd(?:\/|$))/g;

async function generatedFiles(directory, matches) {
  const files = [];

  for (const name of await readdir(directory)) {
    const path = `${directory}/${name}`;
    const entry = await stat(path);

    if (entry.isDirectory()) {
      files.push(...(await generatedFiles(path, matches)));
    } else if (matches(name)) {
      files.push(path);
    }
  }

  return files;
}

const requiredFiles = ["index.html", "index.md", "llms.txt", "llms-full.txt", "llms-small.txt"];
await Promise.all(requiredFiles.map((name) => stat(`${dist}/${name}`)));

// @wave-rf/starlight-llm-tools 0.3.1 builds absolute links from
// Astro.site.origin, which drops Astro's GitHub Pages base path. Restrict the
// compatibility rewrite to plugin-generated Markdown and LLM manifests.
for (const path of await generatedFiles(
  dist,
  (name) => name.endsWith(".md") || /^llms(?:-full|-small)?\.txt$/.test(name),
)) {
  const content = await readFile(path, "utf8");
  const corrected = content.replace(unbasedDocsUrl, docsOrigin);

  if (corrected !== content) {
    await writeFile(path, corrected);
  }

  if (unbasedDocsUrl.test(corrected)) {
    throw new Error(`Generated documentation still contains a URL outside the /sdd base: ${path}`);
  }
}

const unbasedCopyUrl = /data-url="\/(?!sdd\/)([^"]+\.md)"/g;
for (const path of await generatedFiles(dist, (name) => name.endsWith(".html"))) {
  const content = await readFile(path, "utf8");
  const corrected = content.replace(unbasedCopyUrl, 'data-url="/sdd/$1"');

  if (corrected !== content) {
    await writeFile(path, corrected);
  }

  if (unbasedCopyUrl.test(corrected)) {
    throw new Error(`Copy Markdown still points outside the /sdd base: ${path}`);
  }
}

const html = await readFile(`${dist}/index.html`, "utf8");
if (!html.includes('href="/sdd/')) {
  throw new Error("Generated HTML does not use the /sdd GitHub Pages base path");
}
