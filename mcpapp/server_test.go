package mcpapp_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	localadapter "github.com/networkteam/sdd/local"
	mcpserver "github.com/networkteam/sdd/mcpapp"
)

const (
	fixtureGapID = "20260601-100000-s-tac-aaa"

	// embeddedCaptureID is the base capture procedure shipped inside the
	// binary (internal/baseprocedures/entries). The fixture procedure
	// supersedes it, exercising the project-head-wins fork rule on the
	// production resolution path.
	embeddedCaptureID = "20260703-094500-d-prc-cap"
)

// captureProcedure is the capture-shaped fixture from the surface spec §3,
// stored as a normal graph entry superseding the embedded base capture — a
// project override, so canonical resolution picks it under project-head-wins
// while the spec load runs the production path. Its minimal instruction
// units keep the shell assertions decoupled from the base entry's prose.
const captureProcedure = `---
type: decision
layer: process
kind: procedure
canonical: capture
supersedes:
    - ` + embeddedCaptureID + `
participants:
    - Tester
confidence: medium
summary: The capture procedure fixture.
params:
    anchor: {type: entry-id, optional: true, desc: entry this capture is anchored on}
    supersedes: {type: entry-id, optional: true, desc: chain head this capture replaces}
    closes: {type: list<entry-id>, optional: true, desc: entries this capture resolves}
    kind: {type: entry-kind, optional: true, desc: pre-selected target kind}
state:
    body: {type: text, desc: entry description; self-describing first sentence}
    entryKind: {type: entry-kind, desc: signal or decision kind}
    layer: {type: layer, desc: strategic | conceptual | tactical | operational | process}
    refs: {type: list<ref>, desc: "each {id, kind, desc?}"}
    topics: {type: list<label>, desc: reuse existing labels}
    confidence: {type: confidence, desc: honest confidence}
    intent: {type: intent, optional: true, desc: required when entryKind is directive}
    attachments: {type: list<attachment-handle>, optional: true, desc: staged attachment handles}
    widenReport: {type: text, desc: searches run and entries inspected before drafting}
    fidelityNote: {type: text, optional: true, desc: one-line fidelity note}
    correctedSummary: {type: text, optional: true, desc: corrected summary on drift}
steps:
    - id: assemble
      collect: [body, entryKind, layer, refs, topics, confidence, "intent?", "attachments?", widenReport]
      inject:
          - {fn: viewLayout, args: {layout: "active:as-counts"}}
      transitions:
          - when: hasBody and hasRefs and hasTopics and hasWidenReport
                  and refsResolve and refKindsValid
                  and participantsCanonical and intentPresentIfDirective
            to: playback
    - id: playback
      chooser: user
      options:
          - {choice: confirm, call: confirmPlayback, to: write}
          - {choice: adjust, collect: ["body?", "refs?", "topics?", "confidence?", "intent?"], to: assemble}
          - {choice: abort, to: end(abandoned)}
    - id: write
      guard: playbackConfirmed
      op: newEntry
      transitions:
          - when: noHighFindings
            to: verifySummary
          - otherwise: reviseOrOverride
    - id: reviseOrOverride
      chooser: user
      render: findings
      options:
          - {choice: revise, collect: ["body?", "refs?", "topics?"], to: assemble}
          - {choice: override, call: recordOverride, to: write}
          - {choice: abort, to: end(abandoned)}
    - id: verifySummary
      chooser: agent
      inject:
          - {fn: generatedSummary}
      options:
          - {choice: faithful, collect: [fidelityNote], to: end(completed)}
          - {choice: drifted, collect: [correctedSummary], call: replaceSummary, to: end(completed)}
---

The shared capture spine.

## unit: assemble

Draft the entry. Existing topics: {{.viewLayout}}

## unit: playback

Play back to the user: {{.body}}

## unit: findings

Pre-flight findings to resolve or override.

## unit: verifySummary

Verify the generated summary: {{.generatedSummary}}
`

// captureProbeProcedure is a minimal capture-shaped fixture whose assemble
// gate deliberately omits the refsResolve / refsInspected predicates the
// shipped base capture carries. Dropping them lets a report with a dangling ref sail through
// assemble to the newEntry write op, where CreateEntry rejects it with the real
// dangling-ref validation error — the legitimate write-gate rejection that
// exposed s-tac-tq7. It exists only to drive that write failure on a single
// graph, without staging the branch-divergence trigger of the live report.
const captureProbeProcedure = `---
type: decision
layer: process
kind: procedure
canonical: capture-probe
participants:
    - Tester
confidence: medium
summary: Minimal capture-shaped fixture driving an unresolvable ref to the write op.
state:
    body: {type: text, desc: entry description}
    entryKind: {type: entry-kind, desc: signal or decision kind}
    layer: {type: layer, desc: strategic | conceptual | tactical | operational | process}
    refs: {type: list<ref>, desc: "each {id, kind, desc?}"}
    topics: {type: list<label>, desc: topic labels}
    confidence: {type: confidence, desc: honest confidence}
    widenReport: {type: text, desc: searches run and entries inspected before drafting}
    fidelityNote: {type: text, optional: true, desc: one-line fidelity note}
steps:
    - id: assemble
      collect: [body, entryKind, layer, refs, topics, confidence, widenReport]
      transitions:
          - when: hasBody and hasRefs and hasTopics and hasWidenReport
            to: playback
    - id: playback
      chooser: user
      options:
          - {choice: confirm, call: confirmPlayback, to: write}
          - {choice: abort, to: end(abandoned)}
    - id: write
      guard: playbackConfirmed
      op: newEntry
      transitions:
          - when: noHighFindings
            to: verifySummary
          - otherwise: reviseOrOverride
    - id: reviseOrOverride
      chooser: user
      render: findings
      options:
          - {choice: revise, collect: ["body?", "refs?"], to: assemble}
          - {choice: abort, to: end(abandoned)}
    - id: verifySummary
      chooser: agent
      inject:
          - {fn: generatedSummary}
      options:
          - {choice: faithful, collect: [fidelityNote], to: end(completed)}
---

Minimal capture probe spine.

## unit: assemble

Draft the entry: {{.body}}

## unit: playback

Play back: {{.body}}

## unit: findings

Pre-flight findings to resolve or override.

## unit: verifySummary

Verify the generated summary: {{.generatedSummary}}
`

const gapEntry = `---
type: signal
layer: tactical
kind: gap
participants:
    - Tester
confidence: medium
topics:
    - testing/fixture
summary: Pre-flight verdict oscillation produces contradictory findings across runs.
---

Pre-flight verdict oscillation produces contradictory findings across runs on identical input.
`

func writeFixtureGraph(t *testing.T) string {
	t.Helper()
	graphDir := filepath.Join(t.TempDir(), "graph")

	entries := map[string]string{
		"2026/06/01-100000-s-tac-aaa.md": gapEntry,
		"2026/07/04-120000-d-prc-tst.md": captureProcedure,
	}
	for rel, content := range entries {
		path := filepath.Join(graphDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// One attachment on the gap entry for read_attachment paging.
	attachDir := filepath.Join(graphDir, "2026/06/01-100000-s-tac-aaa")
	if err := os.MkdirAll(attachDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attachDir, "notes.md"), []byte("0123456789"), 0644); err != nil {
		t.Fatal(err)
	}
	return graphDir
}

type testEnv struct {
	srv         *mcpserver.Server
	graphDir    string
	sessionsDir string
	sessions    *localadapter.FilesystemSessionStore
	targets     *testBranchTargets
}

type testBranchTargets struct {
	mu       sync.RWMutex
	fallback sdd.GraphStore
	graphs   map[string]sdd.GraphStore
	errors   map[string]error
}

func (t *testBranchTargets) Acquire(_ context.Context, target sdd.MutationTarget) (*sdd.AcquiredTarget, error) {
	t.mu.RLock()
	acquireErr := t.errors[target.Branch]
	graph := t.graphs[target.Branch]
	if graph == nil {
		graph = t.fallback
	}
	t.mu.RUnlock()
	if acquireErr != nil {
		return nil, acquireErr
	}
	return &sdd.AcquiredTarget{Target: target, Graph: graph, Release: func() error { return nil }}, nil
}

func (t *testBranchTargets) set(branch string, graph sdd.GraphStore) {
	t.mu.Lock()
	t.graphs[branch] = graph
	t.mu.Unlock()
}

func (t *testBranchTargets) setError(branch string, err error) {
	t.mu.Lock()
	if t.errors == nil {
		t.errors = make(map[string]error)
	}
	t.errors[branch] = err
	t.mu.Unlock()
}

var testRuntimeGeneration atomic.Int64

// newTestServer builds a server over a fixture graph with deterministic
// pre-flight findings and text-mode search. Passing existing dirs re-hosts
// them on a fresh server (the restart scenario); mutate tweaks Options.
func newTestServer(t *testing.T, findings []query.Finding, graphDir, sessionsDir string, mutate ...func(*mcpserver.Options)) testEnv {
	return newTestServerConfig(t, findings, graphDir, sessionsDir, "", mutate...)
}

func newTestServerConfig(t *testing.T, findings []query.Finding, graphDir, sessionsDir, language string, mutate ...func(*mcpserver.Options)) testEnv {
	t.Helper()
	if graphDir == "" {
		graphDir = writeFixtureGraph(t)
	}
	if sessionsDir == "" {
		sessionsDir = filepath.Join(t.TempDir(), "sessions")
	}

	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "test", GraphDir: graphDir})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := localadapter.NewFilesystemSessionStoreAt(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(filepath.Join(filepath.Dir(sessionsDir), "staged-blobs"))
	if err != nil {
		t.Fatal(err)
	}
	targets := &testBranchTargets{fallback: graph, graphs: map[string]sdd.GraphStore{"main": graph}}
	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC).Add(time.Duration(testRuntimeGeneration.Add(1)) * time.Hour)
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "test", DisplayName: "Test"}, DefaultBranch: "main", Language: language,
		Graph: graph, Targets: targets, Sessions: sessions, StagedBlobs: blobs, Now: func() time.Time { return now },
		Branches: sdd.BranchValidatorFunc(func(_ context.Context, target sdd.MutationTarget) error {
			if target.Project != "test" {
				return fmt.Errorf("unexpected branch project %q", target.Project)
			}
			if target.Branch == "missing" {
				return fmt.Errorf("branch %q has no registered checkout", target.Branch)
			}
			return nil
		}),
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return []string{"json-schema"}, nil },
			ExecuteFunc: func(_ context.Context, request sdd.LLMRequest) (sdd.LLMResult, error) {
				if request.Purpose == "summary" {
					return sdd.LLMResult{Output: []byte("Test capture entry summary."), ExecutorFingerprint: "test"}, nil
				}
				items := make([]map[string]string, 0, len(findings))
				for _, finding := range findings {
					items = append(items, map[string]string{"severity": string(finding.Severity), "category": finding.Category, "observation": finding.Observation})
				}
				output, marshalErr := json.Marshal(map[string]any{"findings": items})
				return sdd.LLMResult{Output: output, ExecutorFingerprint: "test"}, marshalErr
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
	opts := mcpserver.Options{
		Application: application, Project: "test", LocalIdentity: sdd.RequestIdentity{Subject: "tester"}, Version: "test",
		LocalAttachmentPath: func(entryID, filename string) (string, error) {
			dir, pathErr := model.AttachDirRelPath(entryID)
			if pathErr != nil {
				return "", pathErr
			}
			return filepath.Abs(filepath.Join(graphDir, dir, filename))
		},
	}
	for _, m := range mutate {
		m(&opts)
	}
	srv, err := mcpserver.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return testEnv{srv: srv, graphDir: graphDir, sessionsDir: sessionsDir, sessions: sessions, targets: targets}
}

// connect attaches an in-memory client session to the server.
func connect(t *testing.T, srv *mcpserver.Server) *mcp.ClientSession {
	t.Helper()
	cs, _ := connectPair(t, srv)
	return cs
}

// connectPair connects like connect but also returns the server session, so a
// test can drive the server-side lifecycle (Disconnect/Shutdown) directly.
func connectPair(t *testing.T, srv *mcpserver.Server) (*mcp.ClientSession, *mcp.ServerSession) {
	t.Helper()
	ctx := t.Context()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverTransport)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = srv.Disconnect(context.Background(), serverSession)
		_ = cs.Close()
	})
	return cs, serverSession
}

// openSession enters through the door: start_session opens the dialogue
// session with its user-dialogue shell and returns the opening serve.
func openSession(t *testing.T, cs *mcp.ClientSession) mcpserver.ServeResult {
	t.Helper()
	var serve mcpserver.ServeResult
	call(t, cs, "start_session", map[string]any{}, &serve)
	if serve.Procedure != "user-dialogue" || serve.Status != "running" {
		t.Fatalf("start_session should serve the running user-dialogue shell, got %s %s", serve.Procedure, serve.Status)
	}
	return serve
}

// call invokes a tool and decodes its structured content into out. Fails the
// test on protocol errors or tool errors.
func call[T any](t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any, out *T) {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	if res.IsError {
		t.Fatalf("%s returned tool error: %s", tool, contentText(res))
	}
	decodeStructured(t, res, out)
}

// callExpectError invokes a tool expecting a tool error; returns its text.
func callExpectError(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) string {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	if !res.IsError {
		t.Fatalf("%s: expected tool error, got success", tool)
	}
	return contentText(res)
}

func decodeStructured[T any](t *testing.T, res *mcp.CallToolResult, out *T) {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	// Zero the target first — tests reuse result structs across calls, and
	// Unmarshal leaves fields absent from the payload untouched.
	var zero T
	*out = zero
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decoding structured content: %v (%s)", err, raw)
	}
}

func contentText(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

// assembleReport is the one-shot batch draft that cascades assemble →
// playback.
func assembleReport() map[string]any {
	return map[string]any{
		"body": "Test capture entry: the fixture oscillation gap also shows up in integration tests. " +
			"This entry exists to verify the engine write path end to end.",
		"entryKind":  "gap",
		"layer":      "tactical",
		"refs":       []map[string]any{{"id": fixtureGapID, "kind": "related", "desc": "the fixture gap this test entry sits beside"}},
		"topics":     []string{"testing/fixture"},
		"confidence": "low",
		"widenReport": "searched terms oscillation and fixture; inspected " + fixtureGapID + " — " +
			"no existing entry covers the integration-test angle.",
	}
}

// TestSearchDefaults_HeaderOnlyHitCapped pins the drill-serve defaults
// (d-tac-dbk): no limit and no max_citations means at most 8 hits and zero
// citation lines — the measured 26.5KB drill was this tool with snippet
// defaults. Snippets and a higher cap stay one explicit parameter away.
func TestSearchDefaults_HeaderOnlyHitCapped(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "graph")
	for i := range 12 {
		path := filepath.Join(graphDir, fmt.Sprintf("2026/06/02-1000%02d-s-tac-h%02d.md", i, i))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf(`---
type: signal
layer: tactical
kind: gap
summary: Probe entry %02d observes the flux capacitor drill cost.
---

Probe entry %02d: the flux capacitor drill needs observing.
`, i, i)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)

	var res mcpserver.SearchResult
	call(t, cs, "search", map[string]any{"terms": []string{"flux capacitor"}}, &res)
	if strings.Contains(res.Results, "↳") {
		t.Fatalf("default search must be header-only, got citations: %q", res.Results)
	}
	if hits := strings.Count(res.Results, "-s-tac-h"); hits != 8 {
		t.Fatalf("default search returned %d hits, want the cap of 8", hits)
	}

	call(t, cs, "search", map[string]any{"terms": []string{"flux capacitor"}, "max_citations": 2, "limit": 12}, &res)
	if !strings.Contains(res.Results, "↳") {
		t.Fatalf("explicit max_citations should render citation lines, got %q", res.Results)
	}
	if hits := strings.Count(res.Results, "-s-tac-h"); hits != 12 {
		t.Fatalf("explicit limit should raise the cap, got %d hits", hits)
	}
}

