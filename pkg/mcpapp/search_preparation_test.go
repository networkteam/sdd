package mcpapp_test

import (
	"context"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
	"github.com/networkteam/sdd/pkg/local"
	mcpserver "github.com/networkteam/sdd/pkg/mcpapp"
)

func TestMCPUsesApplicationPreparationAndCoverage(t *testing.T) {
	for _, synchronous := range []bool{false, true} {
		name := "external consumer"
		if synchronous {
			name = "local"
		}
		t.Run(name, func(t *testing.T) {
			graph, err := local.NewFilesystemGraphStore(local.FilesystemGraphStoreOptions{Project: "root-test", GraphDir: writeFixtureGraph(t)})
			if err != nil {
				t.Fatal(err)
			}
			sessions, err := local.NewFilesystemSessionStoreAt(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			blobs, err := local.NewFilesystemStagedBlobStoreAt(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{Project: sdd.ProjectRef{ID: "root-test"}, Graph: graph, ExcludeEmbeddedFromIndex: true,
				LLM: llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) { return llm.Result{}, nil }), SearchIndex: local.NewMemorySearchIndexStore(),
				Embedder: embed.EmbedderFunc{Space: "fixture", Run: func(_ context.Context, req embed.Request) (embed.Result, error) {
					r := embed.Result{Vectors: make([][]float32, len(req.Texts))}
					for i := range r.Vectors {
						r.Vectors[i] = []float32{1, 1}
					}
					return r, nil
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			app, err := sdd.NewApplication(sdd.ApplicationOptions{Access: rootAccess{runtime: runtime}, Sessions: sessions, StagedBlobs: blobs, PrepareSearch: func(ctx context.Context, target sdd.SearchTarget) error {
				calls++
				for item, err := range target.Entries(ctx) {
					if err != nil {
						return err
					}
					if synchronous {
						if err := runtime.IndexSearchEntry(ctx, sdd.IndexSearchEntryCmd{Entry: item.Entry}); err != nil {
							return err
						}
					}
				}
				return nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			server, err := mcpserver.New(mcpserver.Options{Application: app, SearchSyncMode: sdd.SearchSyncNone, LocalClient: true, LocalIdentity: sdd.RequestIdentity{Subject: "tester"}})
			if err != nil {
				t.Fatal(err)
			}
			client := connect(t, server)
			opened := openSession(t, client)
			var result mcpserver.SearchResult
			call(t, client, "search", map[string]any{"session": opened.Session, "query": "oscillation"}, &result)
			if calls != 1 || len(result.Coverage) != 1 || result.Coverage[0].Complete != synchronous {
				t.Fatalf("calls=%d result=%+v", calls, result)
			}
			if (result.Notice == "") != synchronous {
				t.Fatalf("notice=%q", result.Notice)
			}
		})
	}
}
