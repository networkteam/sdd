import { describe, locate } from "./anchor.js";

const endpoint = "/__sdd-annotations";
const base = import.meta.env.BASE_URL.replace(/\/$/, "");
const prefsKey = "sdd-annotations";

function pageOf(pathname) {
  const path = pathname.endsWith("/") ? pathname : `${pathname}/`;
  return path.startsWith(`${base}/`) ? path.slice(base.length) : path;
}

const here = pageOf(location.pathname);
const urlOf = (a) => a.url ?? base + a.page;

// Mirrors mainText in scripts/annotations.mjs: the text of every text node in
// <main> outside script, style, noscript and template.
function textIndex(root) {
  const nodes = [];
  let text = "";
  const walker = root.ownerDocument.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode: (node) =>
      node.parentElement?.closest("script, style, noscript, template")
        ? NodeFilter.FILTER_REJECT
        : NodeFilter.FILTER_ACCEPT,
  });
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    nodes.push({ node, start: text.length });
    text += node.data;
  }
  return { nodes, text };
}

function offsetAt(index, container, offset) {
  const entry = index.nodes.find((e) => e.node === container);
  if (entry) return entry.start + offset;
  const point = document.createRange();
  point.setStart(container, offset);
  const next = index.nodes.find((e) => point.comparePoint(e.node, 0) >= 0);
  return next ? next.start : index.text.length;
}

function pointAt(index, offset, isEnd) {
  for (const { node, start } of index.nodes) {
    const end = start + node.data.length;
    if (isEnd ? start < offset && offset <= end : offset < end) return [node, offset - start];
  }
  const last = index.nodes.at(-1).node;
  return [last, last.data.length];
}

function headingAt(root, range) {
  const point = range.cloneRange();
  point.collapse(true);
  let heading = null;
  for (const h of root.querySelectorAll("h1[id], h2[id], h3[id], h4[id], h5[id], h6[id]")) {
    if (point.comparePoint(h, 0) > 0) break;
    heading = { id: h.id, text: h.textContent.trim() };
  }
  return heading;
}

async function api(method, path = "", body) {
  const res = await fetch(endpoint + path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body && JSON.stringify(body),
  });
  if (!res.ok) {
    const { error } = await res.json().catch(() => ({}));
    throw new Error(error ?? `${method} ${endpoint}${path} failed with ${res.status}`);
  }
  return res.status === 204 ? null : res.json();
}

function report(err) {
  console.error(err);
  alert(`Annotations: ${err.message}`);
}

const prefs = { open: false, tab: "page", resolved: false, highlights: true };
try {
  Object.assign(prefs, JSON.parse(localStorage.getItem(prefsKey)));
} catch {}
function savePrefs() {
  try {
    localStorage.setItem(prefsKey, JSON.stringify(prefs));
  } catch {}
}

let root;
let annotations = [];
let anchors = new Map();
let elsewhere = new Map();
let pending = null;
let editing = null;
let reattaching = null;
let active = null;

const byId = (id) => annotations.find((a) => a.id === id);

function anchorOf(a) {
  if (a.page === here) return anchors.has(a.id) ? "anchored" : "orphaned";
  const found = elsewhere.get(a.id);
  return found === undefined ? "unchecked" : found ? "anchored" : "orphaned";
}

function reanchor() {
  const index = textIndex(root);
  anchors = new Map();
  for (const a of annotations) {
    if (a.page !== here) continue;
    const hit = locate(index.text, a.quote);
    if (!hit) continue;
    const range = document.createRange();
    range.setStart(...pointAt(index, hit.start, false));
    range.setEnd(...pointAt(index, hit.end, true));
    anchors.set(a.id, range);
  }
}

function visibleAnchors() {
  if (!prefs.highlights) return [];
  return [...anchors].filter(([id]) => prefs.resolved || byId(id).status === "open");
}

