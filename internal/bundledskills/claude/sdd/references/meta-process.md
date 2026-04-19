# SDD Meta Process

Guidelines for how to work with the decision graph in a session.

## Starting a Session

1. Run `sdd status` to see the current graph state (binary is pre-built at `bin/sdd`)
3. Based on the state, suggest what's next — don't wait for the user to figure it out

## Modes of Working

A session can move fluidly between these modes:

**Check in**: "Where are we?" Run `sdd status`. Summarize: active decisions, open signals, recent actions. Suggest what deserves attention.

**Capture**: Something happened — a user observation, market signal, idea, implementation finding. Always dialogue before recording: play back what you'd capture, ask if it's right, adjust. Never silently create entries.

**Evaluate**: An action was completed. Collect signals: did it meet the target? What gaps remain? Consider multiple perspectives (technical, product, brand). Each evaluation is: decision (what to evaluate against) → action (who reviewed) → signals (findings).

**Reflect/Dialogue**: Open exploration around a signal, decision, or open question. The goal is to synthesize insights, shape understanding, and move toward decisions. Like a thinking partner, not a task executor.

**Decide**: Open signals or tensions need resolution. Summarize the relevant signals, discuss options, capture the decision with confidence level and refs.

**Act/Implement**: A decision exists and it's time to execute. Before starting: check if enough decisions exist for the scope. Prefer reducing scope over building into the unknown. Decompose into operational sub-decisions. Track actions against them. Know when to stop and evaluate.

## Capture Guidelines

- **Always dialogue before capturing.** Play back: "I'd capture this as a [type] at the [layer] layer: '[content]'. Does that look right?" Let the user adjust.
- **Keep entries digestable.** One idea per entry. If it needs more detail, externalize to a referenced file.
- **Right type**: Signal = observation/fact. Decision = commitment to direction. Action = something done.
- **Right layer**: Strategic = why/direction. Conceptual = approach/shape. Tactical = structure/trade-offs. Operational = individual steps. Process = how we work.
- **Refs matter.** Always link to the signals/decisions that led to this entry.
- **Confidence is honest.** High = strong conviction. Medium = reasonable but unvalidated. Low = hypothesis/experiment.

## After Every Action

Ask: "The last action was [X]. Any signals from it? Did it meet the target? What gaps remain?" This drives the evaluation loop.

## Suggesting Next Steps

Always end an interaction by offering concrete options:
- "You have N open signals. Want to address [specific one]?"
- "The last action was [X]. Want to evaluate it?"
- "There's a tension between [signal A] and [decision B]. Worth discussing?"
- "Or capture something new — what's on your mind?"

Keep it open-ended but guided. The user chooses, the system doesn't prescribe.

## Session Boundaries

- If the graph has changed since the session started (other participants added entries), suggest a fresh start to avoid stale context.
- It's fine to collect open ends in one session and continue in focused sessions later.
- Multiple sessions can work concurrently on the same graph — Git handles conflicts.
