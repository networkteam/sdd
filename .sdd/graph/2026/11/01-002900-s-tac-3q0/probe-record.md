# Ollama Cloud schema probe, 2026-11-01

Request body, sent to `https://ollama.com` with the account's bearer key (cloud) and to `http://localhost:11434` (local daemon):

- schema: `{"type":"object","properties":{"greeting":{"type":"string"},"language":{"type":"string","enum":["de","en"]}},"required":["greeting","language"],"additionalProperties":false}`
- prompt: "Write two sentences about the weather in Hamburg, in German, as plain prose." (a prompt that pulls away from JSON, so only enforcement yields JSON)
- `stream: false`

| Endpoint | Route | Schema field | Thinking | Model | Result |
|---|---|---|---|---|---|
| ollama.com | `/api/generate` | `format` | unset | glm-5.3-flash:cloud | prose |
| ollama.com | `/api/generate` | `format` | `low` | glm-5.3-flash:cloud | prose |
| ollama.com | `/api/generate` | `format` | `false` | glm-5.3-flash:cloud | prose (leaked reasoning) |
| ollama.com | `/api/chat` (system + user) | `format` | unset | glm-5.3-flash:cloud | prose |
| ollama.com | `/api/chat` (system + user) | `format` | `low` | glm-5.3-flash:cloud | prose |
| ollama.com | `/v1/chat/completions` | strict `response_format` | unset | glm-5.3-flash:cloud | prose |
| localhost | `/api/generate` | `format` | unset | gemma4:26b | schema-valid JSON |
| localhost | `/api/generate` | `format` | `low` | gemma4:26b | schema-valid JSON |
| localhost | `/api/generate` | `format` | `false` | gemma4:26b | schema-valid JSON |

Format evals (`TestWritingGuideEval_Format`, `TestPreflightEval_Format`, 3 runs per specimen and operation, 60 runs) on `ollama glm-5.3-flash:cloud think=low`:

- before the structured-output change (prompts ask for JSON, no schema sent): 58 of 60 parseable; both failures were a second JSON object after leaked reasoning
- after it (schema sent as `format`, no JSON instructions): 5 of 60 parseable; the rest were Markdown prose, some opening with "Findings: []"

For comparison after the change: claude-sonnet-4-6 (Anthropic API) 60 of 60, gpt-5-mini (OpenAI API) 60 of 60, claude CLI sonnet 60 of 60.
