// Vendored from github.com/tmc/langchaingo/textsplitter (MIT). See
// LICENSE-langchaingo and NOTICE.md.
//
// Copyright (c) Travis Cline <travis.cline@gmail.com>
//
// SDD adaptations:
//   - SplitText is replaced by SplitChunks, which returns []rawChunk —
//     each carrying body text, the heading-chain breadcrumb as a []string,
//     and the deepest heading depth — instead of strings with inlined
//     heading prefixes. The high-level splitter (splitter.go) composes the
//     final "Entry: <summary>\nBreadcrumb: <chain>\n\n<body>" preamble per
//     d-tac-jvd, so heading hierarchy is no longer inlined into chunk text
//     here.
//   - Heading-only sections (no body content) emit no chunk. The upstream
//     behavior was to emit a chunk containing just the heading title; this
//     pollutes the embedded text without adding signal.
//   - h1–h6 are all split boundaries (per d-tac-jvd). Upstream already
//     branches on every HeadingOpen regardless of HLevel, so this is
//     naturally satisfied — verified by unit tests.

package textsplitter

import (
	"fmt"
	"reflect"
	"strings"

	"gitlab.com/golang-commonmark/markdown"
)

// NewMarkdownTextSplitter creates a new Markdown text splitter.
func NewMarkdownTextSplitter(opts ...Option) *MarkdownTextSplitter {
	options := DefaultOptions()
	for _, o := range opts {
		o(&options)
	}

	sp := &MarkdownTextSplitter{
		ChunkSize:      options.ChunkSize,
		ChunkOverlap:   options.ChunkOverlap,
		SecondSplitter: options.SecondSplitter,
		CodeBlocks:     options.CodeBlocks,
		ReferenceLinks: options.ReferenceLinks,
		JoinTableRows:  options.JoinTableRows,
		LenFunc:        options.LenFunc,
	}

	if sp.SecondSplitter == nil {
		sp.SecondSplitter = NewRecursiveCharacter(
			WithChunkSize(options.ChunkSize),
			WithChunkOverlap(options.ChunkOverlap),
			WithSeparators([]string{
				"\n\n", // paragraph
				"\n",   // line
				" ",    // space
			}),
			WithLenFunc(options.LenFunc),
		)
	}

	return sp
}

// MarkdownTextSplitter parses CommonMark markdown via the
// gitlab.com/golang-commonmark/markdown tokenizer and splits the input on
// h1–h6 heading boundaries, recursively re-splitting oversized sections via
// SecondSplitter. Output chunks carry breadcrumb metadata (the heading-chain
// as []string) and depth (the deepest heading level encountered). The
// caller is responsible for composing any Entry/Breadcrumb preamble — see
// splitter.go.
type MarkdownTextSplitter struct {
	ChunkSize      int
	ChunkOverlap   int
	SecondSplitter TextSplitter
	CodeBlocks     bool
	ReferenceLinks bool
	JoinTableRows  bool
	LenFunc        func(string) int
}

// rawChunk is the per-section unit emitted by SplitChunks: a body string and
// the breadcrumb / depth captured at emit time. The high-level splitter
// wraps each rawChunk into a public Chunk with the Entry/Breadcrumb preamble
// and provenance metadata (IsAttachment, SourceAttachmentPath).
type rawChunk struct {
	Body       string
	Breadcrumb []string
	Depth      int
}

// SplitChunks tokenizes the input markdown and emits rawChunks split on
// h1–h6 heading boundaries.
func (sp MarkdownTextSplitter) SplitChunks(text string) ([]rawChunk, error) {
	mdParser := markdown.New(markdown.XHTMLOutput(true))
	tokens := mdParser.Parse([]byte(text))

	mc := &markdownContext{
		startAt:          0,
		endAt:            len(tokens),
		tokens:           tokens,
		chunkSize:        sp.ChunkSize,
		chunkOverlap:     sp.ChunkOverlap,
		secondSplitter:   sp.SecondSplitter,
		renderCodeBlocks: sp.CodeBlocks,
		useInlineContent: !sp.ReferenceLinks,
		joinTableRows:    sp.JoinTableRows,
		breadcrumbStack:  []string{},
		lenFunc:          sp.LenFunc,
	}

	return mc.splitText(), nil
}

