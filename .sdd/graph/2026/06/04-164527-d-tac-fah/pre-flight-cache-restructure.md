# Pre-flight cache restructure — design

## Problem (s-tac-osb)

Pre-flight's system prompt is not a stable cacheable prefix. Each check type has
its own `_system` block, and the block embeds volatile graph content (active
contracts; and, until d-tac-yt1, the full open-signal set). So two captures —
whether different check types, or the same type after the graph changed — produce
different system prompts and miss Anthropic's prompt cache. The SystemPrompt/
UserPrompt split (d-tac-bes) was introduced for cache-eligible prefix matching,
but the prefix it produces is not byte-identical across captures.

## Chosen approach — Option A: byte-stable universal system preamble

Make the system prompt one type-independent preamble shared by every check type;
push everything variable into the user prompt.

### Universal -> stable system preamble
- Validator role line (already identical across all check templates)
- Ref-kind vocabulary (the largest stable chunk; today inside ref_meta_consistency)
- Output / verdict JSON format
- Always-on partials (entry_quality, unrelated_refs, language, durability,
  unusual_close). Today these are conditionally included per check type; making
  them universal means each must self-guard on the entry's shape ("apply when the
  entry has refs", "applies only when Kind: plan", etc.) — most already do.

### Type-specific + volatile -> user prompt
- The per-check task line and that check's specific checks + calibration
- The proposed entry and its referenced / closed / superseded entries
- Active contracts (universal context, but volatile — see open questions)

## Alternatives considered

- **Option B — per-type system stable, only volatile content to user.** Low risk,
  preserves calibration in place, but only caches *same-type* sequential captures;
  a decision followed by a done-signal still misses. Rejected: leaves most of the
  win on the table.
- **Option C — layered cache breakpoints** (shared preamble cached once for all
  types, per-type block cached per type). Best in theory, but not reachable
  through the current gollm path — see mechanism below. Rejected on that basis.

## gollm / Anthropic caching mechanism (why C isn't reachable; what actually caches)

- `ephemeral` is Anthropic's **server-side** prompt cache: ~5-min sliding TTL
  (refreshed on hit), keyed on exact token prefix + model + API key. It is NOT
  tied to local process lifetime — the gollm `CacheType` doc comment
  ("duration of the program's execution", prompt.go:16-18) is misleading.
- The fork's Anthropic provider always sends `anthropic-beta: prompt-caching-2024-07-31`.
- `splitSystemPrompt(systemPrompt, 3)` splits the system prompt into up to 3
  chunks **by paragraph** (`

`), then attaches `cache_control: ephemeral` to
  every chunk except the first (the `i > 0` test, anthropic.go:184-194). Anthropic
  caches the prefix up to each breakpoint, so the last breakpoint covers the whole
  system prompt. User content gets a breakpoint when `enable_caching` is set
  (sdd sets it).
- **Consequence:** breakpoints are placed by mechanical paragraph math, not
  semantic boundaries. A shared preamble across two different check types will not
  land on a byte-identical breakpoint boundary (each prompt's paragraph count
  shifts the cuts), so Option C's cross-type preamble reuse does not work here.
  What caches is a **fully byte-identical system prompt across calls** — which is
  exactly what Option A produces.
- **Edges to respect:** a system prompt with fewer than 2 paragraphs (no `

`)
  yields one chunk, `i > 0` never fires, and nothing is cached — so the stable
  preamble must stay multi-paragraph. Anthropic also only caches prefixes above a
  token floor (~1024); the ref-kind vocabulary likely keeps the preamble above it,
  but verify.

## Cost model

First call writes the cache (`cache_creation_input_tokens`, ~25% surcharge);
each byte-identical repeat within the TTL reads it (`cache_read_input_tokens`,
~90% cheaper).

## Validation

Re-run the `preflight_eval_test` golden cases after moving per-type calibration
into the user turn, to confirm instruction-following does not degrade when the
rules sit in the user message rather than the system message.

## Dependency

Verifying the win requires the cache token counts to be observable, which they are
not on the gollm path today (s-tac-sae). Land that first, or in parallel, so the
restructure can be measured rather than assumed.

## Open questions

- **ActiveContracts placement.** Contracts are universal context but volatile.
  Default to the user prompt (keeps the system prompt stable); revisit if contracts
  prove stable enough in practice to live in the preamble.
- Whether the stripped preamble stays above the ~1024-token cache floor.
- Whether moving calibration to the user turn measurably shifts validator behavior
  (the eval cases answer this empirically).