function paint() {
  if (!CSS.highlights) return;
  const open = [];
  const resolved = [];
  for (const [id, range] of visibleAnchors()) (byId(id).status === "open" ? open : resolved).push(range);
  const focused = [pending?.range, anchors.get(active)].filter(Boolean);
  CSS.highlights.set("sdd-annotation", new Highlight(...open));
  CSS.highlights.set("sdd-annotation-resolved", new Highlight(...resolved));
  const highlight = new Highlight(...focused);
  highlight.priority = 1;
  CSS.highlights.set("sdd-annotation-active", highlight);
}

async function checkElsewhere() {
  const urls = new Set(annotations.filter((a) => a.page !== here).map(urlOf));
  await Promise.all(
    [...urls].map(async (url) => {
      const res = await fetch(url);
      const main = res.ok && new DOMParser().parseFromString(await res.text(), "text/html").querySelector("main");
      const text = main ? textIndex(main).text : "";
      for (const a of annotations) if (urlOf(a) === url) elsewhere.set(a.id, Boolean(locate(text, a.quote)));
    }),
  );
  render();
}

async function refresh() {
  annotations = await api("GET");
  reanchor();
  paint();
  render();
  if (prefs.open && prefs.tab === "all") await checkElsewhere();
}

const style = document.createElement("style");
style.textContent = `
::highlight(sdd-annotation) { background-color: rgb(250 204 21 / 0.35); }
::highlight(sdd-annotation-resolved) { text-decoration: underline dotted rgb(34 197 94); }
::highlight(sdd-annotation-active) { background-color: rgb(250 204 21 / 0.85); color: #111; }
`;

const host = document.createElement("sdd-annotations");
const shadow = host.attachShadow({ mode: "open" });
shadow.innerHTML = `
<style>
:host { position: absolute; top: 0; left: 0; z-index: 999; font: 14px/1.45 var(--sl-font, system-ui, sans-serif); color: var(--sl-color-text, #222); }
[hidden] { display: none !important; }
button { font: inherit; color: inherit; cursor: pointer; background: var(--sl-color-bg, #fff); border: 1px solid var(--sl-color-gray-5, #ccc); border-radius: 6px; padding: 2px 8px; }
button:hover { border-color: var(--sl-color-accent, #7c3aed); }
button[type="submit"], .add { background: var(--sl-color-accent, #7c3aed); border-color: transparent; color: var(--sl-color-text-invert, #fff); }
textarea { box-sizing: border-box; width: 100%; min-height: 5em; resize: vertical; padding: 6px; font: inherit; color: inherit; background: var(--sl-color-bg, #fff); border: 1px solid var(--sl-color-gray-5, #ccc); border-radius: 6px; }
blockquote { margin: 0; padding-left: 8px; border-left: 3px solid rgb(250 204 21); color: var(--sl-color-gray-2, #555); font-style: italic; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
.add, .composer { position: absolute; box-shadow: 0 4px 16px rgb(0 0 0 / 0.25); }
.composer { box-sizing: border-box; width: 320px; display: grid; gap: 8px; padding: 10px; background: var(--sl-color-bg, #fff); border: 1px solid var(--sl-color-gray-5, #ccc); border-radius: 8px; }
.row, .actions { display: flex; flex-wrap: wrap; gap: 6px; }
.toggle { position: fixed; right: 16px; bottom: 16px; padding: 6px 12px; border-radius: 999px; box-shadow: 0 4px 16px rgb(0 0 0 / 0.2); }
.toggle.warn::after { content: ""; display: inline-block; width: 8px; height: 8px; margin-left: 6px; border-radius: 50%; background: var(--sl-color-red, #e11d48); }
.panel { position: fixed; top: 0; right: 0; bottom: 0; box-sizing: border-box; width: min(380px, 100vw); overflow-y: auto; padding: 12px 16px 48px; background: var(--sl-color-bg, #fff); border-left: 1px solid var(--sl-color-gray-5, #ccc); box-shadow: -6px 0 24px rgb(0 0 0 / 0.15); }
header { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.tabs { display: flex; gap: 6px; }
[aria-pressed="true"] { background: var(--sl-color-accent-low, #ede9fe); border-color: var(--sl-color-accent, #7c3aed); }
.filters { display: flex; gap: 12px; margin: 10px 0; font-size: 13px; color: var(--sl-color-gray-3, #777); }
.notice { margin: 0 0 10px; padding: 6px 8px; border-radius: 6px; background: var(--sl-color-accent-low, #ede9fe); }
ul { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; }
.group { margin-top: 8px; font-weight: 600; }
.group a { color: inherit; }
.item { display: grid; gap: 6px; padding: 8px 10px; border: 1px solid var(--sl-color-gray-5, #ddd); border-radius: 8px; }
.item.active { border-color: rgb(250 204 21); box-shadow: 0 0 0 2px rgb(250 204 21 / 0.5); }
.item.orphaned { border-style: dashed; border-color: var(--sl-color-red, #e11d48); }
.item.resolved { opacity: 0.65; }
.item blockquote { cursor: pointer; }
.meta { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; font-size: 12px; color: var(--sl-color-gray-3, #777); }
.badge { padding: 0 5px; border-radius: 4px; font-size: 11px; text-transform: uppercase; }
.badge.orphaned { background: var(--sl-color-red-low, #fde2e2); color: var(--sl-color-red-high, #9b1c1c); }
.badge.resolved { background: var(--sl-color-green-low, #dcfce7); color: var(--sl-color-green-high, #166534); }
.comment { margin: 0; white-space: pre-wrap; }
.empty, .hint { margin: 0; font-size: 13px; color: var(--sl-color-gray-3, #777); }
.actions button { font-size: 12px; }
</style>
<button class="add" hidden></button>
<form class="composer" hidden></form>
<button class="toggle" data-action="toggle"></button>
<aside class="panel" hidden></aside>
`;
const addButton = shadow.querySelector(".add");
const composer = shadow.querySelector(".composer");
const toggle = shadow.querySelector(".toggle");
const panel = shadow.querySelector(".panel");

