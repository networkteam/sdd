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
an existing session by its handle. start_session is the only tool that takes a
project: one server serves every project the principal can reach, a sole
project is inferred, and when several exist and none was passed the response
lists them (status project-required) instead of opening a session — choose
with the user, then call again with project. Attaching to a session this
connection did not open carries the user's verbatim request in userWords (a
fresh ask that merely resembles the work is not consent); taking over one that
another client is actively driving additionally needs takeover:true, and only
recorded session state resumes — not the other conversation's context.

The session handle (project:session-id) is the dialogue's identity and binds it
to one project. Every other tool carries it as a required argument naming the
session this connection is attached to — the work tools (start_procedure, next,
park, stage_attachment, bind_branch, abandon of a move) and the reads (search,
view, show, read_attachment, info, registry) alike; a read runs in the session's
project and branch. Retain the handle across context compaction. The one
exception is abandon's teardown mode (session alone, no instance): it tears
down a session by handle without attaching, so the handle there names a session
you need not be attached to. Reads are free in the sense that matters: no move
has to be running and no procedure state gates them. Discovery is free too:
list_sessions shows every session with open work (no handle needed).

If you lose the handle — your context was compacted or summarized — recover
without guessing: resume_session with no arguments re-serves the session this
connection is already attached to (every running move at its current step with
the schema to continue it), or, if this connection is not attached to one,
names the open sessions to attach to. list_sessions lists them too. A plain
reorient stubs blocks you were already served — it assumes you still hold them;
if a compaction dropped them, pass fullReplay:true for the complete re-serve.
Then re-establish which one with the user before continuing.`

const resumeInstructions = `Session resumed: step position and collected evidence persist; the
open_instances list carries each running instance's current serve — the
session shell (user-dialogue) among them, carrying the open-threads block.
Brief the user on where the work stands (procedure, step, goal) before
continuing, and continue through next, carrying this session's handle. If you
lose your place again later, resume_session with no session re-serves this
list; if a context compaction dropped the instructions a stub only points back
to, add fullReplay:true for the full text.`