// TestVocabularyBlockForNonEnglishGraphs serves the bundled translation
// table exactly once per connection when the graph language is non-English —
// locale rendering's engine-surface home (d-tac-dbk, s-tac-fgy).
func TestVocabularyBlockForNonEnglishGraphs(t *testing.T) {
	env := newTestServerConfig(t, nil, "", "", "de")
	cs := connect(t, env.srv)
	door := openSession(t, cs)
	if !strings.Contains(door.Vocabulary, "Vokabular") {
		t.Fatalf("a German graph's first serve should carry the vocabulary table, got %q", door.Vocabulary)
	}
	if !strings.Contains(door.Framing, "Language: de") {
		t.Fatalf("the framing info block should state the locale, got %q", door.Framing)
	}

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &serve)
	if serve.Vocabulary != "" {
		t.Fatalf("the vocabulary serves once per connection, got it again: %q", serve.Vocabulary)
	}
}

// TestVocabularyBlockAbsentForEnglish keeps English (default) graphs free of
// the block entirely.
func TestVocabularyBlockAbsentForEnglish(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	if door := openSession(t, cs); door.Vocabulary != "" {
		t.Fatalf("an English graph serves no vocabulary block, got %q", door.Vocabulary)
	}
}

// TestVocabularyBlockMissingLocaleNote serves an explicit note when the
// configured locale has no bundled reference — the commitment never drops
// silently.
func TestVocabularyBlockMissingLocaleNote(t *testing.T) {
	env := newTestServerConfig(t, nil, "", "", "fr")
	cs := connect(t, env.srv)
	door := openSession(t, cs)
	if !strings.Contains(door.Vocabulary, "no bundled vocabulary") || !strings.Contains(door.Vocabulary, "vocabulary-fr.md") {
		t.Fatalf("a locale without a bundled reference should serve the explicit note, got %q", door.Vocabulary)
	}
}

// TestToolSurfaceMatchesSpec pins the tool list to the surface spec §5 —
// exactly these tools, and in particular no direct write tool.
func TestToolSurfaceMatchesSpec(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	res, err := cs.ListTools(t.Context(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tool := range res.Tools {
		got = append(got, tool.Name)
	}
	sort.Strings(got)
	want := []string{
		"abandon", "bind_branch", "info", "list_sessions", "next", "park", "read_attachment",
		"registry", "resume_session", "search", "show", "stage_attachment",
		"start_procedure", "start_session", "view",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tool surface diverges from spec:\n got %v\nwant %v", got, want)
	}
}

func TestBindBranchToolPersistsAndProjectsAcrossResume(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)

	var bound mcpserver.BindBranchResult
	call(t, cs, "bind_branch", map[string]any{"session": door.Session, "branch": "feature/session"}, &bound)
	if bound.Branch != "feature/session" || bound.Status != "bound" {
		t.Fatalf("bind result = %+v", bound)
	}

	// A fresh start_session call re-serves this connection's current door and
	// carries the now-durable branch in the session info.
	var reopened mcpserver.ServeResult
	call(t, cs, "start_session", map[string]any{}, &reopened)
	if reopened.Session != door.Session || reopened.Branch != "feature/session" {
		t.Fatalf("door projection = %+v", reopened)
	}
	if !strings.Contains(reopened.Framing, "Branch binding: feature/session") {
		t.Fatalf("bound door framing = %q", reopened.Framing)
	}

	// Keep one move open so the dialogue appears in discovery.
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &serve)

	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	var listed mcpserver.ListSessionsResult
	call(t, cs2, "list_sessions", map[string]any{}, &listed)
	if len(listed.Sessions) != 1 || listed.Sessions[0].Branch != "feature/session" {
		t.Fatalf("list projection after restart = %+v", listed.Sessions)
	}

	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{
		"session": door.Session, "userWords": "resume the bound branch session",
	}, &resumed)
	if resumed.Branch != "feature/session" {
		t.Fatalf("resume projection = %+v", resumed)
	}
	if !strings.Contains(resumed.Framing, "Branch binding: feature/session") {
		t.Fatalf("fresh resume framing = %q", resumed.Framing)
	}
	call(t, cs2, "resume_session", map[string]any{}, &resumed)
	if resumed.Branch != "feature/session" {
		t.Fatalf("reorientation projection = %+v", resumed)
	}

	var cleared mcpserver.BindBranchResult
	call(t, cs2, "bind_branch", map[string]any{"session": door.Session, "clear": true}, &cleared)
	if cleared.Branch != "" || cleared.Status != "cleared" {
		t.Fatalf("clear result = %+v", cleared)
	}
}

func TestBindBranchToolValidatesArgumentsAndAttachment(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)

	for name, args := range map[string]map[string]any{
		"neither":    {"session": door.Session},
		"both":       {"session": door.Session, "branch": "feature", "clear": true},
		"whitespace": {"session": door.Session, "branch": " feature "},
	} {
		msg := callExpectError(t, cs, "bind_branch", args)
		if name == "whitespace" {
			if !strings.Contains(msg, "whitespace") {
				t.Fatalf("%s error = %q", name, msg)
			}
		} else if !strings.Contains(msg, "exactly one") {
			t.Fatalf("%s error = %q", name, msg)
		}
	}

	unbound := connect(t, env.srv)
	msg := callExpectError(t, unbound, "bind_branch", map[string]any{"session": door.Session, "branch": "feature"})
	if !strings.Contains(msg, "not attached") || !strings.Contains(msg, "resume_session") {
		t.Fatalf("attachment-gate error = %q", msg)
	}
}

