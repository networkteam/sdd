# Store-format compatibility analysis — monotonic binaries vs directory isolation

Context: slice 2 of the persistent index repair introduces a multi-version manifest and version-qualified chunk IDs into the machine-global store. Older shipped binaries (v0.16.x) write single-version semantics into the same store dirs. This note records the analysis behind the decision to evolve the format in place under a monotonicity assumption.

## The assumption and its one crack

Stores are keyed per (repo, embedder fingerprint) and are per-machine. Per-repo binary assignment is naturally disjoint (dev binary in the developed repo, release binary elsewhere) — so each store sees one binary lineage, EXCEPT via connected repos: a project running an older binary with repo X connected writes into X's store dir on cross-repo search/serve. That sharing is deliberate (d-cpt-6cq dedup: checkout, worktrees, and connected caches share one store per repo). Verified live: a clone cache of sdd exists on this machine, so a release-binary project connects sdd today.

## What actually happens in a mixed window

Old binary touching a store with multi-version entries:
- Manifest load: a {"versions":[...]} entry parses as zero-value flat EntryState (Go json ignores unknown fields) → hash mismatch → re-embed → UpsertEntry with an EMPTY old-chunk list → deletes nothing. Version rows survive as rows.
- Manifest save: rewrites the whole file from the loaded map → version bookkeeping is dropped; the new binary later re-embeds affected entries (churn, self-healing).
- Reads: the old binary's nearest-neighbor query is version-blind (built on the pre-slice-2 invariant "everything in the store is current") → it can serve another branch's version as citations during the window.

Net: silent churn + possibly stale citations, machine-local, bounded by the window, self-healing after convergence. No corruption, no crashes.

## Why bidirectional on-disk compatibility was rejected

- Any encoding the old binary CAN see feeds its delete-and-replace path: listed chunk IDs of another world's version get deleted on its next re-embed.
- Any encoding it CANNOT see gets erased by its wholesale manifest rewrite (unknown fields dropped on load, whole map re-serialized on save), orphaning the hidden rows.
- Escape constructions (second manifest file, chunk-ID namespace ownership, separate chromem collection) each reconstruct "two stores" with shared mutable files and a page of standing invariants — and save no disk, because both worlds hold their own rows for the same entry regardless.
- The old binary's version-blind READ path is unfixable in shipped binaries: sharing one collection means old readers surface other versions' rows.

## The escape hatch: format-versioned store directories with copy-seeding

If a future format change cannot promise a short mixing window:
- StoreDir gains a format component (e.g. <fp-hash> → <fp-hash>-f3): old binaries own the old dir, new binaries the new one — isolation enforced structurally.
- Seed the new dir by CONVERTING A COPY of the prior store (move-if-absent pattern like the existing MigrateDir, but copy so old binaries keep their warm store) → zero re-embedding; the cost is transient disk duplication until old binaries retire.
- Precedent already in the codebase: fingerprint keying ("a changed embedder starts a fresh store instead of drifting inside an existing one") and MigrateDir.

## Decision

Land slice 2 in place, no directory isolation. Ship format-affecting changes in a release promptly after merging and update the installed binary. Reassess (dir-versioning) if a future format change can't keep the window short.
