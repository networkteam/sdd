// Package proctest is the integration harness for the shipped base
// procedures: a real application over temp filesystem stores — real registry,
// real template values, real embedded procedure entries — with a scripted
// LLM. Procedure behavior is tested here, at the composition layer that owns
// the wiring, so no test fakes an application contract; internal/engine tests
// stay generic over synthetic specs.
//
// Per-procedure suites live in this directory as package proctest_test, one
// file per procedure, mirroring internal/baseprocedures/entries.
package proctest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

// PreflightFinding is a scripted pre-flight finding, marshaled in the shape
// the real pre-flight parser expects.
type PreflightFinding struct {
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Observation string `json:"observation"`
}

// GuideFinding is a scripted writing-guide finding, marshaled in the shape
// the real guide parser expects.
type GuideFinding struct {
	Reasoning string `json:"reasoning"`
	Axis      string `json:"axis"`
	Quote     string `json:"quote"`
	Repair    string `json:"repair"`
	Severity  string `json:"severity"`
}

// LLMScript scripts the runtime's LLM runner per purpose and counts calls,
// so "the guide ran once" is asserted on real runner invocations instead of
// a faked op. Fields may be changed between calls; a nil findings slice
// scripts a clean pass.
type LLMScript struct {
	mu                sync.Mutex
	calls             map[llm.Purpose]int
	PreflightFindings []PreflightFinding
	GuideFindings     []GuideFinding
	Summary           string
}

// Calls returns how often the runner ran for the purpose.
func (s *LLMScript) Calls(purpose llm.Purpose) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[purpose]
}

var scriptIdentity = llm.Identity{Provider: "proctest", Model: "scripted"}

func (s *LLMScript) Run(_ context.Context, request llm.Request) (llm.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.calls == nil {
		s.calls = map[llm.Purpose]int{}
	}
	s.calls[request.Purpose]++
	switch request.Purpose {
	case llm.PurposeSummarize:
		return llm.Result{Text: s.Summary, Identity: scriptIdentity}, nil
	case llm.PurposePreflight:
		return marshalFindings(s.PreflightFindings)
	case llm.PurposeWritingGuide:
		return marshalFindings(s.GuideFindings)
	default:
		return llm.Result{}, fmt.Errorf("proctest: unscripted LLM purpose %q", request.Purpose)
	}
}

func marshalFindings[F any](findings []F) (llm.Result, error) {
	if findings == nil {
		findings = []F{}
	}
	output, err := json.Marshal(map[string]any{"findings": findings})
	return llm.Result{Text: string(output), Identity: scriptIdentity}, err
}

// MustTopics parses topic labels into the typed paths a fixture entry
// carries; fixtures are static, so a bad label is a programming error.
func MustTopics(labels ...string) []model.TopicPath {
	topics := make([]model.TopicPath, 0, len(labels))
	for _, label := range labels {
		path, err := model.ParseTopicPath(label)
		if err != nil {
			panic(err)
		}
		topics = append(topics, path)
	}
	return topics
}

// WriteEntry writes one fixture entry into the graph dir at its ID-derived
// path, rendered by the production serializer exactly as the write handler
// persists it — usable before NewWorld and mid-test (reads snapshot per
// call, so later reads see it).
func WriteEntry(t *testing.T, graphDir string, e *model.Entry) {
	t.Helper()
	WriteRawEntry(t, graphDir, e.ID, model.FormatFrontmatter(e)+"\n"+e.Content+"\n")
}

