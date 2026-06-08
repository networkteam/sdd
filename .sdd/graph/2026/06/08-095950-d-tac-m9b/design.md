# Supersede-chain resolution — design

## The defect: one root, two faces

`Graph.DerivedStatus` (internal/model/status.go) resolves lifecycle by taking the first entry in the reverse index, one hop deep:

    if ids := g.SupersededBy[e.ID]; len(ids) > 0 { return Status{Kind: StatusSupersededBy, By: ids[0]} }
    if ids := g.ClosedBy[e.ID];     len(ids) > 0 { return Status{Kind: StatusClosedBy,     By: ids[0]} }

Two faces of the same naive resolution:

- **Transitive (s-tac-5p5).** A ref to a multiply-superseded entry resolves only one hop (`[0]`), so a reader hops link by link to reach the live entry — e.g. d-cpt-ydf -> d-cpt-ygn -> d-cpt-ni0. Tooling never surfaces the live head.
- **Selection (s-tac-ohl).** `[0]` is "first by graph insertion order". When an entry has multiple closers and the earliest-inserted one is later superseded, the stale closer is reported (observed: d-cpt-lgs renders closed-by the superseded s-tac-mxi instead of the active s-tac-uo2).

Unifying rule: **read-time resolution always lands on the active head, never a superseded link.**

## Why read-time, not capture-time

The capture-time norm (ref the current head) cannot carry this alone. The graph is immutable, so a historical ref can never be rewritten; and under concurrent work the head an author refs may already be superseded by the time the entry lands. So the durable fix is mechanical, applied when reading — exactly as actor-identity chains already resolve to a head for canonical/role derivation rather than rewriting old role entries.

## The type: ResolvedRef

The resolver returns a value type rather than a bare `[]string`, because a bare slice invites the very bug we are fixing — a caller grabbing `chain[0]` (the stale origin) as if it were the head.

    // ResolvedRef is the resolution of a target ID to its live head, retaining
    // the stale trail for display. path is unexported so callers cannot index
    // an intermediate and mistake it for the head.
    type ResolvedRef struct {
        path []string // ordered: origin first ... live head last; len >= 1
    }

    func (r ResolvedRef) Head() string   { return r.path[len(r.path)-1] }
    func (r ResolvedRef) Origin() string { return r.path[0] }
    func (r ResolvedRef) IsStale() bool  { return len(r.path) > 1 }
    func (r ResolvedRef) Path() []string { return r.path } // rendering only

`Head()` is the accessor every reasoning consumer reaches for; the stale trail only comes out via `Path()`, whose name signals "I am rendering the trail." A ref to an active target yields `len(path) == 1`, `Head() == Origin()`, `IsStale() == false` — the common case, which is why the type is named for ref resolution, not for supersession.

Mirrors the existing `ActorChain{ Head *Entry }` abstraction (internal/model/actor.go); the actor head-walk (`ChainForCanonical`) is the precedent this generalizes.

## The two faces stay separate in code

- The **transitive** face uses `ResolvedRef` — resolve a ref target by walking `SupersededBy` to the head.
- The **selection** face (s-tac-ohl) does not need a chain shape. It chooses among sibling closers, so it returns a plain active ID via the shared "is this superseded?" predicate — forcing a chain type onto a sibling-selection would be the wrong fit.

Both share the single-hop primitive (follow `SupersededBy`, prefer the active successor); the transitive face iterates it to the head.

## Consumers (CQRS placement)

- `internal/model` — the resolver and `ResolvedRef` (pure computation, no I/O).
- `internal/finders` + `internal/presenters` — ref sub-lines in `sdd show` and `sdd view` `expand(refs)` render `Path()` for stale targets only; active targets render unchanged.
- pre-flight finder — ref context takes `Head()`.
- lint handler — fork detection.

`sdd show`'s restructure (d-tac-z3k) is a downstream consumer: its acceptance criteria keep status derivation out of the renderer, so it renders whatever status the read side hands it. Its tree-node status slot must accept a path, not a single ID — the one interface note to coordinate.

## Linear-chain assumption and forks

The walk assumes a linear supersession chain. A fork — one entry with more than one superseder — makes "walk to head" ambiguous. Rather than encode a tie-break in the resolver, `sdd lint` flags a fork as an anomaly (it should not occur under normal capture; it signals direct file edits, corruption, or a validator bypass). The resolver may take the first active successor; lint is where the anomaly surfaces.

## Display semantics

For a stale ref the sub-line shows the path with the live head emphasized — the origin followed by its trail to the head — so a reader who meets the origin ID elsewhere (including in immutable body prose that still names the old ID) can connect it to the live entry. Active refs render exactly as today. The precise rendered form is owned by the `sdd show` restructure (d-tac-z3k); this plan supplies the data (`ResolvedRef`), not the final glyphs.

## Alternatives considered

- **Head-only (drop the trail).** Rejected: loses the connection value for stale IDs met in prose; the trail is cheap to retain and bounded to the few stale refs.
- **Bare `[]string`.** Rejected: invites indexing the wrong end; the type makes `Head()` the safe default.
- **Replace the displayed target with the head.** Rejected in favour of showing the path: an immutable body keeps naming the origin, so the reader needs the origin->head mapping, not a silent substitution.
- **Rewrite refs to the head at capture.** Rejected: immutability forbids it, and concurrent supersession would stale it anyway.