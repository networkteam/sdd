package mcpapp_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/model"
	localadapter "github.com/networkteam/sdd/local"
	mcpserver "github.com/networkteam/sdd/mcpapp"
)

type rootAccess struct{ runtime *sdd.ProjectRuntime }

func (r rootAccess) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	return sdd.Principal{Subject: identity.Subject, Participant: "Tester"}, nil
}

func (r rootAccess) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	return sdd.ProjectList{Projects: []sdd.ProjectSummary{{ProjectRef: r.runtime.Project(), CanRead: true, CanWrite: true, State: sdd.ProjectReady}}}, nil
}

func (r rootAccess) ResolveProject(context.Context, sdd.Principal, sdd.ProjectID, sdd.Access) (*sdd.ProjectRuntime, error) {
	return r.runtime, nil
}

func (rootAccess) ResolveDependency(context.Context, sdd.Principal, sdd.ProjectID, string) (*sdd.ProjectRuntime, error) {
	return nil, &sdd.ApplicationError{Code: sdd.ErrorProjectUnavailable, Message: "dependency unavailable"}
}

func TestPublicMCPApplicationRunsStatefulWorkflowOnRootRuntime(t *testing.T) {
	graphDir := writeFixtureGraph(t)
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "root-test", GraphDir: graphDir})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := localadapter.NewFilesystemSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "root-test", DisplayName: "Root test"}, DefaultBranch: "main",
		Graph: graph, Sessions: sessions, StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return []string{"json-schema"}, nil },
			ExecuteFunc: func(_ context.Context, request sdd.LLMRequest) (sdd.LLMResult, error) {
				if request.Purpose == "preflight" {
					return sdd.LLMResult{Output: []byte(`{"findings":[]}`), ExecutorFingerprint: "test"}, nil
				}
				return sdd.LLMResult{Output: []byte("Root runtime generated summary."), ExecutorFingerprint: "test"}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(rootAccess{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "tester"}
	server, err := mcpserver.New(mcpserver.Options{
		Application: application, Project: "root-test", LocalIdentity: identity, LocalClient: true, Version: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	client := connect(t, server)
	opened := openSession(t, client)
	if opened.Framing == "" {
		t.Fatal("root runtime did not serve session framing")
	}

	var search mcpserver.SearchResult
	call(t, client, "search", map[string]any{"terms": []string{"oscillation"}}, &search)
	if search.Results == "" {
		t.Fatal("root runtime search returned no result")
	}
	session := opened.Session
	var staged mcpserver.StageAttachmentResult
	call(t, client, "stage_attachment", map[string]any{"session": session, "name": "root-evidence.md", "content": "root-owned attachment"}, &staged)
	if staged.Handle == "" {
		t.Fatal("root runtime returned no staged blob handle")
	}

	var capture mcpserver.ServeResult
	call(t, client, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "root-owned capture"}, &capture)
	if capture.Procedure != "capture" || capture.Status != "running" {
		t.Fatalf("root runtime capture = %+v", capture)
	}
	report := assembleReport()
	report["attachments"] = []string{staged.Handle}
	call(t, client, "next", map[string]any{"session": session, "instance": capture.Instance, "report": report}, &capture)
	if capture.Step != "playback" {
		t.Fatalf("root runtime report stopped at %q: %+v", capture.Step, capture)
	}
	call(t, client, "next", map[string]any{"session": session, "instance": capture.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "confirm root write",
	}}, &capture)
	if capture.Step != "verifySummary" {
		t.Fatalf("root write gate stopped at %q: %+v", capture.Step, capture)
	}
	call(t, client, "next", map[string]any{"session": session, "instance": capture.Instance, "report": map[string]any{
		"chooser": "verifySummary", "choice": "faithful", "fields": map[string]any{"fidelityNote": "faithful"},
	}}, &capture)
	if capture.Status != "completed" {
		t.Fatalf("root capture did not complete: %+v", capture)
	}
	entryID, _ := capture.Produced["entryId"].(string)
	rel, err := model.IDToRelPath(entryID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(graphDir, rel)); err != nil {
		t.Fatalf("root runtime entry missing: %v", err)
	}
	attachDir, err := model.AttachDirRelPath(entryID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(graphDir, attachDir, "root-evidence.md"))
	if err != nil || string(content) != "root-owned attachment" {
		t.Fatalf("root runtime attachment = %q, %v", content, err)
	}
	stored, err := sessions.List(t.Context(), sdd.SessionFilter{Subject: "tester", Project: "root-test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || len(stored[0].Events) == 0 {
		t.Fatalf("root runtime did not durably append workflow events: %+v", stored)
	}
	for _, event := range stored[0].Events {
		if event.Code == sdd.WorkflowEventCode {
			return
		}
	}
	t.Fatal("session has no versioned root workflow event")
}
