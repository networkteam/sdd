package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/query"
)

// fakeEmbedder produces deterministic 4-dim embeddings keyed off the
// SHA-256 of the input. Stable across runs; usable for similarity tests
// because identical inputs always produce identical vectors.
type fakeEmbedder struct {
	calls       int
	totalInputs int
	fingerprint string
}

func (f *fakeEmbedder) EmbedDocuments(_ context.Context, texts []string) ([][]float32, error) {
	return f.embed(texts)
}
func (f *fakeEmbedder) EmbedQueries(_ context.Context, texts []string) ([][]float32, error) {
	return f.embed(texts)
}
func (f *fakeEmbedder) embed(texts []string) ([][]float32, error) {
	f.calls++
	f.totalInputs += len(texts)
	out := make([][]float32, len(texts))
	for i, t := range texts {
		h := sha256.Sum256([]byte(t))
		v := make([]float32, 4)
		for j := 0; j < 4; j++ {
			b := binary.BigEndian.Uint32(h[j*4 : j*4+4])
			v[j] = float32(b) / float32(^uint32(0))
		}
		out[i] = v
	}
	return out, nil
}
func (f *fakeEmbedder) Dimensions() int { return 4 }
func (f *fakeEmbedder) BatchSize() int  { return 64 }
func (f *fakeEmbedder) Fingerprint() string {
	if f.fingerprint == "" {
		return "fake/v1/4"
	}
	return f.fingerprint
}

// readFinderFor builds a Finder the IndexHandler can use as Reader. It
// only needs LoadGraph to work; preflight stays nil and would error on
// invocation, but indexing never calls it.
func readFinderFor(t *testing.T) *finders.Finder {
	t.Helper()
	return finders.New(finders.Options{
		PreflightRunner: noopRunner{},
	})
}

type noopRunner struct{}

func (noopRunner) Run(context.Context, llm.Request) (*llm.RunResult, error) {
	return nil, fmt.Errorf("no llm runner configured")
}

func writeEntry(t *testing.T, graphDir, id, body, summary string) {
	t.Helper()
	yyyy := id[:4]
	mm := id[4:6]
	short := id[6:]
	dir := filepath.Join(graphDir, yyyy, mm)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, short+".md")
	content := "---\ntype: signal\nlayer: tactical\nkind: gap\n"
	content += "confidence: medium\nparticipants:\n  - Test\n"
	if summary != "" {
		content += "summary: |-\n  " + summary + "\n"
	}
	content += "---\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIndexHandler_Build(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()

	writeEntry(t, graphDir, "20260101-100000-s-tac-aaa", "## Section A\nFirst entry body.", "Summary of A.")
	writeEntry(t, graphDir, "20260101-100001-s-tac-bbb", "## Section B\nSecond entry body.", "Summary of B.")

	emb := &fakeEmbedder{}
	idx := index.OpenInMemory()
	h := NewIndexHandler(IndexHandlerOptions{
		GraphDir:   graphDir,
		IndexDir:   indexDir,
		Embedder:   emb,
		IndexStore: idx,
		Reader:     readFinderFor(t),
	})

	var indexed []string
	cmd := &command.BuildIndexCmd{
		OnEntryIndexed: func(id string, n int) { indexed = append(indexed, id) },
		OnEntrySkipped: func(id string) { t.Errorf("unexpected skip for %s", id) },
	}
	if err := h.Build(context.Background(), cmd); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(indexed) != 2 {
		t.Errorf("expected 2 entries indexed, got %d (%v)", len(indexed), indexed)
	}
	if emb.calls != 1 {
		t.Errorf("expected 1 cross-entry batched embed call, got %d", emb.calls)
	}
	// Each entry: 1 summary + 1 body = 2 chunks. 4 chunks total.
	if emb.totalInputs != 4 {
		t.Errorf("expected 4 chunks embedded, got %d", emb.totalInputs)
	}
	// Manifest should record both entries.
	manifest, err := index.LoadManifest(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) != 2 {
		t.Errorf("expected manifest to have 2 entries, got %d", len(manifest.Entries))
	}
}

func TestIndexHandler_BuildSkipsUnchanged(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()

	writeEntry(t, graphDir, "20260101-100000-s-tac-aaa", "body", "summary")

	emb := &fakeEmbedder{}
	idx := index.OpenInMemory()
	h := NewIndexHandler(IndexHandlerOptions{
		GraphDir:   graphDir,
		IndexDir:   indexDir,
		Embedder:   emb,
		IndexStore: idx,
		Reader:     readFinderFor(t),
	})

	if err := h.Build(context.Background(), &command.BuildIndexCmd{}); err != nil {
		t.Fatal(err)
	}
	firstCalls := emb.calls

	var skipped []string
	cmd := &command.BuildIndexCmd{
		OnEntrySkipped: func(id string) { skipped = append(skipped, id) },
		OnEntryIndexed: func(id string, n int) { t.Errorf("unexpected re-index of %s", id) },
	}
	if err := h.Build(context.Background(), cmd); err != nil {
		t.Fatalf("second Build: %v", err)
	}
	if len(skipped) != 1 {
		t.Errorf("expected 1 skip on second build, got %d (%v)", len(skipped), skipped)
	}
	if emb.calls != firstCalls {
		t.Errorf("second build should not call embedder when nothing changed (got %d additional calls)", emb.calls-firstCalls)
	}
}

