package application_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

const capturedBody = "Captured entries keep their titles apart from their neighbours."

// rankedIDs ranks the index's hits: an untitled entry, then six titled ones.
var rankedIDs = []string{
	"20260201-100009-s-tac-unt",
	"20260201-100001-s-tac-nb1", "20260201-100002-s-tac-nb2", "20260201-100003-s-tac-nb3",
	"20260201-100004-s-tac-nb4", "20260201-100005-s-tac-nb5", "20260201-100006-s-tac-nb6",
}

// rankedHits answers every nearest-neighbour query with rankedIDs, best first.
type rankedHits struct {
	sdd.SearchIndexStore
}

func (rankedHits) Nearest(context.Context, []sdd.IndexNamespace, []float32, int) ([]sdd.ScoredChunkHit, error) {
	hits := make([]sdd.ScoredChunkHit, 0, len(rankedIDs))
	for i, id := range rankedIDs {
		hits = append(hits, sdd.ScoredChunkHit{EntryID: id, ChunkID: id + "#0", Score: 1 - float64(i)/100})
	}
	return hits, nil
}

func writeNeighbours(t *testing.T, graphDir string) {
	t.Helper()
	for i, id := range rankedIDs {
		title := ""
		if i > 0 {
			title = fmt.Sprintf("Neighbour title %d", i)
		}
		entry := &model.Entry{ID: id, Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindGap, Title: title, Summary: "A neighbour.", Content: "A neighbour."}
		rel, err := model.IDToRelPath(id)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(graphDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(model.FormatFrontmatter(entry)+"\n"+entry.Content+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// summaryPrompts records the user prompts of the summary and title calls.
type summaryPrompts struct {
	mu      sync.Mutex
	prompts []string
}

func (p *summaryPrompts) runner() pkgllm.Runner {
	return pkgllm.RunnerFunc(func(_ context.Context, request pkgllm.Request) (pkgllm.Result, error) {
		identity := pkgllm.Identity{Provider: "test", Model: "test"}
		if request.Purpose == pkgllm.PurposePreflight {
			return pkgllm.Result{Text: `{"findings":[]}`, Identity: identity}, nil
		}
		p.mu.Lock()
		p.prompts = append(p.prompts, request.UserPrompt)
		p.mu.Unlock()
		return pkgllm.Result{Text: `{"title": "Fresh title", "summary": "A captured entry."}`, Identity: identity}, nil
	})
}

func similarTitlesIn(prompt string) []string {
	_, section, ok := strings.Cut(prompt, "## Titles of the most similar entries")
	if !ok {
		return nil
	}
	var titles []string
	for _, line := range strings.Split(section, "\n") {
		if title, ok := strings.CutPrefix(line, "- "); ok {
			titles = append(titles, title)
		}
	}
	return titles
}

func embedderFor(texts *[]string, err error) embed.Embedder {
	return embed.EmbedderFunc{Space: "fake/v1/4", Run: func(_ context.Context, req embed.Request) (embed.Result, error) {
		if err != nil {
			return embed.Result{}, err
		}
		*texts = append(*texts, req.Texts...)
		vectors := make([][]float32, len(req.Texts))
		for i := range vectors {
			vectors[i] = []float32{1, 0, 0, 0}
		}
		return embed.Result{Vectors: vectors}, nil
	}}
}

func TestCreateEntryShowsTheTitlesOfTheFiveNearestTitledEntries(t *testing.T) {
	var texts []string
	prompts := &summaryPrompts{}
	f := newWriteFixture(t, writeFixtureOptions{
		Runner: prompts.runner(), Embedder: embedderFor(&texts, nil),
		SearchIndex: rankedHits{SearchIndexStore: localadapter.NewMemorySearchIndexStore()},
	})
	writeNeighbours(t, f.graphDir)
	draft := f.identifiedDraft(sdd.EntryDraft{Kind: "gap", Layer: "tactical", Confidence: "high", Body: capturedBody})
	created, err := f.app.CreateEntry(t.Context(), f.identity, "example", f.binding, draft)
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if !slices.Equal(texts, []string{capturedBody}) {
		t.Errorf("the search should embed the entry's body, got %q", texts)
	}
	want := []string{"Neighbour title 1", "Neighbour title 2", "Neighbour title 3", "Neighbour title 4", "Neighbour title 5"}
	if len(prompts.prompts) != 1 || !slices.Equal(similarTitlesIn(prompts.prompts[0]), want) {
		t.Fatalf("one summary call should list the five nearest titled entries, got %d calls: %q", len(prompts.prompts), prompts.prompts)
	}
	if got := loadEntryByID(t, f.graphDir, created.EntryID).Title; got != "Fresh title" {
		t.Errorf("the generated title should be stored, got %q", got)
	}
}

func TestCreateEntryWithoutAnEmbedderShowsNoSimilarTitles(t *testing.T) {
	prompts := &summaryPrompts{}
	f := newWriteFixture(t, writeFixtureOptions{Runner: prompts.runner()})
	writeNeighbours(t, f.graphDir)
	draft := f.identifiedDraft(sdd.EntryDraft{Kind: "gap", Layer: "tactical", Confidence: "high", Body: capturedBody})
	if _, err := f.app.CreateEntry(t.Context(), f.identity, "example", f.binding, draft); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if len(prompts.prompts) != 1 || strings.Contains(prompts.prompts[0], "most similar entries") {
		t.Errorf("without an embedder the summary call should carry no similar titles: %q", prompts.prompts)
	}
}

func TestCreateEntryFailsWhenTheSimilarSearchFails(t *testing.T) {
	var texts []string
	prompts := &summaryPrompts{}
	f := newWriteFixture(t, writeFixtureOptions{
		Runner: prompts.runner(), Embedder: embedderFor(&texts, errors.New("embedding provider down")),
		SearchIndex: rankedHits{SearchIndexStore: localadapter.NewMemorySearchIndexStore()},
	})
	draft := f.identifiedDraft(sdd.EntryDraft{Kind: "gap", Layer: "tactical", Confidence: "high", Body: capturedBody})
	_, err := f.app.CreateEntry(t.Context(), f.identity, "example", f.binding, draft)
	if err == nil || !strings.Contains(err.Error(), "embedding provider down") {
		t.Fatalf("a failed search should fail the capture as a failed summary call does, got %v", err)
	}
	if len(prompts.prompts) != 0 {
		t.Errorf("no summary call should run without its similar titles, got %d", len(prompts.prompts))
	}
}
