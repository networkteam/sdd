# Attachment read access — design note

## Problem
Reading an entry's attachment has no CLI surface. `sdd show` prints a graph-dir-relative path with no fetch hint, so agents fall back to `find`/`grep` through `.sdd/graph/` — coupling to a layout that isn't promised stable.

## Two surfaces
- **Local CLI**: `sdd show` prints each attachment as an **absolute** filesystem path; the agent opens it with its native read tool (partial reads, large/binary files). Absolute over relative because Claude Code's read tool needs an absolute path and to avoid cwd guesswork.
- **MCP**: a dedicated tool returns attachment **content with paging** (offset/limit, mirroring the local read tool), since a remote agent can't open a local path. The MCP `show` equivalent returns attachment *names*, not paths.
- **Shared**: a read accessor on the `AttachmentStore` (d-tac-0v4) backs both, so path resolution and paged content share one source of truth and the on-disk layout stays internal — the agent only ever opens what `sdd` emitted.

## Alternatives rejected
- **Checkout / materialize to temp** — adds a lifecycle (cleanup, staleness) to solve what a stateless read accessor already solves.
- **Return content locally** — loses partial reads on large files.
- **`sdd show` prints only a command hint** — costs a second round-trip to fetch.
- **Relative path locally** — cwd guesswork; Claude Code's read tool needs absolute.

## Open questions
- MCP paging unit: lines (like the read tool) vs bytes vs chunks. Lean lines for text/markdown.
- Whether to also add headless local commands (`sdd attach path` / `sdd attach cat`) for scripting / non-`show` use, or rely solely on the `sdd show` display fix.
- MCP tool naming/shape (e.g. `sdd_attach_get`) — likely deferred to MCP implementation under d-tac-0v4.

## Sequencing
- The local `sdd show` absolute-path fix is independent and shippable now — removes today's pain immediately.
- The store read accessor + MCP paged-content tool ride along with d-tac-0v4.
