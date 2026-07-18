package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sdd "github.com/networkteam/sdd/application"
	localadapter "github.com/networkteam/sdd/local"
)

const terminalProcedure = `---
type: decision
kind: procedure
layer: process
canonical: terminal-test
confidence: high
summary: A one-step terminal workflow test procedure.
state:
    body: {type: text, desc: completion note}
steps:
    - id: only
      collect: [body]
      transitions:
          - when: hasBody
            to: end(completed)
---

Completes after one report.
`

type failAtPrincipalResolver struct {
	mu      sync.Mutex
	runtime *sdd.ProjectRuntime
	calls   int
	failAt  int
}

func (r *failAtPrincipalResolver) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.failAt != 0 && r.calls == r.failAt {
		return sdd.Principal{}, errors.New("injected shell authorization failure")
	}
	return sdd.Principal{Subject: identity.Subject, Participant: "Christopher"}, nil
}

func (r *failAtPrincipalResolver) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	return sdd.ProjectList{Projects: []sdd.ProjectSummary{{ProjectRef: r.runtime.Project(), CanRead: true, CanWrite: true, State: sdd.ProjectReady}}}, nil
}

func (r *failAtPrincipalResolver) ResolveProject(context.Context, sdd.Principal, sdd.ProjectID, sdd.Access) (*sdd.ProjectRuntime, error) {
	return r.runtime, nil
}

func (*failAtPrincipalResolver) ResolveDependency(context.Context, sdd.Principal, sdd.ProjectID, string) (*sdd.ProjectRuntime, error) {
	return nil, &sdd.ApplicationError{Code: sdd.ErrorProjectUnavailable, Message: "dependency unavailable"}
}

// failResolutionAfter arms the injected authorization failure to fire n
// resolutions from now. Callers arm it so the failure lands on the shell
// re-serve that trails a terminal action — after the action's own appends have
// resolved and durably succeeded — exercising the shell-serve failure path.
func (r *failAtPrincipalResolver) failResolutionAfter(n int) {
	r.mu.Lock()
	r.failAt = r.calls + n
	r.mu.Unlock()
}

func TestTerminalWorkflowActionsSurfaceShellServeFailure(t *testing.T) {
	for _, action := range []string{"advance", "abandon", "park"} {
		t.Run(action, func(t *testing.T) {
			application, resolver := newShellFailureApplication(t)
			identity := sdd.RequestIdentity{Subject: "christopher"}
			workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-" + action})
			if err != nil {
				t.Fatal(err)
			}
			serve, err := workflow.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "terminal-test"})
			if err != nil {
				t.Fatal(err)
			}

			switch action {
			case "advance":
				// The completing advance appends report+transition+completed (three
				// resolves) before the shell re-serve resolves; fail that re-serve.
				resolver.failResolutionAfter(5)
				result, actionErr := workflow.Advance(t.Context(), identity, sdd.WorkflowAdvanceRequest{Instance: serve.Instance, Report: map[string]any{"body": "done"}})
				if actionErr == nil || result == nil || result.Status != "completed" || !strings.Contains(actionErr.Error(), "serving session shell after advancing") {
					t.Fatalf("Advance = %+v, %v", result, actionErr)
				}
			case "abandon":
				// The abandon append resolves and succeeds, then the shell re-serve
				// resolves; fail that re-serve.
				resolver.failResolutionAfter(2)
				result, actionErr := workflow.Abandon(t.Context(), identity, serve.Instance, "test")
				if actionErr == nil || !result.Abandoned || !strings.Contains(actionErr.Error(), "serving session shell after abandoning") {
					t.Fatalf("Abandon = %+v, %v", result, actionErr)
				}
			case "park":
				resolver.failResolutionAfter(2)
				result, actionErr := workflow.Park(t.Context(), identity, serve.Instance, "test")
				if actionErr == nil || result.Instance != serve.Instance || !strings.Contains(actionErr.Error(), "serving session shell after parking") {
					t.Fatalf("Park = %+v, %v", result, actionErr)
				}
			}
		})
	}
}

func newShellFailureApplication(t *testing.T) (*sdd.Application, *failAtPrincipalResolver) {
	t.Helper()
	graphDir := t.TempDir()
	path := filepath.Join(graphDir, "2026", "07", "13-060000-d-prc-trm.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(terminalProcedure), 0o644); err != nil {
		t.Fatal(err)
	}
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: graphDir})
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
		Project: sdd.ProjectRef{ID: "example"}, Graph: graph, Sessions: sessions, StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil }, ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) { return sdd.LLMResult{}, nil }},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := &failAtPrincipalResolver{runtime: runtime}
	application, err := sdd.NewApplication(resolver)
	if err != nil {
		t.Fatal(err)
	}
	return application, resolver
}