func TestAttachedFreeReadsUseBoundBranchOnly(t *testing.T) {
	const (
		branch     = "feature/read-routing"
		branchID   = "20260724-220000-s-tac-brn"
		branchText = "Branch-only nebula routing evidence"
	)
	env := newTestServer(t, nil, "", "")
	branchDir := filepath.Join(t.TempDir(), "graph")
	branchPath := filepath.Join(branchDir, "2026/07/24-220000-s-tac-brn.md")
	if err := os.MkdirAll(filepath.Dir(branchPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(branchPath, []byte(`---
type: signal
kind: gap
layer: tactical
confidence: high
topics:
    - testing/routing
summary: Branch-only nebula routing evidence.
---

Branch-only nebula routing evidence exists exclusively on the bound branch.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	branchGraph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "test", GraphDir: branchDir})
	if err != nil {
		t.Fatal(err)
	}
	env.targets.set(branch, branchGraph)

	boundClient := connect(t, env.srv)
	door := openSession(t, boundClient)
	var binding mcpserver.BindBranchResult
	call(t, boundClient, "bind_branch", map[string]any{"session": door.Session, "branch": branch}, &binding)

	assertBranchReads := func(t *testing.T, client *mcp.ClientSession, seesBranch bool) {
		t.Helper()
		var showOutput string
		if seesBranch {
			var shown mcpserver.ShowResult
			call(t, client, "show", map[string]any{"ids": []string{branchID}}, &shown)
			showOutput = shown.Entries
		} else {
			showOutput = callExpectError(t, client, "show", map[string]any{"ids": []string{branchID}})
		}
		var searched mcpserver.SearchResult
		call(t, client, "search", map[string]any{"terms": []string{"nebula routing evidence"}}, &searched)
		var viewed mcpserver.ViewResult
		call(t, client, "view", map[string]any{"layout": "active:as-list"}, &viewed)
		for surface, output := range map[string]string{
			"show": showOutput, "search": searched.Results, "view": viewed.Sections,
		} {
			if got := strings.Contains(output, branchText); got != seesBranch {
				t.Fatalf("%s branch visibility = %v, want %v:\n%s", surface, got, seesBranch, output)
			}
		}
	}
	assertBranchReads(t, boundClient, true)

	unattached := connect(t, env.srv)
	assertBranchReads(t, unattached, false)

	unboundClient := connect(t, env.srv)
	openSession(t, unboundClient)
	assertBranchReads(t, unboundClient, false)
}

func TestAttachedFreeReadDriftNamesSessionBindingOnEverySurface(t *testing.T) {
	const branch = "feature/drifted"
	env := newTestServer(t, nil, "", "")
	client := connect(t, env.srv)
	door := openSession(t, client)
	var binding mcpserver.BindBranchResult
	call(t, client, "bind_branch", map[string]any{"session": door.Session, "branch": branch}, &binding)

	env.targets.setError(branch, errors.New("target factory is temporarily unavailable"))
	for name, args := range map[string]map[string]any{
		"show":   {"ids": []string{fixtureGapID}},
		"search": {"terms": []string{"oscillation"}},
		"view":   {"layout": "active:as-list"},
	} {
		message := callExpectError(t, client, name, args)
		for _, want := range []string{
			`session is bound to branch "feature/drifted"`,
			"acquiring that branch failed",
			"re-declare the binding or clear it",
			"target factory is temporarily unavailable",
		} {
			if !strings.Contains(message, want) {
				t.Fatalf("%s drift error missing %q: %q", name, want, message)
			}
		}
		if strings.Contains(message, "no longer resolves to a checkout") {
			t.Fatalf("%s drift error overclaimed checkout state: %q", name, message)
		}
	}
}

// TestToolContractSnapshot freezes the complete public MCP contract before it
// moves packages: names, descriptions, annotations, and input/output schemas.
// Semantic replay remains covered by TestCaptureProcedureLoop and
// TestEmbeddedCaptureProcedure; this hash catches accidental schema drift that
// their typed calls would otherwise tolerate.
func TestToolContractSnapshot(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	res, err := cs.ListTools(t.Context(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(res.Tools, func(i, j int) bool { return res.Tools[i].Name < res.Tools[j].Name })
	encoded, err := json.Marshal(res.Tools)
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%x", sha256.Sum256(encoded))
	const want = "e484f739234681aee541f086325bbcc1fdfc84b76076a7344a8e1e685136a092"
	if got != want {
		t.Fatalf("MCP tool contract changed: got %s, want %s", got, want)
	}
}

// TestOrientationListsMoveParamSignatures verifies the shell orientation's
// move listing carries each move's accepted start-param signature, so a
// caller sees the params where the move is offered rather than only through a
// rejected start_procedure round-trip (s-tac-ay5). The capture fixture
// declares anchor/closes/kind/supersedes, all optional, rendered in name
// order with a trailing "?" per optional param.
func TestOrientationListsMoveParamSignatures(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	serve := openSession(t, cs)
	// Params render in name order; the type render is VarType.String().
	want := "capture(anchor?: entry-id, closes?: list<entry-id>, kind?: entry-kind, supersedes?: entry-id)"
	if !strings.Contains(serve.Instructions, want) {
		t.Fatalf("orientation move listing should carry the capture param signature %q, got:\n%s", want, serve.Instructions)
	}
	// A move with no declared params renders bare — no empty parens. A space
	// after the canonical (before the " - " summary separator) holds only when
	// no "(" was appended; the em-dash separator itself is left out of the
	// match to stay byte-agnostic.
	if !strings.Contains(serve.Instructions, "- catch-up ") {
		t.Fatalf("a paramless move should render without parens, got:\n%s", serve.Instructions)
	}
}

func TestOpeningServeIncludesDerivedFactIndex(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	serve := openSession(t, connect(t, env.srv))
	const pointer = "- `20260717-110000-s-prc-vwg` — How to compose graph views (view tool): layout grammar, filters, ranking, quoting, and examples"
	if !strings.Contains(serve.Instructions, pointer) {
		t.Fatalf("opening serve missing fact pointer %q:\n%s", pointer, serve.Instructions)
	}
}

func TestOpeningServeOmitsEmptyDerivedFactIndex(t *testing.T) {
	graphDir := writeFixtureGraph(t)
	path := filepath.Join(graphDir, "2026/07/17-110000-s-prc-vwg.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	const override = `---
type: signal
layer: process
kind: fact
topics: [cli/view]
summary: Project override deliberately leaves this fact out of session discovery.
---

Project-local reference override.
`
	if err := os.WriteFile(path, []byte(override), 0644); err != nil {
		t.Fatal(err)
	}
	env := newTestServer(t, nil, graphDir, "")
	serve := openSession(t, connect(t, env.srv))
	if strings.Contains(serve.Instructions, "Reference facts available") {
		t.Fatalf("opening serve rendered an empty fact-index block:\n%s", serve.Instructions)
	}
}

// TestOpeningServeOmitsUnsafeFactIndexTitle pins the read-side contract: a
// project-local fact whose index is malformed (a block-injecting title) loads
// with a warning but is not a member of the indexed population, so the opening
// serve succeeds and neither the injected content nor an indexed pointer for it
// reaches the instructions. Write-path rejection of such titles is unchanged
// and covered at the model layer.
func TestOpeningServeOmitsUnsafeFactIndexTitle(t *testing.T) {
	graphDir := writeFixtureGraph(t)
	path := filepath.Join(graphDir, "2026/07/17-110000-s-prc-vwg.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	const override = `---
type: signal
layer: process
kind: fact
topics: [cli/view]
index: {title: "Cue\u2028## Injected block", topic: cli/view}
---

Unsafe project-local reference override.
`
	if err := os.WriteFile(path, []byte(override), 0644); err != nil {
		t.Fatal(err)
	}
	env := newTestServer(t, nil, graphDir, "")
	serve := openSession(t, connect(t, env.srv))
	if strings.Contains(serve.Instructions, "## Injected block") {
		t.Fatalf("opening serve leaked injected content from a malformed fact index:\n%s", serve.Instructions)
	}
	if strings.Contains(serve.Instructions, "Reference facts available") {
		t.Fatalf("opening serve rendered a malformed fact as an indexed pointer:\n%s", serve.Instructions)
	}
}

// TestOpeningServeIncludesGraphHealthNotice pins that when the loaded graph
// carries an entry warning, the opening serve's framing includes a compact,
// self-contained graph-health notice naming the entry — so the agent notices
// without running any command. The notice must not tell the agent to run a CLI.
func TestOpeningServeIncludesGraphHealthNotice(t *testing.T) {
	graphDir := writeFixtureGraph(t)
	// A fact whose index.topic is not among its topics loads with an index
	// warning at graph construction — a clean, deterministic warning source.
	const warned = `---
type: signal
layer: tactical
kind: fact
topics: [cli/view]
index: {title: Topic mismatch, topic: agent/other}
---

Body with a mismatched index enrollment.
`
	path := filepath.Join(graphDir, "2026/06/02-100000-s-tac-wrn.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(warned), 0644); err != nil {
		t.Fatal(err)
	}

	env := newTestServer(t, nil, graphDir, "")
	serve := openSession(t, connect(t, env.srv))
	if !strings.Contains(serve.Framing, "Graph health") {
		t.Fatalf("opening serve missing graph-health notice:\n%s", serve.Framing)
	}
	if !strings.Contains(serve.Framing, "20260602-100000-s-tac-wrn") {
		t.Fatalf("graph-health notice missing the warning entry ID:\n%s", serve.Framing)
	}
	if strings.Contains(serve.Framing, "sdd lint") || strings.Contains(serve.Framing, "run `sdd") {
		t.Fatalf("graph-health notice leaked a host-specific CLI directive:\n%s", serve.Framing)
	}
}

// TestOpeningServeLoadsPartialGraphWithUnreadableEntry is the MCP partial-read
// proof: a project graph containing one entry whose frontmatter cannot be
// decoded no longer makes the whole snapshot (and every session over it)
// unopenable. Opening a session succeeds — the parseable entries still load —
// and the graph-health framing reports the unreadable entry as a load error.
// Before the load-path unification, LoadSnapshotFS aborted here and the serve
// never opened.
func TestOpeningServeLoadsPartialGraphWithUnreadableEntry(t *testing.T) {
	graphDir := writeFixtureGraph(t)
	// Frontmatter with an unterminated flow sequence — YAML cannot decode it,
	// so the file is unreadable rather than merely warning-producing.
	const broken = `---
type: signal
layer: tactical
kind: fact
topics: [cli/view
---

Body whose frontmatter will not parse.
`
	path := filepath.Join(graphDir, "2026/06/03-100000-s-tac-bad.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(broken), 0644); err != nil {
		t.Fatal(err)
	}

	env := newTestServer(t, nil, graphDir, "")
	// openSession fails the test internally if the serve does not open — reaching
	// past it proves the malformed file did not abort the load.
	serve := openSession(t, connect(t, env.srv))
	if !strings.Contains(serve.Framing, "Graph health") {
		t.Fatalf("opening serve missing graph-health notice for the unreadable entry:\n%s", serve.Framing)
	}
	// The unreadable file surfaces by its logical path — it could not be
	// decoded far enough to derive a clean entry ID.
	if !strings.Contains(serve.Framing, "s-tac-bad") {
		t.Fatalf("graph-health notice missing the unreadable entry reference:\n%s", serve.Framing)
	}
	if !strings.Contains(serve.Framing, "unreadable") {
		t.Fatalf("graph-health notice should count the unreadable entry:\n%s", serve.Framing)
	}
}

// TestOpeningServeOmitsGraphHealthNoticeWhenClean pins the omit-when-clean
// contract: a graph with no warnings or load errors yields no health block.
// (The shared fixture is deliberately not reused — its "Tester" participant
// matches no actor, which is itself a warning.)
func TestOpeningServeOmitsGraphHealthNoticeWhenClean(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "graph")
	path := filepath.Join(graphDir, "2026/06/01-100000-s-tac-cln.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	const clean = `---
type: signal
layer: tactical
kind: gap
summary: A clean fixture entry that produces no warnings.
---

Body.
`
	if err := os.WriteFile(path, []byte(clean), 0644); err != nil {
		t.Fatal(err)
	}
	env := newTestServer(t, nil, graphDir, "")
	serve := openSession(t, connect(t, env.srv))
	if strings.Contains(serve.Framing, "Graph health") {
		t.Fatalf("clean graph should not render a health notice:\n%s", serve.Framing)
	}
}

// TestCaptureProcedureLoop drives the full capture spine over MCP: batch
// report, playback chooser with served-instruction memory, staged
// attachment materialized by the write gate, summary verification, and the
// produced entry ID.
func TestCaptureProcedureLoop(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var staged mcpserver.StageAttachmentResult
	call(t, cs, "stage_attachment", map[string]any{
		"session": session,
		"name":    "evidence.md",
		"content": "# Evidence\n\nStaged before the capture ran.",
	}, &staged)
	if staged.Handle != "evidence.md" {
		t.Fatalf("unexpected handle %q", staged.Handle)
	}

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &serve)
	if serve.Step != "assemble" || serve.Status != "running" {
		t.Fatalf("expected running at assemble, got %s at %q", serve.Status, serve.Step)
	}
	if !strings.Contains(serve.Instructions, "Existing topics:") {
		t.Fatalf("assemble unit should render with the viewLayout injection, got %q", serve.Instructions)
	}
	if serve.ReportSchema == nil {
		t.Fatal("serve should carry a report schema")
	}

	report := assembleReport()
	report["attachments"] = []string{staged.Handle}
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": report}, &serve)
	if serve.Step != "playback" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("expected pending user chooser at playback, got step %q chooser %+v", serve.Step, serve.PendingChooser)
	}
	if serve.Framing != "" {
		t.Fatalf("framing must be delivered once per session, got again: %q", serve.Framing)
	}
	firstPlayback := serve.Instructions
	if !strings.Contains(firstPlayback, "Play back to the user") {
		t.Fatalf("playback unit should serve full text first, got %q", firstPlayback)
	}

	// Adjust bounces through assemble and back to playback. The body changed,
	// so the re-rendered unit is different bytes — served in full again: a
	// confirm must never bind to an unseen description (content-hash dedup,
	// no per-step memory; d-tac-dbk).
	const tightened = "Test capture entry, tightened: the fixture oscillation gap also " +
		"shows up in integration tests, verifying the engine write path end to end."
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "adjust", "userWords": "tighten the first sentence",
		"fields": map[string]any{"body": tightened},
	}}, &serve)
	if serve.Step != "playback" {
		t.Fatalf("adjust should cascade back to playback, got %q", serve.Step)
	}
	if !strings.Contains(serve.Instructions, "Play back to the user") || !strings.Contains(serve.Instructions, "tightened") {
		t.Fatalf("a changed playback must serve in full, got %q", serve.Instructions)
	}

	// A second adjust that changes nothing re-renders identical bytes — the
	// serve stubs to the one-line reminder.
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "adjust", "userWords": "no, keep it",
		"fields": map[string]any{"body": tightened},
	}}, &serve)
	if serve.Step != "playback" {
		t.Fatalf("no-op adjust should cascade back to playback, got %q", serve.Step)
	}
	if strings.Contains(serve.Instructions, "Play back to the user") || !strings.Contains(serve.Instructions, "served earlier this session") {
		t.Fatalf("an unchanged playback re-serve should stub to the reminder, got %q", serve.Instructions)
	}

	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "yes, capture it",
	}}, &serve)
	if serve.Step != "verifySummary" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "agent" {
		t.Fatalf("confirm should write and reach verifySummary, got step %q status %s (%s)", serve.Step, serve.Status, serve.Instructions)
	}

	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "verifySummary", "choice": "faithful", "userWords": "",
		"fields": map[string]any{"fidelityNote": "summary matches the confirmed body"},
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("expected completion, got %s at %q", serve.Status, serve.Step)
	}
	entryID, _ := serve.Produced["entryId"].(string)
	if entryID == "" {
		t.Fatalf("completion should produce the created entry ID, got %v", serve.Produced)
	}

	rel, err := model.IDToRelPath(entryID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(env.graphDir, rel)); err != nil {
		t.Fatalf("created entry file missing: %v", err)
	}
	attachRel, err := model.AttachDirRelPath(entryID)
	if err != nil {
		t.Fatal(err)
	}
	materialized, err := os.ReadFile(filepath.Join(env.graphDir, attachRel, "evidence.md"))
	if err != nil {
		t.Fatalf("staged attachment was not materialized: %v", err)
	}
	if !strings.Contains(string(materialized), "Staged before the capture ran") {
		t.Fatalf("materialized attachment content diverged: %q", materialized)
	}
}

// TestEmbeddedCaptureProcedure drives the shipped base capture procedure
// (internal/baseprocedures/entries) over MCP with no project override in the
// graph: canonical resolution falls through to the embedded entry, its
// instruction units render over the store and injections, and the full spine
// completes with a written entry.
func TestEmbeddedCaptureProcedure(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "graph")
	path := filepath.Join(graphDir, "2026/06/01-100000-s-tac-aaa.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(gapEntry), 0644); err != nil {
		t.Fatal(err)
	}
	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &serve)
	if serve.Step != "assemble" || serve.Status != "running" {
		t.Fatalf("expected running at assemble, got %s at %q", serve.Status, serve.Step)
	}
	if !strings.Contains(serve.Instructions, "Ground before you draft") {
		t.Fatalf("expected the embedded assemble unit, got %q", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, "testing/fixture") {
		t.Fatalf("assemble unit should render the injected topic counts, got %q", serve.Instructions)
	}

	// A draft ref'ing an entry never served in full holds the assemble gate —
	// the rejection names exactly the un-inspected ID (d-tac-dbk).
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": assembleReport()}, &serve)
	if serve.Step != "assemble" {
		t.Fatalf("un-inspected ref should hold the gate at assemble, got %q", serve.Step)
	}
	if !strings.Contains(serve.Instructions, fixtureGapID) {
		t.Fatalf("rejection should name the un-inspected ID, got %q", serve.Instructions)
	}

	// The agent reads it through the same free tool; the gate passes on
	// re-report.
	var shown mcpserver.ShowResult
	call(t, cs, "show", map[string]any{"ids": []string{fixtureGapID}}, &shown)
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": assembleReport()}, &serve)
	if serve.Step != "playback" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("expected pending user chooser at playback, got step %q", serve.Step)
	}
	if !strings.Contains(serve.Instructions, "Test capture entry") {
		t.Fatalf("playback unit should render the drafted body verbatim, got %q", serve.Instructions)
	}

	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "yes, capture it",
	}}, &serve)
	if serve.Step != "verifySummary" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "agent" {
		t.Fatalf("confirm should write and reach verifySummary, got step %q status %s (%s)", serve.Step, serve.Status, serve.Instructions)
	}

	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "verifySummary", "choice": "faithful", "userWords": "",
		"fields": map[string]any{"fidelityNote": "summary matches the confirmed body"},
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("expected completion, got %s at %q", serve.Status, serve.Step)
	}
	if entryID, _ := serve.Produced["entryId"].(string); entryID == "" {
		t.Fatalf("completion should produce the created entry ID, got %v", serve.Produced)
	}
}

// TestEmbeddedEvaluateToCaptureHandoff is the slice-A evidence run over MCP:
// an evaluation widens once, then dispatches a capture as a sub-move. The
// capture inherits the evaluation's grounding through the seeding handoff, so
// its assemble gate passes without a second widen and the whole capture
// completes in four tool calls (the d-tac-tlo evaluate→capture friction the
// contract removes). Uses a graph with no capture override so both procedures
// resolve to the shipped embedded entries.
func TestEmbeddedEvaluateToCaptureHandoff(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "graph")
	path := filepath.Join(graphDir, "2026/06/01-100000-s-tac-aaa.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(gapEntry), 0644); err != nil {
		t.Fatal(err)
	}
	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	// A known anchor passed as a start input seeds the anchor state and
	// auto-advances the resolver straight to scope (the uniform anchor
	// contract — no separate resolver turn for a known entry).
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"session":   session,
		"canonical": "evaluate",
		"params":    map[string]any{"anchor": fixtureGapID},
	}, &serve)
	if serve.Procedure != "evaluate" || serve.Step != "scope" {
		t.Fatalf("a seeded anchor should auto-advance evaluate to scope, got %s at %q", serve.Procedure, serve.Step)
	}
	evalInstance := serve.Instance

	// The one widen of the whole flow happens here, in the evaluation. The
	// batched report cascades scope → carryOut → junction in one call.
	const widen = "searched post-landing signals and neighbors; inspected " + fixtureGapID +
		" — the teardown edge is the only new thing bearing on the work."
	call(t, cs, "next", map[string]any{"session": session, "instance": evalInstance, "report": map[string]any{
		"plan":            "Inner only: verify the work against its ACs; outer coverage left for a later run.",
		"widenReport":     widen,
		"innerEvidence":   "read the done's claims against the ACs; smoke check run",
		"innerEvaluation": "sound against its ACs; one small teardown rough edge remains",
	}}, &serve)
	if serve.Step != "junction" {
		t.Fatalf("evaluation should reach the junction, got %q", serve.Step)
	}

	// Record a finding — the evaluation completes, its grounding retained in
	// its store for the dispatched capture to inherit.
	call(t, cs, "next", map[string]any{"session": session, "instance": evalInstance, "report": map[string]any{
		"chooser": "junction", "choice": "record", "userWords": "record the teardown finding",
		"fields": map[string]any{"selectedFindings": "the teardown rough edge"},
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("record should complete the evaluation, got %s at %q", serve.Status, serve.Step)
	}

	// Capture #1: dispatch the finding as a sub-move of the evaluation.
	captureCalls := 1
	var cap mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"session":   session,
		"canonical": "capture",
		"parent":    evalInstance,
		"params":    map[string]any{"anchor": fixtureGapID, "kind": "gap"},
	}, &cap)
	if cap.Step != "assemble" {
		t.Fatalf("capture should open at assemble, got %q", cap.Step)
	}
	if slices.Contains(cap.Missing, "widenReport") {
		t.Fatalf("capture should inherit widenReport via the handoff, but it is missing: %v", cap.Missing)
	}
	if !strings.Contains(cap.Instructions, "inherited") || !strings.Contains(cap.Instructions, "teardown edge is the only new thing") {
		t.Errorf("assemble should surface the inherited grounding, got %q", cap.Instructions)
	}

	// Capture #2: draft with NO widenReport — the handoff already satisfied
	// it, so one report cascades assemble → playback.
	captureCalls++
	call(t, cs, "next", map[string]any{"session": session, "instance": cap.Instance, "report": map[string]any{
		"body":       "Record the teardown rough edge from the evaluation as a follow-up gap to fix next.",
		"entryKind":  "gap",
		"layer":      "tactical",
		"refs":       []map[string]any{{"id": fixtureGapID, "kind": "related", "desc": "the evaluated work this follow-up sits beside"}},
		"topics":     []string{"testing/fixture"},
		"confidence": "low",
	}}, &cap)
	if cap.Step != "playback" {
		t.Fatalf("assemble should cascade to playback without a re-widen, got %q (missing %v)", cap.Step, cap.Missing)
	}

	// Capture #3: confirm — writes through the (stubbed) gate to verifySummary.
	captureCalls++
	call(t, cs, "next", map[string]any{"session": session, "instance": cap.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "yes, capture it",
	}}, &cap)
	if cap.Step != "verifySummary" {
		t.Fatalf("confirm should write and reach verifySummary, got %q status %s", cap.Step, cap.Status)
	}

	// Capture #4: verify the summary — completes.
	captureCalls++
	call(t, cs, "next", map[string]any{"session": session, "instance": cap.Instance, "report": map[string]any{
		"chooser": "verifySummary", "choice": "faithful",
		"fields": map[string]any{"fidelityNote": "summary matches the confirmed body"},
	}}, &cap)
	if cap.Status != "completed" {
		t.Fatalf("capture should complete, got %s at %q", cap.Status, cap.Step)
	}
	if captureCalls > 4 {
		t.Fatalf("capture took %d calls; the evidence bar is one widen and <= 4 capture calls", captureCalls)
	}
}

// TestBlockedWriteRoutesToOverride drives the write gate into high findings:
// the loop routes to the reviseOrOverride user chooser, and the override
// choice (user-only, recorded) re-runs the write with pre-flight skipped.
func TestBlockedWriteRoutesToOverride(t *testing.T) {
	env := newTestServer(t, []query.Finding{{
		Severity:    query.SeverityHigh,
		Category:    "test-block",
		Observation: "canned high finding",
	}}, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &serve)
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": assembleReport()}, &serve)
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "capture it",
	}}, &serve)
	if serve.Step != "reviseOrOverride" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("high findings should route to the reviseOrOverride user chooser, got %q", serve.Step)
	}

	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "reviseOrOverride", "choice": "override", "userWords": "override it — the finding is wrong",
	}}, &serve)
	if serve.Step != "verifySummary" {
		t.Fatalf("override should re-run the write with pre-flight skipped, got %q (%s)", serve.Step, serve.Instructions)
	}
}

// TestRejectedWriteKeepsConnectionUsable pins s-tac-tq7: a newEntry write that
// fails validation (a dangling ref rejected at the write gate — a normal,
// expected event) must NOT poison the connection's session binding. A wiped
// binding kills every subsequent tool on the connection — reads, recovery, and
// mutations alike — and only a transport reconnect heals it, so the invariant
// is that the failed write leaves the binding fully intact. The regression
// drives a real dangling-ref rejection, then asserts the same connection's
// show and resume_session still work.
func TestRejectedWriteKeepsConnectionUsable(t *testing.T) {
	graphDir := writeFixtureGraph(t)
	probePath := filepath.Join(graphDir, "2026/07/04-130000-d-prc-prb.md")
	if err := os.WriteFile(probePath, []byte(captureProbeProcedure), 0644); err != nil {
		t.Fatal(err)
	}
	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture-probe"}, &serve)

	// A syntactically valid but non-existent ID: it parses as a full entry ID
	// (so the collect schema accepts it) yet resolves to nothing in the graph.
	const danglingRef = "20990101-000000-s-tac-zzz"
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"body": "Probe capture whose ref does not resolve, so the write gate rejects it. " +
			"This exists to exercise the newEntry failure path end to end.",
		"entryKind":   "gap",
		"layer":       "tactical",
		"refs":        []map[string]any{{"id": danglingRef, "kind": "related", "desc": "a ref that does not resolve"}},
		"topics":      []string{"testing/fixture"},
		"confidence":  "low",
		"widenReport": "searched the fixture graph; nothing covers this and " + danglingRef + " does not exist.",
	}}, &serve)
	if serve.Step != "playback" {
		t.Fatalf("assemble should cascade to playback, got %q (missing %v)", serve.Step, serve.Missing)
	}

	// A dangling-ref rejection re-serves in-band as findings at the reviseOrOverride
	// user chooser rather than erroring; no write happened, so the binding stays intact.
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "capture it",
	}}, &serve)
	// This fixture's only path to reviseOrOverride is the dangling-ref rejection
	// (assemble omits the ref predicates, no canned findings exist), so landing here
	// is itself the assertion that the rejection re-served instead of erroring.
	if serve.Step != "reviseOrOverride" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("the dangling-ref rejection should re-serve at the reviseOrOverride user chooser, got %q", serve.Step)
	}

	// The connection must stay usable. A free read appends to the session read
	// log through the binding — before the fix this failed with `invalid session
	// ID ""` because the wiped binding's session ID had gone empty.
	var show mcpserver.ShowResult
	call(t, cs, "show", map[string]any{"ids": []string{fixtureGapID}}, &show)
	if len(show.Entries) == 0 {
		t.Fatalf("show after the rejected write returned no entries — the connection is poisoned")
	}

	// resume_session with no args reorients the attached session — the recovery
	// tool the poisoned-binding error recommends, yet the one the wipe broke: its
	// still-held pre-check called Load("") and died with `invalid session ID ""`.
	// It must re-serve the SAME session, proving the binding is fully intact, not
	// merely non-empty.
	var resumed mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{}, &resumed)
	if resumed.Session != session {
		t.Fatalf("resume_session after the rejected write should re-serve %s, got %q", session, resumed.Session)
	}
}

// TestSessionResumeAcrossServers drives a capture to the playback chooser,
// then re-hosts the sessions dir on a fresh server (the restart): the
// session lists with its open instance, resumes by log replay, serves the
// full unit text again (new agent consumer), and completes.
// TestSessionLabelFallbackAndValidation covers the label's derivation ladder
// on the descriptor — blank before anything is drafted, first line of the
// most recent drafted body as fallback, explicit label winning — and the
// single-line/length validation on the loop tools.
func TestSessionLabelFallbackAndValidation(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &serve)

	// The descriptor is observed from a peer — a fresh server over the same
	// sessions dir, where this session reads as idle — so the label-derivation
	// assertions are isolated from this connection's own live hold.
	descriptor := func() mcpserver.ListSessionsResult {
		t.Helper()
		peer := newTestServer(t, nil, env.graphDir, env.sessionsDir)
		pcs := connect(t, peer.srv)
		openSession(t, pcs)
		var listed mcpserver.ListSessionsResult
		call(t, pcs, "list_sessions", map[string]any{}, &listed)
		return listed
	}

	listed := descriptor()
	if len(listed.Sessions) != 1 || listed.Sessions[0].Label != "" {
		t.Fatalf("nothing drafted and no label supplied — label must be blank, got %+v", listed.Sessions)
	}

	// A drafted body backfills the label from its first line.
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": assembleReport()}, &serve)
	listed = descriptor()
	if !strings.HasPrefix(listed.Sessions[0].Label, "Test capture entry") {
		t.Fatalf("label should fall back to the drafted body's first line, got %q", listed.Sessions[0].Label)
	}

	// An explicit label wins over the derived fallback.
	call(t, cs, "next", map[string]any{
		"session":  session,
		"instance": serve.Instance,
		"report": map[string]any{"chooser": "playback", "choice": "adjust", "userWords": "keep going",
			"fields": map[string]any{"confidence": "medium"}},
		"label": "Oscillation gap capture",
	}, &serve)
	listed = descriptor()
	if listed.Sessions[0].Label != "Oscillation gap capture" {
		t.Fatalf("explicit label must win over the fallback, got %q", listed.Sessions[0].Label)
	}

	// Validation: multi-line and oversized labels are rejected.
	if msg := callExpectError(t, cs, "next", map[string]any{
		"session": session, "instance": serve.Instance, "report": map[string]any{"confidence": "low"},
		"label": "two\nlines",
	}); !strings.Contains(msg, "single line") {
		t.Fatalf("multi-line label should be rejected, got %q", msg)
	}
	if msg := callExpectError(t, cs, "next", map[string]any{
		"session": session, "instance": serve.Instance, "report": map[string]any{"confidence": "low"},
		"label": strings.Repeat("x", 200),
	}); !strings.Contains(msg, "120") {
		t.Fatalf("oversized label should be rejected, got %q", msg)
	}
}

func TestSessionResumeAcrossServers(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"session":   session,
		"canonical": "capture",
		"params":    map[string]any{"anchor": fixtureGapID},
		"label":     "Capture: something about oscillation",
	}, &serve)
	// The label update rides an ordinary next call as the subject sharpens.
	call(t, cs, "next", map[string]any{
		"session":  session,
		"instance": serve.Instance,
		"report":   assembleReport(),
		"label":    "Capture: oscillation gap in integration tests",
	}, &serve)
	if serve.Step != "playback" {
		t.Fatalf("setup: expected playback, got %q", serve.Step)
	}
	sessionID := serve.Session

	// Restart: same graph and sessions dirs, fresh server and connection —
	// the reconnect pays orientation exactly once, at its own door.
	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	door := openSession(t, cs2)
	if door.Framing == "" {
		t.Fatal("a fresh connection's door serve should carry the framing")
	}

	var listed mcpserver.ListSessionsResult
	call(t, cs2, "list_sessions", map[string]any{}, &listed)
	if len(listed.Sessions) != 1 {
		t.Fatalf("expected one open session, got %+v", listed.Sessions)
	}
	desc := listed.Sessions[0]
	if desc.Session != sessionID || desc.Participant != "Tester" || desc.Anchor != fixtureGapID {
		t.Fatalf("descriptor diverged: %+v", desc)
	}
	if desc.Label != "Capture: oscillation gap in integration tests" {
		t.Fatalf("descriptor should carry the last supplied label across restart, got %q", desc.Label)
	}
	if len(desc.Open) != 1 || desc.Open[0].Procedure != "capture" || desc.Open[0].Step != "playback" {
		t.Fatalf("open instance descriptor diverged (the shell must not list): %+v", desc.Open)
	}

	var resumed mcpserver.ResumeSessionResult
	// A fresh connection did not open this session: attaching to it is a foreign
	// attach and carries the user's ask; the session reads idle across the clock
	// gap, so userWords alone suffices.
	call(t, cs2, "resume_session", map[string]any{"session": sessionID, "userWords": "pick the oscillation capture back up"}, &resumed)
	if resumed.Label != "Capture: oscillation gap in integration tests" {
		t.Fatalf("resume briefing should carry the session label, got %q", resumed.Label)
	}
	// Two running instances rehydrate: the session shell and the capture.
	var rehydrated, shellServe *mcpserver.ServeResult
	for i := range resumed.Open {
		switch resumed.Open[i].Procedure {
		case "capture":
			rehydrated = &resumed.Open[i]
		case "user-dialogue":
			shellServe = &resumed.Open[i]
		}
	}
	if rehydrated == nil || shellServe == nil {
		t.Fatalf("resume should rehydrate the capture and the session shell, got %+v", resumed.Open)
	}
	if rehydrated.Step != "playback" || rehydrated.PendingChooser == nil {
		t.Fatalf("resume should rehydrate the pending chooser at playback, got %+v", rehydrated)
	}
	// Served-once memory is per connection: this connection never saw the
	// playback unit, so it serves in full — the report evidence persisted
	// through the log replay.
	if !strings.Contains(rehydrated.Instructions, "Play back to the user") {
		t.Fatalf("resume should serve the full unit text again, got %q", rehydrated.Instructions)
	}
	// This connection already paid orientation at its own door, and the
	// graph hasn't moved — identical framing bytes dedup to nothing on the
	// resume (the s-tac-w3v reconnect double-pay drops to once).
	if resumed.Framing != "" {
		t.Fatalf("a connection that paid orientation must not re-pay it on resume, got %q", resumed.Framing)
	}

	call(t, cs2, "next", map[string]any{"session": sessionID, "instance": rehydrated.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "capture it",
	}}, &serve)
	call(t, cs2, "next", map[string]any{"session": sessionID, "instance": rehydrated.Instance, "report": map[string]any{
		"chooser": "verifySummary", "choice": "faithful",
		"fields": map[string]any{"fidelityNote": "matches"},
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("expected completion after resume, got %s at %q", serve.Status, serve.Step)
	}

	var listedAfter mcpserver.ListSessionsResult
	call(t, cs2, "list_sessions", map[string]any{}, &listedAfter)
	if len(listedAfter.Sessions) != 0 {
		t.Fatalf("completed session must drop off the open list, got %+v", listedAfter.Sessions)
	}

	// Re-knocking on the door clears the connection's served-once memory —
	// the orientation re-serves in full, on demand.
	if reshell := openSession(t, cs2); reshell.Framing == "" {
		t.Fatal("a repeated start_session should re-serve the framing in full")
	}
}

// TestResumeProjectsCollectedState covers the honest-surfacing half of the
// re-entry contract (d-cpt-0tm): a resuming agent sees the param and state
// values an instance already collected — even when the current step's unit
// does not render them — while internal trust machinery stays hidden and an
// oversized value is truncated with an explicit notice rather than dropped.
func TestResumeProjectsCollectedState(t *testing.T) {
	// Mirrors the production per-value cap; the test package can't see the
	// unexported constant.
	const collectedCap = 2000

	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	// An oversized reported value exercises the per-value cap.
	report := assembleReport()
	report["body"] = "Oscillation gap detail. " + strings.Repeat("z", collectedCap)

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"session":   session,
		"canonical": "capture",
		"params":    map[string]any{"anchor": fixtureGapID},
	}, &serve)
	call(t, cs, "next", map[string]any{
		"session":  session,
		"instance": serve.Instance,
		"report":   report,
	}, &serve)
	if serve.Step != "playback" {
		t.Fatalf("setup: expected playback, got %q", serve.Step)
	}
	// Confirm the playback chooser so the engine writes the confirmation record
	// (and, past the summarize gate, the engine-produced summary) — trust and
	// engine fields the projection must exclude.
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "capture it",
	}}, &serve)
	if serve.Status != "running" {
		t.Fatalf("setup: capture should still be running after confirm, got %s", serve.Status)
	}
	sessionID := serve.Session

	// A fresh server + connection resumes from the persisted log alone, then
	// attaches with the user's verbatim ask.
	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	openSession(t, cs2)

	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": sessionID, "userWords": "pick the capture back up"}, &resumed)

	var capture, shell *mcpserver.ServeResult
	for i := range resumed.Open {
		switch resumed.Open[i].Procedure {
		case "capture":
			capture = &resumed.Open[i]
		case "user-dialogue":
			shell = &resumed.Open[i]
		}
	}
	if capture == nil || shell == nil {
		t.Fatalf("resume should rehydrate the capture and the shell, got %+v", resumed.Open)
	}

	// The anchor param and reported state fields the current unit does not
	// render are projected onto the resume serve.
	if capture.Collected["anchor"] != fixtureGapID {
		t.Fatalf("collected should carry the anchor param, got %q", capture.Collected["anchor"])
	}
	if !strings.Contains(capture.Collected["widenReport"], "no existing entry covers") {
		t.Fatalf("collected should carry the reported widenReport, got %q", capture.Collected["widenReport"])
	}
	// Trust machinery and engine-produced fields never surface as collected.
	if _, ok := capture.Collected["playbackConfirmation"]; ok {
		t.Fatalf("collected must not leak the playback confirmation record: %+v", capture.Collected)
	}
	if _, ok := capture.Collected["preflightOverride"]; ok {
		t.Fatalf("collected must not leak the pre-flight override: %+v", capture.Collected)
	}

	// The oversized value is truncated with an explicit notice — near the cap,
	// never silently carrying the full payload.
	body := capture.Collected["body"]
	if !strings.Contains(body, "[collected value truncated at") {
		t.Fatalf("oversized collected value should carry a truncation notice, got %q", body)
	}
	if len(body) > collectedCap+200 {
		t.Fatalf("truncated value should sit near the cap, got %d bytes", len(body))
	}
	if strings.Contains(body, strings.Repeat("z", collectedCap)) {
		t.Fatal("truncated value should not carry the full oversized payload")
	}

	// The projection is resume-only: an ordinary next serve carries no collected
	// block, and the shell (no reported work of its own) contributes nothing of
	// the capture's fields.
	if _, ok := shell.Collected["anchor"]; ok {
		t.Fatalf("the shell serve must not carry the capture's collected fields: %+v", shell.Collected)
	}
	call(t, cs2, "next", map[string]any{"session": sessionID, "instance": capture.Instance, "report": map[string]any{
		"chooser": "verifySummary", "choice": "faithful",
		"fields": map[string]any{"fidelityNote": "matches"},
	}}, &serve)
	if serve.Collected != nil {
		t.Fatalf("collected must ride the resume path only, not an ordinary next serve: %+v", serve.Collected)
	}
}