const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);

function item(a) {
  const anchor = anchorOf(a);
  const open = a.status === "open";
  const comment =
    editing === a.id
      ? `<form class="edit"><textarea name="comment" required>${esc(a.comment)}</textarea>
         <div class="row"><button type="submit">Save</button><button type="button" data-action="cancel-edit">Cancel</button></div></form>`
      : `<p class="comment">${esc(a.comment)}</p>`;
  return `<li class="item ${a.status} ${anchor}${active === a.id ? " active" : ""}" data-id="${a.id}">
    <div class="meta"><span>${esc(a.heading?.text ?? "")}</span>
      ${anchor === "orphaned" ? `<span class="badge orphaned">orphaned</span>` : ""}
      ${open ? "" : `<span class="badge resolved">resolved</span>`}</div>
    <blockquote data-action="focus" title="${esc(a.quote.exact)}">${esc(a.quote.exact)}</blockquote>
    ${anchor === "orphaned" ? `<p class="hint">This quote is no longer on the page.</p>` : ""}
    ${comment}
    <div class="actions">
      <button data-action="edit">Edit</button>
      <button data-action="${open ? "resolve" : "reopen"}">${open ? "Resolve" : "Reopen"}</button>
      ${anchor === "orphaned" ? `<button data-action="reattach">${reattaching === a.id ? "Cancel re-attach" : "Re-attach"}</button>` : ""}
      <button data-action="delete">Delete</button>
    </div>
  </li>`;
}

