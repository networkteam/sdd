# Making Obsidian links from Markdown-based decision graph

We have a decision graph in our Git based on Markdown.
This is our structure:

```
.
├── 2026
│ └── 04
│ ├── 06-115516-s-stg-beh.md
│ ├── 06-115521-s-stg-9pd.md
│ ├── 06-115528-s-stg-8d3.md
│ ├── 06-115534-s-stg-cj7.md
│ ├── 06-115540-d-stg-0gh.md
│ ├── 06-115547-d-cpt-o8t.md
│ ├── 06-115559-a-cpt-f8v.md
│ ├── 06-185742-d-tac-0g7.md
│ ├── 06-185748-a-tac-xh9.md
│ ├── 06-185801-s-tac-frb.md
│ ├── 06-190041-d-tac-8yf.md
│ ├── 06-190046-a-tac-txi.md
```

We want to make this browsable in Obsidian.
How could we make refs, closes etc. used as correct links?

File content:

```
---
type: action
layer: conceptual
refs:
  - 20260410-110416-s-cpt-ukp
  - 20260410-110413-s-prc-dk3
closes:
  - 20260410-110416-s-cpt-ukp
participants:
  - Christopher Hlubek
  - Claude
confidence: medium
summary: Evaluated branching and concurrent work mechanics for SDD through design dialogue, producing a draft plan covering worktree-based branching, WIP marker coordination, and branch lifecycle management. Closes the identified branching model gaps (s-cpt-ukp) and addresses the synchronization friction challenges (s-prc-dk3) by proposing coordination mechanisms for multi-participant exploration.
summary_hash: f5e289368ba319c2ddf30bd6db94a9cb
---


Evaluated branching and concurrent work mechanics for SDD through design dialogue. Produced a draft plan covering worktree-based branching, WIP marker coordination on main, branch lifecycle (start, work, conclude/discard), and end-of-branch playbook moves. See draft plan for full details.
```