// TestResumeCollectedRespectsInstanceBudget pins the aggregate per-instance cap
// on the collected projection: many near-cap fields must not stack into an
// oversized resume response (s-tac-jom / s-tac-40d — the ~10K-token host output
// ceiling). Fields are kept whole in sorted-key order until the budget is
// spent; anything past it carries an explicit omission notice, never silently
// vanishing (d-cpt-0tm).
func TestResumeCollectedRespectsInstanceBudget(t *testing.T) {
	// Mirrors the production constants; the test package can't see them.
	const (
		valueCap    = 2000
		instanceCap = 8000
	)

	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	// Four near-cap text fields, each capped to ~valueCap, together exceed the
	// per-instance budget so at least one must be omitted. captureBranch is left
	// alone — it drives graph resolution, not payload size.
	report := assembleReport()
	big := strings.Repeat("z", valueCap)
	report["body"] = "body: " + big
	report["widenReport"] = "widen: " + big
	report["fidelityNote"] = "fidelity: " + big
	report["correctedSummary"] = "corrected: " + big

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"session":   session,
		"canonical": "capture",
		"params":    map[string]any{"anchor": fixtureGapID},
	}, &serve)
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": report}, &serve)
	if serve.Step != "playback" {
		t.Fatalf("setup: expected playback, got %q", serve.Step)
	}
	sessionID := serve.Session

	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	openSession(t, cs2)

	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": sessionID, "userWords": "pick the capture back up"}, &resumed)

	var capture *mcpserver.ServeResult
	for i := range resumed.Open {
		if resumed.Open[i].Procedure == "capture" {
			capture = &resumed.Open[i]
		}
	}
	if capture == nil {
		t.Fatalf("resume should rehydrate the capture, got %+v", resumed.Open)
	}

	// Every collected field is still present as a key — nothing disappears
	// silently — and every large field is either kept whole (truncation notice)
	// or explicitly omitted (omission notice).
	total := 0
	omitted := 0
	for _, name := range []string{"body", "widenReport", "fidelityNote", "correctedSummary"} {
		v, ok := capture.Collected[name]
		if !ok {
			t.Fatalf("large field %q vanished from collected: %+v mapKeys", name, capture.Collected)
		}
		if strings.Contains(v, "[collected value omitted here for size") {
			omitted++
		}
	}
	for _, v := range capture.Collected {
		total += len(v)
	}
	if omitted == 0 {
		t.Fatalf("four near-cap fields should force at least one omission, got none: %+v", capture.Collected)
	}
	// The kept payload stays under the budget; the small omission-notice strings
	// for dropped fields are the only overage, well within slack.
	if total > instanceCap+1000 {
		t.Fatalf("collected total %d bytes exceeds budget+slack (%d)", total, instanceCap+1000)
	}
	// The anchor and the small enum fields are never the ones dropped.
	if capture.Collected["anchor"] != fixtureGapID {
		t.Fatalf("anchor must survive the budget, got %q", capture.Collected["anchor"])
	}
}

// TestAbandonLeavesLogStanding abandons a running instance and checks the
// session drops off the open list without any implicit cleanup of its log.
func TestAbandonLeavesLogStanding(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &serve)

	var abandoned mcpserver.AbandonResult
	call(t, cs, "abandon", map[string]any{"session": session, "instance": serve.Instance, "reason": "test teardown"}, &abandoned)
	if !abandoned.Abandoned {
		t.Fatal("abandon should confirm")
	}
	if abandoned.Base == nil || abandoned.Base.Procedure != "user-dialogue" {
		t.Fatalf("abandon should land back on the session shell, got %+v", abandoned.Base)
	}

	var listed mcpserver.ListSessionsResult
	call(t, cs, "list_sessions", map[string]any{}, &listed)
	if len(listed.Sessions) != 0 {
		t.Fatalf("abandoned instance must not list as open, got %+v", listed.Sessions)
	}
	if _, err := os.Stat(filepath.Join(env.sessionsDir, serve.Session+".jsonl")); err != nil {
		t.Fatalf("session log must stay on disk: %v", err)
	}
}

