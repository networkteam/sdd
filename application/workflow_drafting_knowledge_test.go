package application_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/basefacts"
	"github.com/networkteam/sdd/internal/model"
	localadapter "github.com/networkteam/sdd/local"
)

// Coverage for the capture-time kind knowledge delivery (d-tac-vzz): the
// draftingKnowledge inject serves the type-system overview in capture's
// initial serve at most once per durable session, names a preselected kind's
// authoring fact, and fails loud when the overview cannot resolve — all
// through the real application boundary over the real capture procedure.

// Markers distinguishing a full overview serve from the suppressed pointer.
const (
	overviewBodyMarker      = "a signal records something noticed:"
	overviewServedMarker    = "served here in full"
	overviewSuppressedNote  = "was already served to this session in full"
	kindSequenceInstruction = "Work in this order: read the type-system overview, choose the kind, pull that kind's authoring fact in full, then draft."
)

func newDraftingKnowledgeApp(t *testing.T, graphDir string) (*sdd.Application, sdd.RequestIdentity) {
	t.Helper()
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
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: graph, Sessions: sessions, StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc: func(_ context.Context, request sdd.LLMRequest) (sdd.LLMResult, error) {
				if request.Purpose == "preflight" || request.Purpose == "writing-guide" {
					return sdd.LLMResult{Output: []byte(`{"findings":[]}`)}, nil
				}
				return sdd.LLMResult{Output: []byte("A generated summary.")}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(&runtimeAccessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	return application, sdd.RequestIdentity{Subject: "christopher"}
}

func startCapture(t *testing.T, workflow *sdd.WorkflowSession, identity sdd.RequestIdentity, params map[string]any) *sdd.WorkflowServe {
	t.Helper()
	serve, err := workflow.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "capture", Params: params})
	if err != nil {
		t.Fatal(err)
	}
	return serve
}

func TestCaptureStartServesOverviewOncePerSession(t *testing.T) {
	application, identity := newDraftingKnowledgeApp(t, t.TempDir())
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "dk-once"})
	if err != nil {
		t.Fatal(err)
	}

	first := startCapture(t, workflow, identity, nil)
	if !strings.Contains(first.Instructions, overviewServedMarker) || !strings.Contains(first.Instructions, overviewBodyMarker) {
		t.Fatalf("a fresh capture start must serve the overview body in full, got: %.300q", first.Instructions)
	}
	if !strings.Contains(first.Instructions, basefacts.OverviewFactID) {
		t.Errorf("the overview serve must name the overview fact ID")
	}
	if !strings.Contains(first.Instructions, kindSequenceInstruction) {
		t.Errorf("an unselected capture must carry the overview → kind → fact → draft sequence")
	}
	if strings.Contains(first.Instructions, overviewSuppressedNote) {
		t.Errorf("a fresh start must not carry the already-served note")
	}

	second := startCapture(t, workflow, identity, nil)
	if strings.Contains(second.Instructions, overviewBodyMarker) {
		t.Fatalf("a second capture in the same session must not repeat the overview body")
	}
	if !strings.Contains(second.Instructions, overviewSuppressedNote) || !strings.Contains(second.Instructions, basefacts.OverviewFactID) {
		t.Errorf("the suppressed serve must still point at the overview fact, got: %.300q", second.Instructions)
	}
	// Capture adds no discriminator cue of its own: with the overview body
	// suppressed, the discrimination fact appears nowhere in the serve.
	if strings.Contains(second.Instructions, basefacts.DiscriminationFactID) {
		t.Errorf("capture must not carry a parallel discrimination-fact cue outside the overview body")
	}
	if !strings.Contains(first.Instructions, basefacts.DiscriminationFactID) {
		t.Errorf("the served overview body should remain the home of the discrimination pointer")
	}
}

func TestCaptureStartPriorShowReadSuppressesOverview(t *testing.T) {
	application, identity := newDraftingKnowledgeApp(t, t.TempDir())
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "dk-show"})
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.LogRead(t.Context(), identity, "show", []string{basefacts.OverviewFactID}, nil); err != nil {
		t.Fatal(err)
	}

	serve := startCapture(t, workflow, identity, nil)
	if strings.Contains(serve.Instructions, overviewBodyMarker) {
		t.Fatalf("a prior full show of the overview must suppress the automatic serve")
	}
	if !strings.Contains(serve.Instructions, overviewSuppressedNote) {
		t.Errorf("the suppressed serve must say the overview was already served")
	}
}

