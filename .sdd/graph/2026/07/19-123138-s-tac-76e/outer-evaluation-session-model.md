# Outer evaluation: completed session model

## Scope

Outer lens only. Exercise the completed session model in genuine engine use, including an assisted second-client handoff. Inner verification was deliberately not repeated.

## Concrete observation

The SDD session log persisted these evaluate-state values before the second client attached:

- `anchor`: `20260719-015148-s-tac-khq`
- `plan`: explicitly began `Outer lens only, as requested` and specified an `assisted second-client or branched-conversation exercise`
- `widenReport`: the searches and prior evaluation evidence already gathered

The first `resume_session` MCP response delivered to Codex session `019f795d-0771-7b70-b2f3-522bfd794982` was 22,872 bytes: 10,916 bytes framing, 7,155 bytes shell instructions, and 1,276 bytes evaluate instructions. Its evaluate instance contained only `goal`, `instance`, `instructions`, `procedure`, `report_schema`, `session`, `status`, and `step`.

The `report_schema` named `anchor`, `plan`, `widenReport`, `innerEvidence`, `innerEvaluation`, `outerEvidence`, `outerEvaluation`, and `selectedFindings`, but the response carried none of their collected values. The evaluate instructions generically described both lenses and rendered diagnostics for both missing judgments. They did not render the persisted anchor, the outer-only selection encoded in `plan`, the assisted posture, or the prior `widenReport`.

The new agent then searched for and showed the anchor again, described both verification and validation as outstanding, and ran `go vet`, `golangci-lint`, and `go test`. Christopher observed that the intended assisted outer evaluation had drifted into autonomous inner verification.

## Mechanics that held

- Session discovery exposed the target.
- Christopher's verbatim request supplied structural consent.
- Plain reorientation converged to stubs; explicit full replay restored full instructions and then returned to stubs.
- A switch without `userWords` was rejected.
- Taking the evaluation back required explicit takeover.
- The displaced writer failed immediately with a typed takeover message.

## Judgment and open design question

This one Codex handoff did not give the new dialogue enough of the recorded evaluation intent to proceed as Christopher expected, while the exercised concurrency and recovery mechanics held. The observation does not establish that all cross-client handoffs or procedures lose intent.

It leads to a hypothesis for later investigation: other procedures may show the same behavior when the current step depends on state collected earlier but its served instructions do not render those values.

The evaluation does not settle the handover projection. Serving the full current typed state is the simplest fidelity baseline. A filtered procedure trajectory may also matter because the order of starts, reports, chooser answers, transitions, parks, resumes, and completed ancestors can explain how the current work was reached; unrelated finished instances and mechanical read/serve events may instead be noise. This is a design question for the surfaced gap. The boundary that did crystallize is that the event log is a procedure trajectory, not the full dialogue, and durable intent-bearing state or trajectory must not disappear silently at handover.