// TestParkMove shelves a seeded capture back to the shell junction: the
// response lands on the shell with the move listed as an open thread, the
// state keeps across a restart, and next resumes it where it stood
// (d-tac-dbk park affordance).
func TestParkMove(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	// A mid-dialogue capture-worthy item: start the capture, seed what is
	// known through the normal start params, and park it.
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"session":   session,
		"canonical": "capture",
		"params":    map[string]any{"anchor": fixtureGapID, "body": "A draft noted for later."},
		"label":     "noted for later",
	}, &serve)

	var parked mcpserver.ParkResult
	call(t, cs, "park", map[string]any{"session": session, "instance": serve.Instance, "note": "user wants this after the main work"}, &parked)
	if !parked.Parked || parked.Procedure != "capture" || parked.Step != serve.Step {
		t.Fatalf("park should confirm the shelved move, got %+v", parked)
	}
	if parked.Base == nil || parked.Base.Procedure != "user-dialogue" {
		t.Fatalf("park should land the dialogue on the shell junction, got %+v", parked.Base)
	}
	if !strings.Contains(parked.Base.OpenThreads, "capture at "+serve.Step) {
		t.Fatalf("the parked move should list as an open thread, got %q", parked.Base.OpenThreads)
	}

	// The parked event is in the log — legible to whoever resumes.
	events, err := readSessionLog(t, env.sessionsDir, serve.Session)
	if err != nil {
		t.Fatal(err)
	}
	sawParked := false
	for _, ev := range events {
		if ev.Event == engine.EventParked && ev.Instance == serve.Instance {
			sawParked = true
			if note, _ := ev.Data["note"].(string); note != "user wants this after the main work" {
				t.Fatalf("parked event should carry the note, got %v", ev.Data)
			}
		}
	}
	if !sawParked {
		t.Fatal("parking should log a parked event")
	}

	// Restart: the parked draft survives — session lists, and next resumes
	// the move with its seeded state intact.
	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	openSession(t, cs2)
	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": serve.Session, "userWords": "resume the parked draft"}, &resumed)
	var capServe *mcpserver.ServeResult
	for i := range resumed.Open {
		if resumed.Open[i].Procedure == "capture" {
			capServe = &resumed.Open[i]
		}
	}
	if capServe == nil || capServe.Step != serve.Step {
		t.Fatalf("the parked capture should rehydrate at its step, got %+v", resumed.Open)
	}
	if slices.Contains(capServe.Missing, "body") {
		t.Fatalf("the seeded draft state should survive the park, missing = %v", capServe.Missing)
	}

	// Park guard rails: the shell never parks, unknown instances are named.
	var shellServe *mcpserver.ServeResult
	for i := range resumed.Open {
		if resumed.Open[i].Procedure == "user-dialogue" {
			shellServe = &resumed.Open[i]
		}
	}
	if shellServe == nil {
		t.Fatalf("resume should rehydrate the shell, got %+v", resumed.Open)
	}
	if msg := callExpectError(t, cs2, "park", map[string]any{"session": serve.Session, "instance": shellServe.Instance}); !strings.Contains(msg, "park is for moves") {
		t.Fatalf("parking the shell should be refused, got %q", msg)
	}
	if msg := callExpectError(t, cs2, "park", map[string]any{"session": serve.Session, "instance": "i_99"}); !strings.Contains(msg, "not found") {
		t.Fatalf("unknown instance should be named, got %q", msg)
	}
}

// TestAbandonSessionByHandle_Parked tears down a parked-on-disk session in
// one unbound call — no resume, no framing (d-tac-dbk; baseline was six
// calls + ~28KB framing per session). The response names the label and the
// discarded threads, and the session drops off the open list.
func TestAbandonSessionByHandle_Parked(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"session":   session,
		"canonical": "capture",
		"label":     "Stale capture to tear down",
	}, &serve)
	sessionID := serve.Session

	// Restart: the session is parked on disk, known to no live server.
	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)

	// One call, no session bound, no framing anywhere in the response.
	var down mcpserver.AbandonResult
	call(t, cs2, "abandon", map[string]any{"session": sessionID, "reason": "stale"}, &down)
	if !down.Abandoned || down.Session != sessionID {
		t.Fatalf("teardown should confirm the session, got %+v", down)
	}
	if down.Label != "Stale capture to tear down" {
		t.Fatalf("teardown should name the label, got %q", down.Label)
	}
	if len(down.DiscardedThreads) != 1 || !strings.Contains(down.DiscardedThreads[0], "capture at") {
		t.Fatalf("teardown should name the discarded threads, got %v", down.DiscardedThreads)
	}
	if down.Base != nil {
		t.Fatalf("an unbound teardown has no dialogue to land, got %+v", down.Base)
	}

	openSession(t, cs2)
	var listed mcpserver.ListSessionsResult
	call(t, cs2, "list_sessions", map[string]any{}, &listed)
	if len(listed.Sessions) != 0 {
		t.Fatalf("torn-down session must drop off the open list, got %+v", listed.Sessions)
	}
	if _, err := os.Stat(filepath.Join(env.sessionsDir, sessionID+".jsonl")); err != nil {
		t.Fatalf("teardown closes the log, never deletes it: %v", err)
	}
}

// TestAbandonSessionByHandle_InMemory tears down a session parked in memory
// (left behind by a client disconnect with an open move) and lands the
// bound caller back on their own shell junction.
func TestAbandonSessionByHandle_InMemory(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "parked draft"}, &serve)
	parkedID := serve.Session
	_ = cs.Close() // disconnect with an open move: the session parks in memory

	cs2 := connect(t, env.srv)
	openSession(t, cs2)
	// The disconnect watcher parks asynchronously — wait until it lists.
	var listed mcpserver.ListSessionsResult
	deadline := time.Now().Add(5 * time.Second)
	for {
		call(t, cs2, "list_sessions", map[string]any{}, &listed)
		if len(listed.Sessions) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("disconnected session never parked, got %+v", listed.Sessions)
		}
		time.Sleep(10 * time.Millisecond)
	}

	var down mcpserver.AbandonResult
	call(t, cs2, "abandon", map[string]any{"session": parkedID, "reason": "not needed"}, &down)
	if !down.Abandoned || down.Session != parkedID || down.Label != "parked draft" {
		t.Fatalf("teardown diverged: %+v", down)
	}
	if len(down.DiscardedThreads) != 1 || !strings.Contains(down.DiscardedThreads[0], "capture at") {
		t.Fatalf("teardown should name the discarded threads, got %v", down.DiscardedThreads)
	}
	if down.Base == nil || down.Base.Procedure != "user-dialogue" {
		t.Fatalf("a bound caller lands back on their shell junction, got %+v", down.Base)
	}

	call(t, cs2, "list_sessions", map[string]any{}, &listed)
	if len(listed.Sessions) != 0 {
		t.Fatalf("torn-down session must drop off the open list, got %+v", listed.Sessions)
	}
}

// TestAbandonSessionByHandle_SurfacesHeldMarkers folds WIP markers out of a
// parked log so teardown surfaces them — left standing for grooming, never
// silently dropped. The log is synthetic: descriptors and teardown folds are
// self-derived from events, no procedure resolution.
func TestAbandonSessionByHandle_SurfacesHeldMarkers(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	sessionID := "s_20260706-090000-deadbeef"
	events := []engine.Event{
		{V: 1, TS: time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC), Session: sessionID, Seq: 1, Event: engine.EventSessionMeta, Data: map[string]any{"participant": "Tester"}},
		{V: 1, TS: time.Date(2026, 7, 6, 9, 0, 1, 0, time.UTC), Session: sessionID, Seq: 2, Instance: "i_1", Event: engine.EventStarted, Data: map[string]any{"procedure": "implementation", "step": "work"}},
		{V: 1, TS: time.Date(2026, 7, 6, 9, 0, 2, 0, time.UTC), Session: sessionID, Seq: 3, Instance: "i_1", Event: engine.EventOpResult, Data: map[string]any{"step": "setup", "fn": "wipStart", "writes": map[string]any{"wipMarker": "wip-123"}}},
	}
	stored, err := env.sessions.Create(t.Context(), sdd.SessionMetadata{
		CodecVersion: 1, ID: sdd.SessionID(sessionID), Subject: "tester", Project: "test", Participant: "Tester",
		UpdatedAt: time.Date(2026, 7, 6, 9, 0, 2, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	storedEvents := make([]sdd.StoredEvent, 0, len(events))
	for _, event := range events {
		payload, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		storedEvents = append(storedEvents, sdd.StoredEvent{CodecVersion: 1, Code: sdd.WorkflowEventCode, Payload: payload})
	}
	if _, err := env.sessions.Append(t.Context(), sdd.SessionID(sessionID), stored.Version, sdd.SessionAppend{Events: storedEvents}); err != nil {
		t.Fatal(err)
	}

	cs := connect(t, env.srv)
	var down mcpserver.AbandonResult
	call(t, cs, "abandon", map[string]any{"session": sessionID, "reason": "orphaned"}, &down)
	if len(down.HeldMarkers) != 1 || down.HeldMarkers[0] != "wip-123" {
		t.Fatalf("teardown should surface held markers, got %+v", down)
	}
	if !strings.Contains(down.Instructions, "grooming") {
		t.Fatalf("held markers should route the user to grooming, got %q", down.Instructions)
	}
}

// TestAbandonSessionByHandle_Rejections pins the teardown guard rails.
func TestAbandonSessionByHandle_Rejections(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	serve := openSession(t, cs)

	if msg := callExpectError(t, cs, "abandon", map[string]any{}); !strings.Contains(msg, "pass instance") {
		t.Fatalf("neither instance nor session should be rejected, got %q", msg)
	}
	// Instance mode requires the session the move belongs to; a handle this
	// connection is not attached to funnels into resume_session.
	if msg := callExpectError(t, cs, "abandon", map[string]any{"instance": "i_1", "session": "s_x"}); !strings.Contains(msg, "not attached to s_x") {
		t.Fatalf("a move abandon naming an unattached session should funnel to resume_session, got %q", msg)
	}
	// Instance set but session omitted is a work-tool call missing its handle:
	// it names both doors, not the pass-one-of message.
	if msg := callExpectError(t, cs, "abandon", map[string]any{"instance": "i_1"}); !strings.Contains(msg, "start_session") || !strings.Contains(msg, "resume_session") {
		t.Fatalf("a move abandon without a session handle should name both doors, got %q", msg)
	}
	if msg := callExpectError(t, cs, "abandon", map[string]any{"session": serve.Session}); !strings.Contains(msg, "own junction") {
		t.Fatalf("tearing down the bound session should point at conclude, got %q", msg)
	}
	if msg := callExpectError(t, cs, "abandon", map[string]any{"session": "s_nope"}); !strings.Contains(msg, "unknown session") {
		t.Fatalf("unknown session should be named, got %q", msg)
	}

	// Tearing down a session another connection is actively driving is refused:
	// destruction must not be cheaper than attachment (I5). The refusal names the
	// holder and points at the alternatives.
	cs2 := connect(t, env.srv)
	other := openSession(t, cs2)
	msg := callExpectError(t, cs, "abandon", map[string]any{"session": other.Session})
	for _, want := range []string{"test-client", "actively driven", "take it over"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("recent teardown refusal %q missing %q", msg, want)
		}
	}
}

// TestUnboundRejectionInlinesParkedSessions pins the discovery half of the
// teardown flow: a stateful call with no session bound is rejected with the
// parked-sessions list inline (handle + label), so the agent's next call can
// already be the resume or the teardown.
func TestUnboundRejectionInlinesParkedSessions(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "the parked one"}, &serve)

	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	msg := callExpectError(t, cs2, "next", map[string]any{"instance": "i_2", "report": map[string]any{"x": "y"}})
	if !strings.Contains(msg, "start_session") || !strings.Contains(msg, "resume_session") {
		t.Fatalf("a work tool without a handle should name both doors, got %q", msg)
	}
	if !strings.Contains(msg, serve.Session) || !strings.Contains(msg, "the parked one") {
		t.Fatalf("the no-handle rejection should inline the open session (handle + label), got %q", msg)
	}
}

// TestWorkToolWrongSessionFunnels: every work tool naming a session this
// connection is not attached to funnels into resume_session — the single attach
// point (d-cpt-9of). The handle must name the connection's own attachment.
func TestWorkToolWrongSessionFunnels(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &serve)

	const foreign = "s_not-mine"
	cases := map[string]map[string]any{
		"start_procedure":  {"session": foreign, "canonical": "capture"},
		"next":             {"session": foreign, "instance": serve.Instance, "report": assembleReport()},
		"park":             {"session": foreign, "instance": serve.Instance},
		"bind_branch":      {"session": foreign, "branch": "feature"},
		"stage_attachment": {"session": foreign, "name": "a.md", "content": "x"},
		"abandon":          {"session": foreign, "instance": serve.Instance},
	}
	for tool, args := range cases {
		msg := callExpectError(t, cs, tool, args)
		if !strings.Contains(msg, "not attached to "+foreign) || !strings.Contains(msg, "resume_session") {
			t.Fatalf("%s naming a foreign session should funnel to resume_session, got %q", tool, msg)
		}
	}
}

// TestNamedResumeOnUnboundConnection: resume_session by explicit handle attaches
// and serves the session's position on a fresh, never-opened connection — the
// slice-2 attach door works unbound (d-cpt-9of). Consent for a foreign session
// arrives in slice 4; here the session is parked on disk with no live holder.
func TestNamedResumeOnUnboundConnection(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"session": session, "canonical": "capture", "label": "resume me unbound",
	}, &serve)
	handle := serve.Session

	// Fresh server, fresh connection, never opened a session: attach by handle.
	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	var resumed mcpserver.ResumeSessionResult
	// Foreign attach on a fresh connection: the session reads idle across the
	// clock gap, so the user's ask (userWords) alone consents.
	call(t, cs2, "resume_session", map[string]any{"session": handle, "userWords": "attach to that unbound session"}, &resumed)
	if resumed.Session != handle {
		t.Fatalf("named resume on an unbound connection should attach to %s, got %s", handle, resumed.Session)
	}
	var capServe *mcpserver.ServeResult
	for i := range resumed.Open {
		if resumed.Open[i].Procedure == "capture" {
			capServe = &resumed.Open[i]
		}
	}
	if capServe == nil || capServe.Step != serve.Step {
		t.Fatalf("attach should serve the parked move at its step, got %+v", resumed.Open)
	}
	if len(capServe.ReportSchema) == 0 {
		t.Fatalf("attach should serve the running move's report schema, got %+v", capServe)
	}

	// The connection is now attached: a work tool with the same handle advances.
	call(t, cs2, "next", map[string]any{"session": handle, "instance": capServe.Instance, "report": assembleReport()}, &serve)
	if serve.Step != "playback" {
		t.Fatalf("the attached connection should advance the move, got %q", serve.Step)
	}
}

// TestSwitchParksSessionWithOpenMove: switching a connection to another session
// applies the leave rule to the one it leaves — a session with an open move is
// parked (not concluded), its move kept and resumable (d-cpt-9of decision 6).
// The target B is seeded on an earlier server so its attachment reads idle to
// the later server that does the switch (the per-server clock gap exceeds the
// recency window), so the consenting switch needs only the user's words.
func TestSwitchParksSessionWithOpenMove(t *testing.T) {
	seed := newTestServer(t, nil, "", "")
	csSeed := connect(t, seed.srv)
	sessionB := openSession(t, csSeed).Session
	var serveB mcpserver.ServeResult
	call(t, csSeed, "start_procedure", map[string]any{"session": sessionB, "canonical": "capture", "label": "target B"}, &serveB)

	// Later server over the same store: A is opened here with an open move.
	env := newTestServer(t, nil, seed.graphDir, seed.sessionsDir)
	csA := connect(t, env.srv)
	sessionA := openSession(t, csA).Session
	var serveA mcpserver.ServeResult
	call(t, csA, "start_procedure", map[string]any{"session": sessionA, "canonical": "capture", "label": "left-behind A"}, &serveA)

	// Switch csA to B: A is left with an open move, so the leave rule parks it.
	// B reads idle across the clock gap, so the user's ask alone consents.
	var resumed mcpserver.ResumeSessionResult
	call(t, csA, "resume_session", map[string]any{"session": sessionB, "userWords": "switch to target B"}, &resumed)
	if resumed.Session != sessionB {
		t.Fatalf("switch should attach to B, got %s", resumed.Session)
	}

	// A is parked, not concluded: it lists with its open capture still standing.
	var listed mcpserver.ListSessionsResult
	call(t, csA, "list_sessions", map[string]any{}, &listed)
	found := false
	for _, s := range listed.Sessions {
		if s.Session == sessionA {
			found = true
			if len(s.Open) != 1 || s.Open[0].Procedure != "capture" || s.Open[0].Step != serveA.Step {
				t.Fatalf("the parked session should keep its open capture at its step, got %+v", s.Open)
			}
		}
	}
	if !found {
		t.Fatalf("the switched-away session should be parked and listed, got %+v", listed.Sessions)
	}

	// A's move is resumable: switching back re-serves the capture at its step.
	// A was parked with its attachment released, so switching back is a foreign
	// attach onto an unheld session — the user's ask consents.
	var reA mcpserver.ResumeSessionResult
	call(t, csA, "resume_session", map[string]any{"session": sessionA, "userWords": "go back to the left-behind work"}, &reA)
	var capA *mcpserver.ServeResult
	for i := range reA.Open {
		if reA.Open[i].Procedure == "capture" {
			capA = &reA.Open[i]
		}
	}
	if capA == nil || capA.Instance != serveA.Instance || capA.Step != serveA.Step {
		t.Fatalf("the parked move should resume intact at its step, got %+v", reA.Open)
	}
}

