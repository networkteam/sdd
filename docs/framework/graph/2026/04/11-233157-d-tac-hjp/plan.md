# Pre-flight Validator Implementation Plan

Parent decision: `d-tac-s6w` — sdd new invokes claude -p as a pre-flight validator before creating graph entries.

## 1. Prompt templates

Create embedded prompt templates via `go:embed` in a `sdd/preflight_templates/` directory. One template per check type:

- `closing-action.tmpl` — action closing a plan/decision (checks plan item coverage, no silent omissions, no overclaiming)
- `closing-decision.tmpl` — decision closing signals (checks signals genuinely addressed, no silent additions)
- `decision-refs.tmpl` — decision with refs, no closes (checks ref completeness, grounding, no scope smuggling)
- `action-closes-signals.tmpl` — action directly closing signals (checks evidence supports closure, no premature closure)
- `signal-capture.tmpl` — signal validation (checks it's an observation not a smuggled decision, layer appropriate, confidence honest)
- `supersedes.tmpl` — supersedes operations (checks new entry covers superseded ground, no silent scope narrowing)
- `contracts.tmpl` — cross-cutting contract enforcement fragment, included in all templates

Templates use `text/template` with structured context variables (`{{.ProposedEntry}}`, `{{.ReferencedEntries}}`, `{{.ClosedEntries}}`, `{{.SupersededEntries}}`, `{{.ActiveContracts}}`, `{{.PlanItems}}`).

**Verifiable**: Each template file exists, renders without error given test context, contains the checks specified in the design attachment of `d-tac-s6w`.

## 2. Pre-flight library (`sdd/preflight.go`)

All core logic in the `sdd` package, no CLI or OS dependencies beyond the Runner interface.

Components:
- `PreflightRunner` interface: `Run(ctx context.Context, prompt string) (string, error)` — production impl calls `claude -p`, tests inject mock
- `CheckType` enum: one value per check type from item 1
- `SelectCheckType(entry *Entry, graph *Graph) CheckType` — pure function, selects check based on entry type + closes/supersedes/refs flags
- `AssembleContext(entry *Entry, graph *Graph) (*PreflightContext, error)` — gathers referenced entries, closed entries, superseded entries, active contracts, plan items from the graph
- `RenderPrompt(checkType CheckType, ctx *PreflightContext) (string, error)` — renders the appropriate template with context
- `ParseResult(output string) (*PreflightResult, error)` — parses PASS/FAIL + gap list from validator output
- `RunPreflight(ctx context.Context, runner PreflightRunner, entry *Entry, graph *Graph) (*PreflightResult, error)` — orchestrates the full flow: select check type → assemble context → render prompt → call runner → parse result
- `PreflightResult` struct: Pass bool, Gaps []string
- `PreflightFailedError` type: carries gaps, distinguishable from infra errors

**Verifiable**: Each function exists and is exported. No imports of `os/exec`, `fmt.Fprintf(os.Stderr, ...)`, or CLI packages. All dependencies injected.

## 3. CLI integration (`cmd/sdd/main.go`)

Thin wiring in `newCmd()`, inserted between `ValidateEntry` (structural) and file write:

- Construct `ClaudeRunner` (production Runner impl) with model from `--preflight-model` flag
- Call `sdd.RunPreflight(ctx, runner, entry, graph)`
- On `PreflightFailedError`: print gaps to stderr, exit non-zero (entry not created)
- On infra error: warn to stderr, proceed with `preflight: error` frontmatter annotation
- `--skip-preflight` flag: skip validation, add `preflight: skipped` frontmatter annotation
- `--preflight-model` flag: override model (default: haiku)

**Verifiable**: Both flags exist and work. Failed validation blocks entry creation. Infra errors produce warning + annotation. Skip produces annotation.

## 4. Graceful degradation

- `claude` CLI not found in PATH → warn to stderr, create entry with `preflight: error` annotation
- `claude -p` times out (configurable, default 30s) → warn to stderr, create entry with `preflight: error` annotation
- `claude -p` returns non-zero exit → treat as infra error, same handling
- Validation fails (FAIL + gaps) → reject entry, print gaps, exit non-zero (this is NOT graceful — it's the hard gate working as intended)

**Verifiable**: Each degradation path tested with mock runner returning appropriate errors/timeouts.

## 5. Unit tests (`sdd/preflight_test.go`)

Fully deterministic, runs in standard `go test ./sdd/...`. Mock runner, no external dependencies.

Test cases:
- `TestSelectCheckType` — table-driven: entry type + flags → check type for all 6 types
- `TestAssembleContext` — builds graph with test entries, verifies context contains correct referenced/closed/superseded entries and contracts
- `TestRenderPrompt` — renders each template, verifies output contains expected sections and entry content
- `TestParseResult` — table-driven: PASS, FAIL with gaps, malformed output, empty output
- `TestRunPreflight` — integration of the full flow with mock runner: pass case, fail case, runner error case
- `TestPreflightFailedError` — error type is distinguishable, carries gaps

**Verifiable**: All tests pass with `go test ./sdd/...`. No test requires network, LLM, or claude CLI.

## 6. Evaluation tests (`sdd/preflight_eval_test.go`, `//go:build eval`)

Behind build tag, manually triggered with `go test -tags=eval -run TestPreflightEval ./sdd/...`. Uses real LLM via `claude -p`. Not part of normal test suite.

File header documents intent: accuracy measurement for template tuning, not deterministic correctness assertions.

Golden cases:
- **True positive**: Action claims to close a 4-item plan but only addresses 2 items → should FAIL with missing items listed
- **True positive**: Based on real graph history — action like `a-tac-tsd` that silently scoped out attachment validation from `d-tac-kfo` → should FAIL
- **True negative**: Action that legitimately closes a plan with different wording than plan items → should PASS
- **True negative**: Well-formed signal capture → should PASS
- **Contract violation**: Entry that contradicts an active contract → should FAIL

Reports per-case results via `t.Log`. Uses `t.Errorf` (not `t.Fatal`) so all cases run even if some fail.

**Verifiable**: File exists with build tag. `go test ./sdd/...` (without tag) does not run these tests. With tag, produces a readable accuracy report.
