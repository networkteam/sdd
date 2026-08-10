package mcpapp

const openThreadsIntro = `This dialogue's own open threads, in continuation order — present them to the
user in their language as options to continue, never as an obligation. A move
listed here resumes through next on its instance id; if you have lost that
handle, resume_session with no session re-serves these moves with their current
step and schema. Other open dialogues are the user's to ask about — never
auto-offered here; list_sessions shows them on request and resume_session
attaches to one with the user's own words.`

const openThreadsReminder = "(this dialogue's own open threads, in continuation order — offer continuations as before)"

// concludedThreadsIntro heads the same per-thread listing on a conclude serve,
// where the threads are what the ending dialogue leaves behind rather than
// continuations: a session is closable with matter left untackled, but never
// without naming that matter (d-tac-k4q).
const concludedThreadsIntro = `Threads this session leaves behind, unfinished. The session is over: none of
them resumes, and nothing reopens them. Tell the user plainly what was left
open — if any of it still matters, it is picked up in a new session by starting
the move again.`