func TestIndexHandler_BuildForceReindexes(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()

	writeEntry(t, graphDir, "20260101-100000-s-tac-aaa", "body", "summary")

	emb := &fakeEmbedder{}
	idx := index.OpenInMemory()
	h := NewIndexHandler(IndexHandlerOptions{
		GraphDir:   graphDir,
		IndexDir:   indexDir,
		Embedder:   emb,
		IndexStore: idx,
		Reader:     readFinderFor(t),
	})

	if err := h.Build(context.Background(), &command.BuildIndexCmd{}); err != nil {
		t.Fatal(err)
	}
	firstCalls := emb.calls

	if err := h.Build(context.Background(), &command.BuildIndexCmd{Force: true}); err != nil {
		t.Fatalf("forced Build: %v", err)
	}
	if emb.calls != firstCalls+1 {
		t.Errorf("Force=true should re-call embedder; got %d additional calls", emb.calls-firstCalls)
	}
}

func TestIndexHandler_LazyFillCoversMissingEntries(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()

	writeEntry(t, graphDir, "20260101-100000-s-tac-aaa", "body A", "summary A")

	emb := &fakeEmbedder{}
	idx := index.OpenInMemory()
	h := NewIndexHandler(IndexHandlerOptions{
		GraphDir:   graphDir,
		IndexDir:   indexDir,
		Embedder:   emb,
		IndexStore: idx,
		Reader:     readFinderFor(t),
	})

	// First build covers the existing entry.
	if err := h.Build(context.Background(), &command.BuildIndexCmd{}); err != nil {
		t.Fatal(err)
	}
	calls := emb.calls

	// Add a new entry. LazyFill should pick it up.
	writeEntry(t, graphDir, "20260101-100001-s-tac-bbb", "body B", "summary B")

	var indexed []string
	cmd := &command.LazyFillIndexCmd{
		OnEntryIndexed: func(id string, n int) { indexed = append(indexed, id) },
	}
	if err := h.LazyFill(context.Background(), cmd); err != nil {
		t.Fatalf("LazyFill: %v", err)
	}
	if len(indexed) != 1 || indexed[0] != "20260101-100001-s-tac-bbb" {
		t.Errorf("LazyFill should re-index only the new entry, got %v", indexed)
	}
	if emb.calls != calls+1 {
		t.Errorf("expected one new embed call from LazyFill, got %d additional", emb.calls-calls)
	}
}

func TestIndexHandler_BuildPicksUpEntryEdits(t *testing.T) {
	t.Parallel()

	graphDir := t.TempDir()
	indexDir := t.TempDir()

	id := "20260101-100000-s-tac-aaa"
	writeEntry(t, graphDir, id, "old body", "old summary")

	emb := &fakeEmbedder{}
	idx := index.OpenInMemory()
	h := NewIndexHandler(IndexHandlerOptions{
		GraphDir:   graphDir,
		IndexDir:   indexDir,
		Embedder:   emb,
		IndexStore: idx,
		Reader:     readFinderFor(t),
	})
	if err := h.Build(context.Background(), &command.BuildIndexCmd{}); err != nil {
		t.Fatal(err)
	}
	calls := emb.calls

	// Edit the entry's body. (In practice entries are immutable, but the
	// summary regen + manual fix paths can change file bytes; the
	// indexer must follow.)
	writeEntry(t, graphDir, id, "new body", "old summary")

	var indexed []string
	cmd := &command.BuildIndexCmd{
		OnEntryIndexed: func(eid string, n int) { indexed = append(indexed, eid) },
	}
	if err := h.Build(context.Background(), cmd); err != nil {
		t.Fatalf("Build after edit: %v", err)
	}
	if len(indexed) != 1 {
		t.Errorf("expected re-index of edited entry, got %v", indexed)
	}
	if emb.calls != calls+1 {
		t.Errorf("expected one new embed call after edit, got %d additional", emb.calls-calls)
	}
}

// Ensure the placeholder _ avoids "imported and not used" if the index
// reader-only contract drifts in the future. Using query.PreflightQuery
// here to assert the package import in a test surface.
var _ = query.PreflightQuery{}