func TestCaptureStartSummaryReadDoesNotSuppressOverview(t *testing.T) {
	application, identity := newDraftingKnowledgeApp(t, t.TempDir())
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "dk-summary"})
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.LogRead(t.Context(), identity, "search", nil, []string{basefacts.OverviewFactID}); err != nil {
		t.Fatal(err)
	}

	serve := startCapture(t, workflow, identity, nil)
	if !strings.Contains(serve.Instructions, overviewBodyMarker) {
		t.Fatalf("a summary-only read must not suppress the full overview serve")
	}
}

func TestCaptureStartPreselectedKindNamesAuthoringFact(t *testing.T) {
	application, identity := newDraftingKnowledgeApp(t, t.TempDir())
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "dk-kind"})
	if err != nil {
		t.Fatal(err)
	}

	serve := startCapture(t, workflow, identity, map[string]any{"kind": "directive"})
	if !strings.Contains(serve.Instructions, basefacts.DirectiveFactID) {
		t.Fatalf("a directive-preselected capture must name the directive authoring fact, got: %.300q", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, "directive kind's authoring fact") {
		t.Errorf("the authoring-fact pointer should say which kind it belongs to")
	}

	// The pointer is instructional, never a gate: a valid draft reported
	// without any fact read advances past assemble.
	advanced := advanceWorkflow(t, workflow, identity, serve.Instance, map[string]any{
		"body":        "New graph writes go through the construction boundary so validation stays single-path.",
		"entryKind":   "directive",
		"layer":       "tactical",
		"confidence":  "high",
		"intent":      "guiding",
		"widenReport": "searched the graph for prior write-path decisions before drafting",
	})
	if advanced.Step == "assemble" {
		t.Fatalf("an unread authoring fact must not gate assemble; still held with missing=%v diagnostics=%v", advanced.Missing, advanced.Diagnostics)
	}
}

func TestCaptureStartKindWithoutAuthoringFactGetsNoPointer(t *testing.T) {
	application, identity := newDraftingKnowledgeApp(t, t.TempDir())
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "dk-contract"})
	if err != nil {
		t.Fatal(err)
	}

	serve := startCapture(t, workflow, identity, map[string]any{"kind": "contract"})
	if strings.Contains(serve.Instructions, "Before writing the first draft, pull the") {
		t.Fatalf("a kind shipping no authoring fact must get no pointer, got: %.300q", serve.Instructions)
	}
}

func TestCaptureStartReattachedSessionKeepsOverviewGrounding(t *testing.T) {
	application, identity := newDraftingKnowledgeApp(t, t.TempDir())
	workflow, opened, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "dk-conn1"})
	if err != nil {
		t.Fatal(err)
	}
	first := startCapture(t, workflow, identity, nil)
	if !strings.Contains(first.Instructions, overviewBodyMarker) {
		t.Fatalf("the first capture must serve the overview in full")
	}

	resumed, result, err := application.ResumeWorkflow(t.Context(), identity, "example", sdd.WorkflowResumeRequest{
		SessionID: opened.Session, MCPSessionID: "dk-conn2", UserWords: "continue the capture session", Takeover: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, open := range result.Open {
		if open.Procedure == "capture" && strings.Contains(open.Instructions, overviewBodyMarker) {
			t.Errorf("re-attachment must not replay the overview body in the re-served capture step")
		}
	}

	serve := startCapture(t, resumed, identity, nil)
	if strings.Contains(serve.Instructions, overviewBodyMarker) {
		t.Fatalf("a capture started after re-attachment must not repeat the overview body")
	}
	if !strings.Contains(serve.Instructions, overviewSuppressedNote) {
		t.Errorf("the re-attached serve must still point at the overview fact")
	}
}

func TestCaptureStartFailsLoudOnEmptyOverviewFact(t *testing.T) {
	dir := t.TempDir()
	writeEmptyOverviewOverride(t, dir, "20260819-100000-s-prc-ovr")

	application, identity := newDraftingKnowledgeApp(t, dir)
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "dk-fail"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = workflow.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "capture"})
	if err == nil {
		t.Fatal("a capture over an empty overview override must fail loudly, not drop the guidance")
	}
	if !strings.Contains(err.Error(), "draftingKnowledge") || !strings.Contains(err.Error(), "empty body") {
		t.Fatalf("the failure must name the inject and the empty fact, got: %v", err)
	}
}

// writeEmptyOverviewOverride plants a project fact that supersedes the base
// type-system overview with an empty body — the live-head resolution then
// lands on a fact with nothing to serve.
func writeEmptyOverviewOverride(t *testing.T, dir, id string) {
	t.Helper()
	rel, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntype: signal\nkind: fact\nlayer: process\nsummary: An empty project override of the type-system overview.\nsupersedes:\n    - " + basefacts.OverviewFactID + "\n---\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
