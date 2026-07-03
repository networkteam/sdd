package mcpserver

// Server-level instructions and the few fixed instruction snippets the shell
// itself contributes. This is knowledge tier zero — the connection handshake:
// the minimum an agent needs to not misread a read, plus the door. It must
// survive truncation (some clients render instructions poorly), so it stays
// short and front-loads what matters. Tier one is the session shell's opening
// serve (the user-dialogue base procedure); tier two is the per-step units.

const serverInstructions = `This server hosts an SDD (Signal-Dialogue-Decision) graph: an append-only
record of project observations (signals) and commitments (decisions) grown
through dialogue between the user and their agents.

The minimum to read it right:

- Every entry is a signal (something noticed) or a decision (something
  committed to). Entries are immutable — current status is derived from
  the graph, never edited in place.
- An entry's one-line summary is a pointer, not a fact. Read entries in
  full (show) before relying on them.

Everything else is served. Call start_session to begin: it opens the
dialogue session and returns your orientation — the process core, how the
dialogue should feel, and the moves you can start. All stateful tools
require a session and point back to this door; the read tools (search,
view, show, read_attachment, info, registry) are always free.`

const resumeInstructions = `Session resumed: step position and collected evidence persist; the
open_instances list carries each running instance's current serve — the
session shell (user-dialogue) among them, carrying the open-threads block.
Brief the user on where the work stands (procedure, step, goal) before
continuing, and continue through next.`
