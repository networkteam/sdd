package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/pkg/application"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

func newBranchBindingApplication(t *testing.T, validator sdd.BranchValidator) (*sdd.Application, *localadapter.FilesystemSessionStore) {
	return newBranchBindingApplicationWithStore(t, validator, nil)
}

func newBranchBindingApplicationWithStore(t *testing.T, validator sdd.BranchValidator, wrap func(sdd.SessionStore) sdd.SessionStore) (*sdd.Application, *localadapter.FilesystemSessionStore) {
	t.Helper()
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
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
	var sessionStore sdd.SessionStore = sessions
	if wrap != nil {
		sessionStore = wrap(sessionStore)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, Graph: graph, Branches: validator,
		Targets: sdd.TargetAcquirerFunc(func(_ context.Context, target sdd.MutationTarget) (*sdd.AcquiredTarget, error) {
			return &sdd.AcquiredTarget{Target: target, Graph: graph, Release: func() error { return nil }}, nil
		}),
		Sessions: sessionStore, StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc:      func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) { return sdd.LLMResult{}, nil },
		},

		LLMTimeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(&runtimeAccessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	return application, sessions
}

// advancingBranchConflictStore simulates two same-attachment writers winning
// immediately before this workflow's two CAS attempts. Each injected conflict
// leaves a newer authoritative branch and version in the underlying store.
type advancingBranchConflictStore struct {
	sdd.SessionStore
	mu       sync.Mutex
	armed    bool
	branches []string
	next     int
}

func (s *advancingBranchConflictStore) Append(ctx context.Context, id sdd.SessionID, version uint64, appendData sdd.SessionAppend) (uint64, error) {
	s.mu.Lock()
	if !s.armed || s.next >= len(s.branches) {
		s.mu.Unlock()
		return s.SessionStore.Append(ctx, id, version, appendData)
	}
	branch := s.branches[s.next]
	s.next++
	s.mu.Unlock()

	stored, err := s.Load(ctx, id)
	if err != nil {
		return 0, err
	}
	metadata := stored.Metadata
	metadata.Branch = branch
	payload, err := json.Marshal(map[string]string{"branch": branch})
	if err != nil {
		return 0, err
	}
	if _, err := s.SessionStore.Append(ctx, id, stored.Version, sdd.SessionAppend{
		Metadata: &metadata,
		Events: []sdd.StoredEvent{{
			CodecVersion: sdd.SessionCodecVersion,
			Code:         sdd.BranchBoundEventCode,
			Payload:      payload,
		}},
	}); err != nil {
		return 0, err
	}
	return version, &sdd.ApplicationError{Code: sdd.ErrorSessionConflict, Message: "session version changed"}
}

func (s *advancingBranchConflictStore) arm(branches ...string) {
	s.mu.Lock()
	s.armed = true
	s.branches = append([]string(nil), branches...)
	s.next = 0
	s.mu.Unlock()
}

