# LLM Metadata Surfacing

## Overview

Surface per-call LLM metadata (tokens, cost, timing) through an agent-neutral abstraction
in the \`llm\` package, with structured logging at configurable verbosity.

## Design

### Types in \`llm\` package (agent-neutral)

- \`RunResult\` struct: \`Text string\`, \`Meta *LLMMetadata\`
- \`LLMMetadata\` struct: \`TotalCostUSD float64\`, \`InputTokens int\`, \`OutputTokens int\`,
  \`CacheReadTokens int\`, \`CacheCreateTokens int\`, \`NumTurns int\`,
  \`DurationMs int64\`, \`DurationAPIMs int64\`, \`Models map[string]ModelUsage\`
- \`ModelUsage\` struct: \`InputTokens int\`, \`OutputTokens int\`, \`CacheReadTokens int\`,
  \`CacheCreateTokens int\`, \`CostUSD float64\`
- \`Runner\` interface changes: \`Run(ctx context.Context, prompt string) (*RunResult, error)\`

### \`llm/claude\` subpackage

- Moves \`claudeRunner\` from \`cmd/sdd/main.go\`
- Adds \`--output-format json\` to claude invocation
- Parses JSON response, extracts \`result\` field as text, maps usage/modelUsage/timing
  to \`LLMMetadata\`
- Exports \`NewRunner(model string) llm.Runner\`

### Logging

- Add \`github.com/networkteam/slogutils\` dependency
- \`cmd/sdd/main.go\` sets up \`slogutils.NewCLIHandler(os.Stderr, &CLIHandlerOptions{Level: level})\`
  where level is \`slog.LevelWarn\` (default), \`slog.LevelInfo\` (\`-v\`), \`slog.LevelDebug\` (\`-vv\`).
  Puts logger in ctx via \`slogutils.WithLogger\`.
- CLI command handlers enrich ctx with \`"command", "<name>"\` attr before calling into llm.
- \`llm\` package has an internal \`logCallResult(ctx context.Context, meta *LLMMetadata, op string)\`
  that emits one debug-level log call under \`slog.Group("llm")\` with:
  - \`"op"\` — the llm operation (preflight, summarize)
  - \`"duration"\` — caller-measured wall-clock duration
  - A sub-group per model (keyed by model name) containing tokens in/out and cost (when available)
- Called by \`Preflight\` and \`Summarize\` after \`Run\` returns.
- Example output: \`command=new llm.op=preflight llm.duration=1200ms llm.claude-haiku-4-5.tokens.in=343 llm.claude-haiku-4-5.tokens.out=10 llm.claude-haiku-4-5.cost=0.0004 llm.claude-opus-4-6.tokens.in=2 llm.claude-opus-4-6.tokens.out=4 llm.claude-opus-4-6.cost=0.0101\`

### Caller updates

- \`llm/preflight.go\`: extract \`result.Text\` for validation, call \`logCallResult\` after
- \`llm/summary.go\`: extract \`result.Text\` for summary, call \`logCallResult\` after
- \`cmd/sdd/main.go\`: instantiate \`claude.NewRunner(model)\`, set up slog handler with
  verbosity from CLI flags, put logger in ctx, set \`"command"\` attr in command handlers
- Test fakes: return \`&RunResult{Text: "...", Meta: nil}\` — metadata optional in fakes

## Acceptance criteria

- [ ] LLM calls return agent-neutral metadata (tokens, timing, per-model breakdown, cost when available) alongside response text
- [ ] Claude-specific code is isolated in its own subpackage; the llm package has no claude imports
- [ ] CLI accepts \`-v\` for info level logging, \`-vv\` for debug level logging, defaults to warn
- [ ] \`sdd new\` and \`sdd summarize\` with \`-vv\` print one log line per LLM call with op, duration, and per-model tokens in/out and cost
- [ ] Log lines show which CLI command and which llm operation produced them
- [ ] Existing preflight validation and summary generation work unchanged
