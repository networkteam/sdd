package finders_test

// End-to-end search tests: write entries to a temp graph dir, build the
// index via IndexHandler with a fake embedder, then run SearchFinder
// queries through the same index/embedder combination. This locks the
// full ingest → query path together so a regression in any layer surfaces
// here.

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/baseprocedures"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// e2eEmbedder produces deterministic 4-dim vectors. Inputs containing
// canonical token strings ("apple", "orange", "banana") map to fixed
// corners of the 4-dim space; everything else hashes into a fallback
// component.
type e2eEmbedder struct{}

func (e *e2eEmbedder) EmbedDocuments(_ context.Context, texts []string) ([][]float32, error) {
	return e.embed(texts)
}
func (e *e2eEmbedder) EmbedQueries(_ context.Context, texts []string) ([][]float32, error) {
	return e.embed(texts)
}
func (e *e2eEmbedder) embed(texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = e2eVector(t)
	}
	return out, nil
}
func (e *e2eEmbedder) Dimensions() int     { return 4 }
func (e *e2eEmbedder) BatchSize() int      { return 64 }
func (e *e2eEmbedder) Fingerprint() string { return "e2e/v1/4" }

func e2eVector(t string) []float32 {
	tl := strings.ToLower(t)
	v := []float32{0, 0, 0, 0}
	if strings.Contains(tl, "apple") {
		v[0] = 1
	}
	if strings.Contains(tl, "orange") {
		v[1] = 1
	}
	if strings.Contains(tl, "banana") {
		v[2] = 1
	}
	// Always non-zero in dim 3 for fallback similarity.
	h := sha256.Sum256([]byte(tl))
	v[3] = float32(binary.BigEndian.Uint32(h[:4]))/float32(^uint32(0))*0.2 + 0.05
	return v
}

