# SDD CLI Reference

The `sdd` binary is pre-built at `./framework/bin/sdd`. Do NOT build it — just use it. Run from the repo root.

## Commands

- `sdd status` — overview of active decisions, open signals, recent actions
- `sdd show <id>` — full entry with reference chain
- `sdd show <id> --downstream` — entries that reference, close, or supersede the target
- `sdd list [--type d|s|a] [--layer stg|cpt|tac|ops|prc]` — filtered listing
- `sdd list --kind contract` — list active contracts
- `sdd new <type> <layer> [flags] <description>` — create entries
- `sdd lint` — check graph integrity (dangling refs, type mismatches, broken attachment links)
- `sdd wip start <entry-id> --exclusive --participant <name> <description>` — create WIP marker
- `sdd wip start <entry-id> --branch --exclusive --participant <name> <description>` — create WIP marker, create git branch and check out to it
- `sdd wip done <marker-id>` — remove WIP marker (deletes branch if merged)
- `sdd wip done <marker-id> --force` — remove WIP marker and force-delete unmerged branch (discard flow)
- `sdd wip list` — list active WIP markers

## `sdd new` flags

- `--refs id1,id2` — referenced entry IDs (context/foundation)
- `--supersedes id` — entry ID this replaces
- `--closes id1,id2` — entry IDs this resolves/fulfills
- `--participants p1,p2` — participant names
- `--confidence high|medium|low` — confidence level
- `--kind contract|directive` — decision kind (decisions only)
- `--attach spec` — file to attach (repeatable, see below)

**Always use full entry IDs** (e.g. `20260408-104102-d-prc-oka`, not `oka`). The CLI validates that referenced entries exist and rejects short suffixes.

## Attachments

The `--attach` flag is repeatable and supports filename mapping:

- `--attach path/to/file.md` — attach with original filename
- `--attach path/to/file.md:renamed.md` — attach with a custom target filename
- `--attach -:plan.md` — read stdin and save as `plan.md` (at most one `-` per invocation)

Use `{{attachments}}/filename` in the entry description to link to attachments. The CLI resolves these to relative paths. Example:

```
sdd new d tac --attach /tmp/design.md:plan.md "See [plan]({{attachments}}/plan.md) for details."
```