// TestResumeReorientsCurrentSession drives the post-compaction reach path
// (s-cpt-h31, d-tac-zm2): a connection that has lost its handles calls
// resume_session with no session and gets its live position back — every
// running move at its current step, each with the report schema to continue
// it (the field the observed thrash had to reconstruct from memory). Passing
// the connection's own session id behaves identically instead of erroring.
func TestResumeReorientsCurrentSession(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	// A move is in flight, standing at a step that serves a report schema.
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"session":   session,
		"canonical": "capture",
		"params":    map[string]any{"anchor": fixtureGapID, "kind": "gap"},
		"label":     "mid-flight capture",
	}, &serve)
	if len(serve.ReportSchema) == 0 {
		t.Fatalf("precondition: the capture step should serve a report schema, got %+v", serve)
	}

	assertReoriented := func(label string, resumed mcpserver.ResumeSessionResult) {
		t.Helper()
		if resumed.Session != serve.Session {
			t.Fatalf("%s: should re-serve the bound session %s, got %s", label, serve.Session, resumed.Session)
		}
		var capServe, shellServe *mcpserver.ServeResult
		for i := range resumed.Open {
			switch resumed.Open[i].Procedure {
			case "capture":
				capServe = &resumed.Open[i]
			case "user-dialogue":
				shellServe = &resumed.Open[i]
			}
		}
		if shellServe == nil {
			t.Fatalf("%s: reorientation should re-serve the shell too, got %+v", label, resumed.Open)
		}
		if capServe == nil || capServe.Instance != serve.Instance || capServe.Step != serve.Step {
			t.Fatalf("%s: reorientation should re-serve the in-flight move at its step, got %+v", label, resumed.Open)
		}
		if len(capServe.ReportSchema) == 0 {
			t.Fatalf("%s: reorientation must return the running move's report schema, got %+v", label, capServe)
		}
	}

	// No handle at all — the compacted-agent case.
	var resumed mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{}, &resumed)
	assertReoriented("no session", resumed)

	// The connection's own session id resolves to the same re-serve.
	var resumedByID mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{"session": serve.Session}, &resumedByID)
	assertReoriented("current id", resumedByID)
}

// TestResumeNoSessionUnboundListsParked: with nothing bound to the
// connection, a no-session resume_session still bootstraps — the rejection
// names the parked sessions to resume by handle, the reach path's unbound
// entry (d-tac-zm2 AC2).
func TestResumeNoSessionUnboundListsParked(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "parked work"}, &serve)

	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	msg := callExpectError(t, cs2, "resume_session", map[string]any{})
	if !strings.Contains(msg, "start_session") || !strings.Contains(msg, "resume_session") {
		t.Fatalf("unbound no-session resume should name both doors, got %q", msg)
	}
	if !strings.Contains(msg, serve.Session) || !strings.Contains(msg, "parked work") {
		t.Fatalf("unbound no-session resume should inline the open session, got %q", msg)
	}
}

// jsonSize marshals v and returns its byte length — the rendered wire size a
// reorientation/door serve costs the client's context.
func jsonSize(t *testing.T, v any) int {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return len(raw)
}

// TestConvergingReorientation pins I6: repeated no-args resumes converge —
// blocks this connection already saw stub, so payloads are monotonically
// non-increasing and framing is never re-served (the replay-amplification the
// old clear-and-replay produced). Removing the forgetConnection from the
// same-session path is what makes this hold.
func TestConvergingReorientation(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "converge"}, &serve)

	var prev mcpserver.ResumeSessionResult
	var prevSize int
	for i := range 3 {
		var resumed mcpserver.ResumeSessionResult
		call(t, cs, "resume_session", map[string]any{}, &resumed)
		// Framing was already served at the door — a reorient never re-pays it.
		if resumed.Framing != "" {
			t.Fatalf("reorient %d re-served framing (%d bytes) — convergence broken", i, len(resumed.Framing))
		}
		size := jsonSize(t, resumed)
		if i > 0 && size > prevSize {
			t.Fatalf("reorient %d payload grew (%d > %d) — reorientation must be non-increasing", i, size, prevSize)
		}
		prev, prevSize = resumed, size
	}
	// The move is still re-served at its step so a converged reorient stays
	// actionable — convergence trims repetition, not the position.
	found := false
	for _, open := range prev.Open {
		if open.Instance == serve.Instance {
			found = true
		}
	}
	if !found {
		t.Fatalf("a converged reorient must still re-serve the in-flight move, got %+v", prev.Open)
	}
}

// TestFullReplayReservesOnce pins the compaction escape: a no-args reorient
// converges (framing stubbed), fullReplay serves the complete position exactly
// once, and the next no-args reorient converges again — and the converged
// response never exceeds the full replay (I6: a reorientation never exceeds the
// original serve).
func TestFullReplayReservesOnce(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "replay"}, &mcpserver.ServeResult{})

	var converged mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{}, &converged)
	if converged.Framing != "" {
		t.Fatalf("a plain reorient should stub framing, got %d bytes", len(converged.Framing))
	}

	var full mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{"fullReplay": true}, &full)
	if full.Framing == "" {
		t.Fatal("fullReplay must re-serve the complete framing")
	}
	if jsonSize(t, converged) > jsonSize(t, full) {
		t.Fatalf("a converged reorient (%d) must not exceed the full replay (%d)", jsonSize(t, converged), jsonSize(t, full))
	}

	// Full replay is one-shot: the very next no-args reorient converges again.
	var again mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{}, &again)
	if again.Framing != "" {
		t.Fatalf("fullReplay must be one-shot — the next reorient should stub framing, got %d bytes", len(again.Framing))
	}
}

// TestShellComposedFraming pins the framing composition (I6, A1): the engine
// supplies the info block and the shell's spec-declared lanes render through
// the injection mechanism — including the byte-capped recent-moves lane —
// with no hardcoded Go layout constant behind it.
func TestShellComposedFraming(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)
	if !strings.Contains(door.Framing, "Local participant:") {
		t.Fatalf("framing must carry the engine-supplied info block, got %q", door.Framing)
	}
	if !strings.Contains(door.Framing, "Recent graph movement") {
		t.Fatalf("framing must carry the shell's declared recent-moves lane, got %q", door.Framing)
	}
}

// TestShellWithoutFramingServesInfoOnly pins the fail-loud fallback: a shell
// that declares no framing lanes serves the info block alone — never a silent
// fallback to a deleted Go constant.
func TestShellWithoutFramingServesInfoOnly(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "graph")
	path := filepath.Join(graphDir, "2026/07/04-130000-d-prc-bare.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(bareShellProcedure), 0644); err != nil {
		t.Fatal(err)
	}
	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)
	var serve mcpserver.ServeResult
	call(t, cs, "start_session", map[string]any{"shell": "bare-shell"}, &serve)
	if !strings.Contains(serve.Framing, "Local participant:") {
		t.Fatalf("an info-only framing must still carry the info block, got %q", serve.Framing)
	}
	if strings.Contains(serve.Framing, "Recent graph movement") || strings.Contains(serve.Framing, "Guiding directives") {
		t.Fatalf("a shell with no declared lanes must serve info only, got %q", serve.Framing)
	}
}

// TestDoorPayloadUnder25KB asserts the default shell's door serve — info block,
// declared lanes, the other-work count line, and the shell instructions —
// stays within the 25KB contract against a representative graph (A1, AC10).
func TestDoorPayloadUnder25KB(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "graph")
	// A representative graph: many entries with realistic summaries, plus a few
	// aspirations and guiding directives so every framing lane has content.
	for i := range 60 {
		typeCode, layerCode, kind, extra := "s", "tac", "gap", "layer: tactical\n"
		switch i % 5 {
		case 0:
			typeCode, layerCode, kind, extra = "d", "stg", "aspiration", "layer: strategic\n"
		case 1:
			typeCode, layerCode, kind, extra = "d", "cpt", "directive", "layer: conceptual\nintent: guiding\n"
		}
		path := filepath.Join(graphDir, fmt.Sprintf("2026/06/%02d-%06d-%s-%s-x%02d.md", (i%28)+1, 100000+i, typeCode, layerCode, i))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf(`---
type: %s
kind: %s
%sconfidence: medium
summary: Representative entry %02d records a realistic first-sentence-heavy observation about the flux subsystem — the kind of ~350-character one-liner a real graph accumulates, naming the concrete surface that moved, the trade-off weighed in dialogue, and the follow-up it opens, so the recent-moves and heat lanes have honestly-sized content to rank and cap against rather than a toy stub of a summary line.
---

Body of entry %02d: a paragraph of realistic content that the door serve never renders in full but that inflates the graph the framing lanes rank over.
`, typeName(typeCode), kind, extra, i, i)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)
	if size := jsonSize(t, door); size > 25000 {
		t.Fatalf("default shell door payload is %d bytes, exceeds the 25KB contract", size)
	}
}

// typeName maps a filename type code to its frontmatter type for the fixture.
func typeName(code string) string {
	if code == "d" {
		return "decision"
	}
	return "signal"
}

// bareShellProcedure is a minimal session shell declaring no framing lanes —
// used to pin the info-only framing fallback.
const bareShellProcedure = `---
type: decision
layer: process
kind: procedure
canonical: bare-shell
class: shell
confidence: medium
summary: A minimal shell that declares no framing lanes.
steps:
    - id: junction
      chooser: user
      render: open
      goal: "dialogue freely"
      options:
          - {choice: conclude, to: end(completed)}
---

A bare shell.

## unit: open

You are in a bare session shell.
`

// runCaptureToCompletion drives the fixture capture spine to a written entry:
// assemble → playback confirm → verifySummary faithful. Findings must be nil
// (newTestServer(t, nil, …)) so the write cascades past the no-high-findings
// gate. Returns the created entry ID.
func runCaptureToCompletion(t *testing.T, cs *mcp.ClientSession, session, label string) string {
	t.Helper()
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": label}, &serve)
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": assembleReport()}, &serve)
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "yes, capture it",
	}}, &serve)
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "verifySummary", "choice": "faithful", "fields": map[string]any{"fidelityNote": "matches"},
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("capture did not complete, got %s at %q", serve.Status, serve.Step)
	}
	id, _ := serve.Produced["entryId"].(string)
	return id
}

// TestFramingLaneDedupAfterWrite pins per-lane framing dedup (I6, AC8): after a
// graph write changes only the recent-moves lane, a reorient re-serves THAT
// lane alone — the stable info block stays stubbed — so the reorientation stays
// under the original serve. Under whole-block hashing the entire framing would
// re-serve on any write, the reproduced regression.
func TestFramingLaneDedupAfterWrite(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	// Connection B: the reorienting connection.
	csB := connect(t, env.srv)
	door := openSession(t, csB)
	if !strings.Contains(door.Framing, "Local participant:") || !strings.Contains(door.Framing, "Recent graph movement") {
		t.Fatalf("the door should serve the full framing, got %q", door.Framing)
	}
	var converged mcpserver.ResumeSessionResult
	call(t, csB, "resume_session", map[string]any{}, &converged)
	if converged.Framing != "" {
		t.Fatalf("with nothing changed, a reorient converges to empty framing, got %q", converged.Framing)
	}

	// Connection A writes an entry — the recent-moves lane now changes for B.
	csA := connect(t, env.srv)
	sessionA := openSession(t, csA).Session
	if id := runCaptureToCompletion(t, csA, sessionA, "churn write"); id == "" {
		t.Fatal("precondition: the capture should produce an entry")
	}

	var afterWrite mcpserver.ResumeSessionResult
	call(t, csB, "resume_session", map[string]any{}, &afterWrite)
	if !strings.Contains(afterWrite.Framing, "Recent graph movement") {
		t.Fatalf("the changed recent-moves lane must re-serve, got %q", afterWrite.Framing)
	}
	if strings.Contains(afterWrite.Framing, "Local participant:") {
		t.Fatalf("the unchanged info block must stay stubbed (per-lane dedup), got %q", afterWrite.Framing)
	}
	if len(afterWrite.Framing) >= len(door.Framing) {
		t.Fatalf("a post-write reorient (%d) must stay under the original serve (%d)", len(afterWrite.Framing), len(door.Framing))
	}
}

// TestReorientCarriesCompactionBreadcrumb pins the B4 self-trigger: a compacted
// agent's reorient stubs blocks it already saw, and those stubs (both the resume
// instructions and the per-step reminder) must name the fullReplay escape — a
// bare "served earlier, follow them" is useless to an amnesiac.
func TestReorientCarriesCompactionBreadcrumb(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "breadcrumb"}, &serve)

	var resumed mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{}, &resumed)
	if !strings.Contains(resumed.Instructions, "fullReplay") {
		t.Fatalf("resume instructions should point a compaction victim at fullReplay, got %q", resumed.Instructions)
	}
	var reminder *mcpserver.ServeResult
	for i := range resumed.Open {
		if resumed.Open[i].Instance == serve.Instance {
			reminder = &resumed.Open[i]
		}
	}
	if reminder == nil {
		t.Fatalf("the reorient should re-serve the in-flight move, got %+v", resumed.Open)
	}
	if !strings.Contains(reminder.Instructions, "served earlier this session") || !strings.Contains(reminder.Instructions, "fullReplay") {
		t.Fatalf("a stubbed per-step reminder must carry the compaction breadcrumb, got %q", reminder.Instructions)
	}
}

// TestChooserSequenceValidation exercises the trust property over MCP: a
// chooser cannot be answered before it is pending.
func TestChooserSequenceValidation(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &serve)

	msg := callExpectError(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "early answer",
	}})
	if !strings.Contains(msg, "gate") && !strings.Contains(msg, "pending") {
		t.Fatalf("early chooser answer should be rejected with sequence guidance, got %q", msg)
	}
}

// TestReadAttachmentPaging pages through a fixture attachment on a remote
// (non-local) server: content pages, and no filesystem path leaks out.
func TestReadAttachmentPaging(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	var page1 mcpserver.ReadAttachmentResult
	call(t, cs, "read_attachment", map[string]any{"id": fixtureGapID, "max_bytes": 6}, &page1)
	if page1.Name != "notes.md" || page1.Content != "012345" || !page1.More {
		t.Fatalf("first page diverged: %+v", page1)
	}
	if page1.Path != "" {
		t.Fatalf("a remote client must not receive a filesystem path, got %q", page1.Path)
	}
	if !slices.Equal(page1.Available, []string{"notes.md"}) {
		t.Fatalf("available attachments = %v", page1.Available)
	}
	var page2 mcpserver.ReadAttachmentResult
	call(t, cs, "read_attachment", map[string]any{
		"id": fixtureGapID, "name": "notes.md", "offset": page1.NextOffset, "max_bytes": 6,
	}, &page2)
	if page2.Content != "6789" || page2.More {
		t.Fatalf("second page diverged: %+v", page2)
	}
	if page1.TotalBytes != 10 {
		t.Fatalf("total bytes diverged: %d", page1.TotalBytes)
	}
}

