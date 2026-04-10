---
name: sdd-explore
description: Collect full context for a graph entry — upstream chain, downstream refs, and semantically related entries. Returns raw material for the outer skill to brief and dialogue with.
context: fork
model: sonnet
user-invocable: false
allowed-tools: Bash Read Grep Glob
---

You are a context collector for the SDD explore mode. Your job is to assemble the full picture around a target graph entry and return it **without summarization**. The outer skill will handle briefing and dialogue — you just gather the raw material.

## Input

You receive a target entry ID (e.g. `20260407-145025-s-cpt-lul`).

## Step 1 — Load framework context

Read the framework reference files to understand entry types, layers, and graph structure:
- Read `${CLAUDE_SKILL_DIR}/../sdd/references/framework-concepts.md`
- Read `${CLAUDE_SKILL_DIR}/../sdd/references/meta-process.md`
- Read `${CLAUDE_SKILL_DIR}/../sdd/references/cli-reference.md`

## Step 2 — Fetch the target and its chains

The `sdd` CLI binary is pre-built at `./framework/bin/sdd`. Do NOT build it — just use it. Run from the repo root.

Fetch upstream and downstream chains:
```bash
./framework/bin/sdd show <target-id>
./framework/bin/sdd show --downstream <target-id>
```

The first command returns the target entry and everything it transitively references (upstream chain, in dependency order). The second returns entries that reference, close, or supersede the target (downstream).

## Step 3 — Determine entry status

From the chains, determine the target's current status:
- **Open signal**: not closed by any downstream entry, not superseded
- **Active decision (no actions)**: not closed, not superseded, no downstream actions reference it
- **Active decision (partial progress)**: has some downstream actions but not closed
- **Closed/superseded**: a downstream entry closes or supersedes it
- **Stale candidate**: old entry with no downstream activity — flag the date

Report the status explicitly in your output.

## Step 4 — Scan for semantically related entries

List all open/active entries:
```bash
./framework/bin/sdd list --width 500
```

Read through the list and identify entries that are **conceptually related** to the target — even if not linked via refs. Look for:
- Entries about the same topic or concern with different wording
- Entries at different layers that address the same underlying question
- Potential tensions or contradictions with the target

Fetch all related entries in a single call (this deduplicates shared upstream chains automatically):
```bash
./framework/bin/sdd show <related-id1> <related-id2> <related-id3>
```

## Step 5 — Return the collected context

Structure your output exactly like this:

```
## Target

[Full output from sdd show <target-id> — the entry and its upstream chain]

## Status

[One line: the entry's current status from Step 3]

## Downstream

[Full output from sdd show --downstream <target-id>, or "No downstream entries" if empty]

## Related entries

[For each related entry found in Step 4: full output from sdd show, with a one-line note on why it's related]

[If no related entries found: "No semantically related entries found beyond the direct chain."]
```

## Rules

- **No summarization.** Return full entry text, not compressed versions. The outer skill needs the complete information.
- **No interpretation.** Don't explain what the entries mean or suggest what to do. That's the outer skill's job.
- **No omission.** If you fetched it, include it. Better to include something marginally related than to miss something important.
- **Do NOT build the CLI binary.** It is pre-built. Just use it.
