// Package textsplitter chunks markdown documents into metadata-bearing
// units suitable for embedding. It is vendored from
// github.com/tmc/langchaingo/textsplitter (MIT, see LICENSE-langchaingo)
// and adapted to the SDD chunker contract — see NOTICE.md for the full
// list of divergences from upstream.
//
// Two splitters are exposed:
//
//   - MarkdownTextSplitter: parses CommonMark, splits on h1–h6 headings,
//     recursively re-splits oversized sections via a second splitter, and
//     emits []Chunk with breadcrumb (heading-chain array), depth, and
//     IsSummary / IsAttachment flags. The TextSplitter string interface is
//     not satisfied; use SplitChunks for the chunk-bearing output.
//
//   - RecursiveCharacter: splits on a separator hierarchy with overlap
//     control. Used internally as the second splitter for oversized
//     sections; satisfies the original TextSplitter string interface.
//
// The high-level entry point is Splitter.Split, which strips YAML
// frontmatter, prepends an "Entry: <summary>" + "Breadcrumb: <chain>"
// preamble to each chunk's text per d-tac-jvd, and tags chunks with
// attachment provenance.
package textsplitter
