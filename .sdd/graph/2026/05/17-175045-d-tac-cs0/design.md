# Per-ref kind qualifiers — design

## Origin

The ref-metadata gap (`s-cpt-k7z`, April 15) proposed optional per-ref 'why' annotations to improve scannability of upstream chains. The motivating observation: presenting `d-prc-g1h`'s refs during capture dialogue, annotating each with a one-line 'why' significantly improved scannability.

The heat-algorithm gap (`s-cpt-zsd`, May 7) named the structural consequence: the heat algorithm in `sdd view` weighs grounding citations (refs to contracts, aspirations, standing directives) the same as extension/closure refs, causing standing entries to rank artificially high. Resolution depends on per-ref qualifiers landing first.

Both gaps converge on the same structural change: refs need richer metadata than just an ID and a relationship word.

## Label vs description — the split

The originating proposal in `s-cpt-k7z` framed the annotation as a single 'why' field — free text. Dialogue revealed this was conflating two distinct concerns:

- **Mechanical category** (used by algorithms — heat weighting, filters, lint). Must be from a closed set so machines can dispatch on it.
- **Narrative description** (used by humans and agents reading the chain). Free text for scannability.

A free-text 'why' alone can't drive heat weighting without semantic interpretation. Splitting them dissolves several open questions:

- Format: two fields instead of one
- Optional vs required: different answers — kind required, desc optional
- Heat-weighting: keys on kind only; clean lookup table
- Agent capture: picking from a closed set is fast and consistent

## Kind vocabulary

Derived empirically against a 33-ref sample (see below). Seven kinds + `unknown`:

| Kind | Use |
|---|---|
| `grounds` | anchors to standing structure (contract, aspiration, standing directive) |
| `builds-on` | extends prior lineage |
| `addresses` | responds to a gap, question, or signal |
| `surfaces` | created or discovered the referenced gap during this work |
| `evidence` | empirical observation supporting the claim |
| `depends-on` | functional prerequisite |
| `related` | parallel sibling, no other axis fits |
| `unknown` (implicit) | legacy bare-string refs only — not a valid new-capture choice |

Notes on the set:

- `grounds` ↔ `builds-on` form a pair on the lineage axis (anchoring vs extending). `grounds` should pull heat lightly so standing entries don't rank artificially.
- `addresses` ↔ `surfaces` form a pair on the gap-relationship axis (responding to vs originating). Both pull heat fully.
- `related` is the catch-all for parallel siblings. Risk: it becomes a dumping ground; mitigation: agents pick from the labeled set first, fall back only when nothing else fits.
- `unknown` is a read-time parse result for legacy bare-string entries, not a write-time choice.

## Empirical sample

Labeled 33 refs across 11 source entries (mix of strategic / tactical / conceptual / process, decisions and signals across kinds).

### First batch (5 source entries, 17 refs)

| Source | Target | Body says | Kind |
|---|---|---|---|
| `d-tac-k4l` plan | `d-tac-1du` activity | parallel work this partially replaces | `related` |
| `d-tac-k4l` | `s-cpt-k8i` gap | "brings the structural-injection pattern into concrete use" | `addresses` |
| `d-tac-k4l` | `s-tac-bv9` done | "preserves modularization by encapsulating voice/format" | `builds-on` |
| `d-tac-k4l` | `s-cpt-jq7` gap | "addresses the synthesis bottleneck" | `addresses` |
| `d-tac-k4l` | `s-cpt-rwd` insight | ranking mechanism it uses | `builds-on` |
| `d-tac-k4l` | `d-tac-kv5` directive | parent directive for catch-up mode | `builds-on` |
| `d-tac-k4l` | `d-tac-qom` directive | parent directive for topic-drill | `builds-on` |
| `s-prc-fc0` insight | `s-prc-kaw` gap | "emerged from observing stale guidance in /sdd-explore" | `addresses` |
| `s-prc-fc0` | `s-tac-2y8` done | "flagged re-evaluation at the scale we've reached" | `builds-on` |
| `s-prc-fc0` | `s-tac-96a` done | "the search shipment that makes the new approach possible" | `depends-on` |
| `s-cpt-tdp` gap | `s-cpt-ed2` insight | "Refines the cross-model template insight" | `builds-on` |
| `s-cpt-tdp` | `d-cpt-e1i` contract | "surfaces tension with the immutability contract" | `grounds` |
| `s-tac-m09` gap | `d-cpt-ah1` contract | "structural consequence runs against the CQRS layering" | `grounds` |
| `s-tac-m09` | `d-tac-uww` plan | "surfaced during slice 8 of sdd view" | `evidence` |
| `d-tac-uww` plan | `d-tac-kv5` directive | "operational context" | `builds-on` |
| `d-tac-uww` | `d-tac-qom` directive | "operational context" | `builds-on` |
| `d-tac-uww` | `d-cpt-ah1` contract | "CQRS decomposition per d-cpt-ah1" | `grounds` |

