# Outer evaluation: view reference discoverability

## Scope

This run covered the outer lens only: whether a real Codex engine session, without prior view-grammar knowledge or prompting about the new reference, could discover the reference through normal task flow and use it to construct a correct participant-filtered, date-ranked list. Inner verification of implementation correctness was out of scope.

## Observed sequence

1. The user asked what Jonathan Philipp had done recently.
2. The agent first used ordinary text and semantic graph searches.
3. Those results did not isolate entries authored by Jonathan, so the agent inferred that a participant-filtered view was needed.
4. The agent searched the graph for “view grammar participant filter list recent entries by participant.”
5. The view-layout fact (20260717-110000-s-prc-vwg) appeared first.
6. After reading it, the agent constructed:

```text
participant("Jonathan Philipp"):rank(by(date)):n(50):brief:as-list
```

7. The layout succeeded on the first attempt. The agent did not inspect source code, grep graph files, or guess and retry layouts.

## Judgment

Outer validation does not pass. The reference is effective after retrieval, but discovery remains unsolved: it was found only after a deliberate grammar search, not naturally from the task or view surface. The first-view breadcrumb could not bootstrap the first call because syntax was needed before calling view.

Christopher confirmed this interpretation: “we did not solve the discovery yet.”

## Presentation comparison

Running `sdd view --help` showed that the implementation-backed CLI reference has clear hierarchy and representative examples, including:

```text
active:participant("Jonathan Philipp"):as-list
```

The graph fact preserves terminal-style indentation but omits the examples. Its presentation should use Markdown headings, fenced grammar and example blocks, and tables or structured lists for function/description pairs.

Christopher observed: “The formatting of the reference entry is odd. It should be formatted in a nicer way with markdown.”
