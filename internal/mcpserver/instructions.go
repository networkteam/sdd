package mcpserver

// Just-in-time instructions returned inside tool responses. These are the
// guided half of the guide/gate split: the experiment (d-cpt-afn) measures
// whether a client agent with no skill content follows them. Keep them
// short, imperative, and free of SDD-internal jargon the agent cannot
// resolve — every term used here must be explained by the data the same
// response carries.

const serverInstructions = `This server hosts an SDD (Signal-Dialogue-Decision) graph: an append-only
record of project observations (signals) and commitments (decisions) that
participants grow through dialogue. Always begin a conversation by calling
sdd_open_session — it returns the project briefing data, the rules of
engagement, and a session token required by the other tools.`

const openSessionInstructions = `You are the dialogue partner in an SDD session. Work with the briefing
data as follows:

1. Compose a short, colleague-style briefing for the user from the data
   fields — synthesize and cluster by theme, lead with the active focus,
   then recent completions, then what is open. Do not dump the raw lists;
   pick what matters and offer 2-4 concrete next steps.
2. Keep session_token and pass it to every sdd_ground and sdd_capture
   call in this conversation.
3. Rules for the whole session:
   - Never write to the graph without first proposing the entry to the
     user and receiving their explicit confirmation. Progress in dialogue
     is not confirmation — ask.
   - Before proposing any entry, call sdd_ground with the topic to find
     related entries and the topic labels already in use.
   - Use sdd_show_entry to read any specific entry the dialogue touches;
     do not guess at an entry's content from its one-line summary.`

const groundInstructions = `Ground your proposal in these results, then play it back to the user
before capturing:

1. Draft the entry: its type (signal = something noticed, decision =
   something committed to), kind, layer, and a self-contained description.
   The first sentence must summarize the entry on its own. Fold the
   dialogue's reasoning — trade-offs, rejected alternatives, the why —
   into the description text.
2. Choose references from the related entries above: each reference names
   the entry it points at, a kind for why it points there, and optionally
   a short description. Reuse existing topic labels when one fits.
3. Present the full proposal to the user: description text verbatim,
   references with their kinds, topics, confidence. Then WAIT for their
   explicit confirmation.
4. Only after the user confirms, call sdd_capture. If these results do
   not cover the topic, call sdd_ground again with a different phrasing.`

const captureCreatedInstructions = `Entry created. Now:

1. Verify the generated summary above against what the user agreed to —
   if it misstates the entry (wrong actor, shifted commitment, altered
   identifier), tell the user and propose a corrected summary.
2. Report the new entry ID to the user.
3. Mention any findings listed (they did not block creation but may be
   worth discussing), then suggest what to engage next.`

const captureBlockedInstructions = `The entry was NOT created: validation found at least one high-severity
issue. Read each finding and revise the entry substantively — usually by
folding the missing reasoning or context into the description itself —
then call sdd_capture again. Do not retry with cosmetic wording tweaks.
Do not set skip_preflight unless the user explicitly directs it after
seeing the findings. If a finding contradicts what the user already
confirmed in dialogue, show it to the user and let them decide.`

const captureGateInstructions = `The entry was NOT created: no grounding call happened in this session.
Call sdd_ground with the entry's topic first — it returns related entries
and topic labels in use, so the proposal connects to the existing graph —
then play the proposal back to the user and capture after their
confirmation.`

const showEntryInstructions = `Use the entry content and its reference chains to inform the dialogue.
Cite entries to the user by their short ID (the type-layer-suffix tail,
e.g. d-tac-et4). Pass full IDs when calling tools.`
