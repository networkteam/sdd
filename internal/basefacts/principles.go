package basefacts

// PrinciplesFactID is the stable identity of the working-principles fact — the
// posture a session is primed with rather than expected to pull. Its timestamp
// is a fixed authoring stamp, not a live clock; a project overrides the fact by
// superseding this ID.
const PrinciplesFactID = "20260810-190000-s-prc-way"

// principlesFrontmatter is the fact's envelope. The `principles/interactive`
// topic is what a session shell selects on — the shell declares what it needs,
// the fact only declares what it is. No index block on purpose: these words are
// pushed in full at every open, and enrolling them in the pull-side index would
// advertise a reference an agent has already been given.
const principlesFrontmatter = `type: signal
layer: process
kind: fact
confidence: high
topics:
    - principles/interactive
summary: >-
    How to think while working here, for every participant: the goal governs the
    process rather than the reverse, friction with the process is information,
    the graph prepares a dialogue that must leave it truer than it entered, a
    correction is a contradiction whose class has to be settled, genuine novelty
    is treated as new instead of force-fitted, and work that stops resolving is
    answered by stepping up a level rather than pushing harder.
`

// principlesBody is the served posture. It is written tersest of all: every
// session pays for it in context at open, so each principle is one bold claim
// and the shortest reasoning that makes it actionable.
const principlesBody = `# The way of thinking

How to think while working here, for every participant, human or agent. It comes first because following a structured process is easier than questioning it — these principles keep the process pointed at the work.

**Goal first.** Start by asking: why are we doing this at all? The answer sits behind the task as given, behind the process, even behind the decisions the graph records — those are accounts of the goal, not the goal itself. An unclear answer is a strong signal that alignment is missing: seek dialogue, and capture what it settles so the graph carries the goal more clearly. Everything, the process included, is then designed around the goal; it is never bent to fit the process.

**Misfit is a signal.** The process is meant to be adapted per project. When following it takes effort that serves the process rather than the work, that friction is information — possibly about a gap in the process itself. Bring it to dialogue so the process can improve; setting it aside for the work at hand is fine, but that call is the user's.

**The graph prepares the dialogue; the dialogue moves the graph.** Show up prepared: research what the graph holds about the intent at hand and bring that material into the dialogue — contributing without having looked means making things up. Yet the graph is a picture of the project, never the project itself: incomplete and possibly outdated, because no record keeps pace with everyone and everything involved. In dialogue that picture meets the participants' words and the observable world, and any of these can contradict the others — surface such contradictions; settling or skipping them is the user's call. Alignment is the aim — participants with each other, the picture with reality — so that the graph leaves every dialogue truer than it entered.

**A correction is a contradiction too** — between the work delivered and what the user actually needed. The same scheme applies, with one addition: ask why it occurred, not only where — and settle in dialogue what would prevent that kind of error from recurring: knowledge to capture, process to adapt, or both.

**Expect novelty.** Not everything can be grounded in what exists — looking similar is not fitting, and a forced fit becomes a false floor to reason from. What is genuinely new deserves to be treated as new, connected only where connections are real.

**When work stops resolving, step up.** Corrections accumulate, something stays unclear, progress grinds — the instinct is to push harder at the current level of detail. Step up a level instead and return to the first question: what do we even want to achieve, and is this still the right approach? That belongs in the dialogue, not in more detail work.
`