// TestReadAttachmentLocalPath checks that a local (stdio) client gets the
// absolute path alongside the content, so it can read the file directly.
func TestReadAttachmentLocalPath(t *testing.T) {
	env := newTestServer(t, nil, "", "", func(o *mcpserver.Options) { o.LocalClient = true })
	cs := connect(t, env.srv)

	var res mcpserver.ReadAttachmentResult
	call(t, cs, "read_attachment", map[string]any{"id": fixtureGapID}, &res)
	want, err := filepath.Abs(filepath.Join(env.graphDir, "2026/06/01-100000-s-tac-aaa/notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != want {
		t.Fatalf("local client should receive the absolute path:\n got %q\nwant %q", res.Path, want)
	}
	if content, err := os.ReadFile(res.Path); err != nil || string(content) != "0123456789" {
		t.Fatalf("returned path must be directly readable: %v (%q)", err, content)
	}
}

// TestFreeReads smoke-tests the ungated read tools, including the breadcrumb
// they carry while no session is open.
func TestFreeReads(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	var info mcpserver.InfoResult
	call(t, cs, "info", map[string]any{}, &info)
	if info.Participant != "Tester" || info.Search != "text" {
		t.Fatalf("info diverged: %+v", info)
	}
	if !strings.Contains(info.Hint, "start_session") {
		t.Fatalf("reads without a session must carry the door breadcrumb, got %q", info.Hint)
	}

	var view mcpserver.ViewResult
	call(t, cs, "view", map[string]any{"layout": "active:as-counts"}, &view)
	if !strings.Contains(view.Sections, "testing/fixture") {
		t.Fatalf("view should list the fixture topic, got %q", view.Sections)
	}
	if !strings.Contains(view.Hint, "start_session") {
		t.Fatalf("view without a session must carry the door breadcrumb, got %q", view.Hint)
	}

	var show mcpserver.ShowResult
	call(t, cs, "show", map[string]any{"ids": []string{fixtureGapID}}, &show)
	if !strings.Contains(show.Entries, fixtureGapID) {
		t.Fatalf("show should render the entry, got %q", show.Entries)
	}

	var search mcpserver.SearchResult
	call(t, cs, "search", map[string]any{"terms": []string{"oscillation"}}, &search)
	if !strings.Contains(search.Results, fixtureGapID) {
		t.Fatalf("search should find the fixture gap, got %q", search.Results)
	}

	// Once the session is open, the breadcrumb disappears.
	openSession(t, cs)
	call(t, cs, "info", map[string]any{}, &info)
	if info.Hint != "" {
		t.Fatalf("reads inside a session must carry no breadcrumb, got %q", info.Hint)
	}

	var reg mcpserver.RegistryResult
	call(t, cs, "registry", map[string]any{"class": "command"}, &reg)
	var names []string
	for _, f := range reg.Functions {
		names = append(names, f.Name)
	}
	joined := strings.Join(names, ",")
	for _, want := range []string{"newEntry", "replaceSummary", "confirmPlayback", "recordOverride", "wipStart", "wipDone"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("registry should document command %s, got %v", want, names)
		}
	}
}

// TestEmbeddedEngageExploreProcedures drives the shipped engage and explore
// base entries over MCP with the production entryChains query: anchor
// resolution against the graph, chains rendered into the brief/inspect units,
// the moves junction, and the parent link tying the explore sub-move to the
// engage instance in the session log.
func TestEmbeddedEngageExploreProcedures(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "graph")
	path := filepath.Join(graphDir, "2026/06/01-100000-s-tac-aaa.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(gapEntry), 0644); err != nil {
		t.Fatal(err)
	}
	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	// Engage: anchor step stalls a made-up anchor, accepts the fixture gap.
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "engage"}, &serve)
	if serve.Step != "anchor" || serve.Status != "running" {
		t.Fatalf("expected running at anchor, got %s at %q", serve.Status, serve.Step)
	}
	engageInstance := serve.Instance

	call(t, cs, "next", map[string]any{"session": session, "instance": engageInstance, "report": map[string]any{
		"anchor": "20260601-110000-s-tac-zzz",
	}}, &serve)
	if serve.Step != "anchor" || !strings.Contains(serve.Instructions, "does not resolve") {
		t.Fatalf("unresolved anchor should hold the gate naming it, got step %q: %q", serve.Step, serve.Instructions)
	}

	call(t, cs, "next", map[string]any{"session": session, "instance": engageInstance, "report": map[string]any{
		"anchor": fixtureGapID,
		"goal":   "orient on the oscillation gap",
	}}, &serve)
	if serve.Step != "brief" {
		t.Fatalf("resolved anchor should reach brief, got %q (%s)", serve.Step, serve.Instructions)
	}
	// The injected chains carry the anchor's full body — served, not fetched.
	if !strings.Contains(serve.Instructions, "contradictory findings across runs on identical input") {
		t.Fatalf("brief unit should serve the anchor's full body via entryChains, got %q", serve.Instructions)
	}

	call(t, cs, "next", map[string]any{"session": session, "instance": engageInstance, "report": map[string]any{
		"brief":       "Narrative: open gap, no downstream activity; needs a decision.",
		"widenReport": "searched oscillation and pre-flight angles; chain already saturates",
	}}, &serve)
	if serve.Step != "moves" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("brief should reach the moves user junction, got %q", serve.Step)
	}
	// The pending chooser names itself by step id — the value a chooser answer
	// must supply, so it appears in the payload it answers (s-tac-keb).
	if serve.PendingChooser.Chooser != "moves" {
		t.Fatalf("pending_chooser must name its step id, got %q", serve.PendingChooser.Chooser)
	}

	call(t, cs, "next", map[string]any{"session": session, "instance": engageInstance, "report": map[string]any{
		"chooser": "moves", "choice": "move", "userWords": "let's explore around it first",
		"fields": map[string]any{"selectedMove": "explore the neighborhood"},
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("move pick should complete the engagement, got %s at %q", serve.Status, serve.Step)
	}

	// Explore as a sub-move of the finished engage, parent-linked.
	call(t, cs, "start_procedure", map[string]any{
		"session":   session,
		"canonical": "explore",
		"parent":    engageInstance,
		"params": map[string]any{
			"targets": []string{fixtureGapID},
			"goal":    "overview: what surrounds the oscillation gap",
		},
	}, &serve)
	if serve.Step != "inspect" {
		t.Fatalf("explore should start at inspect, got %q", serve.Step)
	}
	// explore is a task — its serves carry the fork-preferred execution hint
	// (d-tac-tlo, s-tac-p21 finding 3).
	if serve.Execution != "fork-preferred" {
		t.Fatalf("task-class serve should carry execution fork-preferred, got %q", serve.Execution)
	}
	if !strings.Contains(serve.Instructions, "overview: what surrounds the oscillation gap") {
		t.Fatalf("inspect unit should carry the goal verbatim, got %q", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, "contradictory findings across runs on identical input") {
		t.Fatalf("inspect unit should serve the target chains, got %q", serve.Instructions)
	}
	exploreInstance := serve.Instance

	call(t, cs, "next", map[string]any{"session": session, "instance": exploreInstance, "report": map[string]any{
		"widenReport":  "two angles searched, nothing beyond the target",
		"inspectedIds": []string{fixtureGapID},
		"briefing":     "## Goal\noverview: what surrounds the oscillation gap\n\n## Targets\n" + fixtureGapID,
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("batched mission report should complete explore, got %s at %q (%s)", serve.Status, serve.Step, serve.Instructions)
	}

	// The parent link is durable: the started event in the session log
	// carries it, so replay and forensics keep the lineage.
	events, err := readSessionLog(t, env.sessionsDir, serve.Session)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range events {
		if ev.Event == engine.EventStarted && ev.Instance == exploreInstance {
			if p, _ := ev.Data["parent"].(string); p != engageInstance {
				t.Fatalf("explore started event parent = %q, want %q", p, engageInstance)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no started event for the explore instance in the session log")
	}

	// An unknown parent is rejected by the engine's single validation path.
	msg := callExpectError(t, cs, "start_procedure", map[string]any{
		"session":   session,
		"canonical": "explore",
		"parent":    "i_nope",
		"params": map[string]any{
			"targets": []string{fixtureGapID},
			"goal":    "any",
		},
	})
	if !strings.Contains(msg, "parent") {
		t.Fatalf("unknown parent error should name it, got %q", msg)
	}
}

// TestDoorGatingAndShellLifecycle pins the d-cpt-h99 session architecture:
// one door, gated loop tools, the shell's resident junction, auto-parented
// moves, the nested landing when a move ends, and the conclude path.
func TestDoorGatingAndShellLifecycle(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	// Every work tool rejects without a session handle, naming both doors.
	for tool, args := range map[string]map[string]any{
		"start_procedure":  {"canonical": "capture"},
		"next":             {"instance": "i_1", "report": map[string]any{"x": "y"}},
		"park":             {"instance": "i_1"},
		"abandon":          {"instance": "i_1"},
		"stage_attachment": {"name": "a.md", "content": "x"},
	} {
		msg := callExpectError(t, cs, tool, args)
		if !strings.Contains(msg, "start_session") || !strings.Contains(msg, "resume_session") {
			t.Fatalf("%s without a handle should name both doors, got %q", tool, msg)
		}
	}

	// Discovery is free: list_sessions works on the fresh unbound connection,
	// and a named resume of an unknown handle fails on its own terms — never
	// the no-handle door error.
	var discovered mcpserver.ListSessionsResult
	call(t, cs, "list_sessions", map[string]any{}, &discovered)
	if msg := callExpectError(t, cs, "resume_session", map[string]any{"session": "s_nope"}); !strings.Contains(msg, "unknown session") {
		t.Fatalf("a named resume of an unknown handle should name it, got %q", msg)
	}

	// The door serves the shell's orientation: standing goal and the live move
	// enumeration (shells excluded). Participant/language/search live in the
	// engine-supplied framing block now, not the unit — the single source.
	shell := openSession(t, cs)
	session := shell.Session
	if shell.Goal != "dialogue freely; start a move when something crystallizes" {
		t.Fatalf("shell junction should carry the standing goal, got %q", shell.Goal)
	}
	if !strings.Contains(shell.Framing, "Local participant: Tester") {
		t.Fatalf("the framing block should carry the session info header, got %q", shell.Framing)
	}
	if strings.Contains(shell.Instructions, "Local participant:") || strings.Contains(shell.Instructions, "Participant: Tester") {
		t.Fatalf("the orientation unit must not duplicate the info block, got %q", shell.Instructions)
	}
	if !strings.Contains(shell.Instructions, "- capture(") {
		t.Fatalf("opening serve should enumerate the moves, got %q", shell.Instructions)
	}
	if strings.Contains(shell.Instructions, "- user-dialogue") {
		t.Fatalf("the move enumeration must exclude shells, got %q", shell.Instructions)
	}
	if shell.PendingChooser == nil || shell.PendingChooser.Kind != "user" {
		t.Fatalf("the resident junction is a user chooser, got %+v", shell.PendingChooser)
	}
	if shell.Framing == "" {
		t.Fatal("the opening serve should carry the framing block")
	}

	// Knocking again re-serves the orientation in full.
	reshell := openSession(t, cs)
	if reshell.Instance != shell.Instance || !strings.Contains(reshell.Instructions, "Standing goal") {
		t.Fatalf("start_session on an open session should re-serve the same shell in full, got %q", reshell.Instructions)
	}

	// Shells never start through the move path.
	if msg := callExpectError(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "user-dialogue"}); !strings.Contains(msg, "start_session") {
		t.Fatalf("starting a shell as a move should point at the door, got %q", msg)
	}

	// Moves auto-parent to the shell instance.
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &serve)
	events, err := readSessionLog(t, env.sessionsDir, serve.Session)
	if err != nil {
		t.Fatal(err)
	}
	parented := false
	for _, ev := range events {
		if ev.Event == engine.EventStarted && ev.Instance == serve.Instance {
			if p, _ := ev.Data["parent"].(string); p != shell.Instance {
				t.Fatalf("move should auto-parent to the shell, got parent %q want %q", p, shell.Instance)
			}
			parented = true
		}
	}
	if !parented {
		t.Fatal("no started event for the capture instance")
	}

	// Mid-procedure serves carry no open-threads block.
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": assembleReport()}, &serve)
	if serve.OpenThreads != "" {
		t.Fatalf("mid-procedure serve must not carry open threads, got %q", serve.OpenThreads)
	}

	// A move that ends lands back on the shell junction — nested serve.
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "capture it",
	}}, &serve)
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "verifySummary", "choice": "faithful", "userWords": "",
		"fields": map[string]any{"fidelityNote": "matches"},
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("expected completion, got %s at %q", serve.Status, serve.Step)
	}
	if serve.Base == nil || serve.Base.Procedure != "user-dialogue" || serve.Base.Instance != shell.Instance {
		t.Fatalf("a completed move should land on the shell junction, got %+v", serve.Base)
	}
	if !strings.Contains(serve.Base.Instructions, "served earlier this session") {
		t.Fatalf("the shell's re-serve should be the one-line reminder, got %q", serve.Base.Instructions)
	}

	// The shell itself never abandons.
	if msg := callExpectError(t, cs, "abandon", map[string]any{"session": session, "instance": shell.Instance}); !strings.Contains(msg, "conclude") {
		t.Fatalf("abandoning the shell should point at conclude, got %q", msg)
	}

	// Conclude with nothing open ends the session directly.
	call(t, cs, "next", map[string]any{"session": session, "instance": shell.Instance, "report": map[string]any{
		"chooser": "junction", "choice": "conclude", "userWords": "we're done here",
	}}, &serve)
	if serve.Status != "completed" || serve.Procedure != "user-dialogue" {
		t.Fatalf("conclude on a quiescent session should complete the shell, got %s %s at %q", serve.Procedure, serve.Status, serve.Step)
	}
}

// TestShellConcludeWalksOpenThreads pins the wrap path: conclude with open
// moves routes to the threads step for per-thread decisions, park returns to
// the junction, and conclude completes once the threads are settled.
func TestShellConcludeWalksOpenThreads(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	shell := openSession(t, cs)
	session := shell.Session

	var capture mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &capture)

	// Conclude with an open capture: the quiescence gate holds and routes to
	// the threads step, whose serve carries the open work.
	var serve mcpserver.ServeResult
	call(t, cs, "next", map[string]any{"session": session, "instance": shell.Instance, "report": map[string]any{
		"chooser": "junction", "choice": "conclude", "userWords": "wrap it up",
	}}, &serve)
	if serve.Step != "threads" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("conclude with open moves should reach the threads chooser, got %s at %q", serve.Status, serve.Step)
	}
	if !strings.Contains(serve.OpenThreads, capture.Instance) {
		t.Fatalf("the threads serve should list the open capture, got %q", serve.OpenThreads)
	}

	// Park keeps everything and returns to the resident junction.
	call(t, cs, "next", map[string]any{"session": session, "instance": shell.Instance, "report": map[string]any{
		"chooser": "threads", "choice": "park", "userWords": "keep it for later",
	}}, &serve)
	if serve.Step != "junction" || serve.Status != "running" {
		t.Fatalf("park should return to the junction, got %s at %q", serve.Status, serve.Step)
	}

	// Settle the thread (abandon it), then conclude for real.
	var ab mcpserver.AbandonResult
	call(t, cs, "abandon", map[string]any{"session": session, "instance": capture.Instance, "reason": "not needed"}, &ab)
	call(t, cs, "next", map[string]any{"session": session, "instance": shell.Instance, "report": map[string]any{
		"chooser": "junction", "choice": "conclude", "userWords": "done now",
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("conclude after settling threads should complete the shell, got %s at %q", serve.Status, serve.Step)
	}
}

// TestLeavePathsRecordNothingAcrossPaths pins d-cpt-rw7 at the real call sites:
// every way a connection steps away — switching, shutting down, dropping the
// socket — clears the live stamp and ends nothing, while the one participant act
// among them, a teardown by handle, records the terminal abandon.
func TestLeavePathsRecordNothingAcrossPaths(t *testing.T) {
	end := func(t *testing.T, env testEnv, session string) *sdd.SessionEnd {
		t.Helper()
		stored, err := env.sessions.Load(t.Context(), sdd.SessionID(session))
		if err != nil {
			t.Fatal(err)
		}
		if stored.Metadata.Attachment != nil {
			t.Fatalf("session %s still reads held: %+v", session, stored.Metadata.Attachment)
		}
		return stored.Metadata.Ended
	}
	startMove := func(t *testing.T, cs *mcp.ClientSession, session string) {
		t.Helper()
		var serve mcpserver.ServeResult
		call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "work"}, &serve)
	}

	t.Run("switch away from a session with an open move ends nothing", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		other := connect(t, env.srv)
		b := openSession(t, other).Session // a second session to attach away to
		cs := connect(t, env.srv)
		a := openSession(t, cs).Session
		startMove(t, cs, a)
		var resumed mcpserver.ResumeSessionResult
		// b was just opened on this server, so it reads active: the switch takes it over.
		call(t, cs, "resume_session", map[string]any{"session": b, "userWords": "move to the other dialogue", "takeover": true}, &resumed) // switch away from a
		if got := end(t, env, a); got != nil {
			t.Fatalf("switch away (open move) ended the session: %+v", got)
		}
	})

	t.Run("switch away from a quiescent session ends nothing", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		other := connect(t, env.srv)
		b := openSession(t, other).Session
		cs := connect(t, env.srv)
		a := openSession(t, cs).Session // no move — quiescent
		var resumed mcpserver.ResumeSessionResult
		call(t, cs, "resume_session", map[string]any{"session": b, "userWords": "move to the other dialogue", "takeover": true}, &resumed) // switch away; shell auto-concludes
		if got := end(t, env, a); got != nil {
			t.Fatalf("switch away (quiescent) ended the session: %+v", got)
		}
	})

	t.Run("abandon by handle records the terminal abandon", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		csA := connect(t, env.srv)
		a := openSession(t, csA).Session
		startMove(t, csA, a)
		// A later server reads a as idle, so teardown by handle proceeds (a recent
		// session would be refused).
		env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
		csB := connect(t, env2.srv)
		var torn mcpserver.AbandonResult
		call(t, csB, "abandon", map[string]any{"session": a}, &torn)
		got := end(t, env, a)
		if got == nil || got.Act != sdd.SessionAbandoned {
			t.Fatalf("teardown by handle = %+v, want a terminal abandon", got)
		}
	})

	t.Run("shutdown ends nothing", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		cs := connect(t, env.srv)
		a := openSession(t, cs).Session
		startMove(t, cs, a)
		if err := env.srv.Shutdown(t.Context()); err != nil {
			t.Fatalf("shutdown: %v", err)
		}
		if got := end(t, env, a); got != nil {
			t.Fatalf("shutdown ended the session: %+v", got)
		}
	})

	t.Run("disconnect ends nothing", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		cs, ss := connectPair(t, env.srv)
		a := openSession(t, cs).Session
		startMove(t, cs, a)
		if err := env.srv.Disconnect(t.Context(), ss); err != nil {
			t.Fatalf("disconnect: %v", err)
		}
		if got := end(t, env, a); got != nil {
			t.Fatalf("disconnect ended the session: %+v", got)
		}
	})
}

