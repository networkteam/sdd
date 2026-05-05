// Vendored from github.com/tmc/langchaingo/textsplitter (MIT). See
// LICENSE-langchaingo and NOTICE.md. Filename misspelling preserved from
// upstream so future merges from langchaingo stay diff-clean.
//
// Copyright (c) Travis Cline <travis.cline@gmail.com>

package textsplitter

// TextSplitter is the standard interface for splitting texts into string
// fragments. The MarkdownTextSplitter does not satisfy this interface —
// see SplitChunks for the chunk-bearing output. RecursiveCharacter still
// implements TextSplitter and is used internally as the secondary splitter
// for oversized markdown sections.
type TextSplitter interface {
	SplitText(text string) ([]string, error)
}
