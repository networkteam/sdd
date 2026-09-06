package application_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
	"github.com/networkteam/sdd/pkg/local"
)

func preparedRuntime(t *testing.T, project sdd.ProjectID, dependencies ...string) (*sdd.ProjectRuntime, *local.FilesystemGraphStore, string) {
	t.Helper()
	dir := t.TempDir()
	putSearchEntry(t, dir, "aaa", "Alpha searchable body.")
	graph, err := local.NewFilesystemGraphStore(local.FilesystemGraphStoreOptions{Project: project, GraphDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: project}, Graph: graph, Dependencies: dependencies, ExcludeEmbeddedFromIndex: true,
		LLM:         llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) { return llm.Result{}, nil }),
		SearchIndex: local.NewMemorySearchIndexStore(), Embedder: embed.EmbedderFunc{Space: "test-space", Run: func(_ context.Context, req embed.Request) (embed.Result, error) {
			result := embed.Result{Vectors: make([][]float32, len(req.Texts))}
			for i := range result.Vectors {
				result.Vectors[i] = []float32{1, 1}
			}
			return result, nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtime, graph, dir
}

func putSearchEntry(t *testing.T, dir, suffix, body string) {
	t.Helper()
	name := filepath.Join(dir, "2026", "01", "01-100000-s-tac-"+suffix+".md")
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntype: signal\nkind: gap\nlayer: tactical\nsummary: Alpha summary\n---\n\n" + body
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func preparationApp(t *testing.T, base, dep *sdd.ProjectRuntime, prepare func(context.Context, sdd.SearchTarget) error) *sdd.Application {
	t.Helper()
	app, err := sdd.NewApplication(sdd.ApplicationOptions{Access: &multiAccessResolver{base: base, dependency: dep}, Sessions: noSessionStore{}, StagedBlobs: noBlobStore{}, PrepareSearch: prepare})
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func searchPrepared(t *testing.T, app *sdd.Application, includes string) (sdd.SearchResult, error) {
	t.Helper()
	return app.Search(t.Context(), sdd.RequestIdentity{Subject: "reader"}, "base", sdd.SearchRequest{SyncMode: sdd.SearchSyncNone, Phrase: "alpha", AllRepos: true, IncludesRevision: includes})
}

func TestLocalSearchPreparationComposition(t *testing.T) {
	base, _, baseDir := preparedRuntime(t, "base", "dep")
	dep, _, depDir := preparedRuntime(t, "dep")
	runtimes := map[sdd.ProjectID]*sdd.ProjectRuntime{"base": base, "dep": dep}
	calls := 0
	var retained sdd.SearchTarget
	app := preparationApp(t, base, dep, func(ctx context.Context, target sdd.SearchTarget) error {
		calls++
		retained = target
		if len(target.Projects()) != 2 {
			return fmt.Errorf("dependencies not selected before preparation")
		}
		// Both selected sources must survive writes while preparation runs.
		putSearchEntry(t, baseDir, "bbb", "Later home entry")
		putSearchEntry(t, depDir, "bbb", "Later dependency entry")
		for item, err := range target.Entries(ctx) {
			if err != nil {
				return err
			}
			if err := runtimes[item.Entry.Version.Namespace.Project].IndexSearchEntry(ctx, sdd.IndexSearchEntryCmd{Entry: item.Entry}); err != nil {
				return err
			}
		}
		return nil
	})
	result, err := searchPrepared(t, app, "")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(result.Coverage) != 2 || len(result.EntryIDs) != 2 {
		t.Fatalf("result=%+v calls=%d", result, calls)
	}
	for _, coverage := range result.Coverage {
		if !coverage.Complete || coverage.Required != 1 {
			t.Fatalf("moving target: %+v", coverage)
		}
	}
	for _, err := range retained.Entries(t.Context()) {
		if err == nil {
			t.Fatal("expired target remained usable")
		}
		break
	}
}

func TestExternalConsumerPreparationDoesNotClaimCoverage(t *testing.T) {
	base, _, _ := preparedRuntime(t, "base")
	var queued []sdd.SearchEntryDescriptor
	app := preparationApp(t, base, nil, func(ctx context.Context, target sdd.SearchTarget) error {
		for item, err := range target.Entries(ctx) {
			if err != nil {
				return err
			}
			queued = append(queued, item.Entry)
		}
		timer := time.NewTimer(time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			return ctx.Err()
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	result, err := searchPrepared(t, app, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || len(result.EntryIDs) != 0 || len(result.Coverage) != 1 || result.Coverage[0].Complete || result.Notice == "" {
		t.Fatalf("false completion: %+v", result)
	}
}

func TestPreparationErrorsAndLazyIterationErrorsPropagate(t *testing.T) {
	base, _, _ := preparedRuntime(t, "base")
	failure := errors.New("consumer preparation failed")
	app := preparationApp(t, base, nil, func(context.Context, sdd.SearchTarget) error { return failure })
	if _, err := searchPrepared(t, app, ""); !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
	app = preparationApp(t, base, nil, func(ctx context.Context, target sdd.SearchTarget) error {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		for _, err := range target.Entries(canceled) {
			if err != nil {
				return err
			}
		}
		return nil
	})
	if _, err := searchPrepared(t, app, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("iteration error=%v", err)
	}
}

func TestSearchReadYourWritesAndLocalSourceLifetime(t *testing.T) {
	base, graph, _ := preparedRuntime(t, "base")
	initial, err := graph.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	document := sdd.EntryDocument{LogicalPath: "2026/01/01-100000-s-tac-new.md", Frontmatter: map[string]any{"type": "signal", "kind": "gap", "layer": "tactical", "summary": "New"}, Body: "New write"}
	apply := func(id string, doc sdd.EntryDocument) string {
		before, err := graph.Current(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		batch := sdd.MutationBatch{ID: id, Changes: []sdd.DocumentChange{{LogicalPath: doc.LogicalPath, Document: &doc, CanonicalBytes: []byte("---\ntype: signal\nkind: gap\nlayer: tactical\nsummary: New\n---\n" + doc.Body)}}}
		batch.Digest, err = sdd.MutationBatchDigest(batch)
		if err != nil {
			t.Fatal(err)
		}
		result, err := graph.Apply(t.Context(), before.Revision(), batch, nil)
		if err != nil {
			t.Fatal(err)
		}
		return result.Revision
	}
	written := apply("write-one", document)
	document.LogicalPath = "2026/01/01-100000-s-tac-two.md"
	document.Body = "Second write"
	latest := apply("write-two", document)
	app := preparationApp(t, base, nil, func(ctx context.Context, target sdd.SearchTarget) error {
		for item, err := range target.Entries(ctx) {
			if err != nil {
				return err
			}
			if err := base.IndexSearchEntry(ctx, sdd.IndexSearchEntryCmd{Entry: item.Entry}); err != nil {
				return err
			}
		}
		return nil
	})
	result, err := searchPrepared(t, app, written)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Coverage) != 1 || result.Coverage[0].Revision != latest || result.Coverage[0].Required != 3 || !result.Coverage[0].Complete {
		t.Fatalf("freshness=%+v initial=%s", result, initial.Revision())
	}
	if _, err := searchPrepared(t, app, "unknown-write"); err == nil || !strings.Contains(err.Error(), "include") {
		t.Fatalf("unproved freshness=%v", err)
	}
}

func TestPreparedCompleteNoMatchHasNoIncompleteNotice(t *testing.T) {
	runtime, _, _ := preparedRuntime(t, "base")
	app := preparationApp(t, runtime, nil, func(ctx context.Context, target sdd.SearchTarget) error {
		for requirement, err := range target.Entries(ctx) {
			if err != nil {
				return err
			}
			if err := runtime.IndexSearchEntry(ctx, sdd.IndexSearchEntryCmd{Entry: requirement.Entry}); err != nil {
				return err
			}
		}
		return nil
	})
	result, err := app.Search(t.Context(), sdd.RequestIdentity{Subject: "reader"}, "base", sdd.SearchRequest{SyncMode: sdd.SearchSyncNone, Phrase: "alpha", Type: "decision"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.EntryIDs) != 0 || result.Notice != "" || len(result.Coverage) != 1 || !result.Coverage[0].Complete {
		t.Fatalf("result=%+v", result)
	}
}
