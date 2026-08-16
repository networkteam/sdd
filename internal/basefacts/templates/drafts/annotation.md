---
type: signal
layer: process
kind: fact
override: closed
confidence: high
topics:
    - engine/base-facts
    - type-system/kinds
summary: >-
    The annotation kind lays topic labels over entries already written —
    membership indistinguishable from tagging at capture, the refs as the
    member set, labels reused before minted and near-permanent once minted —
    with an empty body as a correct shape and open-forever as its resting
    state.
---

# Laying structure over entries — the annotation kind

An annotation lays structural metadata over the entries it references — today in one form: topic labels declaring which cluster those entries belong to. It is a signal — something noticed: a thread already running through the record, now made findable in one pull. But its reader is most often a query, not a person — an annotation exists to be filtered on, not read as an observation — so the questions it answers are structural: which entries, under which labels, and, where a label covers only some, why those. The kind is broader than its one current form; structure of other sorts can join later without changing what an annotation is.

**Membership is the whole effect.** Declaring a label over its refs makes each referenced entry a member of that topic — indistinguishable, to every later query, from the entry having carried the label at capture. An entry is in a cluster whether it tagged itself or an annotation tagged it: the two routes produce one membership, not two meanings. Membership is binary — in or out, never weighted — so an annotation ranks nothing and says nothing about importance. And the annotation is itself a member of every label it declares, which is why it carries no separate labels of its own: its declared labels are its topics.

**The moment has passed — that is the reason to reach for one.** Labeling at the moment of writing is the primary, zero-ceremony path: tag at capture when you can. An annotation is for when that moment is gone — the entry is immutable, the label did not exist yet, or one act should re-thread many entries at once. "Three entries over two months — a delivery arrived below the spec's grade, a customer reported a sagging purlin, the shop decided to change supplier — and nobody had a word for the thread when each was written": one annotation declaring `timber/stock-quality` over all three makes them one cluster without rewriting anything. And rewriting is never the alternative — superseding an entry just to change its labels replaces a record that is not wrong; the annotation adds the membership and leaves the original untouched.

**The refs are the member set.** What the annotation is about is exactly the entries it references, and by default every label it declares covers all of them. When a label belongs to only some, that label names its members — always a subset of the refs — and the body says in a line why those were singled out: "only the delivery and the supplier change also fall under `suppliers/sourcing`; the sagging report is a customer matter, not a sourcing one." One ref is a legitimate annotation, and so is one act founding a whole family of labels over a dozen entries — scale is not part of the kind's meaning.

**A label is vocabulary, reused before minted.** A label means the same cluster everywhere it appears, and the graph stays coherent only if labels are reused rather than reinvented — so before proposing one, look at what the neighbouring entries already carry. Prefer hierarchical paths for families. And make each label specific enough to discriminate against future captures: a catch-all — miscellaneous, general, other — will never support a filter. A new label is worth minting when it founds a real cluster; a label with one member is a name, not a cluster.

**Minting is close to permanent.** There is no rename. Re-labeling runs through new annotations, and the old label survives the move: labels carried at capture sit in immutable entries, and an earlier annotation keeps declaring its label until it is separately retired — so a cluster's effective labels accumulate the stale alongside the current. Getting a label right once is cheaper than evolving it later.

**Nothing is owed.** An open annotation demands nothing: it puts nothing on the project's attention surface, and open indefinitely is its correct resting state. It never closes anything either — labeling a record is not resolving it. When a clustering stops being wanted, a directive retires the annotation with the why; when the vocabulary sharpens, the usual move is a new annotation beside the old rather than a replacement.

**The body names the cluster, at most.** Its one job is the cluster's name and a one-line gloss — and none at all is a correct shape, never a defect: prose density is not a virtue in this kind. The moment the body starts arguing something — why the pattern matters, what should be done about it — that argument is an entry in its own right, pointing at the same members.

**Choosing annotation at all.** The first cut — structure, not narrative — lives in the type-system introduction; these tests run past it. Strip the refs away: if anything is still worth reading, it is not an annotation — an annotation adds no proposition of its own, only an index over propositions already recorded. "The shop knows exactly as much as before; it just could not find the three in one pull" is an annotation; "deliveries keep arriving below spec" is a gap, because something new is claimed and something is owed. And when the entry commits attention — who attends to what, for this period — it is a focus: a focus prioritizes; an annotation commits nothing and prioritizes nothing.

{{ .Mechanics }}
