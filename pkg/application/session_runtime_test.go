package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

type activityTargetAcquirer struct {
	mu           sync.Mutex
	graph        sdd.GraphStore
	active       bool
	acquisitions int
	releases     int
}

func (a *activityTargetAcquirer) Acquire(_ context.Context, target sdd.MutationTarget) (*sdd.AcquiredTarget, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active {
		return nil, errors.New("target acquisition overlapped another operation")
	}
	a.active = true
	a.acquisitions++
	return &sdd.AcquiredTarget{
		Target: target, Graph: a.graph,
		Release: func() error {
			a.mu.Lock()
			defer a.mu.Unlock()
			a.active = false
			a.releases++
			return nil
		},
	}, nil
}

func (a *activityTargetAcquirer) isActive() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.active
}

// openBinding creates a durable session with an attachment and returns the
// matching write binding.
func openBinding(t *testing.T, sessions sdd.SessionStore, subject string, id sdd.SessionID) sdd.SessionBinding {
	t.Helper()
	created, err := sessions.Create(t.Context(), sdd.SessionMetadata{
		ID: id, Subject: subject, Project: "example", Participant: subject,
		Attachment: &sdd.Attachment{Subject: subject, ClientName: "mcp", LastActivity: time.Now().UTC().Round(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return sdd.SessionBinding{SessionID: id, Subject: subject, Project: "example", Version: created.Version}
}

func TestCreateEntryResolvesConcreteDefaultWithoutCWDAndReleasesAroundLLM(t *testing.T) {
	graphDir := t.TempDir()
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: graphDir})
	if err != nil {
		t.Fatal(err)
	}
	targets := &activityTargetAcquirer{graph: graph}
	sessions, err := localadapter.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	llmCalls := 0
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: branchReadFixture{GraphStore: graph, targets: targets, project: "example"}, Targets: targets,
		LLM: pkgllm.RunnerFunc(func(_ context.Context, request pkgllm.Request) (pkgllm.Result, error) {
			identity := pkgllm.Identity{Provider: "test", Model: "test"}
			if targets.isActive() {
				return pkgllm.Result{}, errors.New("LLM executed while mutation target was acquired")
			}
			llmCalls++
			if request.Purpose == pkgllm.PurposePreflight {
				return pkgllm.Result{Text: `{"findings":[]}`, Identity: identity}, nil
			}
			return pkgllm.Result{Text: `{"title": "Test title", "summary": "Concrete target resolution is independent of cwd."}`, Identity: identity}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: &runtimeAccessResolver{runtime: runtime}, Sessions: sessions, StagedBlobs: blobs})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}
	binding := openBinding(t, sessions, identity.Subject, "create-default")
	t.Chdir(t.TempDir())
	draft := identifiedCaptureDraft(binding, 1, sdd.EntryDraft{
		Kind: "fact", Layer: "tactical", Body: "Concrete target resolution must remain independent of the process working directory.", Confidence: "high",
		Topics: []string{"implementation/engine"}, Index: &sdd.FactIndex{Title: "Concrete target resolution", Topic: "implementation/engine"},
	})
	created, err := preflightAndCreateEntry(t, application, identity, binding, draft)
	if err != nil || created.EntryID == "" {
		t.Fatalf("CreateEntry = %+v, %v", created, err)
	}
	targets.mu.Lock()
	acquisitions, releases, active := targets.acquisitions, targets.releases, targets.active
	targets.mu.Unlock()
	if llmCalls != 2 || acquisitions != 3 || releases != 3 || active {
		t.Fatalf("LLM calls=%d acquisitions=%d releases=%d active=%v", llmCalls, acquisitions, releases, active)
	}
	stored, err := sessions.Load(t.Context(), binding.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range stored.Events {
		if event.Code == "mutation_intent" {
			t.Fatal("CreateEntry persisted a second prepared payload ledger")
		}
	}
	rel, err := model.IDToRelPath(created.EntryID)
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(graphDir, rel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "index:\n    title: Concrete target resolution\n    topic: implementation/engine\n") {
		t.Fatalf("written entry missing nested index:\n%s", written)
	}
}

func TestSessionReplayFailsClosedForUnsupportedCodec(t *testing.T) {
	application, sessions, _ := newDurableApplication(t, time.Now, nil, nil)
	created, err := sessions.Create(t.Context(), sdd.SessionMetadata{ID: "future", Subject: "christopher", Project: "example"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sessions.Append(t.Context(), "future", created.Version, sdd.SessionAppend{
		Events: []sdd.StoredEvent{{CodecVersion: 99, Code: sdd.WorkflowEventCode, Payload: []byte(`{}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = application.ResumeWorkflow(t.Context(), sdd.RequestIdentity{Subject: "christopher"}, sdd.WorkflowResumeRequest{SessionID: "future", ClientName: "mcp"})
	var migration *sdd.ApplicationError
	if !errors.As(err, &migration) || migration.Code != sdd.ErrorMigrationRequired || migration.Version != 99 {
		t.Fatalf("unsupported codec error = %#v", err)
	}
}

type graphFixture struct {
	sdd.GraphStore
	dir string
}

func newDurableApplication(t *testing.T, now func() time.Time, wrap func(sdd.GraphStore) sdd.GraphStore, finalizers []sdd.MutationFinalizer) (*sdd.Application, *localadapter.FilesystemSessionStore, graphFixture) {
	t.Helper()
	dir := t.TempDir()
	baseGraph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var graph sdd.GraphStore = baseGraph
	if wrap != nil {
		graph = wrap(graph)
	}
	sessions, err := localadapter.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	baseBlobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs := baseBlobs
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: graph, Finalizers: finalizers,
		LLM: pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
			return pkgllm.Result{Identity: pkgllm.Identity{Provider: "test", Model: "test"}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := &runtimeAccessResolver{runtime: runtime}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: resolver, Sessions: sessions, StagedBlobs: blobs, Clock: sdd.ClockFunc(now)})
	if err != nil {
		t.Fatal(err)
	}
	return application, sessions, graphFixture{GraphStore: baseGraph, dir: dir}
}
