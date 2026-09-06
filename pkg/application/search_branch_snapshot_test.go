package application_test

import (
	"context"
	"slices"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
	"github.com/networkteam/sdd/pkg/local"
)

type multiBranchSnapshotReader struct {
	sdd.GraphStore
	selected sdd.SnapshotReader
	queries  []sdd.SnapshotReadQuery
}

func (r *multiBranchSnapshotReader) AcquireSnapshot(ctx context.Context, q sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
	r.queries = append(r.queries, q)
	if q.Branch == "work" {
		q.Branch = ""
		q.IncludesRevision = ""
		return r.selected.AcquireSnapshot(ctx, q)
	}
	return r.GraphStore.(sdd.SnapshotReader).AcquireSnapshot(ctx, q)
}

func TestSearchPinnedReadPreservesAuthorizedBranch(t *testing.T) {
	_, base, _ := preparedRuntime(t, "base")
	_, work, workDir := preparedRuntime(t, "base")
	putSearchEntry(t, workDir, "bbb", "Only on selected branch")
	reader := &multiBranchSnapshotReader{GraphStore: base, selected: work}
	options := sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "base"}, Graph: reader, SearchIndex: local.NewMemorySearchIndexStore(),
		Targets: sdd.TargetAcquirerFunc(func(_ context.Context, target sdd.MutationTarget) (*sdd.AcquiredTarget, error) {
			return &sdd.AcquiredTarget{Target: target, Graph: reader, Release: func() error { return nil }}, nil
		}),
		LLM: llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) { return llm.Result{}, nil }),
		Embedder: embed.EmbedderFunc{Space: "test-space", Run: func(_ context.Context, req embed.Request) (embed.Result, error) {
			result := embed.Result{Vectors: make([][]float32, len(req.Texts))}
			for i := range result.Vectors {
				result.Vectors[i] = []float32{1, 1}
			}
			return result, nil
		}}, ExcludeEmbeddedFromIndex: true,
	}
	runtime, err := sdd.NewProjectRuntime(options)
	if err != nil {
		t.Fatal(err)
	}
	options.Graph = work
	options.Targets = nil
	workRuntime, err := sdd.NewProjectRuntime(options)
	if err != nil {
		t.Fatal(err)
	}
	var targetRevision string
	app := preparationApp(t, runtime, nil, func(ctx context.Context, target sdd.SearchTarget) error {
		count := 0
		for item, err := range target.Entries(ctx) {
			if err != nil {
				return err
			}
			count++
			targetRevision = item.Entry.SourceRevision
		}
		if count != 2 {
			t.Errorf("preparation selected %d entries, want work's two", count)
		}
		// A separate worker uses exact source retained by the selected graph.
		return workRuntime.ReconcileSearchIndex(ctx, sdd.ReconcileSearchIndexCmd{})
	})
	result, err := app.Search(t.Context(), sdd.RequestIdentity{Subject: "reader"}, "base", sdd.SearchRequest{Branch: "work", IncludesRevision: "containing-write", SyncMode: sdd.SearchSyncNone, Phrase: "alpha", Terms: []string{"Only"}})
	if !slices.Contains(result.EntryIDs, "20260101-100000-s-tac-bbb") {
		t.Fatalf("retrieval=%v", result.EntryIDs)
	}
	// The source adapter owns the causal guarantee; this fixture records it.
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.queries) != 1 || reader.queries[0].Branch != "work" || reader.queries[0].IncludesRevision != "containing-write" {
		t.Fatalf("queries=%+v", reader.queries)
	}
	if len(result.Coverage) != 1 || result.Coverage[0].Revision != targetRevision || result.Coverage[0].Required != 2 || !result.Coverage[0].Complete {
		t.Fatalf("coverage=%+v", result.Coverage)
	}
}
