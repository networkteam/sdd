# LLM configuration shape: design record (2026-11-06)

Settled in dialogue between Christopher and Claude. The entry's body holds the commitment; this record holds the example, the rules, the legacy mapping and the alternatives weighed. Key spellings follow the dialogue; the implementing plan may refine them.

## Example

```yaml
llm:
  providers:
    opper:                      # preset: protocols and URLs built in, the user adds the key
      api_key: …
    openai-eu:                  # any name: a second OpenAI-compatible endpoint
      generate: {protocol: openai-chat, url: https://gateway.example/v1}
      embed:    {protocol: openai-embeddings, url: https://gateway.example/v1}
      api_key: …
      rate_limit_rps: 2
    typesafe:
      classify: {protocol: systemone, url: https://api.typesafe.ai}
      api_key: …

  generate:                     # default for every operation
    provider: opper
    model: openai/gpt-5.6-luna
    params: {reasoning_effort: low}
    timeout: 2m
    operations:
      summarize: {provider: ollama, model: glm-5.3-flash:cloud, params: {think: low}}

  embed:                        # one selection: one vector space
    provider: ollama
    model: qwen3-embedding

  classify:
    provider: typesafe
    model: jev-1.14.0
    operations: {}              # eval first; engine operations later
```

## Provider entries

- **Names** are the user's choice. One protocol can appear under several names, for example OpenAI itself and an EU gateway speaking its protocol.
- **Per capability**, an entry declares the protocol it speaks and its base URL. An entry declares only the capabilities it serves.
  - generate: `openai-chat`, `anthropic-messages`, `ollama-chat`, `claude-cli` (no URL)
  - embed: `openai-embeddings`, `ollama-embed`
  - classify: `systemone`, `openai-decisions`
- **The base URL** of a new entry is the provider's documented base, taken as given; the client appends each operation's path (20261102-142152-s-tac-xzn).
- **Entry-level settings:**
  - `api_key`: secret, placed in user-global or machine-local config, never in the committed file.
  - `rate_limit_rps`: a limit belongs to the account, so it sits on the entry.
- **Presets** `openai`, `anthropic`, `ollama` and `opper` carry protocols and URLs, so a user adds only a key. An entry named like a preset overrides it field by field, for example a remote Ollama URL, or a key for Ollama Cloud.
- **One protocol per capability per entry.** Where a provider serves two protocols for one capability, such as Opper's System One and Decisions routes, a second entry names the other.

## Selections

- **Fields.** A selection names an entry and carries the model, parameters and timeout. Parameters are provider-specific and go to the provider as typed fields.
- **Per-operation overrides.** `generate` and `classify` take per-operation selections under `operations`, keyed by the `pkg/llm` operation constants: `preflight`, `summarize` and `writing-guide` today, and classify operations as the application mints them.
- **Whole replacement.** An operation selection replaces the default as a unit, with no field inheritance. Otherwise a default's `reasoning_effort` for an OpenAI-compatible model would reach an Ollama call.
- **`embed` stays one selection.** Documents and queries must be embedded into one vector space, and the cross-repo design uses one shared embedder (20260708-184318-d-cpt-6cq).

## Backward compatibility

Legacy keys keep working with their current meaning. They map to two implicit provider entries, one per legacy block, never merged.

| Legacy key | Meaning kept | Maps to |
|---|---|---|
| `llm.provider`, `llm.model`, `llm.params` | generation provider, model, parameters | the implicit chat entry and the `generate` default |
| `llm.endpoint` | base for the openai or anthropic provider, given as host, `/v1` base or full path, normalized as today | URL of the implicit chat entry |
| `llm.ollama_endpoint` | Ollama URL for chat | URL of the implicit chat entry when the provider is ollama |
| `llm.api_keys.<provider>` | key per provider for chat | key of the implicit chat entry |
| `llm.timeout` | per-call timeout | `generate` timeout |
| `llm.rate_limit_rps`, `llm.retry` | cap and retry policy | implicit chat entry |
| `llm.concurrency` | batch worker count | unchanged; not a provider setting |
| `embedding.provider`, `embedding.model`, `embedding.dimensions` | the embedder | the implicit embedding entry and the `embed` selection |
| `embedding.endpoint` | OpenAI-compatible base before `/v1/embeddings`, read as today | URL of the implicit embedding entry |
| `embedding.ollama_endpoint` | Ollama URL for embeddings | URL of the implicit embedding entry |
| `embedding.api_keys.<provider>` | key per provider for embeddings | key of the implicit embedding entry |
| `embedding.rate_limit_rps`, `embedding.timeout` | cap and timeout | implicit embedding entry and `embed` selection |

**Rules:**
- **Two entries, never merged.** A chat key and an embedding key for the same vendor, or OpenAI for chat and a gateway for embeddings, keep working as configured today.
- **Precedence.** A new selection, when set, wins for its capability. The legacy keys it shadows are not reported, as for any known key a run does not consult (20260921-174543-d-cpt-tv9). The `sdd config` provenance view shows which one is live.
- **Older binaries** read the new keys tolerantly, warn that they are unknown, and keep using the legacy keys. A user who still runs an older binary keeps the legacy keys until they stop.
- **Writes.** `sdd config set` writes only new keys, patch-only as today.

## What `sdd config get` shows

The effective view shows the expanded configuration, so nothing a run uses stays invisible:
- **Every built-in preset**, with its protocols and URLs, under a `preset` source beside `default`, `global`, `project` and `local`. Presets no selection uses are listed too, so they can be discovered.
- **The implicit entries legacy keys map to**, each value labelled with the legacy key it came from.
- **The selections**, per-operation overrides included, resolved.
- **Keys masked**, as on every output surface today.

## Alternatives weighed

- **One endpoint per vendor**, with its capabilities fixed by the vendor. Rejected for a protocol and URL per capability: a gateway serving only some capabilities fits, and so does one vendor twice under different names.
- **A single interface over generate, embed and classify.** Rejected in 20261106-155245-d-cpt-eit: the request and result shapes differ.
- **Field inheritance from the default into operation selections.** Rejected, because provider-specific parameters would leak across providers.
- **Per-operation embed selection.** Rejected: there is one vector space.
- **The name for what a call is for.** `operation` replaces `purpose`, matching the stats file's `op`, the `llmops` package and the graph's wording. `use`, `route` and `task` were weighed; `role`, `step` and `check` are taken or too narrow.
- **New top-level keys.** Rejected for nesting under `llm`; the `embedding` block becomes legacy.
- **A public schema.** Kept private to the SDD binary (20260830-234501-d-cpt-q6n); a host composing `pkg/llm` keeps its own configuration.

## Left to the implementing plan

- Exact key spelling, and the presets' contents and URLs.
- Whether TypeSafe and the Claude CLI get presets.
- Where the retry policy sits: on the entry, like the rate limit, is the expectation.
- How the overlay (global, then per-repo, then machine-local) merges entries and selections: entries field by field, and a selection as a unit, is the expectation.
- Validation on write: protocol names, references to unknown entries, operation keys.
- Whether `sdd config` offers to rewrite legacy keys into the new shape. Nothing is committed here.
