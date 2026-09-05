package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

func indexRuntime(t *testing.T, project sdd.ProjectID, index sdd.SearchIndexStore, embeddings *countingEmbeddings, deps ...string) *sdd.ProjectRuntime {
	t.Helper()
	snapshot, err := sdd.BuildSnapshot(t.Context(), sdd.SnapshotData{
		Project: project, Revision: "r1",
		Entries: []sdd.EntryDocument{{
			LogicalPath: "2026/01/01-100000-s-tac-aaa.md",
			Frontmatter: map[string]any{"type": "signal", "kind": "gap", "layer": "tactical", "summary": "Alpha summary"},
			Attachments: []string{"note.md"},
			Body:        "Alpha body.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		ExcludeEmbeddedFromIndex: true,
		Project:                  sdd.ProjectRef{ID: project}, Graph: staticGraphStore{snapshot: snapshot, attachment: "## Attachment\nBeta attachment content."},
		SearchIndex: index, Embedder: embeddings, Dependencies: deps,
		LLM: pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) { return pkgllm.Result{}, nil }),
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

type failingIndex struct {
	sdd.SearchIndexStore
	err error
}

func (s failingIndex) Reconcile(context.Context, sdd.IndexNamespace, string, []sdd.IndexedChunk, []string) error {
	return s.err
}

func TestReconcileSearchIndex(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "entry manifest", true: "chunk identity"}[legacy], func(t *testing.T) {
			memory := localadapter.NewMemorySearchIndexStore()
			var index sdd.SearchIndexStore = memory
			if legacy {
				index = struct{ sdd.SearchIndexStore }{memory}
			}
			embeddings := &countingEmbeddings{}
			runtime := indexRuntime(t, "base", index, embeddings)
			namespace := sdd.IndexNamespace{Project: "base", Fingerprint: embeddings.Fingerprint(), Metric: "cosine"}
			entries, chunks, completions := 0, 0, 0
			cmd := sdd.ReconcileSearchIndexCmd{
				OnEntryIndexed: func(id string, count int) {
					rows, err := index.Manifest(t.Context(), namespace)
					if err != nil || len(rows) == 0 {
						t.Fatalf("callback before persistence: %v, %v", rows, err)
					}
					if id != "20260101-100000-s-tac-aaa" {
						t.Fatalf("entry ID = %s", id)
					}
					entries++
					chunks += count
				},
				OnComplete: func(revision string, indexed, stored int) {
					completions++
					if revision != "r1" || indexed != entries || stored != chunks {
						t.Fatalf("completion = %s, %d, %d", revision, indexed, stored)
					}
				},
			}
			if err := runtime.ReconcileSearchIndex(t.Context(), cmd); err != nil {
				t.Fatal(err)
			}
			if entries != 1 || chunks == 0 || completions != 1 || embeddings.queryEmbeds != 0 {
				t.Fatalf("counts = %d, %d, %d, query %d", entries, chunks, completions, embeddings.queryEmbeds)
			}
			if !strings.Contains(strings.Join(embeddings.docTexts, "\n"), "Beta attachment content") {
				t.Fatal("attachment not indexed")
			}
			embeddings.reset()
			entries, chunks = 0, 0
			if err := runtime.ReconcileSearchIndex(t.Context(), cmd); err != nil {
				t.Fatal(err)
			}
			if embeddings.docEmbeds != 0 || entries != 0 || completions != 2 {
				t.Fatal("repeat reconciliation embedded existing entries")
			}
		})
	}
}

func TestReconcileSearchIndexFailureDoesNotReportSuccess(t *testing.T) {
	want := errors.New("store unavailable")
	index := failingIndex{SearchIndexStore: localadapter.NewMemorySearchIndexStore(), err: want}
	runtime := indexRuntime(t, "base", index, &countingEmbeddings{})
	called := false
	err := runtime.ReconcileSearchIndex(t.Context(), sdd.ReconcileSearchIndexCmd{
		OnEntryIndexed: func(string, int) { called = true },
		OnComplete:     func(string, int, int) { called = true },
	})
	if !errors.Is(err, want) || called {
		t.Fatalf("err = %v, callback = %v", err, called)
	}
}

