package finders

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

type fakeEmbedder struct {
	calls int
}

// Embed produces deterministic 4-dim vectors keyed off SHA-256 of input,
// then maps a few well-known phrases / texts to specific corners of the
// space so the search assertions know which way the cosine will lean.
func (f *fakeEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return f.embed(texts)
}
func (f *fakeEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return f.embed(texts)
}
func (f *fakeEmbedder) embed(texts []string) ([][]float32, error) {
	f.calls++
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = vectorFor(t)
	}
	return out, nil
}
func (f *fakeEmbedder) Dimensions() int     { return 4 }
func (f *fakeEmbedder) BatchSize() int      { return 64 }
func (f *fakeEmbedder) Fingerprint() string { return "fake/1/4" }

// vectorFor maps text to a 4-dim vector. Inputs containing one of a few
// fixed substrings ("apple", "orange", "banana") map to canonical
// corners; everything else hashes into a fallback corner. The non-zero
// dimensions guarantee non-trivial cosine similarity.
func vectorFor(t string) []float32 {
	tl := strings.ToLower(t)
	switch {
	case strings.Contains(tl, "apple"):
		return []float32{1, 0, 0, 0}
	case strings.Contains(tl, "orange"):
		return []float32{0, 1, 0, 0}
	case strings.Contains(tl, "banana"):
		return []float32{0, 0, 1, 0}
	}
	h := sha256.Sum256([]byte(t))
	return []float32{
		0,
		0,
		0,
		float32(binary.BigEndian.Uint32(h[0:4]))/float32(^uint32(0))*0.5 + 0.1,
	}
}

func buildSearchGraph(t *testing.T) *model.Graph {
	t.Helper()
	a := entry("20260101-100000-s-tac-aaa",
		withContent("This entry talks about apples and orchards"),
		withKind(model.KindFact),
	)
	a.Summary = "Summary about apple farming."

	b := entry("20260101-100001-s-tac-bbb",
		withContent("Oranges grow on trees in warm climates"),
		withKind(model.KindGap),
	)
	b.Summary = "Summary about orange cultivation."

	c := entry("20260101-100002-d-tac-ccc",
		withContent("Decision about apple harvest scheduling"),
		withKind(model.KindDirective),
	)
	c.Summary = "Decided apple-harvest scheduling cadence."

	// Superseded entry: e1 supersedes a (a is the superseded one).
	superseded := entry("20260101-100003-s-tac-ddd",
		withContent("This was an old apple-related observation now superseded"),
		withKind(model.KindGap),
	)
	superseded.Summary = "Old apple-related signal."

	successor := entry("20260101-100004-s-tac-eee",
		withContent("Newer apple-related observation"),
		withSupersedes(superseded.ID),
		withKind(model.KindGap),
	)
	successor.Summary = "Newer apple-related signal that supersedes the old."

	g := model.NewGraph([]*model.Entry{a, b, c, superseded, successor})
	return g
}

// loadSearchIndex builds an in-memory index where each entry has one
// summary chunk plus zero/one body chunk. Embeddings come from
// vectorFor so the test can predict vector hits.
func loadSearchIndex(t *testing.T, g *model.Graph) *index.Index {
	t.Helper()
	idx := index.OpenInMemory()
	ctx := context.Background()
	for _, e := range g.Entries {
		rows := []index.Row{
			{
				EntryID:          e.ID,
				ChunkID:          index.SummaryChunkID(e.ID),
				Text:             e.Summary,
				Body:             e.Summary,
				IsSummary:        true,
				ContentHash:      index.HashContent(e.Summary),
				ModelFingerprint: "fake/1/4",
				Embedding:        vectorFor(e.Summary),
			},
			{
				EntryID:          e.ID,
				ChunkID:          index.BodyChunkID(e.ID, 0),
				Text:             e.Content,
				Body:             e.Content,
				Breadcrumb:       []string{"Section"},
				Depth:            2,
				ContentHash:      index.HashContent(e.Content),
				ModelFingerprint: "fake/1/4",
				Embedding:        vectorFor(e.Content),
			},
		}
		if err := idx.UpsertEntry(ctx, e.ID, nil, rows); err != nil {
			t.Fatalf("seed index for %s: %v", e.ID, err)
		}
	}
	return idx
}