// markdownContext is the splitter's per-call state. Its lifetime is bounded
// by a single SplitChunks invocation.
type markdownContext struct {
	startAt int
	endAt   int
	tokens  []markdown.Token

	// breadcrumbStack tracks the current heading hierarchy as text-only
	// entries (no `#` markers). Index 0 is h1, index 1 is h2, etc. Empty
	// strings fill gaps when a deeper heading appears without an explicit
	// parent (the upstream behavior is preserved).
	breadcrumbStack []string
	// depth is the deepest non-empty heading level currently in scope. 0
	// means "no heading seen yet" (pre-heading body).
	depth int

	orderedList bool
	bulletList  bool
	listOrder   int

	indentLevel int

	chunks     []rawChunk
	curSnippet string

	chunkSize      int
	chunkOverlap   int
	secondSplitter TextSplitter

	renderCodeBlocks bool
	useInlineContent bool
	joinTableRows    bool

	lenFunc func(string) int
}

// splitText walks the token stream, dispatching by token type. The cursor
// (startAt) advances as a side effect of the per-token handlers.
//
//nolint:cyclop
func (mc *markdownContext) splitText() []rawChunk {
	for idx := mc.startAt; idx < mc.endAt; {
		token := mc.tokens[idx]
		switch token.(type) {
		case *markdown.HeadingOpen:
			mc.onMDHeader()
		case *markdown.TableOpen:
			mc.onMDTable()
		case *markdown.ParagraphOpen:
			mc.onMDParagraph()
		case *markdown.BlockquoteOpen:
			mc.onMDQuote()
		case *markdown.BulletListOpen:
			mc.onMDBulletList()
		case *markdown.OrderedListOpen:
			mc.onMDOrderedList()
		case *markdown.ListItemOpen:
			mc.onMDListItem()
		case *markdown.CodeBlock:
			mc.onMDCodeBlock()
		case *markdown.Fence:
			mc.onMDFence()
		case *markdown.Hr:
			mc.onMDHr()
		default:
			mc.startAt = indexOfCloseTag(mc.tokens, idx) + 1
		}
		idx = mc.startAt
	}

	mc.applyToChunks()
	return mc.chunks
}

// clone returns a markdownContext over a sub-slice of tokens with the same
// configuration. Used for nested structures (blockquote, list items) that
// recursively re-tokenize a portion of the document.
func (mc *markdownContext) clone(startAt, endAt int) *markdownContext {
	subTokens := mc.tokens[startAt : endAt+1]
	return &markdownContext{
		endAt:  len(subTokens),
		tokens: subTokens,

		breadcrumbStack: append([]string(nil), mc.breadcrumbStack...),
		depth:           mc.depth,

		orderedList: mc.orderedList,
		bulletList:  mc.bulletList,
		listOrder:   mc.listOrder,
		indentLevel: mc.indentLevel,

		chunkSize:      mc.chunkSize,
		chunkOverlap:   mc.chunkOverlap,
		secondSplitter: mc.secondSplitter,

		renderCodeBlocks: mc.renderCodeBlocks,
		useInlineContent: mc.useInlineContent,
		joinTableRows:    mc.joinTableRows,

		lenFunc: mc.lenFunc,
	}
}

