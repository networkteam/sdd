package mcpapp

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

Everything else is served. There are two doors into a dialogue: start_session
opens a fresh one and returns its opening serve — your orientation, how the
dialogue should feel, and the moves you can start; resume_session attaches to
an existing session by its handle.

The session handle is the dialogue's identity. Every work tool (start_procedure,
next, park, stage_attachment, and abandon of a move) carries it as a required
argument naming the session this connection is attached to — retain it across
context compaction. The one exception is abandon's teardown mode (session alone,
no instance): it tears down a session by handle without attaching, so the handle
there names a session you need not be attached to. Discovery and reading stay
free: list_sessions shows every session with open work (no handle needed), and
the read tools (search, view, show, read_attachment, info, registry) are always
ungated.

If you lose the handle — your context was compacted or summarized — recover
without guessing: resume_session with no arguments re-serves the session this
connection is already attached to (every running move at its current step with
the schema to continue it), or, if this connection is not attached to one,
names the open sessions to attach to. list_sessions lists them too. Then
re-establish which one with the user before continuing.`

const resumeInstructions = `Session resumed: step position and collected evidence persist; the
open_instances list carries each running instance's current serve — the
session shell (user-dialogue) among them, carrying the open-threads block.
Brief the user on where the work stands (procedure, step, goal) before
continuing, and continue through next, carrying this session's handle. If you
lose your place again later, resume_session with no session re-serves this
list.`
