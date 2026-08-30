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

	sdd "github.com/networkteam/sdd/application"
	localadapter "github.com/networkteam/sdd/local"
)

type compositionPermission struct {
	read  bool
	write bool
}

type compositionAccess struct {
	mu          sync.Mutex
	runtimes    map[sdd.ProjectID]*sdd.ProjectRuntime
	permissions map[string]map[sdd.ProjectID]compositionPermission
}

func (a *compositionAccess) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	if identity.Subject == "" {
		return sdd.Principal{}, &sdd.ApplicationError{Code: sdd.ErrorAuthenticationRequired, Message: "identity required"}
	}
	return sdd.Principal{Subject: identity.Subject, Participant: identity.Subject}, nil
}

func (a *compositionAccess) ListProjects(_ context.Context, principal sdd.Principal) (sdd.ProjectList, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var result sdd.ProjectList
	for project, permission := range a.permissions[principal.Subject] {
		if !permission.read && !permission.write {
			continue
		}
		result.Projects = append(result.Projects, sdd.ProjectSummary{
			ProjectRef: a.runtimes[project].Project(), CanRead: permission.read, CanWrite: permission.write, State: sdd.ProjectReady,
		})
	}
	return result, nil
}

func (a *compositionAccess) ResolveProject(_ context.Context, principal sdd.Principal, project sdd.ProjectID, access sdd.Access) (*sdd.ProjectRuntime, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	permission := a.permissions[principal.Subject][project]
	allowed := permission.read
	code := sdd.ErrorReadDenied
	if access == sdd.AccessWrite {
		allowed = permission.write
		code = sdd.ErrorWriteDenied
	}
	if !allowed {
		return nil, &sdd.ApplicationError{Code: code, Message: "access denied"}
	}
	return a.runtimes[project], nil
}

func (a *compositionAccess) ResolveDependency(ctx context.Context, principal sdd.Principal, _ sdd.ProjectID, dependency string) (*sdd.ProjectRuntime, error) {
	return a.ResolveProject(ctx, principal, sdd.ProjectID(dependency), sdd.AccessRead)
}

func (a *compositionAccess) revoke(subject string, project sdd.ProjectID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissions[subject][project] = compositionPermission{}
}