// onMDHeader handles h1–h6 (HeadingOpen/Inline/HeadingClose). Each heading
// finalizes any in-progress section (applyToChunks) and updates the
// breadcrumb stack to reflect the new heading's level.
func (mc *markdownContext) onMDHeader() {
	endAt := indexOfCloseTag(mc.tokens, mc.startAt)
	defer func() {
		mc.startAt = endAt + 1
	}()

	header, ok := mc.tokens[mc.startAt].(*markdown.HeadingOpen)
	if !ok {
		return
	}

	inline, ok := mc.tokens[mc.startAt+1].(*markdown.Inline)
	if !ok {
		return
	}

	// Heading boundaries close the prior section. Heading-only sections
	// produce no chunk: applyToChunks emits only when curSnippet is
	// non-empty after the SDD adaptation (see implementation).
	mc.applyToChunks()

	// Pad the breadcrumb stack up to the current heading level so an h3
	// without an h2 parent still indexes correctly. Truncate any deeper
	// entries — descending levels reset their children, ascending levels
	// inherit parents.
	for len(mc.breadcrumbStack) < header.HLevel {
		mc.breadcrumbStack = append(mc.breadcrumbStack, "")
	}
	mc.breadcrumbStack = append(mc.breadcrumbStack[:header.HLevel-1], inline.Content)
	mc.depth = header.HLevel
}

// onMDParagraph handles ParagraphOpen/Inline/ParagraphClose.
func (mc *markdownContext) onMDParagraph() {
	endAt := indexOfCloseTag(mc.tokens, mc.startAt)
	defer func() {
		mc.startAt = endAt + 1
	}()

	inline, ok := mc.tokens[mc.startAt+1].(*markdown.Inline)
	if !ok {
		return
	}

	mc.joinSnippet(mc.splitInline(inline))
}

// onMDQuote handles BlockquoteOpen/.../BlockquoteClose.
func (mc *markdownContext) onMDQuote() {
	endAt := indexOfCloseTag(mc.tokens, mc.startAt)
	defer func() {
		mc.startAt = endAt + 1
	}()

	if _, ok := mc.tokens[mc.startAt].(*markdown.BlockquoteOpen); !ok {
		return
	}

	tmpMC := mc.clone(mc.startAt+1, endAt-1)
	// Nested context inherits breadcrumb but re-emits its own chunks; the
	// outer context absorbs the inner result into its current snippet.
	tmpMC.breadcrumbStack = nil
	tmpMC.depth = 0
	chunks := tmpMC.splitText()

	for _, chunk := range chunks {
		mc.joinSnippet(formatWithIndent(chunk.Body, "> "))
	}

	mc.applyToChunks()
}

// onMDBulletList handles BulletListOpen/.../BulletListClose.
func (mc *markdownContext) onMDBulletList() {
	mc.bulletList = true
	mc.orderedList = false
	mc.onMDList()
}

// onMDOrderedList handles OrderedListOpen/.../OrderedListClose.
func (mc *markdownContext) onMDOrderedList() {
	mc.orderedList = true
	mc.bulletList = false
	mc.listOrder = 0
	mc.onMDList()
}

// onMDList recursively handles list items, indenting nested lists.
func (mc *markdownContext) onMDList() {
	endAt := indexOfCloseTag(mc.tokens, mc.startAt)
	defer func() {
		mc.startAt = endAt + 1
		mc.indentLevel--
	}()

	mc.indentLevel++
	mc.startAt++

	tempMD := mc.clone(mc.startAt, endAt-1)
	tempChunk := tempMD.splitText()
	for _, chunk := range tempChunk {
		body := chunk.Body
		if tempMD.indentLevel > 1 {
			body = formatWithIndent(body, "  ")
		}
		mc.joinSnippet(body)
	}
}

// onMDListItem handles ListItemOpen/.../ListItemClose.
func (mc *markdownContext) onMDListItem() {
	endAt := indexOfCloseTag(mc.tokens, mc.startAt)
	defer func() {
		mc.startAt = endAt + 1
	}()

	mc.startAt++

	for mc.startAt < endAt-1 {
		nextToken := mc.tokens[mc.startAt]
		switch nextToken.(type) {
		case *markdown.ParagraphOpen:
			mc.onMDListItemParagraph()
		case *markdown.BulletListOpen:
			mc.onMDBulletList()
		case *markdown.OrderedListOpen:
			mc.onMDOrderedList()
		default:
			mc.startAt++
		}
	}

	mc.applyToChunks()
}