### Second batch (6 source entries, 16 refs)

| Source | Target | Body says | Kind |
|---|---|---|---|
| `d-stg-574` directive | `s-stg-gtu` gap | landscape observation grounding | `addresses` |
| `d-stg-574` | `s-stg-qg0` gap | "adoption pathway framing" | `addresses` |
| `d-stg-574` | `s-cpt-4gj` gap | "partially answers by ruling out single-agent-marketplace" | `addresses` |
| `d-stg-u2i` directive | `s-stg-qg0` gap | "directly addresses the adoption gap" | `addresses` |
| `d-stg-u2i` | `d-stg-qlt` aspiration | "This focus grounds the alignment aspiration" | `grounds` |
| `d-stg-u2i` | `s-cpt-y33` question | parallel exploration strand | `builds-on` |
| `d-stg-u2i` | `s-cpt-6ct` insight | parallel exploration strand | `builds-on` |
| `d-stg-u2i` | `d-tac-n6y` plan | "load-bearing for multi-participant collaboration" | `builds-on` |
| `d-stg-u2i` | `s-cpt-cca` gap | implicit prioritization gap | `addresses` |
| `s-cpt-5jk` question | `d-tac-q5p` plan | "Builds directly on the participant identity plan" | `builds-on` |
| `s-cpt-5jk` | `s-cpt-wiv` gap | "aligns with the dialogue-shapes-decisions aspiration" | `grounds` |
| `s-cpt-5jk` | `s-tac-7kh` gap | sibling adoption barrier | `related` |
| `s-cpt-6a6` question | `d-tac-6z1` plan | "validator fix didn't address upstream sources" | `builds-on` |
| `s-cpt-6a6` | `s-prc-qom` gap | "extends the local ground-truth insight" | `builds-on` |
| `s-cpt-tn0` question | `s-tac-z2o` done | "AC 13 implemented section-level deduplication" | `evidence` |
| `s-cpt-tn0` | `d-tac-uww` plan | the plan whose AC raised the question | `builds-on` |
| `s-tac-z2o` done | `s-tac-m09` gap | "surfaced as a follow-up" | `related` |

### Combined distribution

| Kind | Count | Share |
|---|---|---|
| `builds-on` | 14 | 42% |
| `addresses` | 8 | 24% |
| `grounds` | 5 | 15% |
| `related` | 3 | 9% |
| `evidence` | 3 | 9% |
| `depends-on` | 1 | 3% |

Coverage: 33/33. No ref fell outside the seven-kind set.

## Edge cases

**Parallel siblings** (k4l ↔ d-tac-1du, 5jk → 7kh, z2o → m09). Captured by `related`. Rare (3/33 ≈ 9%) but recurring.

**Forward-discovery** (z2o → m09). Done signal noting a gap surfaced during its work. Initially read as `related`, but the causal direction is real and matches the inverse of `addresses` — naming this case `surfaces` keeps the symmetry: `addresses` (decision → gap, responding) ↔ `surfaces` (work → gap, originating).

