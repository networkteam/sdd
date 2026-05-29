# Ref-kind misfire / noise — fix design

See the gap for the problem statement. Two mechanisms drive it: an authoring-order inversion (kind picked from intent plus a thin `related` default, body written more committally) and a pre-flight medium band that fires whenever a sharper kind merely exists, not only on contradiction.

## Lever 2 — body-derived kind selection

- Flip the authoring order: write the body first, then tag the strongest relational axis the body actually asserts.
- `related` becomes a true last resort, gated by a forcing question: "why does no other kind fit?"
- Why it works: pre-flight judges the kind against the body, so keying selection off the body makes the two agree by construction.
- Where it lives: the skill's "Refs matter" guidance and the framework-concepts ref-kind table — ideally a single shared fragment (see interlock below).

## Lever 3 — one-notch pre-flight recalibration (`ref_meta_consistency.tmpl`)

| body vs. kind | severity |
|---|---|
| body contradicts the kind | high (keep) |
| body asserts a functional axis the kind drops or softens (genuine undersell) | medium (keep) |
| kind fits; a sharper kind merely exists | low / none (was medium) |

Grounded in the evidence: 7 of 12 `related` refs sit in the demoted band; 0 contradict. The recalibration removes the friction without losing a real catch in the sample.

## Interlock with s-tac-uer (shared reference)

Lever 2's rule should live in one canonical home that both the skill and pre-flight read; otherwise tightening it means editing two hand-synced copies — exactly the drift s-tac-uer names. This directive can ship its discipline as a stopgap in both places if that home is not ready, so it relates to s-tac-uer rather than blocking on it.

## Rejected: remove `related` entirely

Too blunt — about 5 of the 12 sampled refs are genuine, correctly-tagged parallel siblings. The kind is needed; the discipline around it is what is missing.

## Implementation wrinkle

The skill is markdown loaded into the agent's context; pre-flight is an embedded Go `text/template` assembled into an LLM prompt. "One file both read" needs a build/embed decision — for example a canonical fragment embedded into the prompt and included or rendered into the skill reference. This is s-tac-uer's territory.

## Open questions

- Exact home and embed mechanism for the shared fragment (resolved with s-tac-uer).
- Does lever 2 need the forcing question written into the skill, framework-concepts, or both?
- How to confirm the recalibration worked — measure the ref-kind medium-finding rate before and after.