// onMDListItemParagraph handles a paragraph nested inside a list item.
func (mc *markdownContext) onMDListItemParagraph() {
	endAt := indexOfCloseTag(mc.tokens, mc.startAt)
	defer func() {
		mc.startAt = endAt + 1
	}()

	inline, ok := mc.tokens[mc.startAt+1].(*markdown.Inline)
	if !ok {
		return
	}

	line := mc.splitInline(inline)
	if mc.orderedList {
		mc.listOrder++
		line = fmt.Sprintf("%d. %s", mc.listOrder, line)
	}
	if mc.bulletList {
		line = fmt.Sprintf("- %s", line)
	}

	mc.joinSnippet(line)
}

// onMDTable handles TableOpen/.../TableClose. Tables are split row-by-row
// unless joinTableRows is true.
func (mc *markdownContext) onMDTable() {
	endAt := indexOfCloseTag(mc.tokens, mc.startAt)
	defer func() {
		mc.startAt = endAt + 1
	}()

	if _, ok := mc.tokens[mc.startAt+1].(*markdown.TheadOpen); !ok {
		return
	}

	mc.startAt++
	header := mc.onTableHeader()
	bodies := mc.onTableBody()
	mc.splitTableRows(header, bodies)
}

func (mc *markdownContext) splitTableRows(header []string, bodies [][]string) {
	headerNotEmpty := false
	for _, h := range header {
		if h != "" {
			headerNotEmpty = true
			break
		}
	}
	if !headerNotEmpty && len(bodies) != 0 {
		header = bodies[0]
		bodies = bodies[1:]
	}

	headerMD := tableHeaderInMarkdown(header)
	if len(bodies) == 0 {
		mc.joinSnippet(headerMD)
		mc.applyToChunks()
		return
	}

	for _, row := range bodies {
		line := tableRowInMarkdown(row)
		if len(mc.curSnippet) == 0 || mc.lenFunc(mc.curSnippet+line) >= mc.chunkSize {
			line = fmt.Sprintf("%s\n%s", headerMD, line)
		}
		mc.joinSnippet(line)
		if !mc.joinTableRows {
			mc.applyToChunks()
		}
	}
}

func (mc *markdownContext) onTableHeader() []string {
	endAt := indexOfCloseTag(mc.tokens, mc.startAt)
	defer func() {
		mc.startAt = endAt + 1
	}()

	if _, ok := mc.tokens[mc.startAt+1].(*markdown.TrOpen); !ok {
		return []string{}
	}

	var headers []string
	mc.startAt++

	for {
		if _, ok := mc.tokens[mc.startAt+1].(*markdown.ThOpen); !ok {
			break
		}
		mc.startAt++
		mc.startAt++
		inline, ok := mc.tokens[mc.startAt].(*markdown.Inline)
		if !ok {
			break
		}
		headers = append(headers, inline.Content)
		mc.startAt++
	}

	return headers
}

func (mc *markdownContext) onTableBody() [][]string {
	endAt := indexOfCloseTag(mc.tokens, mc.startAt)
	defer func() {
		mc.startAt = endAt + 1
	}()

	var rows [][]string

	for {
		if _, ok := mc.tokens[mc.startAt+1].(*markdown.TrOpen); !ok {
			return rows
		}
		var row []string
		mc.startAt++
		colIdx := 0
		for {
			if _, ok := mc.tokens[mc.startAt+1].(*markdown.TdOpen); !ok {
				break
			}
			mc.startAt++
			mc.startAt++
			inline, ok := mc.tokens[mc.startAt].(*markdown.Inline)
			if !ok {
				break
			}
			row = append(row, inline.Content)
			mc.startAt++
			colIdx++
		}
		rows = append(rows, row)
		mc.startAt++
	}
}

