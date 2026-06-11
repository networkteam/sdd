# Go templates as the render mechanism

Supplies the mechanism for d-tac-4ln ACs 1 (agent-neutral templates with per-agent render profiles) and 5 (per-agent deviations as conditionals from one source, no duplication); operationalizes d-cpt-tdi's "template conditionals, never duplicated definitions".

## The three deviation classes (Claude vs Codex)

1. **Dynamic injection** — Claude's `!`sdd info`` magic tokens (8 across `/sdd` and `/sdd-catchup`), expanded at skill-load by Claude Code. Codex has no equivalent → rewrite as explicit "run `sdd …` first, use its output as framing" instructions.
2. **Sub-skill invocation** — "invoke `/sdd-catchup` via the Skill tool" (Claude) vs instructed `$sdd-catchup` (Codex). Scattered inline prose.
3. **Frontmatter shape** — Claude: `name` / `description` / `allowed-tools` + sdd stamps top-level. Codex (Agent Skills standard, agentskills.io): `name` matches the directory, sdd stamps under `metadata:`, `compatibility:` set.

## Mechanism

Each skill file becomes a Go `text/template` with default `{{ }}` delimiters.

- **Includes**: `{{ template "ref-kinds" . }}` replaces the `<!-- sdd:include … -->` directive and `model.ResolveSkillIncludes`.
- **Conditionals**: `{{ if eq .Agent "claude" }}…{{ else }}…{{ end }}` for classes 1–2 (prose deviations).
- **Helpers (FuncMap)**:
  - `inject "sdd info"` → `!`sdd info`` for Claude; a "Run `sdd info` …" instruction for Codex. Centralizes class 1 so the per-agent split lives in one helper, not at every call site.
  - `stamp` → emits the per-profile frontmatter shape (class 3), replacing `model.RenderSkillFile`'s manual marshal + buffer writes.
- **Data context**: `.Agent` (the `model.AgentTarget`), plus per-file fields as needed.

## Escaping the attachments placeholder

The literal placeholder token (resolved by `sdd new` at capture, and which must survive verbatim into the installed docs that document it) is the one collision with default delimiters. Escape it in template source as:

    {{"{{attachments}}"}}

This emits the literal string. A forgotten escape is a template parse error → render fails → the Claude-parity smoke test (AC 2) goes red. Loud, not silent.

## Code this replaces (net deletion)

- `model.ResolveSkillIncludes` + `parseSkillInclude` → `{{ template }}`
- `model.RenderSkillFile` (manual frontmatter marshal + buffer) → `stamp` helper
- the per-agent comment-conditional parser the rejected option would have added → `{{ if }}`

## Alternatives rejected

- **Custom delimiters (`<% %>`)** — clean source, zero escaping, but non-idiomatic; the collision surface is a single recurring token and a forgotten escape fails loudly, so the marginal benefit didn't justify unfamiliar syntax.
- **Comment-directive conditionals (extend `sdd:include`)** — more bespoke parser code, the opposite of this directive's intent.
- **Profile-overlay (base + per-agent patch)** — duplicates whole bodies to swap one inline sentence; violates the no-duplication commitment.

## Broader migration (incremental, as touched)

Default to templates wherever code does excessive hand-rolled string building to *generate* text: fresh-file writers (config + `.gitignore` creation in `handler_init.go`), pre-flight prompt assembly (`internal/llm/preflight.go`), file-generating presenters.

**Excluded:**

- Interactive TTY rendering (`sdd status` / `view` / `show` via glamour + color) — separate case, decided later.
- Structure-preserving *edits* — the comment- and key-preserving YAML round-trip (`model.SetYAMLField` over a `yaml.Node` tree). A template would clobber comments and unknown keys it must retain.

## Open questions (for slice-1 implementation)

- **Whitespace control**: markdown is whitespace-sensitive (lists, code fences); `{{- -}}` trimming will need iteration. The parity test is the guardrail.
- **Source layout**: where the neutral template tree lives under `internal/bundledskills/`, how the `//go:embed` + per-agent render output is structured, and how `claude/` stops being the source while `.claude/skills/` stays the install target.
- **Frontmatter**: templated inline vs computed in Go and injected — lean helper (`stamp`).
