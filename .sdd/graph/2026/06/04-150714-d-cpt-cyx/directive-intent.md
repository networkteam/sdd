# Directive intent — design

## Motivation: two conflations
- "Active directives" conflates two roles: action-demanding (work pending, closed later by a done signal) and standing/guiding (in force as guidance, never "done", retired only by supersession). Status can't tell them apart, so catch-up and tooling must re-interpret the body each time. Surfaced while resolving the born-closed gap.
- Separately, some decisions are settled at capture — complete on arrival, no follow-up — with no clean home: they linger misleadingly in "active" or need a ceremonial closing done signal (the born-done gap s-cpt-r8s).

## Mechanism: one stored `intent` attribute on directives
- `intent ∈ {pending, guiding, settled}`, written at capture, immutable — a stored attribute like confidence/kind, rendered in brackets.
- **pending** — active, action-demanding; closed later by a done signal.
- **guiding** — active, standing direction; shapes other decisions, retired only by supersession. The soft-standing role d-stg-574 explicitly chose over a hard contract. Distinct from a contract (a hard universal rule).
- **settled** — terminal at capture; complete on arrival, rationale in its own body. Derivation treats it terminal with no closing edge.

## Why this shape — the principle: store only what you can't derive
- pending/guiding/settled is capture-time intent and non-derivable: nothing in the graph distinguishes "demands action" from "guides". To be mechanical (no re-reading the body), it must be stored.
- Closed-ness IS derivable from edges — so we never store it. `DerivedStatus` computes terminality from a close/supersede edge OR `intent == settled`. Status stays a single derived function; intent is just a new input.
- This is why `settled` beats both stored-closed (a second source of truth for a derivable fact) and paired decision+done (an extra entry per settled decision).

## Derivation / rendering
- `DerivedStatus` gains one branch: `intent == settled → terminal`. pending/guiding derive active until a close/supersede edge.
- Renders as its own `{status: settled}`, distinct from `{status: closed-by X}` — more signal (born-settled vs closed-by-later-work).
- Terminality now has two sources the derivation reads: `kind == done` (signals) and `intent == settled` (directives).
- Status sections split: "Directives — pending" (act on these) vs "Directives — guiding" (standing context); catch-up deprioritizes guiding.

## Backward compatibility (read-time)
- An existing directive with no `intent` reads as **unspecified** — rendered exactly as today (plain active), never coerced to pending/guiding/settled. Matches how the graph already treats absent stored attributes (early entries with no confidence render without it, not defaulted to medium).
- The distinction is opt-in: absence means "not specified," not a guessed value. Legacy directives keep today's behavior (active until a close/supersede edge) and appear in a neutral Directives group; the pending/guiding split applies only to entries that carry intent.
- Backfilling a known guiding directive (d-stg-574) is a deliberate supersession, not a blanket sweep.

## Alternatives rejected
- **Stored closed-status** (capture-as-closed): a second source of truth for a derivable fact; tools must check field + edges; no closing entry carries the why. Rejected.
- **Paired decision + closing done signal**: grain-true but an extra entry per settled decision — painful at retroactive-seeding scale (2N entries). Superseded by settled-intent (one entry, status still derived).
- **New terminal decision kind**: duplicates what the settled intent expresses with existing kinds.

## Scope & open items (follow-up plan)
- Scope: the directive kind. activity/plan are inherently pending; contract/aspiration inherently standing. Whether `settled` extends to activity/plan for seeding completed work is open (a done signal is the natural terminal-by-kind for completed *work*).
- Capture-time default for a new directive when `--intent` is omitted (lean `pending`; decide in the plan). The read-time legacy default is already settled here: unspecified.
- Pre-flight: a `settled` directive must justify why it's settled in its body (the done-durability analogue).
- Rendering: the `{status: settled}` label; the pending/guiding split in status/list/show; catch-up treatment.
- Connection: the intake dismiss move (d-cpt-fbi) becomes a `settled` directive that closes the intake, instead of a done signal.
