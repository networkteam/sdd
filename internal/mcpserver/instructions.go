package mcpserver

// Server-level instructions and the few fixed instruction snippets the shell
// itself contributes. Step-by-step guidance lives in procedure entries and is
// served by the engine per step — the shell only frames the loop. Keep these
// short, imperative, and free of SDD-internal jargon the agent cannot
// resolve from the same response.

const serverInstructions = `This server hosts an SDD (Signal-Dialogue-Decision) graph: an append-only
record of project observations (signals) and commitments (decisions) grown
through dialogue between the user and their agents.

Work runs as procedures — guided moves the server drives step by step:

- start_procedure begins a move (e.g. capture); every response carries the
  current step's instructions, a report_schema, and the goal that advances
  it. Follow the served instructions; report with next.
- next either sends state fields (per report_schema) or answers a pending
  chooser. A "user" chooser belongs to the human: put the options to them
  and relay their answer verbatim in userWords — never answer it yourself.
- Graph writes happen only inside procedure transitions. There is no
  direct write tool, and validation runs inside the write step.
- abandon discards an instance explicitly; list_sessions and
  resume_session continue earlier sessions after a restart.
- A session is one dialogue with the user; it can outlive your own agent
  session. Give it a short subject label early (label on start_procedure
  or next) and update the label when the dialogue's subject sharpens —
  labels are how a user tells parked dialogues apart. In dialogue, refer
  to a session's procedure instances as its threads.

Reads are free and never gated: search (find entries), view (overview
layouts), show (full entries with reference chains), read_attachment,
info, and registry. Ground dialogue in them liberally — an entry's
one-line summary is a pointer, not a fact.

The first response of a session carries a framing block (aspirations,
guiding directives, focus, participants): hold it as context, don't dump
it at the user.`

const resumeInstructions = `Session resumed: step position and collected evidence persist; the
open_instances list carries each running instance's current serve. Brief
the user on where the work stands (procedure, step, goal) before
continuing, and continue through next.`