// TestParkedSessionsAcrossConnections pins the leave rule and the discovery
// contract: every session with open work is discoverable through list_sessions
// (an active one labeled active with its client, never hidden), while a serve
// never pushes other dialogues — session entry carries a one-line count of
// them instead (I6, A1). Attaching to one a live client holds still refuses;
// switching away from a quiescent session auto-ends it.
func TestParkedSessionsAcrossConnections(t *testing.T) {
	env := newTestServer(t, nil, "", "")

	// Server 1, connection A: start a capture (stays live-bound here).
	csA := connect(t, env.srv)
	sessionA := openSession(t, csA).Session
	var serveA mcpserver.ServeResult
	call(t, csA, "start_procedure", map[string]any{"session": sessionA, "canonical": "capture", "label": "parked capture A"}, &serveA)

	// Connection B on the same server: A is active. The door serve carries a
	// one-line count of the other open dialogue — never its label or handle —
	// and list_sessions is where the full labeled listing lives.
	csB := connect(t, env.srv)
	shellB := openSession(t, csB)
	if !strings.Contains(shellB.OpenThreads, "1 other open dialogue") || !strings.Contains(shellB.OpenThreads, "list_sessions") {
		t.Fatalf("session entry should carry a one-line count of other open dialogues, got %q", shellB.OpenThreads)
	}
	if strings.Contains(shellB.OpenThreads, sessionA) || strings.Contains(shellB.OpenThreads, "parked capture A") {
		t.Fatalf("other dialogues must never be listed on a serve, got %q", shellB.OpenThreads)
	}
	var listed mcpserver.ListSessionsResult
	call(t, csB, "list_sessions", map[string]any{}, &listed)
	if len(listed.Sessions) != 1 || listed.Sessions[0].Session != sessionA {
		t.Fatalf("the active session must list, got %+v", listed.Sessions)
	}
	if listed.Sessions[0].Activity != "active" || listed.Sessions[0].ClientName != "test-client" {
		t.Fatalf("an active session should list as active with its client name, got %+v", listed.Sessions[0])
	}
	// Attaching to an active session takes it over: the user's ask plus takeover
	// consent it, and the response names the displaced holder and the fidelity
	// limit. The displaced client, A, learns of the takeover at its next append:
	// it fails typed rather than corrupting the shared session log.
	var claimed mcpserver.ResumeSessionResult
	call(t, csB, "resume_session", map[string]any{"session": sessionA, "userWords": "take over the stuck capture", "takeover": true}, &claimed)
	if claimed.Session != sessionA {
		t.Fatalf("attaching to an active session should succeed and displace, got %+v", claimed)
	}
	if !strings.Contains(claimed.Instructions, "Attached over") || !strings.Contains(claimed.Instructions, "recorded session state") {
		t.Fatalf("a takeover response should name the displaced holder and the fidelity limit, got %q", claimed.Instructions)
	}
	if msg := callExpectError(t, csA, "stage_attachment", map[string]any{"session": sessionA, "name": "note.md", "content": "x"}); !strings.Contains(msg, "taken over") {
		t.Fatalf("a displaced client's next append should fail typed naming the takeover, got %q", msg)
	}

	// A fresh server over the same sessions dir sees A as parked: the door
	// carries the other-work count (never the label), and resuming A auto-ends
	// the quiescent session the connection came from.
	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	csC := connect(t, env2.srv)
	shellC := openSession(t, csC)
	if !strings.Contains(shellC.OpenThreads, "1 other open dialogue") || !strings.Contains(shellC.OpenThreads, "list_sessions") {
		t.Fatalf("the door should carry the other-work count, got %q", shellC.OpenThreads)
	}
	if strings.Contains(shellC.OpenThreads, sessionA) || strings.Contains(shellC.OpenThreads, "parked capture A") {
		t.Fatalf("other dialogues must never be listed on a serve, got %q", shellC.OpenThreads)
	}

	var resumed mcpserver.ResumeSessionResult
	// A reads idle from this later server, so the user's ask alone consents.
	call(t, csC, "resume_session", map[string]any{"session": sessionA, "userWords": "resume the parked capture"}, &resumed)
	if resumed.Session != sessionA {
		t.Fatalf("resume diverged: %+v", resumed)
	}
	// The fresh session C stepped away from was quiescent: its shell is
	// auto-ended in the log — a closed tab leaves no corpse.
	events, err := readSessionLog(t, env.sessionsDir, shellC.Session)
	if err != nil {
		t.Fatal(err)
	}
	autoEnded := false
	for _, ev := range events {
		if ev.Event == engine.EventAbandoned && ev.Instance == shellC.Instance {
			if reason, _ := ev.Data["reason"].(string); strings.Contains(reason, "auto-concluded") {
				autoEnded = true
			}
		}
	}
	if !autoEnded {
		t.Fatalf("leaving a quiescent session should auto-end its shell, events: %+v", events)
	}
}

// readSessionLog reads a session's JSONL event log from the sessions dir.
func readSessionLog(t *testing.T, dir, session string) ([]engine.Event, error) {
	t.Helper()
	store, err := localadapter.NewFilesystemSessionStoreAt(dir)
	if err != nil {
		return nil, err
	}
	stored, err := store.Load(t.Context(), sdd.SessionID(session))
	if err != nil {
		return nil, err
	}
	var events []engine.Event
	for _, item := range stored.Events {
		if item.Code != sdd.WorkflowEventCode {
			continue
		}
		var event engine.Event
		if err := json.Unmarshal(item.Payload, &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func TestResumeConsentDecisionTable(t *testing.T) {
	t.Run("foreign attach without userWords is rejected (mwd)", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		cs1 := connect(t, env.srv)
		a := openSession(t, cs1).Session
		cs2 := connect(t, env.srv)
		msg := callExpectError(t, cs2, "resume_session", map[string]any{"session": a})
		if !strings.Contains(msg, "userWords") || !strings.Contains(msg, "verbatim") {
			t.Fatalf("missing-userWords rejection should name what is required, got %q", msg)
		}
	})

	t.Run("recent attach without takeover refuses naming the holder", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		cs1 := connect(t, env.srv)
		a := openSession(t, cs1).Session
		cs2 := connect(t, env.srv)
		msg := callExpectError(t, cs2, "resume_session", map[string]any{"session": a, "userWords": "take it over"})
		for _, want := range []string{"test-client", "last active", "takeover", "recorded session state"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("recent refusal %q missing %q (holder/activity/fidelity)", msg, want)
			}
		}
	})

	t.Run("recent attach with takeover lands, naming displacement and fidelity", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		cs1 := connect(t, env.srv)
		a := openSession(t, cs1).Session
		cs2 := connect(t, env.srv)
		var resumed mcpserver.ResumeSessionResult
		call(t, cs2, "resume_session", map[string]any{"session": a, "userWords": "take it over", "takeover": true}, &resumed)
		if resumed.Session != a {
			t.Fatalf("takeover attach should land on %s, got %+v", a, resumed)
		}
		if !strings.Contains(resumed.Instructions, "Attached over") || !strings.Contains(resumed.Instructions, "recorded session state") {
			t.Fatalf("takeover response should name the displaced holder and fidelity limit, got %q", resumed.Instructions)
		}
	})

	t.Run("missing userWords rejected against an idle target too", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		cs1 := connect(t, env.srv)
		a := openSession(t, cs1).Session
		// A later server reads a as idle: userWords is still required — consent is
		// about crossing dialogues, not about liveness.
		env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
		cs2 := connect(t, env2.srv)
		msg := callExpectError(t, cs2, "resume_session", map[string]any{"session": a})
		if !strings.Contains(msg, "userWords") || !strings.Contains(msg, "verbatim") {
			t.Fatalf("missing-userWords rejection should hold for an idle target, got %q", msg)
		}
	})

	t.Run("idle attach with userWords lands, current attachment carries the words", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		cs1 := connect(t, env.srv)
		a := openSession(t, cs1).Session
		// A later server over the same store reads a's attachment as idle.
		env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
		cs2 := connect(t, env2.srv)
		const words = "pick the earlier thread back up"
		var resumed mcpserver.ResumeSessionResult
		call(t, cs2, "resume_session", map[string]any{"session": a, "userWords": words}, &resumed)
		if resumed.Session != a {
			t.Fatalf("idle attach with userWords should land on %s, got %+v", a, resumed)
		}
		stored, err := env.sessions.Load(t.Context(), sdd.SessionID(a))
		if err != nil {
			t.Fatal(err)
		}
		if stored.Metadata.Attachment == nil || stored.Metadata.Attachment.UserWords != words {
			t.Fatalf("the live stamp should carry the consenting words, attachment: %+v", stored.Metadata.Attachment)
		}
	})

	t.Run("parked (no-attachment) attach with userWords records the words durably (row a)", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		cs, ss := connectPair(t, env.srv)
		a := openSession(t, cs).Session
		// The holder disconnects: a's attachment is released to none — a parked
		// session with no current attachment, the most common attach target.
		if err := env.srv.Disconnect(t.Context(), ss); err != nil {
			t.Fatalf("disconnect: %v", err)
		}
		cs2 := connect(t, env.srv)
		const words = "resume the parked dialogue"
		var resumed mcpserver.ResumeSessionResult
		call(t, cs2, "resume_session", map[string]any{"session": a, "userWords": words}, &resumed)
		stored, err := env.sessions.Load(t.Context(), sdd.SessionID(a))
		if err != nil {
			t.Fatal(err)
		}
		if stored.Metadata.Attachment == nil || stored.Metadata.Attachment.UserWords != words {
			t.Fatalf("attaching a parked session should stamp the consenting words on the live attachment, got %+v", stored.Metadata.Attachment)
		}
	})
}

// TestDisplacedWriterReorientsEndToEnd pins the 2sp regression over MCP: after a
// consented takeover, the displaced connection's next write fails typed naming
// who advanced the session and when. The displaced writer itself then reorients
// — its cached binding is stale, so a no-args reorient points it at re-attach,
// and re-attaching with the user's ask takes the session back and continues.
// Divergence surfaces at the write and reorientation succeeds, instead of the
// old silent lockout.
func TestDisplacedWriterReorientsEndToEnd(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	csA := connect(t, env.srv)
	a := openSession(t, csA).Session
	var serveA mcpserver.ServeResult
	call(t, csA, "start_procedure", map[string]any{"session": a, "canonical": "capture", "label": "branch A"}, &serveA)

	// csB takes over the active session with the user's ask.
	csB := connect(t, env.srv)
	var claimed mcpserver.ResumeSessionResult
	call(t, csB, "resume_session", map[string]any{"session": a, "userWords": "take over the branch", "takeover": true}, &claimed)

	// csA's next write learns of the divergence: typed, naming who and when.
	msg := callExpectError(t, csA, "next", map[string]any{"session": a, "instance": serveA.Instance, "report": assembleReport()})
	if !strings.Contains(msg, "taken over") || !strings.Contains(msg, "2026") {
		t.Fatalf("displaced write should name the takeover and when, got %q", msg)
	}

	// The displaced writer's no-args reorient no longer serves its poisoned,
	// stale in-memory session — the store shows it is not the holder, so it is
	// pointed back at the doors to re-establish.
	reorientMsg := callExpectError(t, csA, "resume_session", map[string]any{})
	if !strings.Contains(reorientMsg, "resume_session") || !strings.Contains(reorientMsg, a) {
		t.Fatalf("a displaced writer's no-args reorient should point at re-attach, got %q", reorientMsg)
	}

	// csA reorients for real: re-attach with the user's ask (a is active under
	// csB, so takeover) and continue the move.
	var reA mcpserver.ResumeSessionResult
	call(t, csA, "resume_session", map[string]any{"session": a, "userWords": "come back to branch A", "takeover": true}, &reA)
	if reA.Session != a {
		t.Fatalf("reorientation should re-attach to %s, got %+v", a, reA)
	}
	var capA *mcpserver.ServeResult
	for i := range reA.Open {
		if reA.Open[i].Procedure == "capture" {
			capA = &reA.Open[i]
		}
	}
	if capA == nil {
		t.Fatalf("reorientation should re-serve the capture, got %+v", reA.Open)
	}
	var serve mcpserver.ServeResult
	call(t, csA, "next", map[string]any{"session": a, "instance": capA.Instance, "report": assembleReport()}, &serve)
	if serve.Step != "playback" {
		t.Fatalf("the reoriented writer should advance the move, got %q", serve.Step)
	}
}

// TestEmbeddedCatchupProcedure drives the shipped catch-up base entry over
// MCP with the production viewLayout query: the multi-section lane layout
// parses and renders against the real graph, the briefing report reaches the
// junction, and the user's pick completes the check-in.
func TestEmbeddedCatchupProcedure(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "catch-up"}, &serve)
	if serve.Step != "compose" || serve.Status != "running" {
		t.Fatalf("expected running at compose, got %s at %q", serve.Status, serve.Step)
	}
	// The multi-section layout rendered for real: lane headers present, and
	// the fixture gap surfaces in the open-and-warm lane by its summary.
	for _, want := range []string{"Recent done", "Open and warm", "Pre-flight verdict oscillation"} {
		if !strings.Contains(serve.Instructions, want) {
			t.Fatalf("compose unit should carry %q from the injected lanes, got %q", want, serve.Instructions)
		}
	}

	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"briefing": "**One open gap.**\n\n1. Decide the oscillation gap (`s-tac-aaa`).\n\n**What do you want to move forward?**",
	}}, &serve)
	if serve.Step != "junction" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("briefing should reach the junction user chooser, got %q", serve.Step)
	}

	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": map[string]any{
		"chooser": "junction", "choice": "pursue", "userWords": "the oscillation gap",
		"fields": map[string]any{"selectedThread": "decide the oscillation gap"},
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("pursue should complete the check-in, got %s at %q", serve.Status, serve.Step)
	}
}
