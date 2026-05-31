# Ref kinds — principle-based reference

The detailed spec for restructuring the skill, framework-concepts, and the pre-flight rubric.
Defines the ref-kind vocabulary by **principle** — not by which target types a kind is "allowed" on.
Validated against two graphs' real ref usage (this repo + a 191-entry sister graph).

## How to read this

- A ref is a contextual pointer with **no status effect**. Closure is `closes`/`supersedes`, never a ref kind — so "this will close the target" never decides the kind.
- A ref kind names **why the pointer exists**, chosen from what the **body asserts**: write the body, then tag the strongest relationship it states.
- Kinds are defined by **principle** (the relationship, its direction, and for some the target's status), **not** by a list of allowed target types. Whether a target is empirical (`fact`/`done`) or conceptual (`insight`/`decision`) is read from the *target's kind*, not encoded in the ref kind.
- **Direction.** A ref is written on the *source*, pointing at the *target*; each kind states its reading as `source → target`. Most kinds point **backward** (you ref prior context). A small **forward class** — `surfaces`, `required-by` — is captured on the source pointing at what comes out of it. Because entries are immutable and you can only ref something that already exists, **the kind flips with capture order**: whichever entry is written second carries the edge.
- **No back-ref needed.** A single ref connects both ways — the target's downstream/`refd-by` shows the source. So forward-class refs need no reverse ref; traversal handles it. Don't add a weak `related` just to "point back."
- `related` is the **floor**, never a default — only when no sharper kind fits.
- **Prose↔ref:** if the body names an entry, give it a ref (`related` if nothing sharper); every ref should be earned by the body.
- **Transitivity:** ref the relationships the body actually engages, not the whole upstream chain — the graph reconstructs ancestry by traversal. If a fact's body reasons about the directive behind an activity, ref the directive; if it's about executing the activity, ref the activity.

---

## grounded-in  *(renamed from `grounds`)*

- **Reads:** `source` is founded on / reasons from `target`.
- **Means:** the target is a basis the source draws on — a constraint, direction, premise, or information that shaped the source's reasoning. The source rests on it; it does **not** move the target forward.
- **Apply when:** the source reasons from or rests on the target — a contract or aspiration it's accountable to, a standing direction it holds to, **a fact** it takes as premise or cites as empirical proof, **an insight** it reasons from, or a prior decision it conforms to. (Empirical "proof of a claim" vs conceptual "premise" is just the target's kind — both are `grounded-in`.)
- **Don't apply when:**
  - you **realize / operationalize** the target (do the work it commits to) → `addresses` (the "operationalizes → grounded-in" reflex is the most common misuse).
  - you **extend** a prior line of work → `builds-on`.