func TestWorkflowSessionBindBranchPersistsTypedEventsAndClears(t *testing.T) {
	var validated sdd.MutationTarget
	application, sessions := newBranchBindingApplication(t, sdd.BranchValidatorFunc(func(_ context.Context, target sdd.MutationTarget) error {
		validated = target
		return nil
	}))
	identity := sdd.RequestIdentity{Subject: "christopher"}
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-a"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := sessions.Load(t.Context(), workflow.ID())
	if err != nil {
		t.Fatal(err)
	}
	unboundFraming, err := workflow.Framing(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}

	if err := workflow.BindBranch(t.Context(), identity, "feature/session", false); err != nil {
		t.Fatal(err)
	}
	bound, err := sessions.Load(t.Context(), workflow.ID())
	if err != nil {
		t.Fatal(err)
	}
	if validated != (sdd.MutationTarget{Project: "example", Branch: "feature/session"}) {
		t.Fatalf("validated target = %+v", validated)
	}
	if bound.Metadata.Branch != "feature/session" || workflow.Branch() != "feature/session" {
		t.Fatalf("binding was not projected in store and memory: metadata=%q workflow=%q", bound.Metadata.Branch, workflow.Branch())
	}
	if bound.Version != before.Version+1 || workflow.Binding().Version != bound.Version {
		t.Fatalf("bind versions: before=%d stored=%d binding=%d", before.Version, bound.Version, workflow.Binding().Version)
	}
	boundFraming, err := workflow.Framing(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(boundFraming) == 0 || !strings.Contains(boundFraming[0], "Branch binding: feature/session") {
		t.Fatalf("bound framing = %q", boundFraming)
	}
	assertBranchEvent(t, bound.Events[len(bound.Events)-1], sdd.BranchBoundEventCode, "feature/session")

	if err := workflow.BindBranch(t.Context(), identity, "", true); err != nil {
		t.Fatal(err)
	}
	cleared, err := sessions.Load(t.Context(), workflow.ID())
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Metadata.Branch != "" || workflow.Branch() != "" {
		t.Fatalf("clear was not projected in store and memory: metadata=%q workflow=%q", cleared.Metadata.Branch, workflow.Branch())
	}
	if cleared.Version != bound.Version+1 || workflow.Binding().Version != cleared.Version {
		t.Fatalf("clear versions: before=%d stored=%d binding=%d", bound.Version, cleared.Version, workflow.Binding().Version)
	}
	clearedFraming, err := workflow.Framing(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(clearedFraming, unboundFraming) {
		t.Fatalf("unbound framing bytes changed after set/clear:\nbefore=%q\nafter=%q", unboundFraming, clearedFraming)
	}
	assertBranchEvent(t, cleared.Events[len(cleared.Events)-1], sdd.BranchClearedEventCode, "feature/session")
}

func TestWorkflowSessionBindBranchValidationFailureDoesNotMutate(t *testing.T) {
	validationErr := errors.New("branch has no registered checkout")
	application, sessions := newBranchBindingApplication(t, sdd.BranchValidatorFunc(func(context.Context, sdd.MutationTarget) error {
		return validationErr
	}))
	identity := sdd.RequestIdentity{Subject: "christopher"}
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-a"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := sessions.Load(t.Context(), workflow.ID())
	if err != nil {
		t.Fatal(err)
	}

	if err := workflow.BindBranch(t.Context(), identity, "missing", false); !errors.Is(err, validationErr) {
		t.Fatalf("validation error = %v", err)
	}
	after, err := sessions.Load(t.Context(), workflow.ID())
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != before.Version || after.Metadata.Branch != "" || len(after.Events) != len(before.Events) || workflow.Branch() != "" {
		t.Fatalf("validation failure mutated session: before=%+v after=%+v workflow=%q", before, after, workflow.Branch())
	}
}

func TestWorkflowSessionBindBranchNoCapabilityIsTypedAndClearStillWorks(t *testing.T) {
	application, sessions := newBranchBindingApplication(t, nil)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-a"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := sessions.Load(t.Context(), workflow.ID())
	if err != nil {
		t.Fatal(err)
	}

	err = workflow.BindBranch(t.Context(), identity, "feature", false)
	var appErr *sdd.ApplicationError
	if !errors.As(err, &appErr) || appErr.Code != sdd.ErrorBranchUnavailable || !strings.Contains(err.Error(), "no branch concept") {
		t.Fatalf("no-capability error = %#v", err)
	}
	unchanged, err := sessions.Load(t.Context(), workflow.ID())
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Version != before.Version || len(unchanged.Events) != len(before.Events) {
		t.Fatalf("no-capability bind mutated session: before=%+v after=%+v", before, unchanged)
	}

	if err := workflow.BindBranch(t.Context(), identity, "", true); err != nil {
		t.Fatalf("clear must not need branch validation: %v", err)
	}
	cleared, err := sessions.Load(t.Context(), workflow.ID())
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Version != before.Version+1 {
		t.Fatalf("clear version = %d, want %d", cleared.Version, before.Version+1)
	}
	assertBranchEvent(t, cleared.Events[len(cleared.Events)-1], sdd.BranchClearedEventCode, "")
}

func TestWorkflowSessionBindBranchVerifiesAttachmentBeforeValidation(t *testing.T) {
	validations := 0
	application, _ := newBranchBindingApplication(t, sdd.BranchValidatorFunc(func(context.Context, sdd.MutationTarget) error {
		validations++
		return nil
	}))
	identity := sdd.RequestIdentity{Subject: "christopher"}
	displaced, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := application.ResumeWorkflow(t.Context(), identity, "example", sdd.WorkflowResumeRequest{
		SessionID: displaced.ID(), MCPSessionID: "mcp-b", UserWords: "take this session over", Takeover: true,
	}); err != nil {
		t.Fatal(err)
	}

	err = displaced.BindBranch(t.Context(), identity, "feature", false)
	var appErr *sdd.ApplicationError
	if !errors.As(err, &appErr) || appErr.Code != sdd.ErrorSessionDisplaced {
		t.Fatalf("displaced bind error = %#v", err)
	}
	if validations != 0 {
		t.Fatalf("displaced session reached branch validation %d time(s)", validations)
	}
}

func TestWorkflowBranchProjectsFromStoreOnResumeAndList(t *testing.T) {
	application, _ := newBranchBindingApplication(t, sdd.BranchValidatorFunc(func(context.Context, sdd.MutationTarget) error { return nil }))
	identity := sdd.RequestIdentity{Subject: "christopher"}
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.BindBranch(t.Context(), identity, "feature", false); err != nil {
		t.Fatal(err)
	}

	resumed, result, err := application.ResumeWorkflow(t.Context(), identity, "example", sdd.WorkflowResumeRequest{
		SessionID: workflow.ID(), MCPSessionID: "mcp-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Branch() != "feature" || result.Branch != "feature" {
		t.Fatalf("resume projection: workflow=%q result=%q", resumed.Branch(), result.Branch)
	}
	items, err := application.ListWorkflowSessions(t.Context(), identity, "example")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Branch != "feature" {
		t.Fatalf("list projection = %+v", items)
	}
}

func TestWorkflowReorientationRefreshesBranchAfterTerminalCASConflict(t *testing.T) {
	conflicts := &advancingBranchConflictStore{}
	application, sessions := newBranchBindingApplicationWithStore(t,
		sdd.BranchValidatorFunc(func(context.Context, sdd.MutationTarget) error { return nil }),
		func(store sdd.SessionStore) sdd.SessionStore {
			conflicts.SessionStore = store
			return conflicts
		},
	)
	identity := sdd.RequestIdentity{Subject: "christopher"}
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{MCPSessionID: "mcp-a"})
	if err != nil {
		t.Fatal(err)
	}
	conflicts.arm("concurrent-one", "authoritative")

	err = workflow.BindBranch(t.Context(), identity, "requested", false)
	var appErr *sdd.ApplicationError
	if !errors.As(err, &appErr) || appErr.Code != sdd.ErrorSessionConflict || !strings.Contains(err.Error(), "resume_session") {
		t.Fatalf("terminal conflict error = %#v", err)
	}
	stored, err := sessions.Load(t.Context(), workflow.ID())
	if err != nil {
		t.Fatal(err)
	}
	if stored.Metadata.Branch != "authoritative" || workflow.Branch() != "concurrent-one" {
		t.Fatalf("setup did not leave the expected stale cache: stored=%q cached=%q", stored.Metadata.Branch, workflow.Branch())
	}

	// MCP same-session reorientation calls StillHeld before ServeAll. That held
	// check is the convergence point: it refreshes only after confirming this
	// connection remains the current attachment.
	held, err := workflow.StillHeld(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if !held {
		t.Fatal("same attachment should still be held")
	}
	resumed, err := workflow.ServeAll(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if workflow.Branch() != "authoritative" || workflow.Binding().Version != stored.Version || resumed.Branch != "authoritative" {
		t.Fatalf("reorientation did not converge: cached=%q bindingVersion=%d storedVersion=%d result=%q",
			workflow.Branch(), workflow.Binding().Version, stored.Version, resumed.Branch)
	}
	for _, serve := range resumed.Open {
		if serve.Branch != "authoritative" {
			t.Fatalf("resumed serve branch = %q", serve.Branch)
		}
	}
	framing, err := workflow.Framing(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(framing) == 0 || !strings.Contains(framing[0], "Branch binding: authoritative") {
		t.Fatalf("refreshed framing = %q", framing)
	}
}

func assertBranchEvent(t *testing.T, event sdd.StoredEvent, code, branch string) {
	t.Helper()
	if event.Code != code {
		t.Fatalf("event code = %q, want %q", event.Code, code)
	}
	var payload struct {
		Branch string `json:"branch"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Branch != branch {
		t.Fatalf("event branch = %q, want %q", payload.Branch, branch)
	}
}
