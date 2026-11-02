# Eval specimens

Drafts with characters that broke the checks' JSON output before, each with
the content a response must carry. `TestWritingGuideEval_Specimens` and
`TestPreflightEval_Specimens` run them. One specimen per file; external
material is anonymized.

Fields: `id` (the file name without extension), `origin` (where the case
comes from), `class` (the characters it carries), `language` (the configured
graph language for pre-flight), `operation` (`writing-guide` or `preflight`),
`expect`, `graph` (entries the draft's refs resolve to) and `draft` (`type`,
`kind`, `layer`, `refs`, `content`).

`expect` names a finding and the draft passage it must point to: `axis` for
the writing guide, `severity` for pre-flight, and `passage`, which the
finding's quote, or an excerpt its observation cites, must locate. Instead,
`no_blocking: true` requires pre-flight to return no high finding.
