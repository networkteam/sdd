package chunking_test

import (
	"context"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/textsplitter"
)

// mapAttachmentReader serves attachment bytes from an in-memory map keyed by
// graph-relative path — the test stand-in for both the disk reader and the
// GraphStore-paging reader.
type mapAttachmentReader map[string][]byte

func (m mapAttachmentReader) ReadAttachment(_ context.Context, _ *model.Entry, relPath string) ([]byte, error) {
	return m[relPath], nil
}

func testEntry() *model.Entry {
	return &model.Entry{
		ID:          "20260101-100000-s-tac-aaa",
		Type:        model.TypeSignal,
		Layer:       model.LayerTactical,
		Summary:     "A summary of the entry.",
		Content:     "## Section\nThe entry body discusses the topic.",
		Attachments: []string{"2026/01/01-100000-s-tac-aaa/design.md", "2026/01/01-100000-s-tac-aaa/diagram.png"},
	}
}

func TestDeriveChunksCoversSummaryBodyAndMarkdownAttachments(t *testing.T) {
	entry := testEntry()
	reader := mapAttachmentReader{
		"2026/01/01-100000-s-tac-aaa/design.md":   []byte("# Design\n\nDesign detail about the topic."),
		"2026/01/01-100000-s-tac-aaa/diagram.png": []byte{0x89, 'P', 'N', 'G'},
	}
	chunks, err := chunking.DeriveChunks(context.Background(), entry, "hash8abc", textsplitter.NewSplitter(), reader)
	if err != nil {
		t.Fatalf("DeriveChunks: %v", err)
	}

	var haveSummary, haveBody, haveAttachment bool
	for _, c := range chunks {
		switch {
		case c.Chunk.IsSummary:
			haveSummary = true
		case c.Chunk.IsAttachment:
			haveAttachment = true
			if c.Chunk.SourceAttachmentPath != "2026/01/01-100000-s-tac-aaa/design.md" {
				t.Errorf("attachment chunk source = %q, want the .md attachment", c.Chunk.SourceAttachmentPath)
			}
			if !strings.HasPrefix(c.ChunkID, entry.ID+"#v-hash8abc#attach-") {
				t.Errorf("attachment chunk ID = %q, want a versioned #attach- id", c.ChunkID)
			}
		default:
			haveBody = true
		}
	}
	if !haveSummary || !haveBody || !haveAttachment {
		t.Fatalf("missing chunk kind: summary=%v body=%v attachment=%v", haveSummary, haveBody, haveAttachment)
	}
	// The non-markdown attachment must not produce chunks.
	for _, c := range chunks {
		if strings.Contains(c.Chunk.Text, "PNG") {
			t.Error("non-markdown attachment leaked into chunks")
		}
	}
}

func TestEntryStateHashDependsOnAttachmentBytes(t *testing.T) {
	entry := testEntry()
	base := mapAttachmentReader{
		"2026/01/01-100000-s-tac-aaa/design.md":   []byte("original"),
		"2026/01/01-100000-s-tac-aaa/diagram.png": []byte("v1"),
	}
	changed := mapAttachmentReader{
		"2026/01/01-100000-s-tac-aaa/design.md":   []byte("original"),
		"2026/01/01-100000-s-tac-aaa/diagram.png": []byte("v2"),
	}
	h1, err := chunking.EntryStateHash(context.Background(), entry, base)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := chunking.EntryStateHash(context.Background(), entry, base)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q vs %q", h1, h2)
	}
	h3, err := chunking.EntryStateHash(context.Background(), entry, changed)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h3 {
		t.Error("hash did not change when a (non-markdown) attachment's bytes changed")
	}
}

func TestIncludeEntry(t *testing.T) {
	embedded := &model.Entry{Embedded: true}
	onDisk := &model.Entry{}
	if !chunking.IncludeEntry(embedded, false) {
		t.Error("base store must include embedded entries")
	}
	if chunking.IncludeEntry(embedded, true) {
		t.Error("connected store must exclude embedded entries")
	}
	if !chunking.IncludeEntry(onDisk, true) {
		t.Error("on-disk entries are always included")
	}
}
