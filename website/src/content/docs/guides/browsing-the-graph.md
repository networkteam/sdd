---
title: Browsing the graph
description: Read the decision graph yourself, from the entry files to sdd show, sdd search, sdd view and sdd info.
---

Your agent reads the graph for you in every session. You can read it yourself too: to check what the agent works from, to look something up, or to get an overview before a dialogue. Every command on this page only reads.

## Entries are markdown files

Each entry is one markdown file under `.sdd/graph/`, filed by the date it was recorded:

```text
.sdd/graph/2026/11/07-123911-d-tac-ims.md
```

This file holds the entry `20261107-123911-d-tac-ims`. The ID reads as date and time, type, layer and a random suffix:

- type: `s` for a signal, something noticed, or `d` for a decision, something committed to
- layer: `stg` strategic, `cpt` conceptual, `tac` tactical, `ops` operational, `prc` process

The file starts with YAML frontmatter: type, kind, layer, confidence, references to other entries, participants, topics and the one-line summary. The entry text follows. An entry with attachments has a directory of the same name next to its file. `.sdd/graph/wip/` holds the markers for work in progress.

You can read the files in an editor or on your git host. Do not edit them. An entry never changes after it is written; a later entry closes or supersedes it. The file also does not say whether the entry is still open. For that, use `sdd show`.

## Read one entry

```bash
sdd show 20261107-123911-d-tac-ims
sdd show d-tac-ims
```

The short form drops the date and time. When it matches more than one entry, sdd lists the candidates. You can pass several IDs at once.

`sdd show` prints the entry's frontmatter with its current `status`, the entry text, and two lists:

- `upstream`: the entries it references, and the entries those reference, two levels deep
- `downstream`: the entries that reference it, one level deep

Each line in those lists gives the reference kind, the other entry's ID, its kind and status, and its summary. The line below, marked `↳`, says why the two entries are linked. To widen or narrow the lists:

```bash
sdd show d-tac-ims --up 4 --down 3
sdd show d-tac-ims --up 0 --down 0   # the entry alone
```

In a terminal the output is styled; piped to another program, it is plain markdown. `--format text` forces plain markdown.

## Search

`sdd search` finds entries by keyword, by meaning, or both:

```bash
sdd search --term importer                         # keyword
sdd search --query "how imports handle variants"   # by meaning
sdd search --term importer --query "variants"      # both
```

- `--term` is a regular expression, matched without regard to case. Repeat it to require every term. It works without any setup.
- `--query` searches by meaning and needs an embedding provider, set up as the [configuration reference](/sdd/reference/configuration/#embedding) describes. In a project that already has entries, run `sdd index` once. After that, each search adds new entries to the index.
- With both, sdd merges the keyword and the meaning ranking into one list.

Under each entry line, sdd shows the best matching passage, where it sits, such as the summary, a section or an attachment, and its score relative to the top hit. `--limit` changes the number of hits, 10 by default. `--type`, `--layer` and `--kind` filter them. Superseded entries are left out unless you add `--include-superseded`.

## Overviews with layouts

`sdd view` renders lists of entries from a layout. Several layouts rank by heat: each reference an entry receives adds to its heat, and references from recent entries add more. These layouts are a good start:

| Layout | Shows |
| --- | --- |
| `top(20)` | the 20 active entries with the most heat |
| `focus` | the active focus decisions, with the entries each covers and their state |
| `signals` | the open gaps and questions, grouped by kind |
| `topic("product/vision")` | one topic's entries, ranked by heat |
| `since("7d"):rank(by(date)):as-list` | everything recorded in the last seven days, newest first |

```bash
sdd view --layout='top(20)'
sdd view --layout='focus,signals'
sdd view --layout='topic("product/vision"):brief'
```

A comma starts another section, so `focus,signals` prints both lists. `brief` shortens each line to the ID, kind and first summary sentence. Quote topic paths, dates and names inside the layout, and wrap the whole layout in single quotes for the shell. `sdd view --help` lists every filter, ranking and output form, with examples.

A full line reads: ID, layer, kind and type, confidence, participants, status, score, topics, summary.

## Check your setup

```bash
sdd info
```

```text
Local participant: Dana
Search: vector,text
```

`sdd info` shows your participant name, the graph language when the project sets one, and which searches work. `text` alone means keyword search only; `vector,text` means search by meaning works too.

## What you are looking at

**Status is derived.** No file records whether an entry is open, active, closed or superseded. sdd works it out from the links: an entry is closed once a later entry closes it, and superseded once a later entry replaces it. That is why `sdd show` reports a status, such as `closed-by 20260708-213621-s-tac-cjr`, that the file does not contain.

**Summaries are pointers.** A model writes the one-line summary when the entry is recorded. It tells you whether an entry is worth reading, and it can miss a condition or a limit the entry states. Read the full entry with `sdd show` before you rely on it.

**References carry the reasoning.** Follow upstream to see what an entry rests on, and downstream to see what came of it.

To check the graph for problems such as references to entries that do not exist, run `sdd lint`. To read the graphs your project depends on as well, add `--repo` or `--all-repos` to `sdd search` and `sdd view`; see [Connected repositories](/sdd/guides/connected-repositories/).