func writeE2EEntry(t *testing.T, graphDir, id, body, summary string) {
	t.Helper()
	yyyy := id[:4]
	mm := id[4:6]
	short := id[6:]
	dir := filepath.Join(graphDir, yyyy, mm)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	c := strings.Builder{}
	c.WriteString("---\ntype: signal\nlayer: tactical\nkind: gap\n")
	c.WriteString("confidence: medium\nparticipants:\n  - Test\n")
	if summary != "" {
		c.WriteString("summary: |-\n  " + summary + "\n")
	}
	c.WriteString("---\n")
	c.WriteString(body)
	c.WriteString("\n")
	if err := os.WriteFile(filepath.Join(dir, short+".md"), []byte(c.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

type e2eSetup struct {
	graphDir string
	indexDir string
	embedder *e2eEmbedder
	reader   handlers.Reader
}

func setupE2E(t *testing.T) *e2eSetup {
	t.Helper()
	graphDir := t.TempDir()
	indexDir := t.TempDir()

	writeE2EEntry(t, graphDir, "20260101-100000-s-tac-aaa",
		"## Section\nThis entry talks about apples.",
		"Notes about apple orchards.")
	writeE2EEntry(t, graphDir, "20260101-100001-s-tac-bbb",
		"## Section\nOranges grow on trees.",
		"Notes about orange cultivation.")
	writeE2EEntry(t, graphDir, "20260101-100002-s-tac-ccc",
		"## Section\nMarsupial migration patterns.",
		"Field study of marsupial migration.")

	reader := finders.New(finders.Options{
		PreflightRunner: e2eNoopRunner{},
	})

	return &e2eSetup{
		graphDir: graphDir,
		indexDir: indexDir,
		embedder: &e2eEmbedder{},
		reader:   reader,
	}
}

func (s *e2eSetup) build(t *testing.T) {
	t.Helper()
	h := handlers.NewIndexHandler(handlers.IndexHandlerOptions{
		GraphDir: s.graphDir,
		IndexDir: s.indexDir,
		Embedder: s.embedder,
		Reader:   s.reader,
	})
	if err := h.Build(context.Background(), &command.BuildIndexCmd{}); err != nil {
		t.Fatalf("Build: %v", err)
	}
}

func (s *e2eSetup) lazyFill(t *testing.T) {
	t.Helper()
	h := handlers.NewIndexHandler(handlers.IndexHandlerOptions{
		GraphDir: s.graphDir,
		IndexDir: s.indexDir,
		Embedder: s.embedder,
		Reader:   s.reader,
	})
	if err := h.LazyFill(context.Background(), &command.LazyFillIndexCmd{}); err != nil {
		t.Fatalf("LazyFill: %v", err)
	}
}

// loadGraph loads the fixture's graph scoped to the entries this fixture
// actually wrote. LoadGraph unconditionally injects the embedded base
// procedures (see graph.go), so a raw load returns ~10 extra d-prc-*
// entries the fixture never intended. IndexHandler.Build indexes all of
// them, and the fake embedder maps every fruit-token-free text — base
// procedures included — to the same collinear fallback vector, so they
// tie at cosine 1.0 and can crowd or tie the intended hit in vector and
// hybrid ranking. Stripping them here keeps the effective corpus (both
// the text candidate set and the vector rollup, which filters through it)
// to just the fixture's entries, making ranking deterministic. See
// s-tac-qtz for the full mechanism.
func (s *e2eSetup) loadGraph(t *testing.T) *model.Graph {
	t.Helper()
	full, err := s.reader.CurrentGraph(s.graphDir)
	if err != nil {
		t.Fatal(err)
	}

	base, err := baseprocedures.Entries()
	if err != nil {
		t.Fatal(err)
	}
	baseIDs := make(map[string]bool, len(base))
	for _, e := range base {
		baseIDs[e.ID] = true
	}

	kept := make([]*model.Entry, 0, len(full.Entries))
	for _, e := range full.Entries {
		if baseIDs[e.ID] {
			continue
		}
		kept = append(kept, e)
	}
	g := model.NewGraph(kept)
	g.SetGraphDir(s.graphDir)
	return g
}

// finder opens the persistent index fresh so it reflects whatever build /
// lazyFill just committed (they write under index.WriteStore's exclusive lock,
// so a snapshot opened earlier would miss the new rows).
func (s *e2eSetup) finder(t *testing.T) *finders.SearchFinder {
	t.Helper()
	store, err := index.Open(s.indexDir)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	return finders.NewSearchFinder(finders.SearchFinderOptions{
		GraphDir:   s.graphDir,
		Embedder:   s.embedder,
		IndexStore: store,
	})
}

type e2eNoopRunner struct{}

func (e2eNoopRunner) Run(context.Context, llm.Request) (*llm.RunResult, error) {
	return nil, fmt.Errorf("noop")
}

func TestE2E_BuildAndVectorSearch(t *testing.T) {
	t.Parallel()
	s := setupE2E(t)
	s.build(t)
	g := s.loadGraph(t)

	res, err := s.finder(t).Search(context.Background(), query.SearchQuery{
		Graph:                g,
		Phrase:               "looking for apples",
		MaxCitationsPerEntry: query.DefaultMaxCitationsPerEntry,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Entries) == 0 {
		t.Fatal("expected hits")
	}
	// Top hit must be the apple-mentioning entry, not the marsupial one.
	if res.Entries[0].Entry.ID != "20260101-100000-s-tac-aaa" {
		t.Errorf("top hit: got %s, want the apple entry", res.Entries[0].Entry.ID)
	}
	// Citation should carry breadcrumb (from a body-section chunk) or
	// IsSummary=true (from the summary chunk).
	c := res.Entries[0].Citation()
	if !c.IsSummary && len(c.Breadcrumb) == 0 {
		t.Errorf("expected citation with breadcrumb or IsSummary, got %#v", c)
	}
}

func TestE2E_LazyFillCoversNewEntry(t *testing.T) {
	t.Parallel()
	s := setupE2E(t)
	s.build(t)

	// Add a new entry after the initial build. Search should still find
	// it because lazy-fill runs before the query in production; here we
	// invoke lazy-fill explicitly to keep the test deterministic.
	writeE2EEntry(t, s.graphDir, "20260101-100003-s-tac-ddd",
		"## Section\nBananas are yellow.",
		"Notes about bananas.")
	s.lazyFill(t)

	g := s.loadGraph(t)
	res, err := s.finder(t).Search(context.Background(), query.SearchQuery{
		Graph:  g,
		Phrase: "I'd like a banana",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) == 0 {
		t.Fatal("expected hits")
	}
	if res.Entries[0].Entry.ID != "20260101-100003-s-tac-ddd" {
		t.Errorf("top hit: got %s, want the banana entry", res.Entries[0].Entry.ID)
	}
}

func TestE2E_BranchReconciliation(t *testing.T) {
	t.Parallel()
	// Index a, b, c. Then "switch branch" by removing b's file from the
	// graph dir. Searching for an orange phrase must NOT return b — even
	// though b's chunks remain in the index, the search must intersect
	// hits with entries currently present on disk.
	s := setupE2E(t)
	s.build(t)

	bPath := filepath.Join(s.graphDir, "2026", "01", "01-100001-s-tac-bbb.md")
	if err := os.Remove(bPath); err != nil {
		t.Fatal(err)
	}

	g := s.loadGraph(t)
	res, err := s.finder(t).Search(context.Background(), query.SearchQuery{
		Graph:  g,
		Phrase: "orange harvest",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, se := range res.Entries {
		if se.Entry.ID == "20260101-100001-s-tac-bbb" {
			t.Error("entry b should be filtered: file removed from graph dir")
		}
	}
}

func TestE2E_HybridFusion(t *testing.T) {
	t.Parallel()
	s := setupE2E(t)
	s.build(t)
	g := s.loadGraph(t)

	res, err := s.finder(t).Search(context.Background(), query.SearchQuery{
		Graph:  g,
		Terms:  []string{"orange"},
		Phrase: "citrus harvest",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != query.SearchModeHybrid {
		t.Errorf("expected hybrid mode, got %q", res.Mode)
	}
	if len(res.Entries) == 0 {
		t.Fatal("expected hybrid hits")
	}
	if res.Entries[0].Entry.ID != "20260101-100001-s-tac-bbb" {
		t.Errorf("top hybrid hit: got %s, want the orange entry", res.Entries[0].Entry.ID)
	}
}

func TestE2E_TextModeDoesNotEmbed(t *testing.T) {
	t.Parallel()
	// Even with embedder/index configured, --term-only queries must not
	// touch the embedder or the index.
	s := setupE2E(t)

	// Track embedder usage via a wrapping runner-counter.
	tracked := &countingEmbedder{inner: s.embedder}
	finder := finders.NewSearchFinder(finders.SearchFinderOptions{
		GraphDir:   s.graphDir,
		Embedder:   tracked,
		IndexStore: index.OpenInMemory(),
	})

	g := s.loadGraph(t)
	_, err := finder.Search(context.Background(), query.SearchQuery{
		Graph: g,
		Terms: []string{"apple"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tracked.calls != 0 {
		t.Errorf("text-only mode should not call embedder; got %d calls", tracked.calls)
	}
}

type countingEmbedder struct {
	inner llm.Embedder
	calls int
}

func (c *countingEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	c.calls++
	return c.inner.EmbedDocuments(ctx, texts)
}
func (c *countingEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	c.calls++
	return c.inner.EmbedQueries(ctx, texts)
}
func (c *countingEmbedder) Dimensions() int     { return c.inner.Dimensions() }
func (c *countingEmbedder) BatchSize() int      { return c.inner.BatchSize() }
func (c *countingEmbedder) Fingerprint() string { return c.inner.Fingerprint() }
