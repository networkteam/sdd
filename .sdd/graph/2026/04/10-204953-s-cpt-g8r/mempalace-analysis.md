# MemPalace Analysis — What SDD Can Learn

## What MemPalace Is

MemPalace (v3.1.0, MIT, Python) is an open-source AI memory system. It stores conversation history and project artifacts in ChromaDB (local, no cloud), organized via a spatial metaphor: wings (people/projects) → rooms (topics) → closets (summaries) → drawers (verbatim originals). Halls connect rooms within a wing; tunnels connect the same room across wings.

Repository: github.com/milla-jovovich/mempalace

## Core Approach

- **Store everything, then make it findable.** Raw verbatim storage in ChromaDB without summarization. 96.6% recall on LongMemEval benchmark in raw mode, zero API calls.
- **Mine conversations after the fact.** `mempalace mine` processes conversation exports (Claude, ChatGPT, Slack) and project files. Three modes: projects (code/docs), convos (conversation exports), general (auto-classifies into decisions, preferences, milestones, problems, emotional context).
- **Spatial navigation for retrieval.** Wing+room filtering improves retrieval by +34% over unfiltered search. Structure is the product — not the storage, but the organization.
- **Memory stack with tiered loading.** L0 (identity, ~50 tokens) + L1 (critical facts, ~120 tokens) always loaded. L2 (room recall) and L3 (deep search) on demand. AI wakes up with ~170 tokens of context.
- **Knowledge graph.** Temporal entity-relationship triples in SQLite (like Zep's Graphiti but local). Facts have validity windows — can be invalidated when no longer true.
- **AAAK dialect (experimental).** Lossy abbreviation system for token compression. Currently regresses recall vs raw mode (84.2% vs 96.6%). Honest about limitations.
- **Auto-save hooks.** Save every 15 messages, emergency save before context compression.
- **Specialist agents.** Agents with focused domains keep their own diaries in the palace.

## Overlapping Observations

MemPalace's problem statement is nearly identical to SDD's founding signals:

> "Every conversation you have with an AI — every decision, every debugging session, every architecture debate — disappears when the session ends."

Both systems value immutability (SDD's graph entries never change; MemPalace stores raw verbatim content). Both use typed structure for navigation (SDD: type+layer; MemPalace: wings+halls+rooms). Both track decisions as first-class concepts (MemPalace's `hall_facts` = "decisions made, choices locked in"). Both have temporal semantics (SDD: supersedes/closes; MemPalace: validity windows on knowledge graph triples).

## Key Differences

| Dimension | MemPalace | SDD |
|-----------|-----------|-----|
| Core question | "How do I find what I knew?" | "How do I make and track decisions well?" |
| Primary mechanism | Mine → index → search (passive) | Signal → Dialogue → Decision → Action (active) |
| Capture timing | After the fact (conversation mining) | In the moment (deliberate dialogue) |
| Structure purpose | Retrieval optimization | Reasoning traceability |
| Entry semantics | Content in drawers, classified by type | Typed entries (s/d/a) with refs, closes, supersedes |
| Commitment tracking | None — memories are facts, not commitments | Decisions have confidence, are closed by actions |
| Process layer | None — no workflow guidance | Modes of working, playbooks, evaluation patterns |
| Dialogue role | Not part of the system | Core to the system — dialogue before capture |

## What's Interesting for SDD

1. **Mining as passive signal source.** MemPalace's `mempalace mine` processes conversations that already happened. For SDD, mining session transcripts could produce low-confidence automatic signals — observations the active process missed. Not replacing deliberate capture, but catching what falls through.

2. **Tiered context loading.** As the SDD graph grows, always loading everything becomes impractical. MemPalace's L0-L3 stack suggests a model: contracts and active high-confidence decisions always loaded, older signals on demand.

3. **Auto-save hooks as prompts.** Their "save every N messages" pattern translates to "prompt for evaluation every N messages" in SDD — a structural nudge to surface signals during long implementation sessions.

4. **Cross-project linking (tunnels).** Same topic appearing in different wings creates automatic cross-references. Relevant for multi-project SDD use.

## What MemPalace Lacks That SDD Provides

- No reasoning structure (refs, closes, supersedes chains)
- No commitment semantics (confidence levels, decision kinds, contracts)
- No evaluation pattern (decision → action → evaluation signals)
- No process guidance (playbooks, modes of working, dialogue scaffolding)
- No participant tracking
- No immutability guarantee on the graph structure (entries can be deleted)
