# Live outer evaluation — unprimed agent, fresh session, new binary

Run by Christopher on 2026-08-19, immediately after the delivery commits (5b0723d8..63b8c39b) were built into the worktree binary and the MCP server restarted on it. A fresh Claude session (no priming from the delivery dialogue) opened its own SDD session and was asked to capture a new idea.

## The user's initial ask (verbatim)

```
I have an idea about providing an UI for users to follow / debug the sdd engine, see query and write operations and the current procedure instances and transitions. That could be helpful for understanding. It should also allow to inspect graph entries that were loaded / mentioned in full (because agents sometimes only name references and it would be nice to have a companion UI that enables to look into these entries).

Let's put that into the graph.
```

## Transcript excerpt (verbatim)

```
⏺ sdd - start_procedure (MCP)(canonical: "capture", session: "s_20260819-171905-c6ca0d85", parent: "i_1", label: "Companion UI for following and debugging the engine")

⏺ sdd - show (MCP)(ids: ["20260818-120000-s-prc-dsc","20260815-100000-s-prc-gpk","20260816-100000-s-prc-syn"], up: 0, down: 0)

Before I draft: reading the type-system guidance, this sits on a fork that changes what gets recorded, and it's your call.

The substance has two halves — a real deficit (during an engine-driven session you can't see what the engine is doing, and when I name an entry by ID you can't look inside it without asking me) and a proposed answer (the companion UI).

Do you want this recorded as the deficit, with the UI held as the candidate answer — so it lands on the open-work surface as something owed, and a commitment to build follows later when you decide — or as a commitment to build the companion UI, with the deficit as its reasoning?
```

## Reading

- The capture's initial serve reached the unprimed agent: immediately after starting the unselected capture, the agent pulled the discrimination fact (20260818-120000-s-prc-dsc) and the two competing candidate kinds' authoring facts — gap (20260815-100000-s-prc-gpk) and insight (20260816-100000-s-prc-syn) — in one show call, before drafting.
- The agent then surfaced the kind fork (deficit vs commitment to build) to the user as their decision, framed in the framework's own terms, instead of guessing a kind and drafting.
- This answers the plan's stated behavioral question — does capture create the pull before the first draft — with a stronger observation than the criterion's minimum: not only a selected-kind fact, but discrimination plus both candidates for an unselected capture.
- Not covered by this excerpt: the writing-guide half of the criterion (what kind-specific correction the guide still supplied) — that capture was still in flight when the excerpt was taken.
