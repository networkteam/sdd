---
type: signal
layer: process
kind: fact
confidence: high
topics:
    - engine/base-facts
    - type-system/kinds
summary: >-
    The done kind records one completed act of work — what happened, on whose
    commitment, with the evidence a reader needs to trust it and find it —
    closing what the act genuinely resolved and feeding the loop that follows.
---

# Recording completed work — the done kind

A done records that an act of work happened: a meeting held, a message sent, a batch roasted and tasted, code shipped. It is a signal — something noticed, not something committed to: it observes that the world already changed, which is why the act lies in the past and a done never records intention or work still underway. Its reader arrives asking three things — what happened, can I trust it, where do I look further — and a good done is the shortest record that answers all three.

**Say what happened, plainly.** Name the act in the words of the work itself — what was made, changed, or settled, concrete enough that a reader recognizes the delivered thing without opening anything else. No invented shorthand, no abstraction that summarizes the acts away. And stop where the evidence takes over: detail the cited trace already carries is not repeated — a commit already lists which files changed, an attached protocol already holds the readings. The body keeps to the act itself, stated in its own words; the detail lives with the trace.

**The doing is part of the record.** Small deviations from the commitment, struggles met along the way, implications noticed while the work was in hand — the noteworthy experience of performing the act belongs in the done, told plainly. This is not evaluation of the work: judging whether it met its target is further work with its own record. And when something noticed in the doing stands on its own — a problem, a claim, an open question — it becomes its own entry pointing back at this one; the done keeps the mention, not the full story.

**One act, one done.** The unit is the delivery event, not the concepts the work touched. Several changes checked and accepted in one landing are one done; two acts are two dones even when one working session produced both — a supplier meeting held and a fix shipped the same afternoon are two records. And anything that would stand as its own entry of another kind — a choice made, a problem noticed, a claim about how the world is, an open question — belongs in that entry: a done records the act; it is not the storage place for everything the act stirred up.

**A completion claim names what it completes.** Close what the act genuinely resolved: the commitment it fulfills, the gap it fixes — or a question the act's own record settles, when the answer travels with the done. Partial progress closes nothing: it points at the commitment, says what is covered and what remains, and leaves closure to the act that finishes. When the graph holds nothing the act belongs to, never invent a pointer to satisfy the form — capture the missing holder first (the activity or direction the act served, or the gap it answered), then record the completion against it. And the closure carries the context with it: everything the closed commitment already references is one step away, so the done does not restate its target's references — its own pointers carry only what the record itself needs, an evidence source, a dependency the act drew on that the commitment does not name.

**Evidence follows the act.** Cite the most specific durable trace the act produced: a change to tracked material cites the exact revision that carries it — a commit, a numbered document version; a result living elsewhere cites where to find it — a released version, a published page; when the deliverable is a document — a design note, a tasting protocol — it is attached; and when the act changed nothing durable — a verification, a conversation — a participant's own attestation in the body is the trace. A done that only retires a commitment delivered elsewhere carries no evidence of its own: it points at the record that has it — reproducing that record would be the fault. Evidence is pointers, not measurements: a figure belongs in the body only when it is the claim itself; counts and durations left over from checking bind every later reader to reconcile them.

**What was done, not what was chosen.** A done describes execution: "updated the label template so the misprint cannot recur" is a done; "switched to pre-printed labels because in-house printing kept failing" is a decision wearing a done's clothes. If describing the act honestly requires explaining a choice or a justification, the decision is missing: capture it first, then the done. The deeper the choice reaches, the firmer this rule.

**Cover the whole commitment.** When a done closes a commitment with stated criteria, every criterion is either confirmed — with its evidence — or its deviation named with the reasoning behind it. Silent omission is the failure; an honest "this stays open" is not.

**The baseline goes without saying.** Claiming an act done already claims the project's standing checks passed — verification the project requires of every act is the floor of the claim, not its content, and restating it in each record is noise. Name a check only when its outcome is part of the story: it was skipped, it failed and the result was accepted anyway, it was tightened or added for this work.

**Status is terminal; the loop is not.** A done is a fact of execution: nothing closes it, it carries no status, and a wrong record is corrected by a superseding corrective done, never by editing. But it is not the end of the work's story — judging the work (did it meet its target? what did it surface?) is further work, recorded as its own done, and what an act surfaces points back at the done that raised it. Later entries never "address" a done — it holds no open concern to address; they build on it, reason from it, or are surfaced by it.

**Topics name the shape of the work.** A done's own topic labels say what kind of work concluded, in the project's own vocabulary (`roasting/first-batch`, `implementation/engine`) — the subject needs no label, because it is already carried by what the done closes and refs.

{{ .Mechanics }}
