package textsplitter

// Chunk is one unit of text destined for embedding, carrying the metadata
// needed to filter, score, and cite it. The Text field already includes the
// `Entry: <summary>\nBreadcrumb: <chain>\n\n<body>` preamble (per
// d-tac-jvd) — embedders consume Text directly.
//
// Breadcrumb is the heading-chain in array form (e.g.
// ["Approach", "Storage"]) without `#` markers. Depth is the deepest
// heading level the chunk sits under (0 means pre-heading body).
//
// IsSummary, IsAttachment, and SourceAttachmentPath are provenance flags
// the indexer uses for filtering and citation rendering.
type Chunk struct {
	Text                 string
	Breadcrumb           []string
	Depth                int
	IsSummary            bool
	IsAttachment         bool
	SourceAttachmentPath string
	// Body is the section text without the Entry/Breadcrumb preamble. Useful
	// for citation snippets (the user-facing surface) where the preamble
	// would be noise.
	Body string
}