function render() {
  const mine = annotations.filter((a) => a.page === here);
  toggle.textContent = `Comments (${mine.filter((a) => a.status === "open").length})`;
  toggle.classList.toggle("warn", mine.some((a) => a.status === "open" && !anchors.has(a.id)));
  panel.hidden = !prefs.open;
  if (!prefs.open) return;

  const draft = panel.querySelector(".edit textarea")?.value;
  const shown = (list) => list.filter((a) => prefs.resolved || a.status === "open");
  const byPosition = (a, b) => {
    const [ra, rb] = [anchors.get(a.id), anchors.get(b.id)];
    if (!ra || !rb) return ra ? 1 : rb ? -1 : 0;
    return ra.compareBoundaryPoints(Range.START_TO_START, rb);
  };
  let rows;
  if (prefs.tab === "page") {
    const list = shown(mine).sort(byPosition);
    rows = list.length ? list.map(item).join("") : `<li class="empty">No comments on this page. Select text to add one.</li>`;
  } else {
    const pages = Map.groupBy(shown(annotations), (a) => a.page);
    rows = [...pages]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([page, list]) => {
        const heading = `<li class="group"><a href="${esc(urlOf(list[0]))}">${esc(page)}</a></li>`;
        return heading + list.sort(byPosition).map(item).join("");
      })
      .join("");
    rows ||= `<li class="empty">No comments yet.</li>`;
  }
  panel.innerHTML = `
    <header>
      <div class="tabs">
        <button data-tab="page" aria-pressed="${prefs.tab === "page"}">This page</button>
        <button data-tab="all" aria-pressed="${prefs.tab === "all"}">All pages</button>
      </div>
      <button data-action="toggle" aria-label="Close">Close</button>
    </header>
    <div class="filters">
      <label><input type="checkbox" data-pref="resolved"${prefs.resolved ? " checked" : ""}> Show resolved</label>
      <label><input type="checkbox" data-pref="highlights"${prefs.highlights ? " checked" : ""}> Highlights</label>
    </div>
    ${reattaching ? `<p class="notice">Select the passage for this comment, then click Attach here.</p>` : ""}
    <ul>${rows}</ul>`;
  const textarea = panel.querySelector(".edit textarea");
  if (textarea && draft !== undefined) textarea.value = draft;
}

function place(el, rect, width) {
  const left = Math.min(Math.max(rect.right - width / 2, 8), innerWidth - width - 8);
  el.style.left = `${left + scrollX}px`;
  el.style.top = `${rect.bottom + scrollY + 8}px`;
}

function selectedRange() {
  const selection = document.getSelection();
  if (!selection.rangeCount || selection.isCollapsed) return null;
  const range = selection.getRangeAt(0);
  return root.contains(range.commonAncestorContainer) ? range : null;
}

function describeRange(range) {
  const index = textIndex(root);
  const start = offsetAt(index, range.startContainer, range.startOffset);
  const end = offsetAt(index, range.endContainer, range.endOffset);
  return { quote: describe(index.text, start, end), heading: headingAt(root, range) };
}

function onSelectionEnd(event) {
  if (event.composedPath().includes(host) || pending) return;
  setTimeout(() => {
    const range = selectedRange();
    addButton.hidden = !range;
    if (!range) return;
    addButton.textContent = reattaching ? "Attach here" : "Comment";
    place(addButton, [...range.getClientRects()].at(-1) ?? range.getBoundingClientRect(), 110);
  });
}

function openComposer(range) {
  const { quote, heading } = describeRange(range);
  if (!quote.exact) return;
  pending = { range: range.cloneRange(), quote, heading };
  composer.innerHTML = `
    <blockquote>${esc(quote.exact)}</blockquote>
    <textarea name="comment" required placeholder="Comment (Cmd/Ctrl+Enter saves)"></textarea>
    <div class="row"><button type="submit">Save</button><button type="button" data-action="discard">Cancel</button></div>`;
  place(composer, [...range.getClientRects()].at(-1) ?? range.getBoundingClientRect(), 320);
  composer.hidden = false;
  addButton.hidden = true;
  document.getSelection().removeAllRanges();
  composer.querySelector("textarea").focus();
  paint();
}

function closeComposer() {
  pending = null;
  composer.hidden = true;
  paint();
}

async function attach(range) {
  const { quote, heading } = describeRange(range);
  const updated = await api("PUT", `/${reattaching}`, { page: here, url: base + here, quote, heading });
  annotations[annotations.indexOf(byId(reattaching))] = updated;
  reattaching = null;
  addButton.hidden = true;
  document.getSelection().removeAllRanges();
  reanchor();
  paint();
  render();
}

function flash(id) {
  active = id;
  paint();
  render();
  setTimeout(() => {
    if (active !== id) return;
    active = null;
    paint();
    render();
  }, 2000);
}

