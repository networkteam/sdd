# Format example set

Drafts carrying characters that broke the checks' JSON output before. The
format evals (`TestWritingGuideEval_Format`, `TestPreflightEval_Format`) run
each file through the operations it lists and require every response to
parse. One specimen per file; external material is anonymized.

Fields: `id` (the file name without extension), `origin` (where the case
comes from), `class` (the characters under test), `language` (the configured
graph language for pre-flight; the writing guide takes none), `operations`
(`writing-guide`, `preflight`), `graph` (entries the draft's refs resolve to)
and `draft` (`type`, `kind`, `layer`, `refs`, `content`).
