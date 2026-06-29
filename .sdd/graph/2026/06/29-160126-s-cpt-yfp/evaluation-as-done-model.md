# Evaluation as a done signal — synthesis

## The model
- Evaluation is work, like implementation. Both produce a `done` signal recording what was done.
- A done references the **move** it followed (proposed edges: `implemented-via` / `evaluated-via`).
- Findings raised during evaluation (gaps, insights, decisions) are `surfaced-by` the evaluation-done — not the ad-hoc fact-signal route.
- "done done" (s-cpt-9qn) becomes readable: an implementation-done with a downstream evaluation-done is corroborated; without one, it is committed-but-unevaluated.

## Coverage map (scoped)
- Computable over **graph entries**: recorded work × axis/move, aggregated by topic → which graph areas hold unevaluated work.
- **Limit 1 — graph, not code.** "Which code is under-inspected" needs a code↔entry bridge (entries tied to files/paths/symbols, or a coverage tool feeding the graph). Not present today; targeted code-review is gated on it.
- **Limit 2 — capture-dependent.** Measures *captured* evaluation; an unrecorded evaluation reads as a false gap → the reference needs a structural pre-flight check (same shape as the intent-framing check, s-tac-oh3), not agent memory.

## Moves vs rules
- **Move** = a procedure (multi-step recipe) to follow — the moves-half of s-cpt-k8i, still unbuilt.
- **Rule** = a checkpoint / constraint (d-tac-eho, in plan) — could enforce "the expected moves were run."
- inner/outer are the bundled defaults; project-specific evaluation moves layer on. Rules are strictly project-local (s-cpt-ov7); the same is assumed for moves.

## Thread / alternatives considered
- Started from "is `fact` the right kind for an evaluation finding?" (the validation capture s-tac-g55).
- fact-as-finding → superseded by evaluation-as-done (the record is a done; findings surfaced-by it).
- work/lens as a closed framework enum → rejected: too prescriptive for arbitrary projects.
- config-declared lens vocabulary → viable, but a second vocabulary; moves subsume it.
- process = rule → rejected: a rule is a checkpoint, a move is a procedure.

## Open questions
- inner/outer: dissolve into freely-customizable moves, or persist as the two axes that project moves realize (keeps "cover both axes")?
- Is an evaluation "move" one entry or a named bundle of steps?
- The done→move edge: a new ref-kind (`evaluated-via` / `implemented-via`) or frontmatter fields?
- Sequencing: fold the typed edge + coverage into rule v1 (d-tac-eho), or a follow-on depending on it?
- Required vs optional classification + a structural pre-flight check (mirrors s-tac-oh3 for intent).
- Code↔entry bridge: a separate future mechanism for code-layer coverage.
