# Interview record — implementation procedure design

Interview run 2026-07-06 (Christopher, Claude) to settle the engine implementation procedure before authoring, in Slice B of the engine completion directive (20260705-010751-d-tac-dbk). One decision per section, with rejected alternatives.

Grounding available going in: the implementation playbook skill text as parity source; the unified seeding contract shipped per 20260706-093531-s-tac-xeq; wipStart/wipDone engine commands already registered by the MCP shell (built for this procedure); session park/resume validated for capture-shaped procedures (20260704-201555-s-cpt-osk).

## 1. Work shape → working loop

**Background.** First procedure where the main work happens away from the engine — hours or days of host work versus minutes of dialogue. The choice: does the agent check back in during the work, or only at the end?

**Decided:** a working junction (agent chooser) the agent returns to between slices: continue (with a progress note), blocked, conclude. Progress notes accumulate in state — the cross-session pickup material the resume-fidelity question (20260703-142504-s-cpt-m6m) says a log-only resume lacks.

**Rejected:** linear single work step (resume relies on git history alone; no next-step pointer); linear plus pause-only note (a deliberate handoff note but no trail).

## 2. Run mode and quick

**Background.** How to run the work — in place, branch, worktree, quick — is the user's call; the playbook is explicit it cannot be inferred. The engine can make the choice structural and tie the marker to it.

**Decided:** setup is a user chooser. Picking a tracked mode collects the marker description and runs wipStart in the same answer — markers can no longer be forgotten. Branch/worktree git and harness moves stay host work with agent-neutral instructions (per-agent recipes stay in the skill layer per 20260628-130403-d-cpt-h4l). Quick stays inside the procedure and skips only the marker: contract, working loop, and record gates still apply.

**Rejected:** quick exits to plain capture — cold, unseeded captures (the measured conversion friction the seeding contract removed) and no home when a small fix grows; drop quick — marker churn for ten-minute fixes. (An initial mis-answer selected "drop quick"; corrected in dialogue to "stay, skip marker".)

## 3. Scope check → structural escape at setup

**Background.** The playbook front-loads a readiness check: enough decisions to build against? Prefer reducing scope over building into the unknown; capture the missing decision first.

**Decided:** the setup junction carries a hold option — a needed decision is missing — that dispatches a seeded capture for the missing decision or plan and loops back to setup. The readiness read itself is instructed prose in the contract unit; proceeding is never blocked.

**Rejected:** a dedicated readiness-confirmation step (one extra turn on every run, including obviously-ready ones); prose-only (the satisficing failure mode the engine exists to remove).

## 4. Blocked path → dialogue-first (user-corrected)

**Background.** The playbook stop rule: hit a design choice no decision covers — stop, don't decide alone. In engine mode the user is present in the dialogue, so many roadblocks resolve on the spot.

**Decided (Christopher's correction shaped this):** the blocked answer only marks the stop and stashes the run's grounding. The served unit instructs: present the roadblock to the user in plain words, propose one or more ways forward, settle it in dialogue. The dialogue's outcome becomes zero or more captures — augmenting directive, directive, gap, question, whatever fits — each proposed and confirmed the normal way, arriving pre-grounded via the stash. Then continue, or pause with the notes carrying the pickup.

**Rejected:** auto-dispatching a capture on blocked — the originally proposed shape, corrected because it skips the dialogue that should shape the outcome; a forced progress-done-plus-signal pair (ceremony when the answer is one dialogue turn away); no blocked option at all (cold captures, stop rule loses its structural home).

## 5. Closeout

**Background.** Playbook order: commit code first, then the done signal citing commit hashes and closing the acceptance criteria, then remove the marker. The evaluation model (20260706-142633-d-cpt-eib) splits the landing read before a merge from the post-landing learning loop, which is now its own procedure.

**Decided:** conclude instructs committing first, then dispatches the closing-done capture — commit hashes cited, acceptance criteria and augmenting commitments closed, implementation/<...> work-shape topics per 20260706-122752-d-cpt-yum. The engine then calls wipDone itself (quick runs skip it), and a final user junction offers starting evaluate seeded from the run, or finishing. The landing read before any merge stays instructed dialogue.

**Rejected:** prose-only evaluation prompt (the discipline that historically gets skipped); always-evaluate (forces a full evaluation onto every run, coupling two moves the three-point model keeps separate).

## Process note

Mid-interview meta-feedback from Christopher: the option-prompt format worked poorly — background must be presented before detailed questions (the counterpart does not have the plan and spec entries in view or memorized), the pace was too slow, and richer presentation with free dialogue beats structured option prompts. Applied from question 5 on. Direct evidence for the interview procedure design queued in this same slice.
