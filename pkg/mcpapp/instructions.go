package mcpapp

// Server-level instructions and the few fixed instruction snippets the shell
// itself contributes. This is knowledge tier zero — the connection handshake:
// the minimum an agent needs to not misread a read, plus the doors. It must
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
dialogue should feel, and the moves you can start — under a new handle;
resume_session re-serves the current position of a session whose handle you
present. start_session is the only tool that takes a project: one server
serves every project the principal can reach, a sole project is inferred, and
when several exist and none was passed the response lists them (status
project-required) instead of opening a session — choose with the user, then
call again with project.

The session handle (project:session-id) is the dialogue's identity and its
capability: it reaches you only by issuance (start_session minted it for you,
or a dispatch carried it) or because the user handed it over, and presenting
it is the whole authorization to continue that dialogue. Every other tool
carries it as a required argument — the work tools (start_procedure, next,
park, stage_attachment, bind_branch, abandon) and the reads (search, view,
show, read_attachment, info, registry) alike; a read runs in the session's
project and branch. Nothing is derived from the connection: a reconnect
changes nothing, and closing a connection ends nothing. Retain the handle
across context compaction. Reads are free in the sense that matters: no move
has to be running and no procedure state gates them.

If you lose your place — your context was compacted or summarized — but
still hold the handle, resume_session re-serves the session's position: every
running move at its current step with the schema to continue it, in full. If
you lost the handle itself, ask the user: sessions are never listed on this
surface, and a handle is never guessed.`

const resumeInstructions = `Session resumed: step position and collected evidence persist; the
open_instances list carries each running instance's current serve — the
session shell (user-dialogue) among them, carrying the open-threads block.
Brief the user on where the work stands (procedure, step, goal) before
continuing, and continue through next, carrying this session's handle. If you
lose your place again later, resume_session with this handle re-serves this
list in full.`
