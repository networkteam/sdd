# Signals from Story Writing

Insights and design requirements that emerged from writing the Kogen Coffee story. These are signals for the SDD system design.

## System Capabilities Identified

- **Onboarding dialogue**: The system greets new projects with open questions ("what's the situation?"), not forms. It plays back its understanding and asks for confirmation before capturing anything.
- **Personal catch-up**: When a participant returns, the system summarizes what's relevant to *them* — filtered by their domain, not a generic status dump. "Here's what changed since you were last here, here's what's waiting for you."
- **Async by default**: Conversations persist. The system explicitly tells participants they can pick up anytime. No urgency, no notification pressure. The thread is there when you're ready.
- **Research in dialogue**: Agents can research but must ground findings in sources. The participant challenges and corrects with their own knowledge. Data isn't magic — it's a conversation with pushback.
- **Background coherence review**: A separate review agent watches the graph for tensions between new signals and existing decisions. Surfaces conflicts to the person whose work triggered them, with a recommendation for who should be involved in resolving it.
- **Action suggestion**: After signals and decisions accumulate, the system proposes concrete next steps — "build this, test with these people, get real signals" — pushing from deliberation to doing.
- **Voice interface**: Non-technical participants interact through voice naturally (e.g. Jun narrating while cleaning). The system meets people where they are — phone chat, voice, whatever fits the moment.
- **Decision capture through dialogue**: Participants don't write structured documents. They say what was decided, the system proposes the structured capture, they confirm or correct.

## System Capabilities Identified (continued)

- **Release process as process decision**: Who decides what goes live isn't prescribed — it emerges from domain contracts. Content changes merge directly (owner's domain), functionality changes require technical review. Feature work stays on branches until evaluated. Captured as a process layer decision (`d-prc`), enforceable by the system.
- **Contracts as process layer decisions**: The system observes who has been deciding what and surfaces the pattern as a signal. The team discusses and captures it as a process decision (`d-prc`). These are immutable, referenceable (the system uses them for routing and review enforcement), and challengeable through new signals. Important distinction: contracts define decision *authority*, not participation boundaries — anyone can weigh in on anything.

## Untested by Kōgen Story

The small-team, high-trust roastery setting doesn't cover:

- **Mandatory compliance reviews**: Evaluations that aren't optional — e.g. no patient data change ships without security + clinical + privacy sign-off. How do mandatory contracts differ from voluntary ones?
- **External guardrails**: Constraints imposed by regulators, legal, or policy — not emerging from the team's experience but arriving as non-negotiable requirements. How do these enter the graph?
- **Audit trails**: Regulators need to see why decisions were made. The graph naturally provides this, but is the structure sufficient for formal audits?
- **Multi-team coordination**: More than 3 people, overlapping domains, competing priorities. Does the contract model scale?
- **Risk-based escalation**: Some signals *must* trigger human involvement — no agent discretion. How are escalation thresholds defined and enforced?
- **Stricter process contracts**: Not "Jun decides content" but "three-party review required before any change touching X." The process layer needs to handle this without becoming the bureaucracy it replaces.

**Next step**: Write a second story in a compliance-heavy setting (healthcare or fintech) to stress-test the framework against these gaps.

## Open Challenges

- **Graph growth and backlog creep**: The graph never shrinks — signals, decisions, and actions accumulate. Open signals that never trigger a dialogue become noise. The graph must never be pruned (immutability), but attention needs active management. Possible mechanisms: natural decay surfacing ("these signals have been open for 3 weeks — still relevant?"), automatic relevance filtering when decisions supersede upstream context, coverage views that show only active/recent by default, and explicit parking decisions ("not addressing reorders now, revisit at 200 subscribers"). The principle: the graph never shrinks, but visibility is actively managed. This is an unsolved design challenge — the mechanisms need to balance completeness with focus.

## Open Design Questions

- **Actors as graph nodes**: The system needs to know who's involved and what their domain is. Participants are partially captured through process layer decisions (contracts define who has authority over what). But initial participant context ("Jun is head roaster, knows sourcing, flavor, curation") might need its own mechanism — perhaps process layer signals that build up a picture of each person's domain and knowledge, enabling routing before explicit contracts are established.
- **Session model**: Sessions can continue naturally within a short time span. But when time passes and the graph has changed (other participants added signals, decisions, or actions), continuing the old session means working from stale context. The system should detect graph changes since the session started and suggest a fresh start: "There are 3 new entries since we last talked, including a decision from Mara. Want me to rebuild context from the current state?" If nothing changed — just continue. The graph IS the memory, not the chat history. If something wasn't captured as a signal, decision, or action, it doesn't survive between sessions. This forces important information to be recorded.

