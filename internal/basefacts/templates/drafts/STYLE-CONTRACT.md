# Style contract for SDD kind authoring facts

Calibrated on the shipped done, gap, directive, and procedure facts and two full review rounds with Christopher. Binding for every new kind fact draft.

## Form (match the exemplars exactly)

- Frontmatter: `type: signal`, `layer: process`, `kind: fact`, `override: closed`, `confidence: high`, topics `engine/base-facts` + `type-system/kinds`, a `summary: >-` block. No `index` block (authoring facts are unindexed — reached from lanes, not the pull index).
- H1: `# <Verb-phrase> — the <kind> kind` — an active phrase for what the kind does ("Recording a deviation", "Deciding how to go forward", "Recording completed work"). Not a noun label.
- Opening paragraph: what the kind records in one plain sentence; that it is a signal (noticed) or decision (committed); the two or three questions its reader arrives with; "a good X is the shortest record that answers them" only if it genuinely fits.
- Body: bold-lead paragraphs, each carrying exactly ONE claim with the reasoning a drafter can argue from — never a field checklist. 6–9 paragraphs total.
- A "Choosing X at all" paragraph near the end: ONLY the neighbor tests the overview does NOT run, each a single question, with the real contenders only (don't list non-contenders). Point at "the type-system introduction" for tests the overview owns.
- Final line before mechanics: `{{ .Mechanics }}` placeholder verbatim (the Go layer renders enforced rules there — do not write mechanics prose yourself).

## Content rules

- **Consolidate, never invent.** Every claim traces to a source in the research brief. A claim you want but can't source goes into a separate "proposed additions" list for user ruling — never blended into the draft.
- **No implementation details.** Serving mechanics, session-shell behavior, always-on lanes, CLI conveniences stay out. The kind default is ruled CLI-surface legacy (20260816-170529-d-cpt-1dk) — never present a default as semantics; never write "resist the default".
- **No absolutes unless truly total.** "always", "never completes", "the one thing" — each survived review only when literally total. Prefer "most often", "a category error", precise scoping.
- **World input over elaboration** (signal kinds): what the entry carries in from the world with its provenance is the substance; derivable statements can be re-reasoned later (20260816-165718-s-cpt-sne).
- **The why over the what** (decision kinds): dialogue material — arguments raised, alternatives weighed and set aside — must be retrievable from the entry; attachments carry bulk, the body summarizes.
- **Audience test:** ships to projects doing no software work. Non-software instantiations (roastery, timber construction, child care) are woven in as short quoted examples where a discriminator needs grounding. No Go names, no repo paths, no entry IDs, no host tools ("sdd", "CLI", "MCP" are all test-enforced forbidden strings), no `{{` except the Mechanics placeholder.
- **One voice:** if a phrase also lives in the overview gloss or a code doc comment, the wording must not contradict them; flag any needed alignment of those surfaces instead of diverging.
- **Point, don't restate:** anything the overview owns (kind questions, kind lists, immutability, the retirement split, layer glosses, cross-kind tests it runs) is referenced as "the type-system introduction", never re-taught.

## Length calibration

gap.md and directive.md are the size target (~900–1100 words of body before mechanics). done.md is the upper bound.
