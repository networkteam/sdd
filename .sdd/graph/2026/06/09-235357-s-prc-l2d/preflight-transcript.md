# Pre-flight transcript — self-retracting high finding + same-ref cross-run flip

Captured 2026-06-09 during creation of `20260609-234656-d-cpt-afn` (workflow-MCP experiment directive).

## Input (run 1)

Entry: `sdd new d cpt --kind directive --confidence low --participants Christopher,Claude --topics portability/mcp` with description (excerpt of the ref-relevant sentence):

> "...as the skill-content half of the portability gap (s-cpt-z7l) predicts and this experiment directly tests."

Refs passed:

```
--refs '{"id":"20260531-161611-s-cpt-r57","kind":"addresses","desc":"realizes the specialist layer in a Go-resident, one-codebase shape"}'
--refs '{"id":"20260525-184931-s-cpt-z7l","kind":"addresses","desc":"tests instruction delivery to foreign runtimes — the skill-content half of the gap"}'
--refs '{"id":"20260531-161006-s-cpt-80v","kind":"grounded-in","desc":"per-session state, sampling, and elicitation as confirmed premises"}'
--refs '{"id":"20260531-165717-d-cpt-313","kind":"related","desc":"runs parallel; the experiment verdict decides its reshaping"}'
--refs '{"id":"20260527-210616-d-tac-0v4","kind":"related","desc":"its infrastructure survives either way; its CLI-parity tool surface is what is in question"}'
--refs '{"id":"20260525-204836-d-cpt-t3j","kind":"related","desc":"Mastra demoted to open question pending the experiment verdict"}'
```

## Output (run 1) — verbatim findings

```
  [high] ref-kind-inapplicable: The ref to s-cpt-r57 (kind: addresses) is inapplicable: s-cpt-r57 is a `kind: insight` signal with `Derived status: open`, but `addresses` applies when responding to a gap/question/insight or realizing a decision's commitment. The body does not respond to the insight as an open concern — it reasons from and extends it, making `grounded-in` or `builds-on` the applicable kind. The body says 'realizes the specialist layer in a Go-resident, one-codebase shape', which is advancing the insight's architecture, so `builds-on` (extending a prior conceptual step) or `grounded-in` (reasoning from it as a basis) would be correct; `addresses` is reserved for acting on a gap/question, not extending an insight.
  [high] ref-kind-inapplicable: The ref to s-cpt-z7l (kind: addresses) carries the desc 'tests instruction delivery to foreign runtimes — the skill-content half of the gap', which accurately frames this entry as acting on that gap. However, s-cpt-z7l is `kind: gap` with `Derived status: open`, so `addresses` is the correct kind here — this finding is withdrawn. No issue.
  [low] ref-kind-sharpness: The ref to d-cpt-313 (kind: related) could be sharpened: the body explicitly says 'This deliberately runs parallel to the staged two-phase directive (d-cpt-313)' and that 'the experiment verdict decides its reshaping' — this proposed entry is context d-cpt-313 must account for, and the body is not realizing d-cpt-313 but rather running alongside it. `related` is defensible as the floor here; no sharper kind clearly applies, so this is a mild observation only.
  [low] directive-bounded-check: The directive is at the `cpt` layer and its closure criterion is stated as 'the experiment returns a verdict' — but the entry itself does not state a close condition explicitly, leaving the bounded-ness implicit. [...]
pre-flight validation blocked: 2 high-severity finding(s)
```

The second `[high]` finding reasons to "the finding is withdrawn. No issue." within its own prose, yet its severity label still counted toward "2 high-severity finding(s)" and blocked the entry.

## Input (run 2)

Same entry with the legitimately flagged ref corrected (`s-cpt-r57` switched to `grounded-in`, body sentence added) and an explicit close condition appended. The `s-cpt-z7l` ref unchanged: `addresses`, same desc.

## Output (run 2) — verbatim findings

```
  [low] ref-kind-questionable: The ref on s-cpt-z7l uses kind `addresses`, but `addresses` applies when responding to a gap/question/insight or realizing a decision's commitment — this directive reasons *from* the gap as evidence that foreign agents need in-band guidance, rather than acting on it or closing it. Kind `grounded-in` would more precisely name the relationship: the gap is a premise the body cites ('as the skill-content half of the portability gap (s-cpt-z7l) predicts and this experiment directly tests') rather than a commitment being fulfilled. Both kinds are defensible here, so this is `low`.
  [low] ref-kind-questionable: The ref on d-cpt-313 uses kind `related`, but the body explicitly states this directive 'deliberately runs parallel to the staged two-phase directive (d-cpt-313)' [...] `related` is defensible. Noting it as a borderline case.
```

Entry created.

## The two leak axes in one transcript

1. **Intra-run severity/conclusion divergence** (s-prc-lbv's pattern): run 1's second `[high]` finding withdrew itself in prose but blocked anyway. The severity label is emitted structurally before/independently of where the reasoning lands, and the finding schema has no retraction path.
2. **Same-ref cross-run flip** (s-prc-pex's pattern): the identical `s-cpt-z7l` ref (same kind, same desc, near-identical body sentence) went from "addresses is the correct kind here — withdrawn" (run 1, after a high label) to "grounded-in would more precisely name the relationship" (run 2, low). Opposite recommendations across consecutive runs on the same input.
