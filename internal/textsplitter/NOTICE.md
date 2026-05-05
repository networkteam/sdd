# textsplitter — vendored from langchaingo

This package is vendored from
[`github.com/tmc/langchaingo/textsplitter`](https://github.com/tmc/langchaingo/tree/main/textsplitter)
under the MIT license (see `LICENSE-langchaingo`).

Per d-tac-85r, the package is adapted to the SDD chunker contract:

- The markdown splitter returns `[]Chunk` (text, breadcrumb array, depth,
  source-attachment path, summary/attachment flags) instead of `[]string`,
  via the new `MarkdownTextSplitter.SplitChunks` method. The `SplitText`
  string-returning interface remains for the internal recursive secondary
  splitter only.
- YAML frontmatter is stripped at the package entry point (`Splitter.Split`)
  and surfaced to the caller via `SplitOutput.Frontmatter`, not embedded.
- The original `WithHeadingHierarchy` multi-line markdown-header stack is
  replaced by an `Entry: <summary>` + `Breadcrumb: <A> > <B> > <C>` preamble
  (per d-tac-jvd) prepended to each chunk's text.
- The `CodeBlocks` default is flipped to `true`. Code blocks carry signal in
  SDD entries; dropping them silently was wrong for our content.
- Heading-only sections (no body content) emit no chunk, replacing the
  upstream behavior of emitting a chunk containing just the heading.
- Token-based defaults (`_defaultTokenModelName`, `_defaultTokenEncoding`,
  `_defaultTokenChunkSize`) are not vendored; see `options.go` for the
  rune-count chunk size we use instead. The `tiktoken-go` dependency is
  consequently not pulled in by this package.
- Helpers (`mergeSplits`, `shouldPop`, `joinDocs`, `maybePrintWarning`) live
  inline at the bottom of `recursive_character.go`; the upstream
  `split_documents.go` is not vendored because its `SplitDocuments` /
  `CreateDocuments` helpers depend on `langchaingo/schema` which we do not
  pull in.
- Heading splits cover h1–h6 (per d-tac-jvd, generalizing from the upstream
  behavior which already handled all six levels but where the SDD plan AC
  initially called only for h2–h6). The behavior here is naturally h1–h6.