**Body-cite drift** (d-stg-u2i → s-cpt-cca). The body excerpt didn't explicitly cite this ref. Labeling forced a guess based on topic adjacency. This is a benefit, not a bug: required kind selection surfaces lazy refs.

**`builds-on` overload**. The label bundles "continues this lineage" (uww → kv5) and "extends this insight loosely" (k4l → s-cpt-rwd). The corpus suggests they're hard to separate by mechanical test and weight the same for heat. Hold as one kind.

## Vocabulary decision: `kind` over `label`

Field naming options considered:

| Field | Pro | Con |
|---|---|---|
| `label` | familiar word | `topics[].label` already uses it for topic paths — vocabulary overload |
| `kind` | matches the schema's existing closed-set-discriminator pattern (entry kinds today) | nested `kind` (entry-level + ref-element) |
| `qualifier` | matches `s-cpt-zsd`'s vocabulary | new word, bureaucratic |
| `relation` | semantically apt | overlap with internal Go `relations` |

Chose `kind` for consistency: the schema uses `kind` everywhere a value is picked from a closed set. Nested fields at different YAML levels are unambiguous to parsers and readers alike. `label` stays reserved for topic paths.

## Backwards / forward compatibility

**Reads remain compatible**: existing entries (~440 today) with bare-string refs continue to parse, mapped to `kind: unknown` for traversal purposes.

**Writes are not forward-compatible**: older SDD binaries can't parse object-form refs in new entries. Given the small current user base, mitigation is shipping the binary and skill upgrades together with a clear note in `cli-reference.md`. Compat shims (legacy-form output, version markers) were considered and rejected as costly relative to the upgrade path.

**No auto-backfill**: historical entries stay as `kind: unknown` forever unless a new entry supersedes or closes them. A retroactive labeling command was discussed and deferred — most consumers (heat algorithm, filters) handle `unknown` gracefully via legacy-weight defaults.

## Pre-flight: two-layer

**Mechanical (high severity, blocking)**:
- Every refs item on a new entry must carry a `kind` from the closed set
- Bare-string capture rejected
- Missing kind rejected
- Invalid kind rejected
- `unknown` rejected at capture (it's a read-time fallback, not a write-time choice)

**LLM (medium severity, advisory)**:
- When a ref carries `desc`, check that the entry description body characterizes the relationship consistently
- Contradictions flagged with the ref ID and the relevant body excerpt
- Skipped when `desc` is absent

This trades capture friction for adoption rate. The agent picks from seven kinds — fast — and writes a one-line desc when the relationship is non-obvious. The LLM check catches drift between desc and body without imposing a hard rule.

## Heat-weighting deferred

`s-cpt-zsd` proposes weighting heat by kind so grounding citations decay and resolution citations weight fully. This plan ships only the structural foundation. The follow-up needs:

- A kind-weight table (e.g., `grounds: 0.25`, `builds-on: 1.0`, `addresses: 1.0`, `surfaces: 1.0`, `evidence: 0.5`, `depends-on: 1.0`, `related: 0.5`, `unknown: 1.0` for legacy parity)
- Comparative findings (like `d-tac-uww`'s slice-8 attachment) on the live graph to validate the chosen weights
- A `sdd view` rendering decision on how to surface heat-by-kind in the layout output

Refed but not closed by this plan: `s-cpt-zsd`.

## Out of scope

- Label-aware heat weighting (deferred per above)
- Filter primitives keyed on kind (e.g. `ref-kind(grounds)` in `sdd view`)
- Search-ranking changes consuming kind
- Retroactive labeling tooling (`sdd relabel` or similar)
- Compact-form CLI sugar (could be added if JSON-per-ref becomes friction)
- Pre-flight phase-aware severity refinement (covered by `s-prc-ljg`)
