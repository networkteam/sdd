package application_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

const applicationEntry = `---
type: signal
kind: gap
layer: tactical
confidence: high
participants:
  - Christopher
summary: Protocol-neutral application fixture.
---

The protocol-neutral runtime owns graph reads and search semantics.`

type runtimeAccessResolver struct {
	mu         sync.Mutex
	runtime    *sdd.ProjectRuntime
	identities []sdd.RequestIdentity
	accesses   []sdd.Access
}

func (r *runtimeAccessResolver) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	r.mu.Lock()
	r.identities = append(r.identities, identity)
	r.mu.Unlock()
	return sdd.Principal{Subject: identity.Subject, Participant: "Christopher"}, nil
}
func (r *runtimeAccessResolver) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	return sdd.ProjectList{Projects: []sdd.ProjectSummary{{ProjectRef: r.runtime.Project(), CanRead: true, CanWrite: true, State: sdd.ProjectReady}}}, nil
}
func (r *runtimeAccessResolver) ResolveProject(_ context.Context, _ sdd.Principal, _ sdd.ProjectID, access sdd.Access) (*sdd.ProjectRuntime, error) {
	r.mu.Lock()
	r.accesses = append(r.accesses, access)
	r.mu.Unlock()
	return r.runtime, nil
}
func (*runtimeAccessResolver) ResolveDependency(context.Context, sdd.Principal, sdd.ProjectID, string) (*sdd.ProjectRuntime, error) {
	return nil, nil
}