func TestMultiProjectCompositionBindingAndCurrentAccess(t *testing.T) {
	root := t.TempDir()
	graphA := filepath.Join(root, "project-a")
	graphB := filepath.Join(root, "project-b")
	entryPath := filepath.Join(graphB, "2026", "07", "13-120000-s-tac-dep.md")
	if err := os.MkdirAll(filepath.Dir(entryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entryPath, []byte(`---
type: signal
kind: fact
layer: tactical
confidence: high
summary: Project B dependency fixture.
---

Project B is readable as an authorized dependency.`), 0o644); err != nil {
		t.Fatal(err)
	}
	storeA, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "project-a", GraphDir: graphA})
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "project-b", GraphDir: graphB})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := localadapter.NewFilesystemSessionStoreAt(filepath.Join(root, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(filepath.Join(root, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	llm := sdd.LLMExecutorFuncs{
		CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
		ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) {
			return sdd.LLMResult{Output: []byte(`{"findings":[]}`), ExecutorFingerprint: "test"}, nil
		},
	}
	runtimeA, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "project-a", DisplayName: "Project A"}, Dependencies: []string{"project-b"},
		Graph: storeA, Sessions: sessions, StagedBlobs: blobs, LLM: llm, LLMTimeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimeB, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "project-b", DisplayName: "Project B"},
		Graph:   storeB, Sessions: sessions, StagedBlobs: blobs, LLM: llm, LLMTimeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	access := &compositionAccess{
		runtimes: map[sdd.ProjectID]*sdd.ProjectRuntime{"project-a": runtimeA, "project-b": runtimeB},
		permissions: map[string]map[sdd.ProjectID]compositionPermission{
			"alice":  {"project-a": {read: true, write: true}, "project-b": {read: true}},
			"bob":    {"project-b": {read: true, write: true}},
			"reader": {"project-a": {read: true}},
			"carol":  {"project-a": {read: true, write: true}},
		},
	}
	application, err := sdd.NewApplication(access)
	if err != nil {
		t.Fatal(err)
	}

	alice := sdd.RequestIdentity{Subject: "alice"}
	bob := sdd.RequestIdentity{Subject: "bob"}
	reader := sdd.RequestIdentity{Subject: "reader"}
	carol := sdd.RequestIdentity{Subject: "carol"}
	infoA, err := application.Info(t.Context(), alice, "project-a", sdd.InfoRequest{})
	if err != nil || infoA.Project.ID != "project-a" || infoA.Project.DisplayName != "Project A" || infoA.Participant != "alice" {
		t.Fatalf("alice project A info = %+v, %v", infoA, err)
	}
	infoB, err := application.Info(t.Context(), bob, "", sdd.InfoRequest{})
	if err != nil || infoB.Project.ID != "project-b" {
		t.Fatalf("bob local alias promotion = %+v, %v", infoB, err)
	}
	if _, err := application.Info(t.Context(), alice, "", sdd.InfoRequest{}); applicationErrorCode(err) != sdd.ErrorProjectRequired {
		t.Fatalf("ambiguous local alias error = %v", err)
	}
	if _, err := application.Info(t.Context(), bob, "project-a", sdd.InfoRequest{}); applicationErrorCode(err) != sdd.ErrorReadDenied {
		t.Fatalf("bob project A access = %v", err)
	}
	qualified := "project-b:20260713-120000-s-tac-dep"
	shown, err := application.Show(t.Context(), alice, "project-a", sdd.ShowRequest{IDs: []string{qualified}})
	if err != nil || shown.Project.ID != "project-a" || !strings.Contains(shown.Entries, "authorized dependency") {
		t.Fatalf("authorized dependency show = %+v, %v", shown, err)
	}

	workflow, _, err := application.OpenWorkflow(t.Context(), reader, "project-a", sdd.WorkflowOpenRequest{MCPSessionID: "reader-mcp"})
	if err != nil {
		t.Fatalf("read-only dialogue open: %v", err)
	}
	if _, err := workflow.Start(t.Context(), reader, sdd.WorkflowStartRequest{Canonical: "capture"}); err != nil {
		t.Fatalf("read-only dialogue start: %v", err)
	}
	if _, err := application.CreateEntry(t.Context(), reader, "project-a", workflow.Binding(), sdd.EntryDraft{}); applicationErrorCode(err) != sdd.ErrorWriteDenied {
		t.Fatalf("read-only mutation = %v", err)
	}

	aliceWorkflow, _, err := application.OpenWorkflow(t.Context(), alice, "project-a", sdd.WorkflowOpenRequest{MCPSessionID: "alice-mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := application.ResumeWorkflow(t.Context(), alice, "project-b", sdd.WorkflowResumeRequest{
		SessionID: aliceWorkflow.ID(), MCPSessionID: "alice-other-project",
	}); applicationErrorCode(err) != sdd.ErrorSessionOwnership {
		t.Fatalf("project binding changed = %v", err)
	}
	if _, _, err := application.ResumeWorkflow(t.Context(), carol, "project-a", sdd.WorkflowResumeRequest{
		SessionID: aliceWorkflow.ID(), MCPSessionID: "carol-mcp",
	}); applicationErrorCode(err) != sdd.ErrorSessionOwnership {
		t.Fatalf("principal binding changed = %v", err)
	}

	access.revoke("alice", "project-a")
	if _, err := aliceWorkflow.ServeAll(t.Context(), alice); applicationErrorCode(err) != sdd.ErrorReadDenied {
		t.Fatalf("workflow reused access after revocation = %v", err)
	}
}

func applicationErrorCode(err error) sdd.ErrorCode {
	var applicationError *sdd.ApplicationError
	if errors.As(err, &applicationError) {
		return applicationError.Code
	}
	return ""
}