// onMDCodeBlock handles indented code blocks.
func (mc *markdownContext) onMDCodeBlock() {
	defer func() { mc.startAt++ }()

	if !mc.renderCodeBlocks {
		return
	}
	codeblock, ok := mc.tokens[mc.startAt].(*markdown.CodeBlock)
	if !ok {
		return
	}
	codeblockMD := "\n" + formatWithIndent(codeblock.Content, strings.Repeat(" ", 4))
	mc.joinSnippet(codeblockMD)
}

// onMDFence handles fenced code blocks.
func (mc *markdownContext) onMDFence() {
	defer func() { mc.startAt++ }()

	if !mc.renderCodeBlocks {
		return
	}
	fence, ok := mc.tokens[mc.startAt].(*markdown.Fence)
	if !ok {
		return
	}
	fenceMD := fmt.Sprintf("\n```%s\n%s```\n", fence.Params, fence.Content)
	mc.joinSnippet(fenceMD)
}

// onMDHr handles thematic breaks.
func (mc *markdownContext) onMDHr() {
	defer func() { mc.startAt++ }()
	if _, ok := mc.tokens[mc.startAt].(*markdown.Hr); !ok {
		return
	}
	mc.joinSnippet("\n---")
}

// joinSnippet appends to the current accumulator, applying when adding the
// snippet would exceed the chunk size.
func (mc *markdownContext) joinSnippet(snippet string) {
	if mc.curSnippet == "" {
		mc.curSnippet = snippet
		return
	}
	if mc.lenFunc(mc.curSnippet+snippet) >= mc.chunkSize {
		mc.applyToChunks()
		mc.curSnippet = snippet
	} else {
		mc.curSnippet = fmt.Sprintf("%s\n%s", mc.curSnippet, snippet)
	}
}

// applyToChunks finalizes the current section. SDD divergence: heading-only
// sections (no body) emit nothing — upstream emitted a chunk containing
// just the heading. Oversized sections are recursively re-split via the
// secondary splitter; the resulting sub-chunks share the same breadcrumb
// snapshot (no overlap across heading boundaries).
func (mc *markdownContext) applyToChunks() {
	defer func() {
		mc.curSnippet = ""
	}()

	if mc.curSnippet == "" {
		return
	}

	var bodies []string
	if mc.lenFunc(mc.curSnippet) <= mc.chunkSize+mc.chunkOverlap {
		bodies = []string{mc.curSnippet}
	} else {
		bodies, _ = mc.secondSplitter.SplitText(mc.curSnippet)
	}

	breadcrumb := nonEmptyCopy(mc.breadcrumbStack)
	depth := mc.depth

	for _, body := range bodies {
		if body == "" {
			continue
		}
		mc.chunks = append(mc.chunks, rawChunk{
			Body:       body,
			Breadcrumb: breadcrumb,
			Depth:      depth,
		})
	}
}

// splitInline renders inline content (Link/Image/Text and friends) back to
// markdown. Preserved verbatim from upstream.
//
//nolint:cyclop
func (mc *markdownContext) splitInline(inline *markdown.Inline) string {
	if len(inline.Children) == 0 || mc.useInlineContent {
		return inline.Content
	}

	var content string
	var currentLink *markdown.LinkOpen

	for _, child := range inline.Children {
		switch token := child.(type) {
		case *markdown.Softbreak:
			content += "\n"
		case *markdown.Hardbreak:
			content += "\\\n"
		case *markdown.StrongOpen, *markdown.StrongClose:
			content += "**"
		case *markdown.EmphasisOpen, *markdown.EmphasisClose:
			content += "*"
		case *markdown.StrikethroughOpen, *markdown.StrikethroughClose:
			content += "~~"
		case *markdown.Text:
			content += token.Content
		case *markdown.HTMLInline:
			content += token.Content
		case *markdown.CodeInline:
			content += fmt.Sprintf("`%s`", token.Content)
		case *markdown.LinkOpen:
			content += "["
			currentLink = token
		case *markdown.LinkClose:
			content += mc.inlineOnLinkClose(currentLink)
		case *markdown.Image:
			content += mc.inlineOnImage(token)
		}
	}
	return content
}