func TestSearchSynchronizationScope(t *testing.T) {
	for _, mode := range []sdd.SearchSyncMode{sdd.SearchSyncNone, sdd.SearchSyncLocal, sdd.SearchSyncAll} {
		t.Run(string(mode), func(t *testing.T) {
			baseEmb, depEmb := &countingEmbeddings{}, &countingEmbeddings{}
			base := indexRuntime(t, "base", localadapter.NewMemorySearchIndexStore(), baseEmb, "dep")
			dep := indexRuntime(t, "dep", localadapter.NewMemorySearchIndexStore(), depEmb)
			app, err := sdd.NewApplication(sdd.ApplicationOptions{Access: &multiAccessResolver{base: base, dependency: dep}, Sessions: noSessionStore{}, StagedBlobs: noBlobStore{}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = app.Search(t.Context(), sdd.RequestIdentity{Subject: "reader"}, "base", sdd.SearchRequest{SyncMode: mode, Phrase: "alpha", AllRepos: true})
			if err != nil {
				t.Fatal(err)
			}
			if (baseEmb.docEmbeds > 0) != (mode != sdd.SearchSyncNone) {
				t.Fatalf("base embeds = %d", baseEmb.docEmbeds)
			}
			if (depEmb.docEmbeds > 0) != (mode == sdd.SearchSyncAll) {
				t.Fatalf("dependency embeds = %d", depEmb.docEmbeds)
			}
			if baseEmb.queryEmbeds != 1 || depEmb.queryEmbeds != 1 {
				t.Fatal("query not searched in both projects")
			}
		})
	}
}

func TestSearchRejectsMissingOrUnknownSynchronization(t *testing.T) {
	app, err := sdd.NewApplication(sdd.ApplicationOptions{Access: &multiAccessResolver{}, Sessions: noSessionStore{}, StagedBlobs: noBlobStore{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []sdd.SearchSyncMode{"", "invalid"} {
		_, err := app.Search(t.Context(), sdd.RequestIdentity{}, "base", sdd.SearchRequest{SyncMode: mode, Terms: []string{"alpha"}})
		var applicationErr *sdd.ApplicationError
		if !errors.As(err, &applicationErr) || applicationErr.Code != sdd.ErrorInvalidArgument {
			t.Fatalf("mode %q: %v", mode, err)
		}
	}
}

func TestSearchWithoutSynchronizationUsesWarmIndex(t *testing.T) {
	memory := localadapter.NewMemorySearchIndexStore()
	embeddings := &countingEmbeddings{}
	runtime := indexRuntime(t, "base", memory, embeddings)
	if err := runtime.ReconcileSearchIndex(t.Context(), sdd.ReconcileSearchIndexCmd{}); err != nil {
		t.Fatal(err)
	}
	embeddings.reset()
	runtime = indexRuntime(t, "base", failingIndex{SearchIndexStore: memory, err: errors.New("unexpected index write")}, embeddings)
	app, err := sdd.NewApplication(sdd.ApplicationOptions{Access: &multiAccessResolver{base: runtime}, Sessions: noSessionStore{}, StagedBlobs: noBlobStore{}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.Search(t.Context(), sdd.RequestIdentity{Subject: "reader"}, "base", sdd.SearchRequest{SyncMode: sdd.SearchSyncNone, Phrase: "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.EntryIDs) != 1 || result.EntryIDs[0] != "20260101-100000-s-tac-aaa" {
		t.Fatalf("results = %+v", result)
	}
	if embeddings.docEmbeds != 0 || embeddings.queryEmbeds != 1 {
		t.Fatalf("document/query embeds = %d/%d", embeddings.docEmbeds, embeddings.queryEmbeds)
	}
}
