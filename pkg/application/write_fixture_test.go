package application_test

import (
	"context"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

type writeFixture struct {
	app      *sdd.Application
	identity sdd.RequestIdentity
	binding  sdd.SessionBinding
	graphDir string
	sessions *localadapter.FilesystemSessionStore
	sequence uint64
}

type writeFixtureOptions struct {
	Runner       pkgllm.Runner
	Finalizers   []sdd.MutationFinalizer
	SourceConfig *sdd.ProjectConfig
	Dependency   *sdd.ProjectRuntime
	Dependencies []sdd.RepoID
}

func newWriteFixture(t *testing.T, options ...writeFixtureOptions) *writeFixture {
	t.Helper()
	dir := t.TempDir()
	baseGraph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: dir})
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
	runner := pkgllm.Runner(pkgllm.RunnerFunc(func(_ context.Context, request pkgllm.Request) (pkgllm.Result, error) {
		identity := pkgllm.Identity{Provider: "test", Model: "test"}
		if request.Purpose == pkgllm.PurposePreflight {
			return pkgllm.Result{Text: `{"findings":[]}`, Identity: identity}, nil
		}
		return pkgllm.Result{Text: "A captured entry for the project record.", Identity: identity}, nil
	}))
	var finalizers []sdd.MutationFinalizer
	var graph sdd.GraphStore = baseGraph
	var dependency *sdd.ProjectRuntime
	var dependencies []sdd.RepoID
	for _, option := range options {
		if option.Runner != nil {
			runner = option.Runner
		}
		finalizers = append(finalizers, option.Finalizers...)
		dependencies = append(dependencies, option.Dependencies...)
		if option.SourceConfig != nil {
			reader := acquiredReadStore{acquire: func(ctx context.Context, query sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
				source, err := baseGraph.AcquireSnapshot(ctx, query)
				if err != nil {
					return nil, err
				}
				source.Config = option.SourceConfig
				return source, nil
			}}
			graph = struct {
				sdd.GraphStore
				sdd.SnapshotReader
				sdd.PublicationStore
			}{baseGraph, reader, baseGraph}
		}
		if option.Dependency != nil {
			dependency = option.Dependency
		}
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: graph,
		LLM: runner, Finalizers: finalizers, Dependencies: dependencies,
	})
	if err != nil {
		t.Fatal(err)
	}
	var access sdd.AccessResolver = &runtimeAccessResolver{runtime: runtime}
	if dependency != nil {
		access = &multiAccessResolver{base: runtime, dependency: dependency}
	}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: access, Sessions: sessions, StagedBlobs: blobs})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}
	return &writeFixture{
		app: application, identity: identity, graphDir: dir, sessions: sessions,
		binding: openBinding(t, sessions, identity.Subject, "write-test"),
	}
}

func (f *writeFixture) identifiedDraft(draft sdd.EntryDraft) sdd.EntryDraft {
	f.sequence++
	return identifiedCaptureDraft(f.binding, f.sequence, draft)
}
