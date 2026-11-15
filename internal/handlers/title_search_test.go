package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
)

const (
	describedEntry = "20260201-100000-s-tac-sel"
	untitledNear   = "20260201-100009-s-tac-unt"
	describedBody  = "The index segments are mapped once per process."
)

// nearestIDs ranks the search index's hits: the described entry itself first,
// then an untitled entry, then six titled neighbours.
var nearestIDs = []string{
	describedEntry, untitledNear,
	"20260201-100001-s-tac-nb1", "20260201-100002-s-tac-nb2", "20260201-100003-s-tac-nb3",
	"20260201-100004-s-tac-nb4", "20260201-100005-s-tac-nb5", "20260201-100006-s-tac-nb6",
}

var nearestTitles = []string{"Neighbour title 1", "Neighbour title 2", "Neighbour title 3", "Neighbour title 4", "Neighbour title 5"}

// rankedIndex is an in-memory index whose one chunk per entry lies the
// further from the query vector the later the entry stands in nearestIDs.
func rankedIndex(t *testing.T) *index.Index {
	t.Helper()
	idx := index.OpenInMemory()
	for i, id := range nearestIDs {
		row := index.Row{
			EntryID: id, ChunkID: index.SummaryChunkID(id), Text: id, Body: id, IsSummary: true,
			ModelFingerprint: "fake/v1/4", Embedding: []float32{1, 0.1 * float32(i), 0, 0},
		}
		if err := idx.UpsertEntry(context.Background(), id, nil, []index.Row{row}); err != nil {
			t.Fatal(err)
		}
	}
	return idx
}

// indexOpener hands out its index, none when it holds none.
type indexOpener struct {
	index  *index.Index
	opened int
}

func (o *indexOpener) OpenSearchIndex(context.Context) (*index.Index, error) {
	o.opened++
	return o.index, nil
}

// queryEmbedder records the texts it embeds, or fails with err.
type queryEmbedder struct {
	mu    sync.Mutex
	texts []string
	err   error
}

func (e *queryEmbedder) embedder() embed.Embedder {
	return embed.EmbedderFunc{Space: "fake/v1/4", Run: func(_ context.Context, req embed.Request) (embed.Result, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.err != nil {
			return embed.Result{}, e.err
		}
		e.texts = append(e.texts, req.Texts...)
		vectors := make([][]float32, len(req.Texts))
		for i := range vectors {
			vectors[i] = []float32{1, 0, 0, 0}
		}
		return embed.Result{Vectors: vectors}, nil
	}}
}

// promptRunner answers its titles in call order, the last one repeated, and
// records every request.
type promptRunner struct {
	mu       sync.Mutex
	titles   []string
	requests []llm.Request
}

func (r *promptRunner) Run(_ context.Context, request llm.Request) (llm.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
	answer := map[string]string{"title": r.titles[min(len(r.requests), len(r.titles))-1]}
	if request.Purpose == llm.PurposeSummarize {
		answer["summary"] = "A fresh summary."
	}
	data, err := json.Marshal(answer)
	if err != nil {
		return llm.Result{}, err
	}
	return llm.Result{Text: string(data)}, nil
}

