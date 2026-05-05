// Tests adapted from github.com/tmc/langchaingo/textsplitter (MIT). See
// LICENSE-langchaingo and NOTICE.md.
//
// Adapted to the SDD chunker contract: the splitter emits []rawChunk with
// body / breadcrumb / depth instead of strings with inlined heading
// markers. Heading-only sections produce no chunk. h1–h6 are all split
// boundaries (verified explicitly).

package textsplitter

import (
	"reflect"
	"testing"
)

// expectedChunk mirrors rawChunk for test ergonomics — the unexported type
// is awkward in test code without an exported equivalent.
type expectedChunk struct {
	Body       string
	Breadcrumb []string
	Depth      int
}

func splitMarkdown(t *testing.T, opts []Option, input string) []rawChunk {
	t.Helper()
	got, err := NewMarkdownTextSplitter(opts...).SplitChunks(input)
	if err != nil {
		t.Fatalf("SplitChunks: %v", err)
	}
	return got
}

func assertChunks(t *testing.T, got []rawChunk, want []expectedChunk) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("chunk count: got %d, want %d\ngot: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Body != want[i].Body {
			t.Errorf("chunk %d body:\n  got  %q\n  want %q", i, got[i].Body, want[i].Body)
		}
		if !reflect.DeepEqual(got[i].Breadcrumb, want[i].Breadcrumb) && !(len(got[i].Breadcrumb) == 0 && len(want[i].Breadcrumb) == 0) {
			t.Errorf("chunk %d breadcrumb: got %#v, want %#v", i, got[i].Breadcrumb, want[i].Breadcrumb)
		}
		if got[i].Depth != want[i].Depth {
			t.Errorf("chunk %d depth: got %d, want %d", i, got[i].Depth, want[i].Depth)
		}
	}
}

// TestMarkdownTextSplitter_HeaderSplit covers the upstream
// TestMarkdownHeaderTextSplitter_SplitText case adapted to the SDD output
// shape: bodies without inlined headings, breadcrumb as a structured
// []string, and heading-only sections producing no chunk.
func TestMarkdownTextSplitter_HeaderSplit(t *testing.T) {
	t.Parallel()

	input := `
## First header: h2
Some content below the first h2.
## Second header: h2
### Third header: h3

- This is a list item of bullet type.
- This is another list item.

 *Everything* is going according to **plan**.

# Fourth header: h1
Some content below the first h1.
## Fifth header: h2
#### Sixth header: h4

Some content below h1>h2>h4.
`

	got := splitMarkdown(t, []Option{WithChunkSize(64), WithChunkOverlap(32)}, input)

	want := []expectedChunk{
		// Body under first h2.
		{Body: "Some content below the first h2.", Breadcrumb: []string{"First header: h2"}, Depth: 2},
		// Heading-only "## Second header: h2" emits nothing — adapter
		// drops the upstream behavior of emitting the bare heading.
		// First list item under h3.
		{Body: "- This is a list item of bullet type.", Breadcrumb: []string{"Second header: h2", "Third header: h3"}, Depth: 3},
		{Body: "- This is another list item.", Breadcrumb: []string{"Second header: h2", "Third header: h3"}, Depth: 3},
		// Paragraph after the bullet list, still under h3.
		{Body: "*Everything* is going according to **plan**.", Breadcrumb: []string{"Second header: h2", "Third header: h3"}, Depth: 3},
		// h1 split — depth 1, single-element breadcrumb.
		{Body: "Some content below the first h1.", Breadcrumb: []string{"Fourth header: h1"}, Depth: 1},
		// h4 with missing h3 parent — nonEmptyCopy filters the
		// padding entry so the breadcrumb is the meaningful chain.
		{Body: "Some content below h1>h2>h4.", Breadcrumb: []string{"Fourth header: h1", "Fifth header: h2", "Sixth header: h4"}, Depth: 4},
	}

	assertChunks(t, got, want)
}