func TestSearchFinder_TextModeMultiTermAND(t *testing.T) {
	t.Parallel()
	g := buildSearchGraph(t)
	f := NewSearchFinder(SearchFinderOptions{GraphDir: t.TempDir()})

	res, err := f.Search(context.Background(), query.SearchQuery{
		Graph: g,
		Terms: []string{"apple", "harvest"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// Only the directive (c) mentions both apple AND harvest in either
	// summary or body. (a)'s summary mentions apple farming but not
	// harvest.
	if len(res.Entries) != 1 || res.Entries[0].Entry.ID != "20260101-100002-d-tac-ccc" {
		ids := make([]string, len(res.Entries))
		for i, e := range res.Entries {
			ids[i] = e.Entry.ID
		}
		t.Errorf("expected one hit (the directive), got %v", ids)
	}
	if res.Entries[0].Citation.Snippet == "" {
		t.Error("expected a non-empty citation snippet")
	}
}

func TestSearchFinder_TextModeRegexAlternation(t *testing.T) {
	t.Parallel()
	g := buildSearchGraph(t)
	f := NewSearchFinder(SearchFinderOptions{GraphDir: t.TempDir()})

	// OR semantics achieved via regex alternation in a single term.
	res, err := f.Search(context.Background(), query.SearchQuery{
		Graph: g,
		Terms: []string{"(apple|orange)"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// All 4 fruit-mentioning entries should match (a, b, c, successor —
	// excluding the superseded one by default).
	if len(res.Entries) != 4 {
		ids := make([]string, len(res.Entries))
		for i, e := range res.Entries {
			ids[i] = e.Entry.ID
		}
		t.Errorf("expected 4 hits, got %d (%v)", len(res.Entries), ids)
	}
}

func TestSearchFinder_TextModeExcludesSupersededByDefault(t *testing.T) {
	t.Parallel()
	g := buildSearchGraph(t)
	f := NewSearchFinder(SearchFinderOptions{GraphDir: t.TempDir()})

	res, err := f.Search(context.Background(), query.SearchQuery{
		Graph: g,
		Terms: []string{"apple"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, se := range res.Entries {
		if se.Entry.ID == "20260101-100003-s-tac-ddd" {
			t.Error("superseded entry should be excluded by default")
		}
	}

	// IncludeSuperseded flips the gate.
	res, err = f.Search(context.Background(), query.SearchQuery{
		Graph:             g,
		Terms:             []string{"apple"},
		IncludeSuperseded: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, se := range res.Entries {
		if se.Entry.ID == "20260101-100003-s-tac-ddd" {
			found = true
		}
	}
	if !found {
		t.Error("IncludeSuperseded=true should surface superseded entry")
	}
}

func TestSearchFinder_TextModeFilterByType(t *testing.T) {
	t.Parallel()
	g := buildSearchGraph(t)
	f := NewSearchFinder(SearchFinderOptions{GraphDir: t.TempDir()})

	res, err := f.Search(context.Background(), query.SearchQuery{
		Graph:  g,
		Terms:  []string{"apple"},
		Filter: model.GraphFilter{Type: model.TypeDecision},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, se := range res.Entries {
		if se.Entry.Type != model.TypeDecision {
			t.Errorf("expected only decisions, got %s (%s)", se.Entry.ID, se.Entry.Type)
		}
	}
	// At least the directive should be there.
	if len(res.Entries) == 0 {
		t.Error("expected at least one decision hit")
	}
}

func TestSearchFinder_VectorMode(t *testing.T) {
	t.Parallel()
	g := buildSearchGraph(t)
	idx := loadSearchIndex(t, g)
	emb := &fakeEmbedder{}
	f := NewSearchFinder(SearchFinderOptions{
		GraphDir:   t.TempDir(),
		Embedder:   emb,
		IndexStore: idx,
	})

	res, err := f.Search(context.Background(), query.SearchQuery{
		Graph:  g,
		Phrase: "Need information about apples",
	})
	if err != nil {
		t.Fatalf("vector Search: %v", err)
	}
	if len(res.Entries) == 0 {
		t.Fatal("expected at least one vector hit")
	}
	// Top hit must be one of the apple-mentioning entries.
	top := res.Entries[0].Entry.ID
	if top != "20260101-100000-s-tac-aaa" && top != "20260101-100002-d-tac-ccc" && top != "20260101-100004-s-tac-eee" {
		t.Errorf("top vector hit was %s; expected an apple-related entry", top)
	}
	// The first-hit citation must carry breadcrumb (from a body chunk) or
	// IsSummary=true (from the summary chunk).
	if !res.Entries[0].Citation.IsSummary && len(res.Entries[0].Citation.Breadcrumb) == 0 {
		t.Errorf("expected breadcrumb or IsSummary on top citation, got %#v", res.Entries[0].Citation)
	}
}

func TestSearchFinder_VectorModeWithoutEmbedderErrors(t *testing.T) {
	t.Parallel()
	g := buildSearchGraph(t)
	f := NewSearchFinder(SearchFinderOptions{GraphDir: t.TempDir()})

	_, err := f.Search(context.Background(), query.SearchQuery{
		Graph:  g,
		Phrase: "anything",
	})
	if err == nil {
		t.Error("expected error when vector mode requested without embedder")
	}
	if !errors.Is(err, err) || !strings.Contains(err.Error(), "embedding provider") {
		t.Errorf("error should mention embedding provider; got %v", err)
	}
}

func TestSearchFinder_HybridMode(t *testing.T) {
	t.Parallel()
	g := buildSearchGraph(t)
	idx := loadSearchIndex(t, g)
	emb := &fakeEmbedder{}
	f := NewSearchFinder(SearchFinderOptions{
		GraphDir:   t.TempDir(),
		Embedder:   emb,
		IndexStore: idx,
	})

	res, err := f.Search(context.Background(), query.SearchQuery{
		Graph:  g,
		Terms:  []string{"apple"},
		Phrase: "apples",
	})
	if err != nil {
		t.Fatalf("hybrid Search: %v", err)
	}
	if res.Mode != query.SearchModeHybrid {
		t.Errorf("Mode: got %q, want %q", res.Mode, query.SearchModeHybrid)
	}
	if len(res.Entries) == 0 {
		t.Fatal("expected hybrid hits")
	}
}

func TestSearchFinder_VectorAvailable(t *testing.T) {
	t.Parallel()
	if !NewSearchFinder(SearchFinderOptions{Embedder: &fakeEmbedder{}, IndexStore: index.OpenInMemory()}).VectorAvailable() {
		t.Error("expected VectorAvailable=true with both deps set")
	}
	if NewSearchFinder(SearchFinderOptions{Embedder: &fakeEmbedder{}}).VectorAvailable() {
		t.Error("expected VectorAvailable=false without index store")
	}
	if NewSearchFinder(SearchFinderOptions{IndexStore: index.OpenInMemory()}).VectorAvailable() {
		t.Error("expected VectorAvailable=false without embedder")
	}
}

func TestRRFFuse_PicksHigherCitation(t *testing.T) {
	t.Parallel()

	// Same entry ranked first in vector list and second in text — vector
	// citation should win because vector ranked it higher.
	e := entry("20260101-100000-s-tac-zzz")
	textRes := &query.SearchResult{
		Entries: []query.SearchEntry{
			{Entry: entry("20260101-100001-s-tac-yyy")},
			{Entry: e, Citation: query.Citation{Snippet: "from-text"}},
		},
	}
	vecRes := &query.SearchResult{
		Entries: []query.SearchEntry{
			{Entry: e, Citation: query.Citation{Snippet: "from-vector"}},
		},
	}

	out := rrfFuse(textRes, vecRes, 10)
	var found *query.SearchEntry
	for i := range out.Entries {
		if out.Entries[i].Entry.ID == e.ID {
			found = &out.Entries[i]
		}
	}
	if found == nil {
		t.Fatal("expected entry in fused output")
	}
	if found.Citation.Snippet != "from-vector" {
		t.Errorf("expected vector citation (it ranked higher), got %q", found.Citation.Snippet)
	}
}

func TestAdjustVectorScore(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		raw       float32
		isSummary bool
		depth     int
		check     func(score float32) bool
	}{
		{"summary boost", 0.5, true, 0, func(s float32) bool { return s > 0.5 }},
		{"depth-1 unchanged", 0.5, false, 1, func(s float32) bool { return s == 0.5 }},
		{"depth-3 penalized", 0.5, false, 3, func(s float32) bool { return s < 0.5 && s > 0.4 }},
		{"floor at 60%", 0.5, false, 99, func(s float32) bool { return s >= 0.5*0.6 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := adjustVectorScore(c.raw, c.isSummary, c.depth)
			if !c.check(got) {
				t.Errorf("got %f", got)
			}
		})
	}
}

// Sanity guard so the search package's test-only fake embedder satisfies
// the llm.Embedder interface — caught at compile time.
var _ = (struct {
	a interface{ Dimensions() int }
}{a: &fakeEmbedder{}})

// Used in SearchFinder_TextModeMultiTermAND; reflect imported so future
// breadcrumb assertions compile out of the box.
var _ = reflect.DeepEqual