// listedTitles reads the similar-titles list out of a prompt, nil when the
// prompt has no such section.
func listedTitles(prompt string) []string {
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

func neighbourEntries() []*model.Entry {
	var entries []*model.Entry
	for i, id := range nearestIDs[2:] {
		entries = append(entries, &model.Entry{
			ID: id, Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindGap,
			Title: fmt.Sprintf("Neighbour title %d", i+1), Summary: "A neighbour.", Content: "A neighbour.",
		})
	}
	return append(entries, &model.Entry{
		ID: untitledNear, Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindGap,
		Summary: "No title yet.", Content: "No title yet.",
	})
}

// writeTitleGraph writes the neighbours and the described entry, which holds
// a title and a summary.
func writeTitleGraph(t *testing.T, graphDir string) {
	t.Helper()
	for _, e := range neighbourEntries() {
		writeEntryFile(t, graphDir, e.ID, model.FormatFrontmatter(e)+"\n"+e.Content+"\n")
	}
	writeEntryFile(t, graphDir, describedEntry, "---\ntype: signal\nlayer: tactical\nkind: gap\ntitle: Segment mapping\nsummary: Segments map once.\n---\n\n"+describedBody+"\n")
}

// summarizeDescribed regenerates the described entry and returns the graph
// directory it lives in.
func summarizeDescribed(t *testing.T, runner llm.Runner, embedder embed.Embedder, opener handlers.SearchIndexOpener) (string, error) {
	t.Helper()
	graphDir := t.TempDir()
	writeTitleGraph(t, graphDir)
	h := handlers.New(handlers.Options{
		GraphDir: graphDir, Reader: &graphReader{}, Committer: &recordingCommitter{},
		LLMRunner: runner, Embedder: embedder, SearchIndex: opener,
	})
	return graphDir, h.Summarize(context.Background(), &command.SummarizeCmd{EntryIDs: []string{describedEntry}})
}

func TestSummarize_ShowsTheTitlesOfTheFiveNearestTitledEntries(t *testing.T) {
	runner := &promptRunner{titles: []string{"Fresh title"}}
	embedder := &queryEmbedder{}
	opener := &indexOpener{index: rankedIndex(t)}
	if _, err := summarizeDescribed(t, runner, embedder.embedder(), opener); err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if !slices.Equal(embedder.texts, []string{describedBody}) {
		t.Errorf("the search should embed the entry's body, got %q", embedder.texts)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("a title repeating no other should cost one call, got %d", len(runner.requests))
	}
	if got := listedTitles(runner.requests[0].UserPrompt); !slices.Equal(got, nearestTitles) {
		t.Errorf("the prompt should list the five nearest titled entries other than the entry, got %q", got)
	}
	if opener.opened != 1 {
		t.Errorf("the index should be opened once per run, opened %d", opener.opened)
	}
}

func TestSummarize_RerunsARepeatedTitleOnce(t *testing.T) {
	runner := &promptRunner{titles: []string{"neighbour title 3", "Distinct title"}}
	graphDir, err := summarizeDescribed(t, runner, (&queryEmbedder{}).embedder(), &indexOpener{index: rankedIndex(t)})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if len(runner.requests) != 2 || runner.requests[1].Purpose != llm.PurposeTitle {
		t.Fatalf("a repeated title should be re-run once by the title-only call, got %d calls", len(runner.requests))
	}
	want := []string{"Neighbour title 3", "Neighbour title 1", "Neighbour title 2", "Neighbour title 4", "Neighbour title 5"}
	if got := listedTitles(runner.requests[1].UserPrompt); !slices.Equal(got, want) {
		t.Errorf("the re-run should list the repeated title first, got %q", got)
	}
	text := readEntryFile(t, graphDir, describedEntry)
	if !strings.Contains(text, "title: Distinct title") || !strings.Contains(text, "summary: A fresh summary.") {
		t.Errorf("the re-run's title should be stored beside the paired call's summary:\n%s", text)
	}
}

func TestTitleCalls_WithoutEmbedderOrIndex_ShowNoSimilarTitles(t *testing.T) {
	cases := []struct {
		name       string
		embedder   *queryEmbedder
		opener     *indexOpener
		wantOpened int
	}{
		{name: "no embedder", opener: &indexOpener{index: rankedIndex(t)}},
		{name: "no index", embedder: &queryEmbedder{}, opener: &indexOpener{}, wantOpened: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var embedder embed.Embedder
			if tc.embedder != nil {
				embedder = tc.embedder.embedder()
			}
			runner := &promptRunner{titles: []string{"Fresh title"}}
			if _, err := summarizeDescribed(t, runner, embedder, tc.opener); err != nil {
				t.Fatalf("Summarize: %v", err)
			}
			if len(runner.requests) != 1 || strings.Contains(runner.requests[0].Combined(), "most similar entries") {
				t.Errorf("the title call should run without a similar-titles section")
			}
			if tc.opener.opened != tc.wantOpened {
				t.Errorf("the index should be opened only beside an embedder, opened %d", tc.opener.opened)
			}
			if tc.embedder != nil && len(tc.embedder.texts) != 0 {
				t.Errorf("nothing should be embedded without an index, got %q", tc.embedder.texts)
			}
		})
	}
}

func TestSummarize_FailsWhenTheSimilarSearchFails(t *testing.T) {
	runner := &promptRunner{titles: []string{"Fresh title"}}
	embedder := &queryEmbedder{err: errors.New("embedding provider down")}
	_, err := summarizeDescribed(t, runner, embedder.embedder(), &indexOpener{index: rankedIndex(t)})
	if err == nil || !strings.Contains(err.Error(), "embedding provider down") {
		t.Fatalf("a failed search should fail the summary, got %v", err)
	}
	if len(runner.requests) != 0 {
		t.Errorf("no title call should run without its similar titles, got %d", len(runner.requests))
	}
}

func TestSummarize_OpensNoIndexWithNothingToDescribe(t *testing.T) {
	graphDir := t.TempDir()
	writeTitledEntry(t, graphDir)
	opener := &indexOpener{index: rankedIndex(t)}
	h := handlers.New(handlers.Options{
		GraphDir: graphDir, Reader: &graphReader{}, Committer: &recordingCommitter{},
		LLMRunner: &promptRunner{titles: []string{"Fresh title"}}, Embedder: (&queryEmbedder{}).embedder(), SearchIndex: opener,
	})
	if err := h.Summarize(context.Background(), &command.SummarizeCmd{}); err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if opener.opened != 0 {
		t.Errorf("a run with nothing to describe should not open the index, opened %d", opener.opened)
	}
}

