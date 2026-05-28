# Research discipline gap — evidence and detail

This attachment supports the signal capturing what we found during dialogue about a recurring failure pattern observed in another project applying SDD.

## Empirical context

The pattern is reported to occur in almost every session in a separate SDD project. The reporter is the project's lead operator working with Claude through Claude Code.

A CLAUDE.md rule extension was tried as a remediation. It was sharper than typical principle text — action-trigger shaped, with explicit citation expectations and clear trigger conditions. The pattern persisted with the rule in place. That is the empirical weight behind the procedural-vs-principles observation in the parent signal: principle text, even when sharpened, did not close the gap.

## The verbatim CLAUDE.md rule that was tried

The exact text added to the other project's CLAUDE.md:

> ## Research before reasoning
>
> **Before stating *or reasoning from* any concrete fact about this project's systems, decisions, or state — check.**
>
> - **Graph:** `sdd search` to find candidates, then `sdd show <full-id> [<full-id>...]` to read the bodies in one call. The summary on its own is not the fact.
> - **Wiki:** `ls` the relevant folder under `wiki/`, then read the page.
>
> If your next response or your next action depends on a fact being true — and you didn't just look — stop and look. The trigger is "my reasoning depends on this," not "I'm debugging" or "I'm about to capture." Speculating costs the next hour; one `sdd search` and one read cost seconds.
>
> **A hypothesis without a citation is the signal you skipped research.** Before asserting how a component, mechanism, or topology works, name the graph entry or wiki page the claim rests on — or stop and read it first, including following `[[wiki-links]]` on a page you already opened and prior-incident signals surfaced by search. Researching once at session start does not discharge this; the obligation re-arms each time the investigation moves to a new part of the system.

## Three interacting failure modes

**Mentions treated as facts.** When standing guidance, an entry body, or any reference *mentions* something (a rule, a workstream, a tool, a focus, a runbook), agents treat the mention itself as enough — as if seeing a name means knowing the thing. Following the mention through to its actual content is skipped. The mention is a pointer; the content is the fact. Until the pointer is followed, the picture has a hole.

**Generative-mode shortcut.** When a user requests new structure ("build a plan", "capture an activity") or new work ("change this code"), agents shift into a generative mode that pattern-matches against shapes from training data rather than starting from what this specific project already has. The shift makes the agent feel they have already done the homework.

**Shallow-on-pushback.** When a user signals "I don't feel aligned" or "look better" or "research again", the agent runs another shallow research pass at the same depth rather than escalating depth. Multiple iterations of pushback are required before the agent actually goes deep. The mechanism is not defensive — the agent is not defending what it did. It is stuck at shallow. Each prompt yields another fresh shallow attempt at the same bar.

## Scope is broader than capture-time

Today's SDD captures split between *capture-time grounding* (s-prc-6ll → d-prc-vlu) and *alignment at conversation entry and mode transitions* (s-prc-7hw → d-prc-nkw). The signal observed here says the same pattern shows up in any action whose next move depends on a fact being true: code changes, wiki edits, general reasoning. The failure mode is not capture-specific; it is research-discipline-specific.

## The procedural-vs-principles observation

The remediation rule above is a *principle* — it states a rule and leaves interpretation to the agent. Questions like "what counts as reasoning?" and "what counts as enough checking?" are left to the agent. The pattern persisted with the rule in place.

A *procedural* version would name the steps tied to specific moments. Something like: "Before drafting an entry: run `sdd search` for the candidate terms; for each candidate, read the body via `sdd show`; name in the entry which entries were consulted." That is still text-instruction, but interpretation latitude is removed.

This is a directional learning at medium confidence:

- We have direct evidence that principle-style text did not close the gap.
- We do not have direct evidence that procedural text would have closed the gap.
- The hypothesis is that procedural shape removes the interpretation latitude that principle shape leaves open.

This shapes how the implementation of d-prc-nkw (with d-prc-vlu's commitment folded in) should ship — as procedural moments tied to concrete actions, not principles to be interpreted — but does not yet justify a new structural-enforcement commitment.
