# Intent backfill — full classification

One-time pass over every active directive carrying no `intent` (the `unknown` set at execution). Scope computed at execution via `sdd view --layout='active:kind(directive):not(intent(pending,guiding,settled))'` → 34 directives.

## Scope decision

- **guiding / settled / promote → stamped** via intent-carrying successors that supersede the originals (20 directives). These change behaviour: guiding drops out of the catch-up action lanes and surfaces as standing context; settled derives `{status: settled}` and leaves active listings.
- **pending → left unspecified** (14 directives). A directive stamped `pending` renders and behaves identically to an unspecified one everywhere (`sdd view`, derived status, catch-up action lanes), so stamping would be invisible churn plus refinement-cascade risk for no observable change. The design (d-cpt-cyx) treats unspecified as a valid resting state and backfill as deliberate, not blanket. The deliberate per-directive call is still recorded — here.

## Successor model

Each successor is a concise faithful re-statement of the commitment + `intent` + `supersedes` the original, with `addresses → d-tac-9lv`. Summaries hand-overridden to lead with substance (the auto-summary foregrounds the supersede; see s-tac-ea2). Full rationale, original refs, and attachments remain on the superseded predecessors.

## guiding (17)

| Original | Successor | Reasoning |
|---|---|---|
| d-stg-574 | d-stg-dn6 | agent-agnostic foundations — standing constraint (named exemplar) |
| d-stg-u2i | d-stg-ous | adopt-to-validate direction; "supersede when priorities shift", no completion |
| d-cpt-rkj | d-cpt-owo | CLI output policy — standing (cluster base) |
| d-cpt-5f4 | d-cpt-qhr | three-renderer split — refines owo |
| d-cpt-mvb | d-cpt-dgk | terminal-experience architecture — refines owo, grounded-in qhr |
| d-cpt-n0f | d-cpt-vye | color palette — standing |
| d-prc-nfz | d-prc-asz | ref-kind sharpness calibration — standing |
| d-prc-nkw | d-prc-34z | alignment-first architecture (cluster base) |
| d-prc-rl7 | d-prc-tg8 | procedural-text refinement — refines 34z |
| d-prc-3wk | d-prc-vnu | bidirectional-production — refines 34z |
| d-cpt-s9j | d-cpt-gm1 | ref-kind vocabulary definition — standing |
| d-cpt-tdi | d-cpt-kvb | multi-agent commitment — more harnesses anticipated |
| d-tac-dtl | d-tac-2oj | text/template standard — applied incrementally |
| d-cpt-ba7 | d-cpt-uh0 | universal capture-time ref invariant |
| d-cpt-927 | d-cpt-h4l | standing boundary: CLI owns no worktree lifecycle |
| d-prc-u61 | d-prc-p7a | the fold-in/promote rule itself — standing |
| d-cpt-b2x | d-cpt-5nn | standing "if-we-ever" constraint on done-topic derivation (low conf) |

## promote → guiding (1)

| Original | Successor | Reasoning |
|---|---|---|
| d-prc-v0h | d-prc-xlg | refined the since-superseded d-prc-2is → promoted to standalone, related to the live calibration sibling d-prc-asz |

## settled (2)

| Original | Successor | Reasoning |
|---|---|---|
| d-tac-e41 | d-tac-zyd | retired the retroactive-backfill approach — terminal, no follow-up |
| d-cpt-zew | d-cpt-dbv | judgment-closure of the topic gap — terminal, residual tracked elsewhere |

## pending — left unspecified (14)

d-tac-qom, d-tac-cpw, d-prc-vlu, d-stg-6za, d-cpt-t3j, d-tac-6zt, d-cpt-v8t, d-cpt-313, d-cpt-fbi, d-cpt-s6q, d-tac-d21, d-cpt-30v, d-cpt-0ah, d-tac-b7f — each demands follow-up work that a done signal will close. Recorded as pending-by-decision, not materialised, since pending is indistinguishable from unspecified in every rendering, status, and catch-up surface.