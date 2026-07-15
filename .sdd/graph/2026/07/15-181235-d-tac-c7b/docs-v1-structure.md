# SDD documentation v1 — structure and content-sourcing map (2026-07-15)

Planned in dialogue between Christopher and Claude during the docs structure-planning session (activity 20260715-174037-d-cpt-eke). Stack per directive 20260715-173844-d-cpt-o0r (Astro Starlight + starlight-theme-nova + @wave-rf/starlight-llm-tools, GitHub Pages). Engine-first per directive 20260715-175731-d-cpt-p3k.

## Design principles

1. **Every explanation earns a visualization.** A concepts page that is only prose is not done. Custom MDX components (entry cards, loop/move diagrams, layer ladders) over walls of text. The visual language cannot be specced in prose — it is established by an early style spike: one concepts page fully designed, reviewed by Christopher on the rendered site ("I can tell you when I see it"), then applied as the pattern.
2. **Real mechanics, distinct example world.** Docs examples are never mocked up and never meta. A real `.sdd` graph is built for the coffee roastery from the founding story (fictional-but-concrete product, clearly distinct from SDD itself); every CLI/session example in the docs is genuine output from running `sdd` against that graph. The SDD-building-SDD dogfood graph stays out of the learning path (grounded in insight 20260420-163550-s-cpt-l2q: usage evidence, not produced artifacts, is the credible signal). Rationale: meta examples force readers to untangle the tool from its subject (Christopher's objection, 2026-07-15).
3. **People first, mechanics discoverable.** Communication leads with people, dialogues, and the curation of knowledge; the technical part is learnable step by step below.
4. **v1 shows the current working state only.** Planned/in-discussion features live exclusively in "What's ahead", clearly marked. No page describes unshipped behavior as available.

## Settled calls

- Engine mode (`/sdd-engine`, MCP-served procedures) is documented as *the* way SDD works; `/sdd` at most a legacy note (20260715-175731-d-cpt-p3k).
- `contract` kind omitted from the vocabulary reference (deprecated by the rules plan 20260622-084244-d-tac-eho).
- The story ships as a light edit of `docs/story.md`, explicitly framed as founding vision — the authentic origin artifact; the example world grows out of it.
- "The loop" reframed as "How the graph moves": moves as interleaved mini-loops (dialogue, work, insight, discovery), signal → dialogue → decision as the shared rhythm, not a prescribed single loop. Honest current-vs-planned split on human/agent roles.

## Page tree with sourcing

### 1. The idea
- **What is SDD?** — people + agents staying aligned through dialogue; the graph as curated knowledge. Sources: README positioning (outcome-first tagline, why-SDD contrast), aspirations d-stg-beb / d-stg-qlt / d-stg-3k0 (rationale), docs/signal-dialogue-decision.md.
- **The story** — light edit of docs/story.md, framed as founding vision: roles and people interacting with agents in a loose setting, many tools, minimal ceremony.
- **How the graph moves** — moves as mini-loops; humans and agents as participants; current roles (dialogue partner, capture assistant, human evaluates and decides) vs planned (semi-autonomous specified work, humans evaluating agent output, answering surfaced questions). Sources: base procedure entries (d-prc-*), framework model doc, d-stg-7lu / d-stg-2wb for the planned side.

### 2. Quickstart
- install → `sdd init` → register the engine (`claude mcp add sdd -- sdd serve` / Codex equivalent) → first session → first capture → what just happened. Executable end-to-end by a newcomer on a fresh project; all output real, roastery world. Sources: install.sh, README quickstart, engine skill pointer.

### 3. Concepts (each page visualized per the approved style)
- **Entries: signals & decisions** — two types, immutability, why append-only. Sources: type-system contract d-cpt-7iy, docs/signal-dialogue-decision.md.
- **Kinds** — what you can notice vs what you can commit to (contract omitted). Source: d-cpt-7iy enumeration.
- **Layers** — strategic → process as thinking depth.
- **References & topics** — ref-kind taxonomy, topic clustering, heat.
- **Sessions & moves** — dialogue sessions, the base moves, junctions, parking. Sources: the nine d-prc-* procedure entries.
- Vocabulary terms link into the Reference tables throughout (learnability requirement).

### 4. Guides (task-shaped, engine-first)
- **Working a session** — check-in/catch-up, capturing, engaging an entry.
- **Evaluating and closing work** — done signals, inner/outer lenses (d-prc-evl).
- **Keeping the graph clean** — grooming (d-prc-grm).
- **Setting up your agent** — Claude Code + Codex, MCP registration, skills install via `sdd init`.
- **Connecting repositories** — cross-repo refs end to end; states the pushed-state cache model plainly (a connected-repo cache is a clone of pushed state — entries resolve only after commit + push). Absorbs gap 20260708-170127-s-tac-ume. Sources: README cross-repo section (from d-tac-rsf batch 2), d-cpt-s6q surfaces.

### 5. Reference
- **Vocabulary** — tables: types, signal kinds, decision kinds (minus contract), layers, ref kinds. Generated/checked against the current binary.
- **CLI commands** — checked against the current command surface.
- **Configuration** — .sdd/config.yaml, global overlay, `sdd config`.

### 6. Customizing the process (advanced track)
- **Procedures** — base procedures as shipped process; graph-local project procedures as the customization path (d-cpt-70z boundary: base host-neutral, project procedures free). Current capability only.
- Points forward to rules (What's ahead) as the upcoming constraint layer.

### 7. What's ahead (outlook — everything explicitly marked planned / in discussion)
- **Anchor page: the knowledge flow** — knowledge goes in, gets used, gets enhanced, gets shaped, goes out (reports, tickets, emails, chat). Inbound: intake model (d-cpt-fbi, decided) + living sources question (s-cpt-jnr). Outbound: emit-and-mirror gap (s-cpt-yve, open). Narrative page, not a feature list.
- Rules v1 (d-tac-eho) — project-grown process constraints.
- Base facts (d-cpt-dtv, in flight).
- Hosted webapp & non-developer access (d-tac-hza, d-stg-x0l).
- Agent automation / semi-autonomous work (d-stg-7lu, d-stg-2wb).

## Sequencing notes

- Structure plan hands the parallel site-stack implementation session its content spec.
- Visual style spike gates the bulk of Concepts/Guides authoring.
- Roastery graph build precedes example-heavy pages (Quickstart, Guides).
- Iterate with user feedback (the original feedback loop that raised gap s-cpt-i5a) before announcing.