// WriteRawEntry writes entry content verbatim for shapes the Entry struct
// does not model.
func WriteRawEntry(t *testing.T, graphDir, id, content string) {
	t.Helper()
	rel, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(graphDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type config struct {
	graphDir    string
	participant string
	entries     []*model.Entry
	branchDirs  map[string]string
}

// Option configures NewWorld.
type Option func(*config)

// WithGraphDir uses an existing graph dir instead of a fresh temp dir —
// e.g. to reopen the same stores as an earlier world (restart scenarios).
func WithGraphDir(dir string) Option { return func(c *config) { c.graphDir = dir } }

// WithEntries writes fixture entries before the stores open.
func WithEntries(entries ...*model.Entry) Option {
	return func(c *config) { c.entries = append(c.entries, entries...) }
}

// WithParticipant sets the resolved participant (default Christopher).
func WithParticipant(name string) Option { return func(c *config) { c.participant = name } }

// WithBranchDir registers an additional branch backed by its own graph dir —
// for procedures that route reads and writes by branch state (implementation's
// work branches). The default branch main stays on the world's GraphDir.
func WithBranchDir(branch, dir string) Option {
	return func(c *config) {
		if c.branchDirs == nil {
			c.branchDirs = map[string]string{}
		}
		c.branchDirs[branch] = dir
	}
}

// branchTargets acquires per-branch graph stores, falling back to the default
// branch's store for unknown branches.
type branchTargets struct {
	fallback sdd.GraphStore
	graphs   map[string]sdd.GraphStore
}

func (b branchTargets) Acquire(_ context.Context, target sdd.MutationTarget) (*sdd.AcquiredTarget, error) {
	graph, ok := b.graphs[target.Branch]
	if !ok {
		graph = b.fallback
	}
	return &sdd.AcquiredTarget{Target: target, Graph: graph, Release: func() error { return nil }}, nil
}

// World is one project: a real application over temp stores with a scripted
// LLM. GraphDir is the on-disk graph — real writes land there.
type World struct {
	App      *sdd.Application
	Identity sdd.RequestIdentity
	GraphDir string
	LLM      *LLMScript
}

type accessResolver struct {
	runtime     *sdd.ProjectRuntime
	participant string
}

func (r *accessResolver) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	return sdd.Principal{Subject: identity.Subject}, nil
}

func (r *accessResolver) ResolveParticipant(context.Context, sdd.Principal, sdd.ProjectID) (string, error) {
	return r.participant, nil
}

func (r *accessResolver) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	return sdd.ProjectList{Projects: []sdd.ProjectSummary{{ProjectRef: r.runtime.Project(), CanRead: true, CanWrite: true, State: sdd.ProjectReady}}}, nil
}

func (r *accessResolver) ResolveProject(context.Context, sdd.Principal, sdd.ProjectID, sdd.Access) (*sdd.ProjectRuntime, error) {
	return r.runtime, nil
}

func (*accessResolver) AuthorizeSession(ctx context.Context, request sdd.SessionAccessRequest) error {
	return sdd.OwnerOnly(ctx, request)
}

func (*accessResolver) ResolveDependency(context.Context, sdd.Principal, sdd.ProjectID, string) (*sdd.ProjectRuntime, error) {
	return nil, nil
}

// NewWorld builds the world: fixture entries written, filesystem stores over
// temp dirs, real project runtime, real application.
func NewWorld(t *testing.T, opts ...Option) *World {
	t.Helper()
	cfg := config{participant: "Christopher"}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.graphDir == "" {
		cfg.graphDir = t.TempDir()
	}
	for _, entry := range cfg.entries {
		WriteEntry(t, cfg.graphDir, entry)
	}

	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "proctest", GraphDir: cfg.graphDir})
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
	script := &LLMScript{Summary: "A generated summary."}
	options := sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "proctest"}, DefaultBranch: "main",
		Graph: graph, LLM: script,
	}
	if len(cfg.branchDirs) > 0 {
		targets := branchTargets{fallback: graph, graphs: map[string]sdd.GraphStore{"main": graph}}
		for branch, dir := range cfg.branchDirs {
			store, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "proctest", GraphDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			targets.graphs[branch] = store
		}
		options.Targets = targets
		options.Branches = sdd.BranchValidatorFunc(func(context.Context, sdd.MutationTarget) error { return nil })
	}
	runtime, err := sdd.NewProjectRuntime(options)
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{
		Access: &accessResolver{runtime: runtime, participant: cfg.participant}, Sessions: sessions, StagedBlobs: blobs,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &World{App: application, Identity: sdd.RequestIdentity{Subject: "tester"}, GraphDir: cfg.graphDir, LLM: script}
}

// Session wraps one workflow session on the world.
type Session struct {
	World *World
	WF    *sdd.WorkflowSession
	ID    sdd.SessionID
}

// Open opens a fresh workflow session; connID names the client for the
// attachment stamp.
func (w *World) Open(t *testing.T, connID string) *Session {
	t.Helper()
	workflow, serve, err := w.App.OpenWorkflow(t.Context(), w.Identity, "proctest", sdd.WorkflowOpenRequest{ClientName: connID})
	if err != nil {
		t.Fatal(err)
	}
	return &Session{World: w, WF: workflow, ID: serve.Session}
}