// TestMarkdownTextSplitter_AllHeadingLevels confirms h1–h6 are all split
// boundaries (per d-tac-jvd's generalization from h2–h6 in the original
// plan AC).
func TestMarkdownTextSplitter_AllHeadingLevels(t *testing.T) {
	t.Parallel()

	input := `# h1
body 1
## h2
body 2
### h3
body 3
#### h4
body 4
##### h5
body 5
###### h6
body 6
`

	got := splitMarkdown(t, []Option{WithChunkSize(64), WithChunkOverlap(16)}, input)

	if len(got) != 6 {
		t.Fatalf("expected 6 chunks (one per heading level), got %d", len(got))
	}
	for i, depth := range []int{1, 2, 3, 4, 5, 6} {
		if got[i].Depth != depth {
			t.Errorf("chunk %d: expected depth %d, got %d (body %q)", i, depth, got[i].Depth, got[i].Body)
		}
	}
}

// TestMarkdownTextSplitter_HeadingOnlySectionsProduceNoChunk is a SDD-only
// test — upstream emits a chunk containing just the heading text when a
// heading has no body. The plan AC says heading-only sections must produce
// no chunk.
func TestMarkdownTextSplitter_HeadingOnlySectionsProduceNoChunk(t *testing.T) {
	t.Parallel()

	input := `## Heading with body
content here
## Heading without body
## Another heading without body
### Nested heading without body
## Heading with body again
more content
`

	got := splitMarkdown(t, []Option{WithChunkSize(64), WithChunkOverlap(16)}, input)
	want := []expectedChunk{
		{Body: "content here", Breadcrumb: []string{"Heading with body"}, Depth: 2},
		{Body: "more content", Breadcrumb: []string{"Heading with body again"}, Depth: 2},
	}
	assertChunks(t, got, want)
}

// TestMarkdownTextSplitter_NoHeadings covers the plan AC "Entries without
// `##` headings produce summary + a single body chunk" at the markdown
// splitter level — the splitter alone returns one body chunk; the summary
// is composed by Splitter.Split / SummaryChunk, tested separately.
func TestMarkdownTextSplitter_NoHeadings(t *testing.T) {
	t.Parallel()

	input := `Just a plain paragraph with no headings.

A second paragraph.`

	got := splitMarkdown(t, []Option{WithChunkSize(256), WithChunkOverlap(32)}, input)
	if len(got) != 1 {
		t.Fatalf("expected single body chunk, got %d: %#v", len(got), got)
	}
	if got[0].Depth != 0 {
		t.Errorf("depth: expected 0 (pre-heading body), got %d", got[0].Depth)
	}
	if len(got[0].Breadcrumb) != 0 {
		t.Errorf("breadcrumb: expected empty, got %#v", got[0].Breadcrumb)
	}
	wantBody := "Just a plain paragraph with no headings.\nA second paragraph."
	if got[0].Body != wantBody {
		t.Errorf("body:\n  got  %q\n  want %q", got[0].Body, wantBody)
	}
}

// TestMarkdownTextSplitter_LeafScoping verifies that a parent section's
// intro paragraph emits its own chunk before any deeper-level subsection
// content does (the plan AC "leaf-scoped: subsection text excludes parent
// prose; parent intro is its own chunk").
func TestMarkdownTextSplitter_LeafScoping(t *testing.T) {
	t.Parallel()

	input := `## Parent
Parent intro paragraph.
### Child A
Child A body.
### Child B
Child B body.
`

	got := splitMarkdown(t, []Option{WithChunkSize(128), WithChunkOverlap(16)}, input)
	want := []expectedChunk{
		{Body: "Parent intro paragraph.", Breadcrumb: []string{"Parent"}, Depth: 2},
		{Body: "Child A body.", Breadcrumb: []string{"Parent", "Child A"}, Depth: 3},
		{Body: "Child B body.", Breadcrumb: []string{"Parent", "Child B"}, Depth: 3},
	}
	assertChunks(t, got, want)
}

