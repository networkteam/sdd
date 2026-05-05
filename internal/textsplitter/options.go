// Vendored from github.com/tmc/langchaingo/textsplitter (MIT). See
// LICENSE-langchaingo and NOTICE.md.
//
// Copyright (c) Travis Cline <travis.cline@gmail.com>
//
// SDD adaptations:
//   - CodeBlocks default flipped to true (upstream default drops code blocks,
//     which carry signal in SDD entries — see NOTICE.md).
//   - Default chunk size / overlap moved off the tiktoken-based constants
//     in token_splitter.go (which is not vendored). The replacements use
//     rune-count units calibrated to ~800 tokens cap with ~10% overlap per
//     d-tac-lqr's chunking ACs. Approximation: 1 token ≈ 4 runes for English
//     prose.
//   - WithHeadingHierarchy is retained for upstream API parity but is unused
//     by the SDD entry point — the breadcrumb preamble is composed by
//     splitter.go from the structured Chunk.Breadcrumb instead.

package textsplitter

import "unicode/utf8"

// Default rune-count chunk size and overlap for markdown body splitting.
// Calibrated to roughly the d-tac-lqr "~800 tokens hard cap, ~10% overlap"
// targets (1 token ≈ 4 runes English prose).
const (
	DefaultChunkSize    = 3200
	DefaultChunkOverlap = 320
)

// Options is a struct that contains options for a text splitter.
type Options struct {
	ChunkSize            int
	ChunkOverlap         int
	Separators           []string
	KeepSeparator        bool
	LenFunc              func(string) int
	SecondSplitter       TextSplitter
	CodeBlocks           bool
	ReferenceLinks       bool
	KeepHeadingHierarchy bool // retained for upstream API parity; unused by the SDD entry point
	JoinTableRows        bool
}

// DefaultOptions returns the default options for all text splitters.
func DefaultOptions() Options {
	return Options{
		ChunkSize:     DefaultChunkSize,
		ChunkOverlap:  DefaultChunkOverlap,
		Separators:    []string{"\n\n", "\n", " ", ""},
		KeepSeparator: false,
		LenFunc:       utf8.RuneCountInString,

		// SDD divergence: code blocks carry signal — keep them by default.
		CodeBlocks: true,

		KeepHeadingHierarchy: false,
	}
}

// Option is a function that can be used to set options for a text splitter.
type Option func(*Options)

// WithChunkSize sets the chunk size for a text splitter.
func WithChunkSize(chunkSize int) Option {
	return func(o *Options) {
		o.ChunkSize = chunkSize
	}
}

// WithChunkOverlap sets the chunk overlap for a text splitter.
func WithChunkOverlap(chunkOverlap int) Option {
	return func(o *Options) {
		o.ChunkOverlap = chunkOverlap
	}
}

// WithSeparators sets the separators for a text splitter.
func WithSeparators(separators []string) Option {
	return func(o *Options) {
		o.Separators = separators
	}
}

// WithLenFunc sets the lenfunc for a text splitter.
func WithLenFunc(lenFunc func(string) int) Option {
	return func(o *Options) {
		o.LenFunc = lenFunc
	}
}

// WithSecondSplitter sets the second splitter for a text splitter.
func WithSecondSplitter(secondSplitter TextSplitter) Option {
	return func(o *Options) {
		o.SecondSplitter = secondSplitter
	}
}

// WithCodeBlocks sets whether indented and fenced codeblocks should be included
// in the output. SDD default is true.
func WithCodeBlocks(renderCode bool) Option {
	return func(o *Options) {
		o.CodeBlocks = renderCode
	}
}

// WithReferenceLinks sets whether reference links (i.e. `[text][label]`)
// should be patched with the url and title from their definition.
func WithReferenceLinks(referenceLinks bool) Option {
	return func(o *Options) {
		o.ReferenceLinks = referenceLinks
	}
}

// WithKeepSeparator sets whether the separators should be kept in the resulting
// split text or not.
func WithKeepSeparator(keepSeparator bool) Option {
	return func(o *Options) {
		o.KeepSeparator = keepSeparator
	}
}

// WithHeadingHierarchy is retained for upstream API parity but unused by the
// SDD entry point: breadcrumb prepending is handled by the high-level
// splitter rather than by the markdown splitter inlining heading-stack text
// into chunks.
func WithHeadingHierarchy(trackHeadingHierarchy bool) Option {
	return func(o *Options) {
		o.KeepHeadingHierarchy = trackHeadingHierarchy
	}
}

// WithJoinTableRows sets whether tables should be split by row or not.
func WithJoinTableRows(join bool) Option {
	return func(o *Options) {
		o.JoinTableRows = join
	}
}
