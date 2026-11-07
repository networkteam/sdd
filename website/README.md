# SDD documentation site

This directory contains the Astro Starlight site. Its content tree is intentionally minimal until the documentation information architecture is settled.

Use the repository's Devbox environment for all development commands:

```sh
devbox run docs:dev
devbox run docs:build
```

The build emits the static site to `website/dist/`. The GitHub Pages workflow publishes it at `https://networkteam.github.io/sdd/`.

## Entry kind cards

[`src/content/docs/reference/entry-kinds.md`](src/content/docs/reference/entry-kinds.md) has one card per entry kind. A card restates for people what the shipped base facts tell an agent drafting an entry. Cards are written, not generated, and every card element traces to a source:

| Card element | Source |
| --- | --- |
| Header line | Type, and the kind's question verbatim from `kindQuestions` in `internal/basefacts/overview.go` |
| Description | The kind fact's opening paragraph, plus any section that changes which kind a person picks or what they can expect of it |
| Choose it over | Only pair tests stated in the type overview, the discrimination fact or the kind fact. Each bullet gives the condition under which this card's kind wins, then the contrast |
| How it ends | The kind fact's retirement section, checked against "Retirement" in the type overview and against `validateCloses` in `internal/model/graph.go` |
| Example | Written fresh: one generic line showing the kind's defining property. No example-world project, no dialogue transcript |

The facts live in `internal/basefacts/templates/` and render with `sdd show <id>`:

| Fact | ID | Template |
| --- | --- | --- |
| Type overview | `20260812-180000-s-prc-typ` | `overview.md` |
| Discrimination | `20260818-120000-s-prc-dsc` | `discrimination.md` |
| Gap | `20260815-100000-s-prc-gpk` | `gap.md` |
| Fact | `20260816-110000-s-prc-kno` | `fact.md` |
| Question | `20260817-100000-s-prc-qry` | `question.md` |
| Insight | `20260816-100000-s-prc-syn` | `insight.md` |
| Done | `20260812-170000-s-prc-dnk` | `done.md` |
| Actor | `20260818-110000-s-prc-act` | `actor.md` |
| Annotation | `20260818-110500-s-prc-ann` | `annotation.md` |
| Directive | `20260815-110000-s-prc-drk` | `directive.md` |
| Activity | `20260818-110200-s-prc-dsp` | `activity.md` |
| Plan | `20260818-100000-s-prc-spc` | `plan.md` |
| Aspiration | `20260818-110400-s-prc-asp` | `aspiration.md` |
| Focus | `20260818-110300-s-prc-foc` | `focus.md` |
| Role | `20260818-110100-s-prc-rol` | `role.md` |
| Procedure | `20260813-170000-s-prc-prd` | `procedure.md` |

Contract has no kind fact; its card follows the type overview's note that the kind takes no new entries.

Leave out what only the drafting agent needs: how to word or ground the body, where figures and evidence go, topic and layer craft, and the facts' Mechanics sections. Where a fact and the code disagree, the card states what the code allows, and the disagreement is raised against the fact.

To check for drift, diff the facts since the cards last changed, `git log -p <card-commit>.. -- internal/basefacts/templates/ internal/basefacts/overview.go`, and reread each affected card element against its source above.

## LLM tools compatibility

`@wave-rf/starlight-llm-tools` 0.3.1 omits Astro's `base` path from absolute links in its generated Markdown and LLM manifests and from the Copy Markdown fetch URL. The package's `postbuild` step runs `scripts/finalize-build.mjs` to correct and verify those generated links. Remove the compatibility rewrite once the plugin handles non-root deployments itself.

## Reviewing pages with annotations

`astro dev` adds a review layer to every page for commenting on passages. The production build contains none of it. Comments are stored in `website/.annotations.json`, which git ignores.

Start the dev server with `devbox run docs:dev` and open a page.

- **Add.** Select text in the page content and click **Comment**. Save with the button or Cmd/Ctrl+Enter.
- **Find.** Commented passages are highlighted. Click a highlight, or the **Comments** button at the bottom right, to open the list. **This page** lists the current page's comments; **All pages** lists every page's, with a link to each page.
- **Edit, resolve, delete.** Each comment in the list has a button for each. Resolved comments stay hidden until you check **Show resolved**. Uncheck **Highlights** to read the page without them.
- **Orphaned comments.** A comment stores its quote, the text just before and after it, and the nearest heading. On load the page searches for the quote again. When the quote is gone, the list marks the comment orphaned and keeps its original quote, and the **Comments** button shows a red dot. Click **Re-attach**, select the new passage and click **Attach here**, or delete the comment.

Agents read and update comments without a browser:

```sh
node website/scripts/annotations.mjs list                    # open comments
node website/scripts/annotations.mjs list --all --json       # resolved ones too, as JSON
node website/scripts/annotations.mjs list --page getting-started
node website/scripts/annotations.mjs resolve <id>
node website/scripts/annotations.mjs reopen <id>
node website/scripts/annotations.mjs delete <id>
```

`list` prints each comment's id, status, anchor state, page with its source file, nearest heading, quote and comment. It checks anchors against the page as the running dev server renders it, found through `website/.astro/dev.json` or given with `--server <url>`. Without a running dev server the anchor state is `unchecked`. Resolve a comment once it is addressed rather than deleting it, so the reviewer sees it marked resolved.

The code is in `integrations/annotations/` and `scripts/annotations.mjs`.
