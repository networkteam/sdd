# Retiring `sdd list` — migration scope

## Why
`sdd view` (d-tac-uww) made hand-built overview commands redundant and a drift
risk. `d-tac-oei` already retires `sdd status` onto `sdd info` + `sdd view`.
`sdd list` is the remaining hand-built listing command; it follows the same
path — fold its filtered-listing role onto `sdd view`.

## Code surface (remove the list-specific stack; keep the shared renderer)
- `cmd/sdd/main.go:281` — `listCmd()` registration → remove
- `cmd/sdd/main.go:495` — `listCmd()` impl (flags `--type`/`--layer`/`--kind`/`--topic`/`--missing-kind`/`--all`) → remove
- `cmd/sdd/main.go:570` — `f.List(query.ListQuery{...})` call site → remove
- `internal/query/list.go` — `ListQuery` / `ListResult` → remove (only other mention is a *shape* comment at `internal/query/search.go:43`)
- `internal/finders/list.go` — `Finder.List` → remove
- `internal/presenters/render_list.go` — list presenter → remove the list entry point, but **preserve the shared `EntryLine` format** reused by `sdd view` and `sdd search` (see `internal/finders/view.go:785`, `internal/presenters/search.go:13`)

## `sdd view` parity prerequisites (gate removal)
list filter → view coverage:
- `--layer` / `--kind` / `--topic` → covered (`layer()` / `kind()` / `topic()`)
- `--all` (include superseded) → covered (omit the `active` filter)
- default open/active-only → covered (`active`)
- `--type d|s` → **NO view equivalent** — `sdd view` has no `type()` filter (confirmed absent from `knownFunctions`). Add `type()` to view, or accept the loss (type is derivable from kind/id prefix).
- `--missing-kind` (migration helper) → no view equivalent; likely obsolete (kinds backfilled). Drop unless still needed.

## Skill templates (edit source under `internal/bundledskills/templates/`, reinstall via `sdd init`; never edit installed renders)
Functional invocations to rewrite to `sdd view`:
- `sdd/SKILL.md.tmpl:176` — "Use `sdd show` and `sdd list` to find the right refs" → sdd view / sdd search
- `sdd/SKILL.md.tmpl:219` — "`sdd list --kind actor|role`" → `sdd view --layout='participants'` / `kind(actor):as-list`
- `references/framework-concepts.md.tmpl:184` — "`sdd list --kind actor` / `--kind role`" → view equivalents
- `references/framework-concepts.md.tmpl:199` — "Filter via `sdd list --topic <label>`" → `sdd view --layout='topic(<label>):as-list'`

Rendering-surface mentions to update (drop `sdd list`):
- `sdd/SKILL.md.tmpl:140` — "Summaries are what `sdd status`, `sdd list`, and catch-up render"
- `sdd/SKILL.md.tmpl:231` — language section, raw CLI-output list
- `references/framework-concepts.md.tmpl:223` — "Entry lines in `sdd status`, `sdd list`, and summary chains"
- `references/search.md.tmpl:24` / `:37` — "matching `sdd list` shape" / "Same as `sdd list`: --type, --layer, --kind"
- `references/playbook-groom.md.tmpl:7` — "matching what `sdd status` / `sdd list` surface"
- `references/vocabulary-de.md.tmpl:102` — German status-notation note

Command doc to remove:
- `references/cli-reference.md.tmpl:11` — the `sdd list [...]` entry → delete (or replace with the equivalent `sdd view` layout)

Already aligned (no change):
- `sdd-explore/SKILL.md.tmpl:60` — already says "Do not dump `sdd list` … that pattern is retired"

**Coordinate with `d-tac-oei`:** it retires `sdd status` and touches several of the same combined "`sdd status`, `sdd list`" lines. Handle each shared line once.

## Docs
- `README.md:288` — drop `sdd list` from the CLI-output example
- `AGENTS.md:53` — drop `sdd list` from the presentation-surfaces list (CLAUDE.md imports this)

## Mooted work
- `s-tac-93s` (open gap: add `--since`/`--limit`/`--tail` to `sdd list`) — retiring list moots it; closed by this directive as won't-pursue, use `sdd view`.
