package application_test

import (
	"context"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

type countingAttachmentStore struct {
	staticGraphStore
	reads map[string]int
}

func (s *countingAttachmentStore) ReadAttachmentPage(ctx context.Context, id, name string, offset int64, limit int) (sdd.AttachmentPage, error) {
	s.reads[id]++
	return s.staticGraphStore.ReadAttachmentPage(ctx, id, name, offset, limit)
}

type candidateIndex struct {
	sdd.SearchIndexStore
	hits []sdd.ScoredChunkHit
}

func (s *candidateIndex) Nearest(context.Context, []sdd.IndexNamespace, []float32, int) ([]sdd.ScoredChunkHit, error) {
	return s.hits, nil
}

func TestNoSyncHashesOnlyReturnedEligibleEntries(t *testing.T) {
	const selected = "20260101-100000-s-tac-aaa"
	var entries []sdd.EntryDocument
	for _, item := range []struct{ suffix, kind string }{{"aaa", "gap"}, {"bbb", "fact"}, {"ccc", "gap"}} {
		entries = append(entries, sdd.EntryDocument{
			LogicalPath: "2026/01/01-100000-s-tac-" + item.suffix + ".md",
			Frontmatter: map[string]any{"type": "signal", "kind": item.kind, "layer": "tactical", "summary": "Alpha " + item.suffix},
			Body:        "Alpha body.", Attachments: []string{"note.md"},
		})
	}
	snapshot, err := sdd.BuildSnapshot(t.Context(), sdd.SnapshotData{Project: "base", Revision: "r1", Entries: entries})
	if err != nil {
		t.Fatal(err)
	}
	graph := &countingAttachmentStore{staticGraphStore: staticGraphStore{snapshot: snapshot, attachment: "Alpha attachment."}, reads: map[string]int{}}
	memory := localadapter.NewMemorySearchIndexStore()
	index := &candidateIndex{SearchIndexStore: memory}
	embeddings := &countingEmbeddings{}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "base"}, Graph: graph, Embedder: embeddings, SearchIndex: index, ExcludeEmbeddedFromIndex: true,
		LLM: pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) { return pkgllm.Result{}, nil }),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ReconcileSearchIndex(t.Context(), sdd.ReconcileSearchIndexCmd{}); err != nil {
		t.Fatal(err)
	}
	namespace := sdd.IndexNamespace{Project: "base", Fingerprint: embeddings.Fingerprint(), Metric: "cosine"}
	hits, err := memory.Nearest(t.Context(), []sdd.IndexNamespace{namespace}, keywordVec("alpha"), 100)
	if err != nil {
		t.Fatal(err)
	}
	var current, filtered sdd.ScoredChunkHit
	for _, hit := range hits {
		if hit.EntryID == selected {
			current = hit
		}
		if hit.EntryID == "20260101-100000-s-tac-bbb" {
			filtered = hit
		}
	}
	if current.EntryHash == "" || filtered.EntryHash == "" {
		t.Fatal("fixture lacks versioned hits")
	}
	stale := current
	stale.EntryHash = "another-branch-version"
	absent := current
	absent.EntryID = "20260101-100000-s-tac-absent"
	app, err := sdd.NewApplication(sdd.ApplicationOptions{Access: &multiAccessResolver{base: runtime}, Sessions: noSessionStore{}, StagedBlobs: noBlobStore{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                   string
		hits                   []sdd.ScoredChunkHit
		wantReads, wantResults int
	}{
		{"current and duplicate candidates", []sdd.ScoredChunkHit{current, current, stale, filtered, absent}, 1, 1},
		{"other version only", []sdd.ScoredChunkHit{stale, filtered, absent}, 1, 0},
		{"no eligible hits", []sdd.ScoredChunkHit{filtered, absent}, 0, 0},
		{"empty index", nil, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index.hits = tc.hits
			graph.reads = map[string]int{}
			embeddings.reset()
			result, err := app.Search(t.Context(), sdd.RequestIdentity{Subject: "reader"}, "base", sdd.SearchRequest{SyncMode: sdd.SearchSyncNone, Phrase: "alpha", Kind: "gap"})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.EntryIDs) != tc.wantResults {
				t.Fatalf("results = %v", result.EntryIDs)
			}
			if graph.reads[selected] != tc.wantReads {
				t.Fatalf("candidate reads = %v", graph.reads)
			}
			for id, count := range graph.reads {
				if id != selected && count != 0 {
					t.Fatalf("read unrelated attachment: %s (%d)", id, count)
				}
			}
			if embeddings.docEmbeds != 0 {
				t.Fatal("no-sync embedded documents")
			}
		})
	}
}