Yes — the cleanest way is to make every ID-like reference resolve to an actual Obsidian note path, and then render those references as normal internal links such as `[[2026/04/10-110416-s-cpt-ukp]]` or `[20260410-110416-s-cpt-ukp](../04/10-110416-s-cpt-ukp.md)`. Obsidian natively supports both wikilinks and Markdown links, including links to headings and blocks, and it updates internal links when files are renamed if that setting is enabled. [forum.obsidian](https://forum.obsidian.md/t/automatic-link-display-aliases-for-headings/20667)

## Core idea

Right now your `refs` and `closes` values are plain strings in frontmatter, so Obsidian treats them as text, not links. Dataview can read frontmatter fields and exposes note metadata such as `file.link`, `file.path`, `file.inlinks`, and `file.outlinks`, which makes it a good fit for turning those IDs into browsable relationships. [forum.obsidian](https://forum.obsidian.md/t/wikilinks-auto-complete-somewhat-prevents-the-creation-of-links-to-headings-alias/64408)

The missing piece is a stable mapping from your canonical ID `20260410-110416-s-cpt-ukp` to the actual file path `2026/04/10-110416-s-cpt-ukp.md`. Once you can derive that path consistently, you can either store true links in frontmatter or generate link views from the IDs. [forum.obsidian](https://forum.obsidian.md/t/wikilinks-auto-complete-somewhat-prevents-the-creation-of-links-to-headings-alias/64408)

## Best options

| Approach                                            | How it works                                                         | Pros                                                                                                                                                                                                      | Cons                                                                          |
| --------------------------------------------------- | -------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| Store wikilinks in frontmatter                      | `refs: - "[[2026/04/10-110416-s-cpt-ukp]]"`                          | Clickable in Obsidian link system; rename support is best with internal links. [forum.obsidian](https://forum.obsidian.md/t/automatic-link-display-aliases-for-headings/20667)                            | YAML becomes less tool-agnostic; generation step needed if IDs are canonical. |
| Store Markdown links in frontmatter                 | `refs: - "[label](../04/10-110416-s-cpt-ukp.md)"`                    | More portable outside Obsidian because Obsidian supports Markdown internal links too. [forum.obsidian](https://forum.obsidian.md/t/automatic-link-display-aliases-for-headings/20667)                     | Relative-path maintenance can get messy unless automated.                     |
| Keep IDs in frontmatter, render links with Dataview | Leave `refs` as IDs; create computed tables/lists using lookup logic | Keeps source metadata simple and Git-friendly; easy migration path. [forum.obsidian](https://forum.obsidian.md/t/wikilinks-auto-complete-somewhat-prevents-the-creation-of-links-to-headings-alias/64408) | Raw frontmatter stays non-clickable unless you add a plugin or derived field. |
| Add generated link fields beside IDs                | Keep `refs` and add `ref_links` with actual links                    | Best of both worlds; raw canonical IDs plus browsable links.                                                                                                                                              | Slight duplication.                                                           |

## Recommended structure

I’d recommend keeping your canonical IDs, but adding one explicit field per note like `id: 20260406-115559-a-cpt-f8v`, then generating links from that ID instead of parsing filenames ad hoc. Dataview works well with frontmatter and implicit file metadata, so having both `id` and `file.path` available makes cross-link rendering much easier. [forum.obsidian](https://forum.obsidian.md/t/wikilinks-auto-complete-somewhat-prevents-the-creation-of-links-to-headings-alias/64408)

For example:

```yaml
---
id: 20260406-115559-a-cpt-f8v
type: action
layer: conceptual
refs:
  - 20260410-110416-s-cpt-ukp
  - 20260410-110413-s-prc-dk3
closes:
  - 20260410-110416-s-cpt-ukp
---
```

Then use the filename strictly as a storage detail, while `id` is the graph key. That avoids coupling every consumer to path logic. [forum.obsidian](https://forum.obsidian.md/t/wikilinks-auto-complete-somewhat-prevents-the-creation-of-links-to-headings-alias/64408)

## Make them clickable

If you want the fields themselves to be true Obsidian links, use actual internal-link syntax. Obsidian supports wikilinks like `[[Note]]` and Markdown internal links like `[Text](Note.md)`, and it also supports heading targets like `[[Note#Heading]]` plus custom display text like `[[Note#Heading|Section name]]`. [forum.obsidian](https://forum.obsidian.md/t/automatic-link-display-aliases-for-headings/20667)

So your generated frontmatter could look like:

```yaml
---
id: 20260406-115559-a-cpt-f8v
refs:
  - "[[2026/04/10-110416-s-cpt-ukp|20260410-110416-s-cpt-ukp]]"
  - "[[2026/04/10-110413-s-prc-dk3|20260410-110413-s-prc-dk3]]"
closes:
  - "[[2026/04/10-110416-s-cpt-ukp|20260410-110416-s-cpt-ukp]]"
---
```

If you want references to specific sections such as a decision summary or rationale heading, use heading links like `[[2026/04/10-110416-s-cpt-ukp#Summary]]`. Obsidian also supports block links with `#^block-id` when you want a reference to a precise paragraph or list item. [forum.obsidian](https://forum.obsidian.md/t/automatic-link-display-aliases-for-headings/20667)

## Better with Dataview

If you prefer not to put Obsidian link markup into YAML, you can keep plain IDs and build browsable sections in the body of each note using Dataview. Dataview can query frontmatter and file metadata, including `file.link`, so it is well suited to resolving IDs into clickable note links. [forum.obsidian](https://forum.obsidian.md/t/wikilinks-auto-complete-somewhat-prevents-the-creation-of-links-to-headings-alias/64408)

A practical pattern is:

1. Add `id` to every note.
2. Keep `refs` and `closes` as plain IDs.
3. Add a generated section in the note body that resolves them.

Example DataviewJS idea:

```dataviewjs
const refs = dv.current().refs ?? [];
const pages = dv.pages().where(p => p.id);

dv.header(3, "Refs");
for (const id of refs) {
  const match = pages.where(p => p.id === id)[0];
  dv.paragraph(match ? match.file.link : `Missing: ${id}`);
}
```

This gives you clickable browsing without changing your canonical metadata model. Dataview’s page metadata model is what makes this possible. [forum.obsidian](https://forum.obsidian.md/t/wikilinks-auto-complete-somewhat-prevents-the-creation-of-links-to-headings-alias/64408)

## For `closes` semantics

If `closes` means “this note resolves that note,” render it as a dedicated outbound relation section and also create reverse lookup views like “closed by.” Dataview exposes incoming and outgoing links via implicit metadata such as `file.inlinks` and `file.outlinks`, but for semantic relations like `closes`, explicit frontmatter fields are still more precise than generic links. [forum.obsidian](https://forum.obsidian.md/t/wikilinks-auto-complete-somewhat-prevents-the-creation-of-links-to-headings-alias/64408)

A good convention is:

- `refs`: related or supporting notes
- `closes`: notes resolved by this note
- `closed_by`: do not store manually; derive it by querying notes whose `closes` contains the current `id`

That keeps the graph directional and avoids duplicated maintenance. [forum.obsidian](https://forum.obsidian.md/t/wikilinks-auto-complete-somewhat-prevents-the-creation-of-links-to-headings-alias/64408)

## Path derivation rule

Given your structure, the ID appears to encode `YYYYMMDD-HHMMSS-...`, while the file path uses `YYYY/MM/DD-HHMMSS-...md`. If that rule is stable, a small generator can derive the Obsidian path automatically and produce links like `[[2026/04/06-115559-a-cpt-f8v]]` from the canonical ID. Obsidian supports linking by note path or filename, so this is valid as long as names stay unique enough in the vault. [forum.obsidian](https://forum.obsidian.md/t/automatic-link-display-aliases-for-headings/20667)

Example transform:

- ID: `20260410-110416-s-cpt-ukp`
- Path: `2026/04/10-110416-s-cpt-ukp.md`
- Wikilink target: `[[2026/04/10-110416-s-cpt-ukp]]` or `[[10-110416-s-cpt-ukp]]` if filenames are unique in the vault. [forum.obsidian](https://forum.obsidian.md/t/automatic-link-display-aliases-for-headings/20667)

## Practical recommendation

The most robust setup is:

- Keep `id`, `refs`, and `closes` as canonical IDs in frontmatter.
- Add a small sync script in your Git workflow that validates IDs against existing files and optionally writes derived `ref_links` / `close_links`.
- In Obsidian, browse through Dataview-generated sections and normal internal links, not raw YAML strings. Dataview is specifically built around frontmatter and file metadata, which fits your repository model well. [forum.obsidian](https://forum.obsidian.md/t/wikilinks-auto-complete-somewhat-prevents-the-creation-of-links-to-headings-alias/64408)

If you want, I can draft:

1. a Dataview query set for `refs`, `closes`, `closed_by`, and backlinks, or
2. a small script that converts your IDs into Obsidian wikilinks automatically.
