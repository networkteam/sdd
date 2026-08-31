# Codex content-projection trials (2026-08-31)

Live usage evidence behind the envelope decision: four exchanges with Codex CLI (direct MCP registration, `$sdd-engine` skill) after the bounded-serves plan shipped structuredContent-only serving (d-tac-4dz). Each trial is the same flow: invoke the skill, let the agent open the engine session.

Codex environment: codex-cli 0.148.0, ChatGPT desktop-app integration, model gpt-5.6-sol. The sdd server registered per repo config, launched over stdio.

## Trial 1 — structuredContent only, empty content (as shipped by d-tac-4dz)

The agent called `start_session`, received a result whose `content` was empty, concluded nothing came back, and called `start_session` again. Asked why:

> The first call succeeded, but my wrapper printed only content, which was empty because this server returns structuredContent only. I mistakenly called start_session again instead of inspecting the first result.

## Trial 2 — one-line pointer in content

`content` carried: `payload in structuredContent (this line is a pointer, not the result; sdd serve --compat mirrors the full text here)`. The transcript rendered the pointer under both calls — the agent still called `start_session` twice:

> The first call succeeded, but I printed only its content field. The server now returns the real payload in structuredContent, so I saw only a pointer. I called start_session again to retrieve and print that payload.

The pointer made the failure legible but did not remove the extra step: on this host the payload must be in `content` to reach the model without agent gymnastics.

## Trial 3 — `--compat` (both fields on the wire)

Registration changed to `args = ["serve", "--compat"]`. One `start_session` call, the full framing rendered under it, clean flow, no truncation. Rollout measurement (primary evidence, `rollout-2026-08-31T15-43-01-…jsonl`): the door serve entered model context once — 28,960 bytes, the sentinel phrase "Standing goal" appearing exactly once, no harness truncation marker. Raw-shape inspection in the same session confirmed the wire carried both fields (a 28,148-character `content` text block plus the semantically identical `structuredContent`, matching the wire measurement in s-tac-x9l) while model context held one copy:

> After the first call, my model context contained the payload once because the wrapper printed only r.content. structuredContent stayed inside the JavaScript call and was not added to my context.

## Trial 4 — how this harness treats structuredContent

Asked when the harness would use `structuredContent` over `content`, the agent researched the official documentation and its own wrapper:

> structuredContent is not automatically preferred over content in this harness. […] An MCP call returns a JavaScript object. My wrapper decides what enters model context. […] It does not inherently save context tokens. Once serialized into model context, it costs roughly the same as equivalent text. For Codex outside this JavaScript wrapper, I cannot establish a documented precedence rule.

No documented precedence exists; `structuredContent` on this host is a programmatic value for scripted selection, never an automatic context channel.

## Conclusion — the doubling was an observer effect

Every measurement that showed the payload twice in model-visible output traced back to an agent explicitly inspecting its response envelope — either because the session's task was to verify truncation (the bounded-serves live verification) or because it was asked whether content was duplicated. In ordinary use, each verified host projects exactly one copy: Codex prints `content`; Claude Code prefers `structuredContent` and drops the duplicate text block. The spec envelope (both fields) was never observed to double on a default projection, and the structured-only default broke the common case. d-tac-4dz's premise — "both copies can reach the model" — was a probe artifact read as default behavior, the same class of unreliable harness-introspection evidence s-tac-x9l warned about.

## Reliability note

The agent's descriptions of its own harness contradicted each other across the trials (direct calls vs. "everything stays inside the JavaScript wrapper"; "I should use structuredContent and ignore content" while demonstrably consuming only content). Conclusions here rest on the rollout files and the rendered transcripts, not on the agent's introspection.