- **Also:** `grounded-in` is the natural **reverse of `surfaces`** — a gap/insight captured *after* the work that surfaced it points back with `grounded-in` (it rests on that work's findings).
- **Note — absorbs the old `evidence` kind.** "Cite empirical data as proof" is `grounded-in` to a `fact`/`done`; the empirical hardness is read from the target's kind, so a separate `evidence` kind was target-type specialization (the trap removed from `addresses`). The audit "which decisions rest only on soft reasoning?" still works: `grounded-in` refs whose targets are all non-`fact`/`done`. Put the proof nuance in the `desc` ("…as evidence of feasibility").
- **Migration:** old `grounds` maps to `grounded-in` at the read layer (on parse), so nothing above the parser sees the old name; history isn't rewritten; new writes emit `grounded-in`. Same mechanism as legacy bare-string refs → `unknown`.

## builds-on

- **Reads:** `source` continues / extends `target`.
- **Means:** the source is a successor that **extends a prior line of work** — the target is closed (you're the next step in a finished chain), or you're the next move after it. Prior art, not a standing basis.
- **Apply when:** the target is closed and you extend it (a directive adding an opt-in, extending a closed sign-up directive; a successor plan after a shipped one).
- **Don't apply when:**
  - the target is **active** and you sharpen its commitments in place → `refines`.
  - you **realize** the target's commitment rather than add new scope → `addresses`.
  - you only **reason from** the target → `grounded-in`.
- **Test:** are you *extending the line* (builds-on) or *fulfilling the commitment* (addresses)?

## refines

- **Reads:** `source` sharpens `target` (in place).
- **Means:** sharpen, narrow, or clarify an **active** target's commitments in place (the augmenting pattern). Target stays active; the source's lifecycle is tied to it (closes alongside it).
- **Apply when:** the target is active and you add to or narrow its commitments without replacing it.
- **Don't apply when:** target is closed → `builds-on`; you realize it → `addresses`; you replace it → `supersedes` (not a ref kind).

## addresses  *(generalized)*

- **Reads:** `source` acts on / realizes `target`.
- **Means:** the source's purpose is to **act on** the target — respond to a need, or realize/fulfill/supply a commitment.
- **Apply when:**
  - responding to a gap, question, or insight;
  - a signal or done that **fulfills or supplies part of a decision's commitment** (a fact that does part of an activity's work or feeds a plan's AC; a done advancing or partially fulfilling an open decision without closing it);
  - a plan or activity that **operationalizes a directive**.
- **Don't apply when:** you only **reason from** the target → `grounded-in`; you extend a closed line → `builds-on`; the target depends on you as a prerequisite → `required-by`.
- **Notes:** today's docs restrict this to signals; both graphs show the decision case is real and already appearing in usage. Considered a separate `implements` kind for the decision case and folded it in — realizing a decision and responding to a signal are the *same* act-on relationship with different targets, so a separate kind would re-introduce target-type carving, and usage already extends `addresses` rather than inventing a word.

## surfaces  *(forward-class)*

- **Reads:** `source` surfaced / discovered `target` (during the source's work). Forward in meaning — captured on the work, pointing at what it found.
- **Means:** doing the work of the source created or discovered the target (an evaluation done that surfaced two gaps; an insight that surfaced a directive).
- **Apply when:** you capture the work *and* the thing it surfaced together (capture the surfaced entries first, then the source that refs them).
- **No back-ref:** the surfaced entry needs nothing pointing back — it's reachable via the source's `refd-by`. If it's captured *later* (the source already written), it points back with the substantive kind instead — usually `grounded-in` (it rests on the work's findings), or `addresses` if it's a decision responding to what surfaced.
- **Don't apply when:** generic neighborly context → `related`.

## depends-on

- **Reads:** `source` needs `target` first (prerequisite).
- **Means:** a **functional prerequisite** — the target must land or hold before the source is meaningful or actionable.
- **Apply when:** your work is gated on the target (a probe gated on an inventory; a deletion gated on a verified-backup done).
- **Don't apply when:** the target is a basis you reason from → `grounded-in`; you realize the target → `addresses`.

## required-by  *(new — forward-class, inverse of `depends-on`)*

- **Reads:** `target` depends on / is gated by `source` (written from the prerequisite's side).
- **Means:** the inverse of `depends-on` — this entry is the thing a later plan/activity/focus was waiting on. Captured on the prerequisite, pointing at what needs it. Lets a reader query "what was needed by X?"
- **Apply when:** this entry unblocks or satisfies a prerequisite of the target, and you record the edge from this side ("X was waiting on this").
- **Don't apply when:** you **do/supply** the target's work rather than gate it → `addresses`.
- **Note:** added because `depends-on`'s forward partner had no existing home (see "completeness" below). Avoids overclaiming (unlike "unblocks") — several entries can each be `required-by` the same target.

## related  *(the floor)*

- **Reads:** `source` is a sibling of / neighbor to `target` (symmetric).
- **Means:** a parallel/sibling or contextual-neighbor relationship where no sharper axis fits.
- **Apply when:** genuine parallel siblings (same incident, companion synthesis, a sibling fact in another cluster); a fact that is **context a decision must account for but does not fulfill** (no sharper kind exists for "informs" — see Open); the floor for the prose↔ref rule.
- **Don't apply when:** any sharper kind fits — check `addresses`, `grounded-in`, `builds-on`/`refines`, `depends-on`/`required-by` **first**.
- **Note:** the most over-reached kind in both graphs. The *last* resort, not the default. For a decision target, split: source *fulfills/realizes* it → `addresses`; source is *context it accounts for* → `related`.

---

## Scenario → kind (coverage check)

| Scenario | Kind |
|---|---|
| Respond to / act on a gap, question, or insight | `addresses` |
| Realize a directive; supply/fulfill a plan's AC or an activity's commitment (incl. partial, without closing) | `addresses` |
| Reason from / rest on a contract, aspiration, standing direction, fact, insight, or prior decision | `grounded-in` |
| Cite a fact's/done's measured data as proof (hardness = target kind) | `grounded-in` |
| A gap/insight rests on the findings of the work that surfaced it (captured later) | `grounded-in` |
| Extend a closed prior line of work with a new step | `builds-on` |
| Sharpen an active decision's commitments in place | `refines` |
| Your work needs a prior thing to land first | `depends-on` |
| You are the prerequisite a later entry was waiting on (captured on the prerequisite) | `required-by` |
| Your work created/discovered a gap or insight, captured alongside it | `surfaces` |
| Parallel sibling / same incident / companion synthesis | `related` |
| A fact is context a decision must account for but does not fulfill | `related` *(floor — no "informs" kind)* |

Every common relationship has a home; the only residue on the floor is "informs/context."

## Axes that organize the set

- **Relationship** — act-on, reason-from, extend, sharpen, prerequisite, discover, sibling.
- **Direction** — backward (most) vs the forward class (`surfaces`, `required-by`). The kind flips with capture order; the reverse is reachable by traversal.
- **Target status** — `grounded-in` (basis), `builds-on` (closed/next), `refines` (active, in place) split on the target's status. The pre-flight validator must see the referenced entry's derived status to tell these apart (currently it can't — a separate fix).

## Completeness principle

Add an inverse kind **only when the other direction has no existing home.** `depends-on`'s forward partner was homeless → `required-by` was added. `surfaces`'s reverse already lands on `grounded-in`/`addresses` → no inverse added. This is the test for any future vocabulary growth, and it confirms the present eight are complete.

## Open / unsettled

1. **"Informs / context a decision accounts for"** — currently the `related` floor (no inverse-of-`grounded-in`). Leave on the floor unless a real query need appears; adding an "informs" kind is the next candidate if it does.
2. **Growth cost** — every added kind is one more judgment call at capture and one more rubric boundary. Weigh that before adding more; the completeness principle above is the gate.
