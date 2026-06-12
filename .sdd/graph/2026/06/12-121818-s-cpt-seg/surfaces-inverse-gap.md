# The `surfaces` inverse gap — the forward class is capture-order-bound

## The asymmetry

Ref kinds split into a backward majority and a small forward class (`surfaces`, `required-by`). A forward-class edge is captured on the source pointing at what comes out of it. Because entries are immutable and you can only ref something that already exists, **the kind flips with capture order**: whichever entry is written second carries the edge.

For prerequisites this is order-independent, because both directions have a kind:

- `depends-on` — the dependent is captured second, points back at the prerequisite.
- `required-by` — the prerequisite is captured second, points forward at the dependent.

Either entry can carry the edge, whichever lands last. The relationship is always expressible.

For discovery there is no such pair:

- `surfaces` — the surfacing *work* is captured second, points at the thing it surfaced. Requires the surfaced entry to exist first.
- *(no inverse)* — when the surfaced entry is captured second, it has no kind meaning "I was surfaced / raised by you."

So `surfaces` is the one forward kind whose relationship becomes inexpressible under a common, hard-to-control capture order.

## Why the common order breaks it

The natural workflow order is: do the work (or make the decision), *then* notice what it raised, and capture that as the next step. That puts the surfacer first and the surfaced entry second — exactly the order `surfaces` cannot serve. The surfaced entry must point back, and none of the admissible backward kinds carries "you spawned me":

- `grounded-in` — reads as "reasons from / rests on as a basis." Too passive: it says the entry *draws on* the surfacer, not that the surfacer *produced* it.
- `builds-on` — reads as "extends a closed line / next step after it." Closer in time, but still says nothing about discovery.
- `addresses` — the most active backward kind, but it means "acts on / realizes," which is the wrong direction; and it is **mechanically blocked** when the surfacer is a terminal `done` (a done records completed work and cannot be addressed).

## The two symptoms

1. **Lossy kind.** Whatever backward kind is chosen, the "raised by / discovered by" relationship is lost from the structure and survives only in prose (`desc`). The discovery direction is not queryable.

2. **Mis-targeted ref (the worse one).** Because no kind says "raised by," and `addresses` is blocked when the true surfacer is a terminal `done`, authors re-anchor the edge onto a more convenient *live decision* nearby — where a backward kind at least applies. The ref then points at the entry the surfaced item is *about* rather than the entry that *spawned* it. This is also where capture-time kind disputes are born: the re-anchored ref invites an `addresses`-vs-`grounded-in` argument that a correctly-targeted edge would never have triggered.

### Worked instance (generalized)

- A `done` signal operationalizes an active directive into ready-to-build work. In doing so it carves an uncertain, out-of-scope assumption out of the acceptance criteria and notes it will be tracked as a separate open question.
- The next step captures that **question**. Its true surfacer is the done's work — but it cannot point there: `addresses` is blocked on the terminal done, `grounded-in`/`builds-on` understate the spawned-by relationship, and forward `surfaces` cannot live on the question (the done was captured first).
- So the question is ref'd to the **directive** instead. On the directive, `addresses` is wrong (the question questions the directive, it does not realize it) and `grounded-in` is the defensible-but-passive fallback — the dispute that surfaced this gap.

## Precedent

This is the same shape as the directionality gap that earned `required-by` — the first paired inverse in the vocabulary. There, `depends-on` had a homeless forward partner; adding `required-by` made the prerequisite relationship order-independent. `surfaces` is the analogous case for discovery, dismissed at the time on the grounds that its reverse "already lands on `grounded-in`/`addresses`." This gap is the evidence that the landing is both lossy and direction-wrong.

## Proposed resolution — `surfaced-by`

Add `surfaced-by` as the symmetric inverse of `surfaces`:

- **Reads:** `source` was surfaced / discovered by `target`.
- **Means:** the source came out of the target's work; captured on the surfaced entry when it is written after its surfacer.
- **Direction:** backward-capture of the forward `surfaces` relationship — same relationship, opposite capture order, exactly as `required-by` is for `depends-on`.
- **Query it unlocks:** "what did this entry surface?" — answerable from the surfacer's incoming refs, instead of being blurred into the `grounded-in`/`related` bucket.
- **Applicability:** like `surfaces`, plausibly applicable across all target classes (live decision, live signal, terminal done, retired) — notably it gives a terminal-`done` surfacer a clean inbound edge, which `addresses` cannot.

### Lighter alternative — documentation only

Pin `grounded-in` as the sanctioned backward kind for a surfaced entry captured later, restore the reverse-of-`surfaces` guidance to the reference (generalized to gap / **question** / insight), and carry the "surfaced by" nuance in `desc`. Leaves the discovery direction prose-only and unqueryable, and does not fix the mis-targeting when the surfacer is a terminal done.

## Open questions

- Exact name: `surfaced-by` vs `discovered-by` vs other.
- Whether the query need ("what did this entry surface?") clears the documented growth-cost bar — every added kind is one more capture-time judgment and one more rubric boundary.
- Whether adding the kind makes the reverse-of-`surfaces` documentation fix moot, or whether both ship (kind for new captures, doc guidance for the judgment call that remains).
- Matrix cells and pre-flight notes for the new kind across the four target classes.