func (mc *markdownContext) inlineOnLinkClose(link *markdown.LinkOpen) string {
	switch {
	case link.Href == "":
		return "]()"
	case link.Title != "":
		return fmt.Sprintf(`](%s "%s")`, link.Href, link.Title)
	default:
		return fmt.Sprintf(`](%s)`, link.Href)
	}
}

func (mc *markdownContext) inlineOnImage(image *markdown.Image) string {
	var label string
	for _, token := range image.Tokens {
		if text, ok := token.(*markdown.Text); ok {
			label += text.Content
		}
	}
	if image.Title == "" {
		return fmt.Sprintf(`![%s](%s)`, label, image.Src)
	}
	return fmt.Sprintf(`![%s](%s "%s")`, label, image.Src, image.Title)
}

// closeTypes records the close-tag type for each open-tag type. Tokens
// without an explicit close (Hr, Fence) are not in the map.
var closeTypes = map[reflect.Type]reflect.Type{ //nolint:gochecknoglobals
	reflect.TypeOf(&markdown.HeadingOpen{}):     reflect.TypeOf(&markdown.HeadingClose{}),
	reflect.TypeOf(&markdown.BulletListOpen{}):  reflect.TypeOf(&markdown.BulletListClose{}),
	reflect.TypeOf(&markdown.OrderedListOpen{}): reflect.TypeOf(&markdown.OrderedListClose{}),
	reflect.TypeOf(&markdown.ParagraphOpen{}):   reflect.TypeOf(&markdown.ParagraphClose{}),
	reflect.TypeOf(&markdown.BlockquoteOpen{}):  reflect.TypeOf(&markdown.BlockquoteClose{}),
	reflect.TypeOf(&markdown.ListItemOpen{}):    reflect.TypeOf(&markdown.ListItemClose{}),
	reflect.TypeOf(&markdown.TableOpen{}):       reflect.TypeOf(&markdown.TableClose{}),
	reflect.TypeOf(&markdown.TheadOpen{}):       reflect.TypeOf(&markdown.TheadClose{}),
	reflect.TypeOf(&markdown.TbodyOpen{}):       reflect.TypeOf(&markdown.TbodyClose{}),
}

// indexOfCloseTag returns the index of the matching close tag for the open
// tag at startAt, balancing nesting of the same open type.
func indexOfCloseTag(tokens []markdown.Token, startAt int) int {
	sameCount := 0
	openType := reflect.ValueOf(tokens[startAt]).Type()
	closeType := closeTypes[openType]
	if closeType == nil {
		return startAt
	}

	idx := startAt + 1
	for ; idx < len(tokens); idx++ {
		cur := reflect.ValueOf(tokens[idx]).Type()
		if openType == cur {
			sameCount++
		}
		if closeType == cur {
			if sameCount == 0 {
				break
			}
			sameCount--
		}
	}
	return idx
}

// formatWithIndent prefixes every line of value with mark.
func formatWithIndent(value, mark string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = fmt.Sprintf("%s%s", mark, line)
	}
	return strings.Join(lines, "\n")
}

// tableHeaderInMarkdown renders a table header row plus a separator row.
func tableHeaderInMarkdown(header []string) string {
	headerMD := tableRowInMarkdown(header)
	var separators []string
	for i := 0; i < len(header); i++ {
		separators = append(separators, "---")
	}
	headerMD += "\n"
	headerMD += tableRowInMarkdown(separators)
	return headerMD
}

// tableRowInMarkdown renders a single table row.
func tableRowInMarkdown(row []string) string {
	var line string
	for i := range row {
		line += fmt.Sprintf("| %s ", row[i])
		if i == len(row)-1 {
			line += "|"
		}
	}
	return line
}

// nonEmptyCopy returns a fresh slice with empty strings filtered out. Used
// when snapshotting the breadcrumb stack so consumers receive only
// real headings, not the padding for skipped levels (e.g. an h3 without an
// h2 parent).
func nonEmptyCopy(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
