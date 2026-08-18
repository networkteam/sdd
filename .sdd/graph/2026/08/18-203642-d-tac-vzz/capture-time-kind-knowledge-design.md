# Capture-time kind knowledge delivery — implementation research

## Observed failure

A live outer evaluation found that a capture started with `directive` preselected let the agent draft from the capture procedure's inline requirements without reading the directive authoring fact. Once prompted to read that fact, the agent added the missing fulfillment horizon. The detailed fact works when pulled; capture does not currently create the pull at the needed moment.

Starting this plan capture with `plan` preselected reproduced the same route: the initial `assemble` serve said the overview need only be pulled unless the kind was already settled, named `plan`, and did not point to the plan authoring fact.

## Current code

- `internal/baseprocedures/entries/20260703-094500-d-prc-cap.md` starts capture at `assemble`. Its only injection is `viewLayout`. Its Kind paragraph points at the overview only when the kind is unsettled.
- `sdd.start_procedure` already returns the first active step's rendered instruction unit. Therefore capture can serve grounding in the initial start response, before the agent chooses a kind, drafts, or reports to `assemble`; no extra procedure step or tool round trip is required.
- `internal/engine/registry.go` exposes the session's folded read ledger as `Context.Reads`.
- `internal/engine/session.go` records full and summary reads. The full-read ledger is suitable for semantic per-session deduplication.
- `application/workflow_registry.go` is the existing home for workflow injection queries. `entryChains` demonstrates an injection query that serves graph content and records it as a full read.
- `mcpapp/server.go` also deduplicates rendered blocks by content hash per connection, but capture currently exposes one whole `InstructionUnit`. That mechanism cannot independently recognize an overview embedded inside a changed assemble unit, and it does not count a prior `show` of the fact.
- `internal/basefacts/basefacts.go` already maps every kind to its authoring fact through `AuthoringFactID`.
- `internal/finders/writingguide.go` already resolves the overview and drafted kind fact from live graph heads for the post-draft writing guide. The drafting side should use the same graph-resident source rather than copy prose.

## Chosen behavior

Register a capture drafting-knowledge injection query. It resolves the live type-system overview and, when a kind is preselected or already stored, the selected kind's authoring-fact identity and retrieval cue.

On the initial capture serve:

1. If the session read ledger does not contain the overview at full depth, render its complete body before the drafting instructions and record a full read.
2. If the overview was already served in full anywhere in the same SDD session, omit the body.
3. Tell the agent to read the overview before choosing a kind or drafting.
4. Tell the agent to pull the chosen kind's detailed authoring fact before writing the draft.
5. If the kind was preselected, name that fact directly in the initial serve.
6. Do not add the overview or authoring fact to `requiredRefs`, `refsInspected`, or any transition predicate.

The overview already points at the discrimination fact and explains when to pull it. Capture adds no separate discriminator cue.

For an unselected capture, the overview supplies the complete authoring-fact index before the agent chooses. A selected-kind cue may reinforce the specific fact when the procedure later knows the kind, but no extra kind-selection round trip and no fact-read gate are introduced. This leaves the behavioral question measurable: does an unprimed agent follow the served pointer and draft well before the writing guide corrects it?

## Verification surface

- Engine/application test: fresh capture session receives the complete overview in the first `start_procedure` response.
- Engine/application test: a prior full read of the overview suppresses it in a later capture in the same session.
- Engine/application test: a summary-only read does not suppress it.
- Engine/application test: a preselected kind receives its precise authoring-fact pointer in the initial serve.
- Engine/application test: an unselected capture instructs the overview → choose kind → pull detail fact → draft order.
- Regression test: the overview contains the discriminator fact pointer, while capture adds no parallel discriminator instruction.
- Regression test: neither base-fact identity participates in assemble transition predicates or `refsInspected`.
- Failure test: missing or empty live base facts surface an error rather than silently dropping guidance.
- Outer evaluation after delivery: observe whether an unprimed capture pulls the selected-kind fact before its first draft and whether the writing guide still needs to supply kind-specific craft that the fact already carries.