// TestMarkdownTextSplitter_OversizedSectionRecursiveSplit covers the AC
// "Sections > 800 tokens are recursively split with ~10% overlap inside
// the section; no overlap across heading boundaries." Approximated with a
// small chunk size to keep the test compact.
func TestMarkdownTextSplitter_OversizedSectionRecursiveSplit(t *testing.T) {
	t.Parallel()

	// Two large sections separated by a heading. Each section's body
	// exceeds chunkSize; the splitter must re-split each section
	// recursively but never produce a chunk that straddles the heading
	// boundary.
	largeBodyA := "AA AA AA AA AA AA AA AA AA AA AA AA AA AA AA AA AA AA AA AA"
	largeBodyB := "BB BB BB BB BB BB BB BB BB BB BB BB BB BB BB BB BB BB BB BB"
	input := "## Section A\n" + largeBodyA + "\n## Section B\n" + largeBodyB + "\n"

	got := splitMarkdown(t, []Option{WithChunkSize(20), WithChunkOverlap(2)}, input)
	if len(got) < 2 {
		t.Fatalf("expected multiple chunks across both sections, got %d: %#v", len(got), got)
	}

	// Every chunk must cleanly belong to either Section A or Section B —
	// no chunk should contain both AA and BB tokens.
	for i, c := range got {
		hasA, hasB := containsAny(c.Body, "AA"), containsAny(c.Body, "BB")
		if hasA && hasB {
			t.Errorf("chunk %d straddles heading boundary: %q", i, c.Body)
		}
		// And every chunk's breadcrumb must reflect which section it's in.
		if hasA && (len(c.Breadcrumb) == 0 || c.Breadcrumb[0] != "Section A") {
			t.Errorf("chunk %d body has Section A content but breadcrumb is %#v", i, c.Breadcrumb)
		}
		if hasB && (len(c.Breadcrumb) == 0 || c.Breadcrumb[0] != "Section B") {
			t.Errorf("chunk %d body has Section B content but breadcrumb is %#v", i, c.Breadcrumb)
		}
	}
}

// TestMarkdownTextSplitter_BulletListNesting ports the upstream
// TestMarkdownHeaderTextSplitter_BulletList case to the new shape: list
// indentation is preserved, top-level item emits as a separate chunk from
// nested-item runs. (Upstream's chunkSize=512 keeps everything in two
// chunks; we replicate that.)
func TestMarkdownTextSplitter_BulletListNesting(t *testing.T) {
	t.Parallel()

	input := `
- [Code of Conduct](#code-of-conduct)
- [I Have a Question](#i-have-a-question)
- [I Want To Contribute](#i-want-to-contribute)
    - [Reporting Bugs](#reporting-bugs)
        - [Before Submitting a Bug Report](#before-submitting-a-bug-report)
`
	got := splitMarkdown(t, []Option{WithChunkSize(512), WithChunkOverlap(64)}, input)
	if len(got) == 0 {
		t.Fatalf("expected at least one chunk")
	}
	// We don't pin the exact chunk boundaries — list-merging is sensitive
	// to chunkSize and not the focus of this test. Just verify nested
	// items are indented in the rendered body.
	combined := ""
	for _, c := range got {
		combined += c.Body + "\n"
	}
	if !containsAny(combined, "  - [Reporting Bugs]") {
		t.Errorf("expected nested list to be indented; combined body:\n%s", combined)
	}
}

// TestMarkdownTextSplitter_CodeBlocksDefaultOn covers the SDD divergence
// from upstream: WithCodeBlocks(true) is the default, so fenced blocks
// appear in chunk bodies without needing an explicit option.
func TestMarkdownTextSplitter_CodeBlocksDefaultOn(t *testing.T) {
	t.Parallel()

	input := "example code:\n```go\nfunc main() {}\n```"
	got := splitMarkdown(t, nil, input)

	if len(got) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(got))
	}
	if !containsAny(got[0].Body, "func main()") {
		t.Errorf("expected fenced code block in body by default; got %q", got[0].Body)
	}
}

