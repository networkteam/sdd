# Hop-factor sensitivity and index-correctness findings

Comparative findings for supersede-aware ranking (s-tac-dmk), produced by the
evaluation run. Methodology mirrors the sensitivity-analysis shape d-tac-cpw
asks of its own weight table.

Graph under test: the live SDD graph at merge commit `3fdf2295`, 1138 entries.
All measurements reproducible from that commit.

## Method

Four binaries built from identical source differing only in
`model.SupersedeHopFactor` (0.0 / 0.5 / 0.8 / 1.0), checksummed to confirm they
differ, each run against the same live graph over three lanes.

0.0 reproduces pre-change behaviour (no inheritance); 1.0 is full undamped
inheritance; 0.8 is what shipped.

> A first attempt produced four *identical* binaries: BSD-style `sed -i ''`
> fails silently under this project's GNU sed. Caught by checksum before any
> conclusion was drawn. Anyone repeating this should verify the constant
> actually changed in each build.

## 1. Guiding-directive lane

`kind(directive):intent(guiding):active:rank(heat(exp-14d)):n(10)`

Rank of `20260730-171311-d-cpt-0cv` (the corrected delivery rule, the only
entry in this lane with a freshly merged two-predecessor chain):

| factor | rank | score |
|--------|------|-------|
| 0.0    | 8    | 1.216 |
| 0.5    | 3    | 4.146 |
| 0.8    | 1    | 5.905 |
| 1.0    | 1    | 7.077 |

Full top-10 ordering, by factor:

```
0.0   u8o dgk 476 0tm uh0 dym dn6 0cv r99 70z
0.5   u8o dgk 0cv 476 0tm uh0 dym dn6 r99 h4l
0.8   0cv u8o dgk 476 0tm uh0 dym dn6 h4l r99
1.0   0cv dgk u8o 476 0tm uh0 dym dn6 h4l r99
```

Every entry other than `0cv` moves at most one rank across the entire range.
Top eight are identically ordered at 0.8 and 1.0; only `h4l`/`r99` swap at 9/10.

## 2. Active-and-hot lane

`kind(plan,activity,directive):active:not(intent(guiding)):rank(heat(exp-7d)):n(8)`

Byte-identical scores at 0.0, 0.8 and 1.0 — no member of this lane has a
superseded predecessor. Top entry `d-tac-wjq` holds 2.581 at every factor.

## 3. Open-loops lane

`kind(plan,activity,directive,gap,question):active:not(intent(guiding)):rank(coldness(exp-30d)):n(8)`

Identical ordering at every factor, verified by checksumming the ordered output.
This is by construction: coldness divides by *undamped* in-degree, so the hop
factor cannot reach it. Resolution still moves this lane relative to
pre-change behaviour; the factor knob does not.

## Verdict on the factor

The sweep validates inheriting at all — 0.0 versus anything else is a five-rank
swing on the affected entry. It does **not** discriminate 0.8 from 1.0.

On this graph the factor has essentially one entry to act on, so the evidence
cannot separate conservative damping from none. 0.8 is a defensible prior, not a
measured optimum. The honest status is that the question is premature at this
graph size, and the revisit trigger should be data-driven (materially more
multi-hop supersession) rather than temporal.

## 4. Reach of the change

Of 1777 resolvable inbound refs across the graph:

| hops | refs |
|------|------|
| 0    | 1632 |
| 1    | 103  |
| 2    | 40   |
| 3    | 1    |
| 4    | 1    |

25 entries gain inherited attention. Top gainers by inherited ref count:
`d-tac-3vz` (15), `d-cpt-7iy` (14), `d-cpt-dgk` (12), `d-cpt-0cv` (11),
`d-prc-asz` (9), `d-cpt-gm1` (7), `d-cpt-h4l` (7), `d-cpt-owo` (7).

Construction cost: full `sdd view` over 1138 entries in 0.08s wall clock,
stable across three runs — 92% of refs terminate the walk at hop zero.

## 5. In-degree double-count

One entry referencing two members of the same supersede chain contributes twice
to the head after resolution, where literal keying counted it once against each
of two distinct targets. All live instances:

| referencing entry | resolved head | counted |
|-------------------|---------------|---------|
| `s-cpt-tpb`       | `d-cpt-0cv`   | 2x      |
| `s-cpt-n95`       | `d-tac-zyd`   | 2x      |
| `s-cpt-zl4`       | `d-tac-zyd`   | 2x      |
| `s-tac-ohl`       | `s-tac-uo2`   | 2x      |

All four are signals describing a supersession — the shape recurs because a gap
that says "X was replaced by Y" naturally references both. Concretely:
`d-cpt-0cv` reads in-degree 13 while only 12 distinct entries reference its
chain.

Belongs to d-tac-cad, which owns weighted in-degree and must state whether
resolution deduplicates by source entry.

## 6. Supersede fork census

One fork in the live graph: `s-prc-ggp`, superseded by both `s-prc-9yt` and
`s-tac-xnm`. Both branches are closed, so it is a settled fork and `sdd lint` is
correctly silent. Only two files reference `ggp`.

`ResolveRef` follows the first superseder, ordered by the loader's file walk —
so which branch inherits is incidental rather than chosen. No observable
misbehaviour today; the finding is that the tie-break is unspecified.
