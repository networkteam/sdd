# Base procedure entries

Every `.md` file in this directory (except this README) is a base procedure
entry compiled into the sdd binary: a `kind: procedure` decision at the
process layer, named by its full entry ID (`YYYYMMDD-HHmmss-d-prc-suffix.md`,
flat — the hierarchical `YYYY/MM/` layout is a graph-directory concern).

Base procedures are always loaded into every graph and must stay correct
independent of any project's entries:

- **No participants.** Actor canonicals are a project-local namespace; base
  entries are framework artifacts, not dialogue captures.
- **No refs outside this directory.** A base entry may reference other base
  entries; it must never reference project entries.
- **Ship with a summary.** There is no file to regenerate a summary into —
  summaries are authored alongside the entry.

A release ships revisions as successor entries (`supersedes` the prior
head); projects customize by superseding a chain head through normal
capture. The loader (`baseprocedures.load`) rejects non-procedure entries
and files that don't parse — `go test ./internal/baseprocedures/...` catches
a broken entry at build time.