## Design Insights

- **Contracts emerge by working, then get formalized**: Nobody assigns roles. Through practice, patterns form. The system observes and surfaces the pattern as a signal. The team formalizes it as a process layer decision (`d-prc`). This makes contracts discoverable, enforceable, and challengeable.
- **Morning coffee round**: The most important decisions happen face-to-face, offline. The system captures them afterward, not during. Real human conversation is the primary dialogue medium; the system supports it, doesn't replace it.
- **The system is introduced by a person**: SDD doesn't appear as ambient infrastructure. Someone (Priya) adopts it, proves it on a small project (website), then invites others when a bigger challenge arises. Adoption is organic, not mandated.
- **Skepticism is normal**: Jun's "if it gets weird I'm going back to my notebook" is the expected reaction. The system earns trust by being useful, not by demanding adoption.
- **Real days have real constraints**: People run shops, take supplier calls, clean equipment. The system fits into existing rhythms, not the other way around. Evening sessions, voice while cleaning, morning catch-ups.
- **Actions are the third type**: Without recording what was actually done, the graph has decisions and signals but no link between them. Actions close the loop: decision → action → new signals.
- **Tensions surface through review, not luck**: The conflict between shipping economics and sharing identity wasn't noticed by coincidence. A background agent found it by checking new signals against existing decisions. This is a concrete, buildable capability.
- **Operational sub-decisions as tasks**: A tactical decision to "build a prototype" decomposes into operational sub-decisions (individual steps). Each gets an action when done. No separate "plan" type needed — the decision graph handles sequencing at the most detailed layer.
- **No plan to update**: When a signal invalidates one sub-decision, a new decision supersedes it directly. Other unaffected sub-decisions continue. No need to go back up a hierarchy to revise a plan, reopen tickets, or update an epic.
- **Review agents can weigh in at any point**: A review agent can check actions against the decisions they reference and flag gaps — "the decision said X, but the implementation only did Y." This signal enters the graph like any other and can trigger new decisions without bureaucracy.
- **Mid-build evaluation checkpoints**: The system can suggest evaluation after partial progress ("two of four steps done, want to check?"). The builder decides whether to pause and evaluate or continue. This prevents premature planning but also prevents building too far without feedback.
- **Agents build from decisions, not from human direction**: Implementation agents take referenced decisions as input and build autonomously. They don't need step-by-step sculpting. When done, they capture the action and prompt for evaluation. Humans sculpt through evaluation and decisions, not by directing agents.
- **Stopping is a signal + action**: When an implementation or review agent can't proceed (obstacle, contradiction with a decision), it captures an action (what was done so far) and a signal (what went wrong). It stops and surfaces. Agents don't decide whether to continue when something contradicts a decision — they surface it.
- **Dependency blocking through the graph**: Sub-decisions express dependencies through refs. If a dependency is blocked, downstream steps can't proceed — the system sees this from the graph structure, no special mechanism needed.
- **Proportional resolution**: A new decision supersedes only the blocked sub-decision. The system checks if downstream steps are still valid given the change — only supersede what's actually affected. Small obstacles don't cascade into rethinking the whole effort.
- **Contracts govern autonomy**: The level of autonomous action vs. stopping-to-surface is defined by contracts/guardrails, which flex based on project maturity and risk.
- **Evaluation is decision → action → signal**: Every evaluation follows the same pattern: a decision defines what to evaluate and against what criteria, an action records who reviewed, and signals capture what they found. Multiple people can evaluate the same thing — each produces their own action and signals. This makes every observation traceable back to who made it, what they were looking at, and what criteria they were judging against.
- **Conflicting signals trigger branching, not debate**: When evaluations produce contradictory signals (e.g. "layout is the problem" vs. "content is the problem"), the cheapest resolution is often to branch — build both approaches in parallel and evaluate side by side. Since agents make building cheap, exploring two paths costs less than arguing about one. The branch decision references both conflicting signals explicitly.
- **The system should surface conflicts and suggest resolution strategies**: When it detects contradictory signals referencing the same evaluation, it should notify participants, name the tension, and suggest options — experiment, branch, or dialogue. It doesn't pick sides.
- **Analytics review agent**: An autonomous agent monitoring user engagement patterns and surfacing signals. Runs within a defined contract (e.g. "report significant engagement patterns weekly"). Produces signals like any other participant — the source is noted but the signal is treated the same as a human observation.