async function act(action, id) {
  const a = byId(id);
  switch (action) {
    case "toggle":
      prefs.open = !prefs.open;
      break;
    case "focus":
      if (a.page !== here) return location.assign(`${urlOf(a)}#${a.heading?.id ?? ""}`);
      anchors.get(id)?.startContainer.parentElement.scrollIntoView({ behavior: "smooth", block: "center" });
      return flash(id);
    case "edit":
      editing = id;
      break;
    case "cancel-edit":
      editing = null;
      break;
    case "resolve":
    case "reopen":
      Object.assign(a, await api("PUT", `/${id}`, { status: action === "resolve" ? "resolved" : "open" }));
      break;
    case "reattach":
      reattaching = reattaching === id ? null : id;
      break;
    case "delete":
      if (!confirm(`Delete this comment?\n\n${a.comment}`)) return;
      await api("DELETE", `/${id}`);
      annotations.splice(annotations.indexOf(a), 1);
      anchors.delete(id);
      break;
    case "discard":
      return closeComposer();
  }
  savePrefs();
  paint();
  render();
  if (action === "toggle" && prefs.open && prefs.tab === "all") await checkElsewhere();
}

addButton.addEventListener("mousedown", (event) => event.preventDefault());
addButton.addEventListener("click", () => {
  const range = selectedRange();
  if (!range) return;
  (reattaching ? attach(range) : Promise.resolve(openComposer(range))).catch(report);
});

shadow.addEventListener("click", (event) => {
  const tab = event.target.closest("[data-tab]");
  if (tab) {
    prefs.tab = tab.dataset.tab;
    savePrefs();
    render();
    if (prefs.tab === "all") checkElsewhere().catch(report);
    return;
  }
  const target = event.target.closest("[data-action]");
  if (target) act(target.dataset.action, target.closest("[data-id]")?.dataset.id).catch(report);
});

shadow.addEventListener("change", (event) => {
  const pref = event.target.dataset.pref;
  if (!pref) return;
  prefs[pref] = event.target.checked;
  savePrefs();
  paint();
  render();
});

shadow.addEventListener("keydown", (event) => {
  if (event.target.tagName !== "TEXTAREA") return;
  if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) event.target.form.requestSubmit();
  if (event.key === "Escape") event.target.form === composer ? closeComposer() : act("cancel-edit").catch(report);
});

shadow.addEventListener("submit", async (event) => {
  event.preventDefault();
  const comment = new FormData(event.target).get("comment").trim();
  try {
    if (event.target === composer) {
      const { quote, heading } = pending;
      annotations.push(await api("POST", "", { page: here, url: base + here, heading, quote, comment }));
      closeComposer();
      reanchor();
    } else {
      const id = event.target.closest("[data-id]").dataset.id;
      Object.assign(byId(id), await api("PUT", `/${id}`, { comment }));
      editing = null;
    }
    paint();
    render();
  } catch (err) {
    report(err);
  }
});

document.addEventListener("pointerup", onSelectionEnd);
document.addEventListener("keyup", onSelectionEnd);
document.addEventListener("selectionchange", () => {
  if (document.getSelection().isCollapsed) addButton.hidden = true;
});

document.addEventListener("click", (event) => {
  if (event.composedPath().includes(host) || !document.getSelection().isCollapsed) return;
  const { clientX: x, clientY: y } = event;
  for (const [id, range] of visibleAnchors()) {
    const hit = [...range.getClientRects()].some((r) => x >= r.left && x <= r.right && y >= r.top && y <= r.bottom);
    if (!hit) continue;
    prefs.open = true;
    prefs.tab = "page";
    savePrefs();
    flash(id);
    shadow.querySelector(`[data-id="${id}"]`)?.scrollIntoView({ block: "nearest" });
    return;
  }
});

// The CLI can resolve or delete comments while the page is open.
window.addEventListener("focus", () => {
  if (!editing && !pending) refresh().catch(report);
});

function init() {
  root = document.querySelector("main");
  if (!root) return;
  document.head.append(style);
  document.body.append(host);
  refresh().catch(report);
}

if (document.readyState === "complete") init();
else addEventListener("load", init);