// Resume loads a session by ID from another client, as a re-attached or
// restarted client would.
func (w *World) Resume(t *testing.T, sessionID sdd.SessionID, connID string) (*Session, sdd.WorkflowResumeResult) {
	t.Helper()
	workflow, result, err := w.App.ResumeWorkflow(t.Context(), w.Identity, sdd.WorkflowResumeRequest{
		SessionID: sessionID, ClientName: connID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Session{World: w, WF: workflow, ID: sessionID}, result
}

// Start starts a procedure instance; params may seed declared params and state.
func (s *Session) Start(t *testing.T, canonical string, params map[string]any) *sdd.WorkflowServe {
	t.Helper()
	serve, err := s.WF.Start(t.Context(), s.World.Identity, sdd.WorkflowStartRequest{Canonical: canonical, Params: params})
	if err != nil {
		t.Fatal(err)
	}
	return serve
}

// StartErr is Start for tests asserting the failure.
func (s *Session) StartErr(t *testing.T, canonical string, params map[string]any) (*sdd.WorkflowServe, error) {
	t.Helper()
	return s.WF.Start(t.Context(), s.World.Identity, sdd.WorkflowStartRequest{Canonical: canonical, Params: params})
}

// Stage places bytes in the session's staged scratch and returns the handle.
func (s *Session) Stage(t *testing.T, filename string, content []byte) string {
	t.Helper()
	handle, err := s.WF.StageAttachment(t.Context(), s.World.Identity, filename, content)
	if err != nil {
		t.Fatal(err)
	}
	return handle
}

// StartChild starts a procedure under a spawning parent, so a dispatch seed
// recorded on the parent's answered junction applies to the new instance.
func (s *Session) StartChild(t *testing.T, canonical string, params map[string]any, parent string) *sdd.WorkflowServe {
	t.Helper()
	serve, err := s.WF.Start(t.Context(), s.World.Identity, sdd.WorkflowStartRequest{Canonical: canonical, Params: params, Parent: parent})
	if err != nil {
		t.Fatal(err)
	}
	return serve
}

// Report sends state fields to the instance's current step.
func (s *Session) Report(t *testing.T, instance string, fields map[string]any) *sdd.WorkflowServe {
	t.Helper()
	serve, err := s.ReportErr(t, instance, fields)
	if err != nil {
		t.Fatal(err)
	}
	return serve
}

// ReportErr is Report for tests asserting the failure.
func (s *Session) ReportErr(t *testing.T, instance string, fields map[string]any) (*sdd.WorkflowServe, error) {
	t.Helper()
	return s.WF.Advance(t.Context(), s.World.Identity, sdd.WorkflowAdvanceRequest{Instance: instance, Report: fields})
}

// Answer answers the pending chooser.
func (s *Session) Answer(t *testing.T, instance, chooser, choice string, fields map[string]any, userWords string) *sdd.WorkflowServe {
	t.Helper()
	serve, err := s.AnswerErr(t, instance, chooser, choice, fields, userWords)
	if err != nil {
		t.Fatal(err)
	}
	return serve
}

// AnswerErr is Answer for tests asserting the failure.
func (s *Session) AnswerErr(t *testing.T, instance, chooser, choice string, fields map[string]any, userWords string) (*sdd.WorkflowServe, error) {
	t.Helper()
	report := map[string]any{"chooser": chooser, "choice": choice}
	if fields != nil {
		report["fields"] = fields
	}
	if userWords != "" {
		report["userWords"] = userWords
	}
	return s.WF.Advance(t.Context(), s.World.Identity, sdd.WorkflowAdvanceRequest{Instance: instance, Report: report})
}

// LogRead records a read into the session ledger, as the host's read tools do.
func (s *Session) LogRead(t *testing.T, tool string, full, summary []string) {
	t.Helper()
	if err := s.WF.LogRead(t.Context(), s.World.Identity, tool, full, summary); err != nil {
		t.Fatal(err)
	}
}

// RequireStep fails unless the serve sits at the step.
func RequireStep(t *testing.T, serve *sdd.WorkflowServe, step string) {
	t.Helper()
	if serve.Step != step {
		t.Fatalf("step = %q, want %q (status %s, missing %v, diagnostics %v)", serve.Step, step, serve.Status, serve.Missing, serve.Diagnostics)
	}
}

// RequireStatus fails unless the serve carries the status.
func RequireStatus(t *testing.T, serve *sdd.WorkflowServe, status string) {
	t.Helper()
	if serve.Status != status {
		t.Fatalf("status = %q, want %q (step %s)", serve.Status, status, serve.Step)
	}
}

// LoadEntry parses a written entry back from the world's graph dir.
func LoadEntry(t *testing.T, graphDir, id string) *model.Entry {
	t.Helper()
	rel, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(graphDir, filepath.FromSlash(rel))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// ParseEntry derives type/layer/time from the filename, so hand it the
	// full ID (the on-disk basename is DD-HHmmss-… under YYYY/MM).
	entry, err := model.ParseEntry(id+".md", string(content))
	if err != nil {
		t.Fatal(err)
	}
	return entry
}
