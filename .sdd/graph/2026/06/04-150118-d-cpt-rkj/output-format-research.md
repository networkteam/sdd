# CLI / tooling output formats for LLM agents — research summary

Gathered 2026-06 (via Perplexity) to inform the SDD CLI output policy. Sources are external web references, listed at the end.

## Headline

Current best practice (2024–2026) favors **dual-mode design**: human-readable by default (tables, colors) with an explicit `--json` / `--raw` / `--format` flag for machine consumption, plus TTY detection and `NO_COLOR` support. For nested data, **YAML or Markdown** outperform JSON on comprehension accuracy; **Markdown is the most token-efficient**.

## Format comparison: Markdown vs YAML vs JSON

| Criterion | Markdown | YAML | JSON |
|---|---|---|---|
| Token efficiency | Best — ~34–38% fewer tokens than JSON, ~10% fewer than YAML | ~10–12% more than Markdown, but ~19–20% fewer than JSON | Worst — highest overhead from quotes/braces |
| Parsing reliability | High; no type coercion, but no schema enforcement | Moderate; can fail on Windows / special chars; strict parsers needed | Highest for strict schema validation; ~60%+ of agent failures are malformed JSON |
| Reading comprehension | Good; best for prose reasoning | Best for nested data (62.1% GPT-5 Nano; 51.9% Gemini Flash Lite) | Poor on non-Llama models (50.3% GPT-5 Nano; 43.1% Gemini); best only on Llama 3.2 3B |
| Failure modes | No type errors; some multi-entity ambiguity | Type-coercion bugs (Norway problem), invalid YAML from LLMs | Schema fragility, field drift, silent coercion; needs explicit JSON Schema |

**Benchmark:** ImprovingAgents — 1,000 questions across GPT-5 Nano, Llama 3.2 3B, Gemini 2.5 Flash Lite on 6–7 level nested data. YAML won accuracy for two of three models; Markdown won token efficiency across all.

**Caveat for SDD:** these accuracy results are on *small* models. SDD's primary reader is a frontier model, where comprehension is high across all formats and the gap narrows. So for SDD the deciding factors are standardness (vs bespoke), token cost, and storage-consistency — not the accuracy delta. Format effects are model-dependent ("test with your model"): GPT-4 favors Markdown, GPT-5 Nano favors YAML, Llama is roughly format-agnostic.

## MCP: content vs structuredContent

| Field | Purpose | Format |
|---|---|---|
| `content` | Unstructured, human-readable | Array of Text/Image/Audio/ResourceLink/EmbeddedResource |
| `structuredContent` | Machine-parseable | JSON object conforming to optional `outputSchema` |

A tool returning `structuredContent` MUST also return the serialized JSON in a `TextContent` block (backward compatibility). Servers MUST conform to `outputSchema` if provided; clients SHOULD validate.

## CLI design pattern (humans + agents)

- TTY/isatty detection → auto-disable colors/progress when piped (base behavior).
- `--format json|yaml|text` → global flag overriding auto-detection (`--json` implies quiet). Highest precedence.
- `NO_COLOR` env var → force color off even in a TTY (CI/accessibility); flags override env.
- `--raw` / `--compact` → strip whitespace for agents (~40% output cut).
- Separate stdout (data) from stderr (logs/errors/progress).
- Return structured data on write operations (e.g. `{"id":"...","status":"created"}`).
- Configuration precedence: flags > env > local config > global config > defaults.

## Sources

- https://community.openai.com/t/markdown-is-15-more-token-efficient-than-json/841742
- https://tianpan.co/blog/2026-05-07-context-format-decision-agent-reasoning-json-markdown-plain-text
- https://www.improvingagents.com/blog/best-nested-data-format/
- https://github.com/rjmurillo/ai-agents/issues/893
- https://discuss.google.dev/t/yaml-parsing-error-when-transferring-to-a-playbook/185831
- https://modelcontextprotocol.io/specification/2025-11-25/server/tools
- https://yarlson.dev/blog/gold-standard-cli-design-principles/
- https://www.linkedin.com/pulse/designing-efficient-cli-tools-ai-agents-ajay-prakash-vsb6e
- https://agent-skills.md/skills/michalvavra/agents/create-cli
- https://archit15singh.github.io/posts/2026-02-28-designing-cli-tools-for-ai-agents/
- https://dev.to/the_bookmaster/the-json-parsing-problem-thats-killing-your-ai-agent-reliability-4gjg
