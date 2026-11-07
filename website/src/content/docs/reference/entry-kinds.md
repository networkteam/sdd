---
title: Entry kinds
description: What each kind of graph entry is for, when to choose it over its neighbors, and how it ends.
---

Everything in an SDD graph is an entry, and every entry has a type and a kind. There are two types:

- A **signal** records something noticed.
- A **decision** records something committed to.

The type is the first test. An entry that commits to what will be done is a decision; everything else is a signal. The kind then says which question the entry answers.

You rarely pick a kind yourself. When your agent drafts an entry it proposes a kind, and you confirm or correct it when the agent plays the entry back to you. This page is for looking up what a kind means.

Entries are never edited. An entry ends when a later entry closes it, saying why it no longer applies, or supersedes it with a replacement. Each card says how its kind ends.

| Kind | Type | Answers |
| --- | --- | --- |
| [Gap](#gap) | Signal | What needs attention? |
| [Fact](#fact) | Signal | What do we know? |
| [Question](#question) | Signal | What do we not know? |
| [Insight](#insight) | Signal | What have we synthesized? |
| [Done](#done) | Signal | What was accomplished? |
| [Actor](#actor) | Signal | Who is participating? |
| [Annotation](#annotation) | Signal | What structure lies over other entries? |
| [Directive](#directive) | Decision | Which way do we go? |
| [Activity](#activity) | Decision | What's next to do? |
| [Plan](#plan) | Decision | What must be true when done? |
| [Aspiration](#aspiration) | Decision | What are we pulling toward? |
| [Focus](#focus) | Decision | What are we attending to in this period, and who is engaged? |
| [Role](#role) | Decision | How does an actor participate here? |
| [Procedure](#procedure) | Decision | How does a playbook move run? |
| [Contract](#contract) | Decision | What must always hold? No new entries. |

## Signals

### Gap

Signal · answers "What needs attention?"

A gap records a deviation: what is actually the case set against what was expected, where the difference calls for action. Each side names its source: how the observation was made, and where the expectation comes from, whether a recorded decision, a customer's request or plain common sense. Open gaps show where the project owes attention. A gap records what is wrong, not the fix; a chosen remedy belongs in a decision.

**Choose it over**

- **Fact** when the knowledge is set against an expectation it fails to meet; knowledge that only informs is a fact.
- **Question** when the expectation is already held; when the expected side is itself what the project lacks, record a question.

**How it ends.** A gap stays open until something closes it: usually a decision that addresses it, a done when the fix needed no choice, or any entry showing that the situation changed and the gap no longer applies. A sharper understanding of the same deviation replaces it with a refined gap. A gap can itself close an earlier entry that its deviation shows no longer holds.

**Example.** "The March invoices went out on the 9th; the agreed schedule says invoices go out by the 3rd."

### Fact

Signal · answers "What do we know?"

A fact records knowledge about the world with the source or method behind it, so a later reader can check it again. It informs later work without demanding any, and later decisions build on it as a premise. A claim that is hard to verify may be recorded, marked as such; a claim with nothing behind it may not.

**Choose it over**

- **Gap** when the knowledge only informs; once it is set against an expectation it fails to meet, the tension is a gap, recorded as its own entry beside the fact.
- **Insight** when the claim can be checked against the world on its own; a claim that exists only as a connection between recorded entries is an insight.
- **Done** when the claim holds whether or not anyone acts; a record that someone performed an act, pointing at the work it completes, is a done.

**How it ends.** Nothing completes a fact. When the world changes or a better measurement arrives, a corrected fact supersedes it; when it stops holding and no successor exists, any entry that says what changed closes it, except a question, actor or annotation. A fact can itself close a question by answering it, as long as it restates what the question asked.

**Example.** "The venue contract caps attendance at 120 guests, per clause 4."

### Question

Signal · answers "What do we not know?"

A question records something the project needs to know and does not. Asking puts the unknown on record; an unasked question gets answered anyway, silently, by whatever the work happens to do. Open questions sit beside open gaps as things the project owes, carried into every session until something answers them. A question names the work that needs the answer and may suggest candidate answers, marked as unsettled, to seed the dialogue that settles it.

**Choose it over**

- **Gap** when the expected side is itself what the project lacks; when an expectation is already held and the world fails to meet it, record a gap.

**How it ends.** A question stays open until something answers it: usually a decision with its reasons, or a fact, insight or done that answers it with no choice needed. A decision may also close a question the project will not pursue, saying why, and a sharper version of the same question supersedes it. A question never closes anything itself.

**Example.** "Before the new opening hours are set, which weekdays bring the most walk-in customers?"

### Insight

Signal · answers "What have we synthesized?"

An insight records an understanding reached by connecting what the project has already recorded: a pattern across entries, a cause behind them, or a limit on an earlier claim. It points at the entries it reasons from and keeps the reasoning that connected them, so a later reader can reach the same conclusion. Open insights are the project's current understanding of itself. An insight earns its place by changing something, such as a coming choice or an assumption, yet it owes nothing and may wait until that work arrives.

**Choose it over**

- **Fact** when the claim lives only in the connection between recorded entries; an observation you could check against the world on its own is a fact.
- **Gap** when you record the pattern behind deviations; each deviation that calls for action is a gap, and the two entries point at each other.

**How it ends.** Being acted on does not retire an insight, and neither does a finding that only narrows where it holds. A replacing understanding supersedes it, and any entry that says why the reading stopped holding closes it. An insight can itself close a question it answers, or a fact or earlier reading it overturns, saying why.

**Example.** "All five late deliveries this quarter came from suppliers outside the region; none of the local ones ran late."

### Done

Signal · answers "What was accomplished?"

A done records that an act of work happened: a meeting held, a message sent, a release published. It is always in the past, never a plan or work still underway. It points at what the act completes and cites the most specific trace the act left, such as a document version, an attached file, or a participant's own word when nothing lasting changed. One act makes one done.

**Choose it over**

- **Fact** when the world changed because someone acted; a claim that holds whether or not anyone acts is a fact.
- **Activity** when the work has already happened; a commitment to work still to come is an activity or another decision.
- **Directive** when the entry reports what was done; if telling the act needs a choice and its reasons, record that choice as a decision first, then the done.

**How it ends.** A done is final: nothing closes it, and a wrong one is corrected by a new done that supersedes it. It closes what the act resolved, such as the commitment it fulfills, the gap it fixes or a question its record settles; partial progress closes nothing and says what remains.

**Example.** "Printed and posted the spring newsletter, completing the mailing activity; the final PDF is attached."

### Actor

Signal · answers "Who is participating?"

An actor declares who a participant is: a person, a team or an AI model family, any identity the project attributes work to. Its canonical name is the one the record already uses for them; it also lists any aliases they sign or chat under and says in a sentence who they are and what they bring from outside. Someone who took part once needs no actor; listing them among that entry's participants is enough.

**Choose it over**

- **Role** when the description says who they are outside this project, including a position they hold elsewhere; what they do within this project is a role.
- **Fact** when the knowledge describes who this participant is, such as their training or background; it belongs in the actor even though it reads like a fact.

**How it ends.** When the identity shifts, such as a changed name or a new alias, a new actor entry supersedes it. When the participant leaves, an entry stating why, usually a directive, closes it, and that also retires every role they held. The canonical name is never reused for anyone else, and an actor closes nothing itself.

**Example.** "Dana is a certified accountant with ten years at a regional firm; she signs invoices as D. Reyes and appears in the team chat as Dee."

### Annotation

Signal · answers "What structure lies over other entries?"

An annotation adds no claim of its own. It puts topic labels such as `suppliers/delivery` on entries already written, so a thread running through them can be found in one search; each entry then counts as carrying the label from the start. Labeling an entry as it is written is the usual path; an annotation adds labels afterwards and leaves the original entries untouched.

**Choose it over**

- **Insight or gap** when nothing worth reading is left once the references are stripped away; an entry that argues or claims something is another kind.
- **Focus** when the entry only groups entries; committing who attends to what in this period is a focus.

**How it ends.** An annotation can stay open indefinitely; that is its normal state, and it closes nothing. When the grouping is no longer wanted, any entry stating why retires it, and sharper labels usually arrive as a new annotation beside the old one.

**Example.** "Groups the three entries about late supplier deliveries under `suppliers/delivery-times`."

## Decisions

### Directive

Decision · answers "Which way do we go?"

A directive records a decision on how to go forward: what was decided, which options were weighed, and the reasoning that carried the choice. It reflects current understanding, and new input can change it. A directive can also sharpen an open plan without replacing it, or retire another decision with the reason it no longer holds.

Every directive states its intent, and the three behave almost like separate kinds:

- **Pending.** Something still has to happen because of it, and a reader can tell what would fulfill it. Later plans and decisions work out the details.
- **Guiding.** A guideline that keeps applying. Work is expected to follow it, and a breach can be seen.
- **Settled.** Deciding was the whole of it. Nothing follows, and the entry says why.

**Choose it over**

- **Aspiration** when work must conform to it and a breach would be visible.
- **Plan** when it sets a route the work must follow rather than a finished state you can check item by item.
- **Activity** when a route is being chosen and its reason must be kept; when unsure, pick the directive, because a reason lost later cannot be recovered.

**How it ends.** A pending directive is closed by the done that records its delivery, closed by a later directive that says why nothing will be built, or superseded by a replacement. A guiding directive is never finished by work; a new guideline supersedes it, or a directive closes it with the reason it stopped holding. A settled directive is finished when written and can only be superseded.

**Example.** "Every change is reviewed by a second person before it is published, because errors caught later cost far more to fix."

### Activity

Decision · answers "What's next to do?"

An activity commits that one specific piece of work happens, such as a review held or a set of samples tested. How to do it was already decided elsewhere, in the plan it runs under, an earlier decision, or the conversation that shaped it. The entry says what work is sent, when it counts as finished, and what it leaves behind. Research and trials are activities too; what they find comes back as separate entries.

**Choose it over**

- **Directive** when the route is already settled and only the work remains; if the work still hides a real choice, it is a directive.
- **Plan** when the shape of the work is known and the only check is "was it done?", even if what the work turns up cannot be known in advance.
- **Procedure** when the work happens once rather than repeating by the same steps.

**How it ends.** A done recording the work closes it, and that done accounts for every part of the activity: delivered, or the deviation named with its reason. Work that gets partway points at the activity without closing it. If the work will not happen, a directive that states why closes it.

**Example.** "Compare the three shortlisted suppliers; finished when each has a written verdict."

### Plan

Decision · answers "What must be true when done?"

A plan records what must be true when a piece of work is done. It states the end result in plain words, then breaks it into acceptance criteria, each a single point a finished piece of work can be checked against. It describes an outcome, not the steps or deliverables that produce it. Work that needs breaking down earns a plan; work clear enough to just do needs none.

**Choose it over**

- **Activity** when the work must be broken into several checkable outcomes, rather than a known piece of work whose only check is "was it done?".
- **Directive** when what counts as finished can be listed and checked item by item, rather than a route the work must follow.

**How it ends.** A done that covers the whole commitment closes it: every criterion met with evidence, or a deviation named with its reason. Partial deliveries move it forward without closing it. A directive can sharpen an open plan in place; a change of direction gets a successor plan that supersedes it, and a plan that will never be built is closed by a directive stating why.

**Example.** "Onboarding is done when every new member can, on their first day, name who to ask for help and find the work waiting for them."

### Aspiration

Decision · answers "What are we pulling toward?"

An aspiration records a direction the work is pulled toward, a quality the project keeps moving toward without ever arriving. It binds no single piece of work, so nothing can comply with it or break it. Work is judged by how well it aligns, never by pass or fail, and tension with an aspiration is a reason to talk, never a reason to block. A project holds a few at once, and they often pull against each other.

**Choose it over**

- **Directive** when it pulls the work in a direction and no single piece of work could break it; a missing finish line does not decide it, since a directive may lack one too.

**How it ends.** Nothing completes an aspiration. It ends only when an evolved direction supersedes it, or when a directive closes it with the reason the direction stopped holding.

**Example.** "Newcomers find their own way into the project without someone guiding them."

### Focus

Decision · answers "What are we attending to in this period, and who is engaged?"

A focus records what the project is working on right now and who carries each piece. It points at existing entries, usually plans and decisions, and adds who is engaged and for what period; the work itself stays in those entries. Several people can share one piece, and a piece can be left open for whoever picks it up. It also says what was deliberately set aside for this period and why, the part only a conversation between the people involved can settle.

**Choose it over**

- **Directive** when it holds for the current period and names who attends to what, rather than a rule that keeps applying after the period ends.
- **Activity** when it declares where attention goes and is replaced when priorities shift, rather than one piece of work closed by doing it.
- **Role** when it describes this period's work rather than someone's usual, standing part.

**How it ends.** A new focus supersedes it when priorities shift, or a done closes it when the period has run its course; whoever writes the ending picks which. Its targets need not be finished, but the closing entry says so.

**Example.** "Through March the team attends to the website relaunch, with Sam on content and Alex on testing; the pricing review waits until April."

### Role

Decision · answers "How does an actor participate here?"

A role records the part one named person plays in this project: what they hold, which decisions are theirs, and when work or questions should come to them. It points at a person already recorded as an actor; the actor entry says who they are, the role says what they do here. The part is agreed with the person, not assigned in their absence. One person may hold several roles.

**Choose it over**

- **Actor** when it describes what the person does in this project rather than who they are or what they bring from outside, such as training or a position held elsewhere.
- **Directive** when the sentence stops being true with someone else's name in it; a rule that holds whoever does the work is a guiding directive.
- **Focus** when it is the person's usual part, not just this period's work.

**How it ends.** When the part changes, a new role supersedes it. When the person leaves and their actor entry is closed, all their roles end with it, and no separate entries are needed. A directive can also close a role directly, stating why, though that is unusual.

**Example.** "Alex holds the budget and approves every purchase above the standing limit; routine spending runs without them."

### Procedure

Decision · answers "How does a playbook move run?"

A procedure defines a repeatable way of working that SDD runs step by step with your agent, such as recording a new entry or preparing a release. SDD holds its structure: no step can be skipped, and where a step asks you something, everything waits for your answer. It is an advanced kind, written only to define how a move runs; a move is one of the routines you start with your agent. SDD ships its own procedures, and a project can add its own beside them or replace a shipped one.

**Choose it over**

- **Activity** when the work runs again and again by the same steps, rather than happening once.
- **Directive** when it is a routine to run, not a rule that work must follow; a guiding rule keeps applying but does not run.

**How it ends.** A new version supersedes it under the same short name, and that name is never reused for a different procedure. A practice that ends without replacement is closed by a directive that says why it is no longer how work happens.

**Example.** "The release routine takes every new version through final checks, packaging and announcement, in that order."

### Contract

Decision · answers "What must always hold?"

A contract recorded a constraint that must always hold. The kind takes no new entries, and existing contracts stay valid. A new constraint that must always hold is written as a directive with guiding intent.

**Choose it over**

- Nothing, for new entries. Record a standing constraint as a **directive** with guiding intent.

**How it ends.** An existing contract holds until it is retired. A directive can close it, stating why it no longer holds.

**Example.** "Personal data never leaves the project's own storage."