// TestMarkdownTextSplitter_CodeBlocksOptOut confirms WithCodeBlocks(false)
// drops the fenced block (matching upstream when explicitly disabled).
func TestMarkdownTextSplitter_CodeBlocksOptOut(t *testing.T) {
	t.Parallel()

	input := "example code:\n```go\nfunc main() {}\n```"
	got := splitMarkdown(t, []Option{WithCodeBlocks(false), WithChunkSize(512), WithChunkOverlap(64)}, input)

	if len(got) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(got))
	}
	if containsAny(got[0].Body, "func main()") {
		t.Errorf("expected no code block when WithCodeBlocks(false); got %q", got[0].Body)
	}
}

// TestMarkdownTextSplitter_TableRowSplit ports the upstream
// TestMarkdownHeaderTextSplitter_Table size(64)-overlap(32) case: each
// table row becomes its own chunk by default.
func TestMarkdownTextSplitter_TableRowSplit(t *testing.T) {
	t.Parallel()

	input := `| Syntax      | Description |
| ----------- | ----------- |
| Header      | Title       |
| Paragraph   | Text        |`

	got := splitMarkdown(t, []Option{WithChunkSize(64), WithChunkOverlap(32)}, input)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks (one per row), got %d: %#v", len(got), got)
	}
	if !containsAny(got[0].Body, "Header") || containsAny(got[0].Body, "Paragraph") {
		t.Errorf("first chunk should contain only Header row: %q", got[0].Body)
	}
	if !containsAny(got[1].Body, "Paragraph") || containsAny(got[1].Body, "Header") {
		t.Errorf("second chunk should contain only Paragraph row: %q", got[1].Body)
	}
}

// TestMarkdownTextSplitter_TableJoinedRows covers the JoinTableRows option
// — ported from upstream big-tables case.
func TestMarkdownTextSplitter_TableJoinedRows(t *testing.T) {
	t.Parallel()

	input := `| Syntax      | Description |
| ----------- | ----------- |
| Header      | Title       |
| Paragraph   | Text        |`

	got := splitMarkdown(t, []Option{WithChunkSize(128), WithChunkOverlap(32), WithJoinTableRows(true)}, input)
	if len(got) != 1 {
		t.Fatalf("expected 1 chunk for joined table rows, got %d: %#v", len(got), got)
	}
	if !containsAny(got[0].Body, "Header") || !containsAny(got[0].Body, "Paragraph") {
		t.Errorf("expected both rows in joined chunk: %q", got[0].Body)
	}
}

// TestMarkdownTextSplitter_InlineEmphasisAndImages ports the relevant
// portions of upstream TestMarkdownHeaderTextSplitter_SplitInline. We
// don't pin exact whitespace at chunk boundaries — that's a tokenizer
// detail — only that key inline tokens survive intact in the body.
func TestMarkdownTextSplitter_InlineEmphasisAndImages(t *testing.T) {
	t.Parallel()

	t.Run("emphasis", func(t *testing.T) {
		t.Parallel()
		got := splitMarkdown(t, []Option{WithChunkSize(512), WithChunkOverlap(64)},
			"text with *emphasis*, **strong emphasis** and ~~strikethrough~~")
		if len(got) != 1 {
			t.Fatalf("expected single chunk, got %d", len(got))
		}
		for _, want := range []string{"*emphasis*", "**strong emphasis**", "~~strikethrough~~"} {
			if !containsAny(got[0].Body, want) {
				t.Errorf("expected %q in body; got %q", want, got[0].Body)
			}
		}
	})

	t.Run("image", func(t *testing.T) {
		t.Parallel()
		got := splitMarkdown(t, []Option{WithChunkSize(512), WithChunkOverlap(64)},
			"images:\n![one](/path/to/one.png)\n![two](/path/to/two.png \"two\")")
		if len(got) != 1 {
			t.Fatalf("expected single chunk, got %d", len(got))
		}
		if !containsAny(got[0].Body, "![one](/path/to/one.png)") {
			t.Errorf("expected basic image markdown preserved; got %q", got[0].Body)
		}
		if !containsAny(got[0].Body, `![two](/path/to/two.png "two")`) {
			t.Errorf("expected titled image markdown preserved; got %q", got[0].Body)
		}
	})
}

func containsAny(s, sub string) bool {
	return len(sub) == 0 || indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