func TestApplicationResolvesCurrentAccessAndOwnsReads(t *testing.T) {
	graphDir := t.TempDir()
	entryPath := filepath.Join(graphDir, "2026", "07", "13-030000-s-tac-api.md")
	if err := os.MkdirAll(filepath.Dir(entryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entryPath, []byte(applicationEntry), 0o644); err != nil {
		t.Fatal(err)
	}
	attachmentDir := filepath.Join(graphDir, "2026", "07", "13-030000-s-tac-api")
	if err := os.MkdirAll(attachmentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attachmentDir, "evidence.md"), []byte("attachment"), 0o644); err != nil {
		t.Fatal(err)
	}
	brokenProcedure := "---\ntype: decision\nlayer: prc\nkind: procedure\ncanonical: broken-move\nparams:\n    goalHint: {type: text, optional: true, desc: hint}\nstate:\n    synthesis: {type: text, desc: outcome}\nsteps:\n    - id: work\n      collect: [synthesis]\n      transitions:\n          - when: hasSynthesis\n            to: nowhere\n---\n\nA procedure whose transition targets a step that does not exist.\n\n## unit: work\n\nWork.\n"
	brokenPath := filepath.Join(graphDir, "2026", "07", "13-040000-d-prc-brk.md")
	if err := os.WriteFile(brokenPath, []byte(brokenProcedure), 0o644); err != nil {
		t.Fatal(err)
	}
	wipDir := filepath.Join(graphDir, "wip")
	if err := os.MkdirAll(wipDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wip := "---\nentry: 20260713-030000-s-tac-api\nparticipant: Codex\n---\n\nProtocol-neutral application work"
	if err := os.WriteFile(filepath.Join(wipDir, "20260713-030000-s-tac-api.md"), []byte(wip), 0o644); err != nil {
		t.Fatal(err)
	}

	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: graphDir})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := localadapter.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	embeddings := sdd.EmbeddingExecutorFuncs{
		SpecFunc: func(context.Context) (sdd.EmbeddingSpec, error) {
			return sdd.EmbeddingSpec{Fingerprint: "application-test"}, nil
		},
		EmbedFunc: func(_ context.Context, inputs []sdd.EmbeddingInput) ([]sdd.EmbeddingVector, error) {
			vectors := make([]sdd.EmbeddingVector, len(inputs))
			for i, input := range inputs {
				values := []float32{0, 1}
				if input.Purpose == sdd.EmbeddingQuery || strings.Contains(input.Text, "protocol-neutral") {
					values = []float32{1, 0}
				}
				vectors[i] = sdd.EmbeddingVector{ID: input.ID, Values: values}
			}
			return vectors, nil
		},
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example", DisplayName: "Example"},
		Graph:   graph, Sessions: sessions, StagedBlobs: blobs, Embeddings: embeddings,
		SearchIndex: localadapter.NewMemorySearchIndexStore(),
		LLM: pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
			return pkgllm.Result{Identity: pkgllm.Identity{Provider: "test", Model: "test"}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := &runtimeAccessResolver{runtime: runtime}
	application, err := sdd.NewApplication(resolver)
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher", Scopes: []string{"project:read"}}

	info, err := application.Info(t.Context(), identity, "", sdd.InfoRequest{})
	if err != nil || info.Project.ID != "example" || info.Participant != "Christopher" || info.Search != "vector,text" {
		t.Fatalf("Info = %+v, %v", info, err)
	}
	show, err := application.Show(t.Context(), identity, "example", sdd.ShowRequest{IDs: []string{"20260713-030000-s-tac-api"}, UpDepth: 1, DownDepth: 1})
	if err != nil || !strings.Contains(show.Entries, "protocol-neutral runtime") {
		t.Fatalf("Show = %q, %v", show.Entries, err)
	}
	view, err := application.View(t.Context(), identity, "example", sdd.ViewRequest{Layout: "kind(gap):as-list"})
	if err != nil || !strings.Contains(view.Sections, "s-tac-api") {
		t.Fatalf("View = %q, %v", view.Sections, err)
	}
	wipView, err := application.View(t.Context(), identity, "example", sdd.ViewRequest{Layout: "source(wip):as-wip-list"})
	if err != nil || !strings.Contains(wipView.Sections, "Protocol-neutral application work") {
		t.Fatalf("WIP View = %q, %v", wipView.Sections, err)
	}
	text, err := application.Search(t.Context(), identity, "example", sdd.SearchRequest{Terms: []string{"protocol-neutral"}, Limit: 5, MaxCitations: 1})
	if err != nil || !strings.Contains(text.Results, "s-tac-api") {
		t.Fatalf("text Search = %q, %v", text.Results, err)
	}
	vector, err := application.Search(t.Context(), identity, "example", sdd.SearchRequest{Phrase: "application runtime", Limit: 5, MaxCitations: 1})
	if err != nil || !strings.Contains(vector.Results, "s-tac-api") {
		t.Fatalf("vector Search = %q, %v", vector.Results, err)
	}
	procedures, err := application.Procedures(t.Context(), identity, "example", sdd.ProcedureListRequest{})
	if err != nil || !strings.Contains(procedures.Procedures, "capture") {
		t.Fatalf("Procedures = %q, %v", procedures.Procedures, err)
	}
	if !strings.Contains(procedures.Procedures, "broken-move — spec fails to load:") || !strings.Contains(procedures.Procedures, "nowhere") {
		t.Fatalf("broken procedure not listed as broken: %q", procedures.Procedures)
	}
	attachment, err := application.ReadAttachment(t.Context(), identity, "example", sdd.ReadAttachmentRequest{
		EntryID: "20260713-030000-s-tac-api", Filename: "evidence.md", MaxBytes: 20,
	})
	if err != nil || string(attachment.Page.Content) != "attachment" {
		t.Fatalf("ReadAttachment = %+v, %v", attachment, err)
	}

	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if len(resolver.identities) != 8 || len(resolver.accesses) != 8 {
		t.Fatalf("current access was not resolved per operation: identities=%d access=%d", len(resolver.identities), len(resolver.accesses))
	}
	for _, access := range resolver.accesses {
		if access != sdd.AccessRead {
			t.Fatalf("read operation requested %q access", access)
		}
	}
}
