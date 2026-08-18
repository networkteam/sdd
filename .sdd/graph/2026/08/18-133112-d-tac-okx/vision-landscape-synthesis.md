# Vision landscape — synthesis and source map

Record of the graph exploration and dialogue (2026-08-18) that settled the landscape structure for presenting SDD's vision to new collaborators. Five parallel explorations mapped the graph (strategic themes, collaboration model, world input, current features, extensible process); the structure below was then settled in dialogue with Christopher. Entry IDs are the reading list for whoever writes the material.

## Presentation principles (settled in dialogue)

- **Why first.** Every region opens with "why do we want this?" and names the aspiration(s) it serves, before any mechanics.
- **Maturity marked, never filtered.** Everything belongs on the map; each element carries its state: works today / decided and taking shape / idea in dialogue. Never round "foundation exists" up to "works".
- **Agency examples, never agency limits.** Example content may come from the digital-agency world (a likely first adopter), but the material must never read as agency-only or software-only (d-cpt-r99).
- **Plain language.** No jargon, no decorative metaphors; SDD's own defined terms are fine.
- **Companion narrative:** docs/story.md (Kōgen) stays the narrative entry point newcomers read first.

## The six regions

### 1. A shared context (works today)
The graph as the living picture of the project world that every participant — human or agent — reads from and reasons with. Its properties: it keeps the why (reasoning, alternatives, corrections), it is derived rather than maintained (append-only, supersede instead of edit), and it is honest (open questions and tensions stay visible). "Knowledge graph" is the outside term people will map it to. Connected graphs across repositories/organizations belong here (e.g. your graph and a client's graph).
- Why: reasoning evaporates in chats, meetings, heads; scattered artifacts drift (d-stg-3k0, s-cpt-5hn).
- Key entries: d-stg-3k0 (sole record), s-stg-miu (public one-liner), s-stg-ob9 (conversation as the sieve), s-prc-way (graph as picture of the project), cross-repo: d-cpt-v8t, d-cpt-s6q, s-tac-cr0.

### 2. Thinking in dialogue (works today)
How the picture grows and gets used: you say what you noticed, the agent drafts grounded in what stands, plays it back, you confirm; briefings re-orient you at the edge of the thinking. Evaluation of finished work belongs here (inner = verification, outer = validation with users/world).
- Why: judgment enters through conversation; tooling serves dialogue, never replaces it (d-stg-beb).
- Key entries: d-stg-beb, d-stg-qlt (edge of thinking), s-cpt-p6f (confirm turn is structural), d-prc-cap/d-prc-cat/d-prc-evl (capture, catch-up, evaluate procedures), d-prc-dlg (session shell).

### 3. Everyone working together (mixed maturity)
People participate through the surfaces; each has an agent as dialogue partner; the same kind of agents also work in the background over the graph — reacting to changes, ingesting information. Dialogue is the connecting element: person↔agent, person↔person (SDD prepares the meeting, then sorts captures out of the notes), agents over the graph. Contributions from agents count like a colleague's.
- Why: coherence without meetings — shared direction visible, so anyone decides locally in their domain (d-stg-7lu + d-stg-qlt, a named permanent tension pair).
- Works today: durable parallel sessions with consent rules (d-cpt-9of, s-tac-4bx), actors/roles as entries (s-cpt-w80), WIP markers, two agent harnesses on one graph (d-cpt-kvb).
- Decided, taking shape: automatic two-way sync (d-cpt-r8k).
- Idea: coherence-review agents (s-cpt-ji9 records that nothing catches semantic conflicts today), routing questions by domain (s-cpt-5jk), per-person briefings (s-cpt-dnh, s-cpt-s37).

### 4. The world flowing in (decided shape, not built)
Outside input — stakeholder messages, meeting notes, tracker items, analytics — arrives as candidates: an agent captures each item already connected, topic-tagged, and reasoned about; only dialogue promotes it into the graph proper.
- Why: today a human must notice and carry every observation in by hand (s-cpt-5ox); the graph should grow from ongoing work (d-stg-2wb).
- Key entries: d-cpt-fbi (the decided intake model — channels are transport), s-cpt-jnr (living sources need identity + change detection), s-cpt-yve (outbound to real systems of record is unmodeled — and the boundary: SDD must not become an issue tracker), s-stg-erj (privacy/visibility question, unanswered).
- Honest gap: nothing yet treats user research or usability findings as input — open design territory for a UX perspective.

### 5. Extending the process (mixed maturity)
The working process itself lives in the graph and is adapted per project by dialogue, not code. Three forms: procedures (ten shipped base procedures, project-authored ones pending — s-prc-tso is the first intended one), rules as enforceable learnings (designed, unbuilt — d-cpt-30v, d-tac-eho), facts as pullable, project-supersedable knowledge (shipped, growing — d-cpt-dtv, d-cpt-rh6). A further case: making a project's own business domain explicit — shared vocabulary with semantics. A graph ontology is one rough idea here; SDD's own kinds/refs/topics/actors already work as such an ontology over the process, and the pattern could extend to project domains. Almost nothing graph-backed on this yet (nearest: s-cpt-bhu Open Knowledge Format, s-cpt-k1b topic evolution).
- Why: every team works differently; misfit with the process is a signal, and the process should improve through the same loop (s-prc-way).

### 6. Surfaces (thread from built to planned)
From CLI coding agents (today's door) over the engine's MCP foundation (built and proven between coding agents) toward low-friction doors for everyone: the hosted version for non-technical people (runtime chosen: agent-sdk-go, d-cpt-8k5; self-hostable web UI direction d-stg-6za), then chat, voice, visual reading (s-tac-bta). Important nuance: the technical foundation exists, but the path for non-technical people is not finished — it needs the hosted version (s-cpt-4v3 measured the friction).
- Why: the participants SDD most wants cannot get in yet (d-stg-x0l, s-tac-7kh).

## The three views

1. **Collaboration** — who works here and how it feels: people via surfaces, agents as partners and background workers, the graph in the middle, dialogue as the connecting element (including human-human with SDD preparing and afterwards ingesting the meeting). Animated by short vignettes per role (e.g. product owner, project lead, designer), each with a one-line note that the same scene plays in any deciding team.
2. **Work lifecycle** — the holistic frame first: work is two motions. One moves the graph — dialogue over gaps and questions, forming directives, making plans — everything that brings the picture the entries form closer to the project world; that is real, important work, not overhead. The other moves the world — the graph demands implementing, building, acting, talking — and what that changes feeds back as signals into the picture. Inside that cycle runs the detailed path: a gap is noticed; dialogue — grounded in aspirations, standing directives, and the graph-carried process — shapes it into a directive, then a plan, then implemented work; evaluation closes the loop. Going forward beats completing chunks: partial implementations land with done signals; outer evaluation (user feedback) returns new signals for the next cycle. Many small concurrent loops replace sprint-after-sprint cadence. Shown against the old lifecycle: emails, meetings, documents, tickets — each holding a slice, drifting apart.
3. **Landscape map** — the six regions with maturity marks, one page, no SDD-internal vocabulary needed to read it.

## Tensions worth keeping visible in the material

- Coherence vs. autonomy (d-stg-qlt × d-stg-7lu) — permanent, resolved case by case.
- Sole record vs. real external systems of record (s-cpt-yve) — SDD replaces the scattering, not every tool; it is not an issue tracker.
- Non-developer reach vs. setup reality (s-cpt-4v3, s-cpt-i5a) — the honesty that motivates the maturity marks.
- Privacy/visibility (s-stg-erj) — newcomers' first question, no answer yet.