func TestSummarize_LeavesTheLinkedEntriesOutOfTheSimilarTitles(t *testing.T) {
	graphDir := t.TempDir()
	writeTitleGraph(t, graphDir)
	linked := &model.Entry{
		ID: describedEntry, Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindGap,
		Title: "Segment mapping", Summary: "Segments map once.", Content: describedBody,
		Refs: []model.Ref{{ID: nearestIDs[2], Kind: model.RefKindRelated, Desc: "the first neighbour"}},
	}
	writeEntryFile(t, graphDir, describedEntry, model.FormatFrontmatter(linked)+"\n"+linked.Content+"\n")
	runner := &promptRunner{titles: []string{"Fresh title"}}
	h := handlers.New(handlers.Options{
		GraphDir: graphDir, Reader: &graphReader{}, Committer: &recordingCommitter{},
		LLMRunner: runner, Embedder: (&queryEmbedder{}).embedder(), SearchIndex: &indexOpener{index: rankedIndex(t)},
	})
	if err := h.Summarize(context.Background(), &command.SummarizeCmd{EntryIDs: []string{describedEntry}}); err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	want := []string{"Neighbour title 2", "Neighbour title 3", "Neighbour title 4", "Neighbour title 5", "Neighbour title 6"}
	if got := listedTitles(runner.requests[0].UserPrompt); !slices.Equal(got, want) {
		t.Errorf("a ref'd entry should be listed as linked only, not among the similar titles, got %q", got)
	}
	if !strings.Contains(runner.requests[0].UserPrompt, "Title: Neighbour title 1") {
		t.Errorf("the ref'd entry should still be listed as linked:\n%s", runner.requests[0].UserPrompt)
	}
}

func newDescribedEntry(t *testing.T, embedder *queryEmbedder, runner llm.Runner) *model.Entry {
	t.Helper()
	tmp := t.TempDir()
	sddDir := filepath.Join(tmp, ".sdd")
	if err := os.MkdirAll(sddDir, 0755); err != nil {
		t.Fatal(err)
	}
	h := handlers.New(handlers.Options{
		GraphDir: tmp, SDDDir: sddDir, Reader: &fakeReader{graph: model.NewGraph(neighbourEntries())},
		Committer: &recordingCommitter{}, LLMRunner: runner, Stderr: &bytes.Buffer{},
		Embedder: embedder.embedder(), SearchIndex: &indexOpener{index: rankedIndex(t)},
	})
	var id string
	err := h.NewEntry(context.Background(), &command.NewEntryCmd{
		Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindGap, Description: describedBody,
		SkipPreflight: true, OnNewEntry: func(newID, _ string) { id = newID },
	})
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	return readWrittenEntry(t, tmp, id)
}

func readWrittenEntry(t *testing.T, graphDir, id string) *model.Entry {
	t.Helper()
	entry, err := model.ParseEntry(id+".md", readEntryFile(t, graphDir, id))
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func TestNewEntry_ShowsTheTitlesOfTheFiveNearestTitledEntries(t *testing.T) {
	runner := &promptRunner{titles: []string{"Fresh title"}}
	embedder := &queryEmbedder{}
	entry := newDescribedEntry(t, embedder, runner)
	if !slices.Equal(embedder.texts, []string{describedBody}) {
		t.Errorf("the search should embed the entry's body, got %q", embedder.texts)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("one paired call expected, got %d", len(runner.requests))
	}
	if got := listedTitles(runner.requests[0].UserPrompt); !slices.Equal(got, nearestTitles) {
		t.Errorf("the prompt should list the five nearest titled entries, got %q", got)
	}
	if entry.Title != "Fresh title" {
		t.Errorf("the generated title should be stored, got %q", entry.Title)
	}
}

func TestNewEntry_SimilarSearchFailureFailsTheDescriptionOnly(t *testing.T) {
	runner := &promptRunner{titles: []string{"Fresh title"}}
	entry := newDescribedEntry(t, &queryEmbedder{err: errors.New("embedding provider down")}, runner)
	if len(runner.requests) != 0 {
		t.Errorf("no description call should run without its similar titles, got %d", len(runner.requests))
	}
	if entry.Title != "" || entry.Summary != "" {
		t.Errorf("the entry should be written undescribed, as after a failed summary call, got title %q summary %q", entry.Title, entry.Summary)
	}
}

func TestSearchIndexDir_OpensNoIndexWhereNoneIsBuilt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "index")
	store, err := handlers.SearchIndexDir(dir).OpenSearchIndex(context.Background())
	if err != nil || store != nil {
		t.Fatalf("a missing index should open as none, got %v, %v", store, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("opening a missing index should not create it, got %v", err)
	}
}
