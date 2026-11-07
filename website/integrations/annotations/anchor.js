// Shared by the browser client and the CLI so both anchor a quote the same way.
// Matching ignores all whitespace: the DOM and the raw HTML disagree on it at
// element boundaries.

const CONTEXT = 32;

export function collapse(text) {
  return text.replace(/\s+/g, " ").trim();
}

function squeeze(text) {
  const chars = [];
  const at = [];
  for (let i = 0; i < text.length; i++) {
    if (!/\s/.test(text[i])) {
      chars.push(text[i]);
      at.push(i);
    }
  }
  return { key: chars.join(""), at };
}

export function describe(text, start, end) {
  return {
    exact: collapse(text.slice(start, end)),
    prefix: collapse(text.slice(Math.max(0, start - CONTEXT * 2), start)).slice(-CONTEXT),
    suffix: collapse(text.slice(end, end + CONTEXT * 2)).slice(0, CONTEXT),
  };
}

// Returns the [start, end) offsets in text of the occurrence of quote.exact
// whose surroundings best match quote.prefix and quote.suffix, or null.
export function locate(text, quote) {
  const { key, at } = squeeze(text);
  const exact = squeeze(quote.exact).key;
  if (!exact) return null;
  const prefix = squeeze(quote.prefix ?? "").key;
  const suffix = squeeze(quote.suffix ?? "").key;

  let best = -1;
  let bestScore = -1;
  for (let i = key.indexOf(exact); i !== -1; i = key.indexOf(exact, i + 1)) {
    let before = 0;
    while (before < prefix.length && key[i - 1 - before] === prefix[prefix.length - 1 - before]) before++;
    let after = 0;
    while (after < suffix.length && key[i + exact.length + after] === suffix[after]) after++;
    if (before + after > bestScore) {
      best = i;
      bestScore = before + after;
    }
  }
  if (best === -1) return null;
  return { start: at[best], end: at[best + exact.length - 1] + 1 };
}
