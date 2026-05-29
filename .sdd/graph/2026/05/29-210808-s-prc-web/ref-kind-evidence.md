# Ref-kind usage evidence — this repo's graph

Snapshot parsed directly from entry frontmatter. `sdd view` cannot yet surface ref kinds — `expand(refs)` (d-tac-yls) and a `refs-of(...)` filter (d-tac-jqq) are unshipped — so this was counted from `.sdd/graph` entry files (object-form refs only, excluding attachment YAML).

## Distribution (object-form refs, entry files only)

| kind | count |
|---|---|
| grounds | 20 |
| addresses | 14 |
| builds-on | 13 |
| related | 12 |
| depends-on | 2 |
| refines | 1 |
| evidence | 1 |
| surfaces | 0 |

Plus 13 legacy bare-string refs (mapped to `unknown`). 28 entries carry object-form refs.

## The 12 `related` refs, judged against their bodies

| # | entry | target | desc? | verdict | sharper kind |
|---|---|---|---|---|---|
| 1 | s-cpt-z7l | d-stg-6za | yes | defensible-but-sharper | grounds |
| 2 | s-tac-uer | d-prc-vlu | no | defensible-but-sharper | evidence (cited as example) |
| 3 | s-cpt-sxn | s-prc-6ll | no | defensible-but-sharper | builds-on |
| 4 | s-cpt-sxn | s-cpt-psu | no | correct sibling | — |
| 5 | s-prc-qj6 | s-prc-uoo | yes | correct sibling | — |
| 6 | s-prc-qj6 | s-prc-yg0 | yes | defensible-but-sharper | builds-on |
| 7 | s-cpt-ghy | s-cpt-sy4 | no | defensible-but-sharper | builds-on |
| 8 | d-prc-rl7 | d-prc-vlu | yes | defensible-but-sharper | depends-on / builds-on |
| 9 | s-prc-7hw | s-prc-5ms | yes | correct sibling | — |
| 10 | s-prc-5ms | s-prc-p6q | no | correct sibling | — |
| 11 | s-prc-5ms | s-prc-fc0 | no | defensible-but-sharper | depends-on ("belongs in") |
| 12 | s-cpt-psu | s-prc-6ll | no | correct sibling | — |

**Totals:** 5 correct siblings · 7 defensible-but-sharper · 0 contradiction. 7 of 12 carry no `desc`.

## Reading

- The pre-flight medium band fires on the 7 defensible-but-sharper cases; none are real errors. Demoting that band to low removes the friction with no lost catch in this sample.
- 7 of 12 are bare `related` with no `desc` — zero relational information conveyed, the "says nothing" case.
- `surfaces` is unused across the entire graph — worth a separate look at whether the kind earns its place.
