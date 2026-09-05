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

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/serveview"
	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
	mcpserver "github.com/networkteam/sdd/pkg/mcpapp"
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
	// The fixture clock stays after every embedded base fact's authoring
	// stamp — the real world's invariant — so recency-sorted lanes rank
	// test-written entries above the merged base facts.
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(testRuntimeGeneration.Add(1)) * time.Hour)
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "test", DisplayName: "Test"}, DefaultBranch: "main", Language: language,
		Graph: graph, Targets: targets,
		Branches: sdd.BranchValidatorFunc(func(_ context.Context, target sdd.MutationTarget) error {
			if target.Project != "test" {
				return fmt.Errorf("unexpected branch project %q", target.Project)
			}
			if target.Branch == "missing" {
				return fmt.Errorf("branch %q has no registered checkout", target.Branch)
			}
			return nil
		}),
		LLM: pkgllm.RunnerFunc(func(_ context.Context, request pkgllm.Request) (pkgllm.Result, error) {
			identity := pkgllm.Identity{Provider: "test", Model: "test"}
			if request.Purpose == pkgllm.PurposeSummarize {
				return pkgllm.Result{Text: "Test capture entry summary.", Identity: identity}, nil
			}
			items := make([]map[string]string, 0, len(findings))
			for _, finding := range findings {
				items = append(items, map[string]string{"severity": string(finding.Severity), "category": finding.Category, "observation": finding.Observation})
			}
			output, marshalErr := json.Marshal(map[string]any{"findings": items})
			return pkgllm.Result{Text: string(output), Identity: identity}, marshalErr
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: rootAccess{runtime: runtime}, Sessions: sessions, StagedBlobs: blobs, Clock: sdd.ClockFunc(func() time.Time { return now })})
	if err != nil {
		t.Fatal(err)
	}
	opts := mcpserver.Options{SearchSyncMode: sdd.SearchSyncAll,
		Application: application, LocalIdentity: sdd.RequestIdentity{Subject: "tester"}, Version: "test",
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
	ctx := t.Context()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, serverTransport); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
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

func TestShowDepthDefaultsPreserveExplicitZero(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "graph")
	entries := map[string]string{
		"2026/08/20-100000-s-tac-upa.md": `---
type: signal
layer: tactical
kind: gap
summary: Upstream fixture.
---

Upstream fixture.
`,
		"2026/08/20-100100-d-tac-mid.md": `---
type: decision
layer: tactical
kind: activity
summary: Primary fixture.
refs:
    - id: 20260820-100000-s-tac-upa
      kind: addresses
---

Primary fixture.
`,
		"2026/08/20-100200-s-tac-dwn.md": `---
type: signal
layer: tactical
kind: done
summary: Downstream fixture.
refs:
    - id: 20260820-100100-d-tac-mid
      kind: builds-on
---

Downstream fixture.
`,
	}
	for rel, content := range entries {
		path := filepath.Join(graphDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var shown mcpserver.ShowResult
	call(t, cs, "show", map[string]any{"session": session, "ids": []string{"20260820-100100-d-tac-mid"}}, &shown)
	for _, heading := range []string{"# upstream", "# downstream"} {
		if !strings.Contains(shown.Entries, heading) {
			t.Fatalf("omitted depths should apply defaults and render %s:\n%s", heading, shown.Entries)
		}
	}

	call(t, cs, "show", map[string]any{
		"session": session, "ids": []string{"20260820-100100-d-tac-mid"}, "up": 0, "down": 0,
	}, &shown)
	for _, heading := range []string{"# upstream", "# downstream"} {
		if strings.Contains(shown.Entries, heading) {
			t.Fatalf("explicit zero depths should omit %s:\n%s", heading, shown.Entries)
		}
	}
	if !strings.Contains(shown.Entries, "refs:") {
		t.Fatalf("explicit zero depths should retain the primary entry's frontmatter:\n%s", shown.Entries)
	}
}

// TestSearchDefaults_EvidenceCitationHitCapped pins the search defaults: no
// limit means at most 8 hits (d-tac-dbk serve sizes), and no max_citations
// means one citation per hit — the match evidence, without which a true
// positive is unreadable as a match (s-tac-rst). Headers-only stays one
// explicit `max_citations: 0` away.
func TestSearchDefaults_EvidenceCitationHitCapped(t *testing.T) {
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
	session := openSession(t, cs).Session

	var res mcpserver.SearchResult
	call(t, cs, "search", map[string]any{"session": session, "terms": []string{"flux capacitor"}}, &res)
	if hits := strings.Count(res.Results, "-s-tac-h"); hits != 8 {
		t.Fatalf("default search returned %d hits, want the cap of 8", hits)
	}
	if citations := strings.Count(res.Results, "↳"); citations != 8 {
		t.Fatalf("default search must carry one citation per hit, got %d citation lines:\n%s", citations, res.Results)
	}

	call(t, cs, "search", map[string]any{"session": session, "terms": []string{"flux capacitor"}, "max_citations": 0}, &res)
	if strings.Contains(res.Results, "↳") {
		t.Fatalf("explicit max_citations 0 must be header-only, got citations: %q", res.Results)
	}

	call(t, cs, "search", map[string]any{"session": session, "terms": []string{"flux capacitor"}, "max_citations": 2, "limit": 12}, &res)
	if !strings.Contains(res.Results, "↳") {
		t.Fatalf("explicit max_citations should render citation lines, got %q", res.Results)
	}
	if hits := strings.Count(res.Results, "-s-tac-h"); hits != 12 {
		t.Fatalf("explicit limit should raise the cap, got %d hits", hits)
	}
}

// TestVocabularyBlockForNonEnglishGraphs serves the bundled translation
// table exactly once per session when the graph language is non-English —
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
		t.Fatalf("the vocabulary serves once per session, got it again: %q", serve.Vocabulary)
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
		"abandon", "bind_branch", "info", "next", "park", "read_attachment",
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

	// A reorientation carries the now-durable branch in the session info.
	var reoriented mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{"session": door.Session}, &reoriented)
	if reoriented.Session != door.Session || reoriented.Branch != "feature/session" {
		t.Fatalf("reorientation projection = %+v", reoriented)
	}
	if !strings.Contains(reoriented.Framing, "Branch binding: feature/session") {
		t.Fatalf("bound framing = %q", reoriented.Framing)
	}

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &serve)

	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": door.Session}, &resumed)
	if resumed.Branch != "feature/session" {
		t.Fatalf("resume projection after restart = %+v", resumed)
	}
	if !strings.Contains(resumed.Framing, "Branch binding: feature/session") {
		t.Fatalf("resume framing after restart = %q", resumed.Framing)
	}

	var cleared mcpserver.BindBranchResult
	call(t, cs2, "bind_branch", map[string]any{"session": door.Session, "clear": true}, &cleared)
	if cleared.Branch != "" || cleared.Status != "cleared" {
		t.Fatalf("clear result = %+v", cleared)
	}
}

func TestBindBranchToolValidatesArguments(t *testing.T) {
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

	// The handle is the capability: another connection presenting it binds.
	other := connect(t, env.srv)
	var bound mcpserver.BindBranchResult
	call(t, other, "bind_branch", map[string]any{"session": door.Session, "branch": "feature"}, &bound)
	if bound.Branch != "feature" || bound.Status != "bound" {
		t.Fatalf("bind from a second connection = %+v", bound)
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

	assertBranchReads := func(t *testing.T, client *mcp.ClientSession, session string, seesBranch bool) {
		t.Helper()
		var showOutput string
		if seesBranch {
			var shown mcpserver.ShowResult
			call(t, client, "show", map[string]any{"session": session, "ids": []string{branchID}}, &shown)
			showOutput = shown.Entries
		} else {
			showOutput = callExpectError(t, client, "show", map[string]any{"session": session, "ids": []string{branchID}})
		}
		var searched mcpserver.SearchResult
		call(t, client, "search", map[string]any{"session": session, "terms": []string{"nebula routing evidence"}}, &searched)
		var viewed mcpserver.ViewResult
		call(t, client, "view", map[string]any{"session": session, "layout": "active:as-list"}, &viewed)
		for surface, output := range map[string]string{
			"show": showOutput, "search": searched.Results, "view": viewed.Sections,
		} {
			if got := strings.Contains(output, branchText); got != seesBranch {
				t.Fatalf("%s branch visibility = %v, want %v:\n%s", surface, got, seesBranch, output)
			}
		}
	}
	assertBranchReads(t, boundClient, door.Session, true)

	// Another connection presenting the handle reads in the bound branch too.
	assertBranchReads(t, connect(t, env.srv), door.Session, true)

	unboundClient := connect(t, env.srv)
	unboundDoor := openSession(t, unboundClient)
	assertBranchReads(t, unboundClient, unboundDoor.Session, false)
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
		"show":   {"session": door.Session, "ids": []string{fixtureGapID}},
		"search": {"session": door.Session, "terms": []string{"oscillation"}},
		"view":   {"session": door.Session, "layout": "active:as-list"},
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
	const want = "62724c76e938ff3fca52aa189b1ee63f5aeef0d63c40849fd6324acbc05ed5f8"
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
	// Empty the derived index by overriding every indexed embedded fact with an
	// unindexed project-local copy. The set is derived from what the binary
	// ships, so adding an indexed base fact cannot silently break this premise.
	base, err := finders.BaseEntries()
	if err != nil {
		t.Fatal(err)
	}
	const override = `---
type: signal
layer: process
kind: fact
topics: [engine/base-facts]
summary: Project override deliberately leaves this fact out of session discovery.
---

Project-local reference override.
`
	overridden := 0
	for _, e := range base {
		if e.Kind != model.KindFact || e.Index == nil {
			continue
		}
		rel, err := model.IDToRelPath(e.ID)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(graphDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(override), 0644); err != nil {
			t.Fatal(err)
		}
		overridden++
	}
	if overridden == 0 {
		t.Fatal("no indexed embedded facts found — the empty-index premise needs at least one override")
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
	if strings.Contains(serve.Instructions, "20260717-110000-s-prc-vwg") {
		t.Fatalf("opening serve rendered the malformed fact as an indexed pointer:\n%s", serve.Instructions)
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
// TestStagedAttachmentEditAndRead pins the staged scratch's two new surfaces
// (20260826-120330-d-tac-8f8): a staged file edits in place through atomic
// search-replace pairs addressed by handle, reads back in bounded pages, and
// a failing pair refuses the edit naming itself with the file unchanged.
func TestStagedAttachmentEditAndRead(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var staged mcpserver.StageAttachmentResult
	call(t, cs, "stage_attachment", map[string]any{
		"session": session,
		"name":    "record.md",
		"content": "alpha beta gamma",
	}, &staged)

	// Edit in place by handle.
	call(t, cs, "stage_attachment", map[string]any{
		"session": session,
		"name":    "record.md",
		"patches": []map[string]any{{"old": "beta", "new": "BETA"}},
	}, &staged)
	if staged.Handle != "record.md" {
		t.Fatalf("edit should keep the handle, got %q", staged.Handle)
	}

	// Read back in bounded pages.
	var page mcpserver.ReadAttachmentResult
	call(t, cs, "read_attachment", map[string]any{
		"session": session, "handle": "record.md", "max_bytes": 6,
	}, &page)
	if page.Content != "alpha " || !page.More || page.TotalBytes != 16 {
		t.Fatalf("first page = %+v", page)
	}
	call(t, cs, "read_attachment", map[string]any{
		"session": session, "handle": "record.md", "offset": page.NextOffset,
	}, &page)
	if page.Content != "BETA gamma" || page.More {
		t.Fatalf("second page = %+v", page)
	}
	if len(page.Available) != 1 || page.Available[0] != "record.md" {
		t.Fatalf("available should list staged handles, got %v", page.Available)
	}

	// A failing pair refuses the edit by name; the staged bytes stay put.
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "stage_attachment", Arguments: map[string]any{
		"session": session,
		"name":    "record.md",
		"patches": []map[string]any{{"old": "absent", "new": "x"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(contentText(res), "pair 1") {
		t.Fatalf("failing pair should refuse the edit naming itself, got %+v", res)
	}
	call(t, cs, "read_attachment", map[string]any{
		"session": session, "handle": "record.md",
	}, &page)
	if page.Content != "alpha BETA gamma" {
		t.Fatalf("refused edit must leave the staged file unchanged, got %q", page.Content)
	}
}

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
	entryID := serve.Produced["entryId"]
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
	call(t, cs, "show", map[string]any{"session": session, "ids": []string{fixtureGapID}}, &shown)
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
	if entryID := serve.Produced["entryId"]; entryID == "" {
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
	call(t, cs, "show", map[string]any{"session": session, "ids": []string{fixtureGapID}}, &show)
	if len(show.Entries) == 0 {
		t.Fatalf("show after the rejected write returned no entries — the connection is poisoned")
	}

	// resume_session reorients the session — the recovery tool the
	// poisoned-binding error recommends, yet the one the wipe broke: its
	// still-held pre-check called Load("") and died with `invalid session ID ""`.
	// It must re-serve the SAME session, proving the binding is fully intact, not
	// merely non-empty.
	var resumed mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{"session": session}, &resumed)
	if resumed.Session != session {
		t.Fatalf("resume_session after the rejected write should re-serve %s, got %q", session, resumed.Session)
	}
}

// TestSessionLabelCarriedAndValidated covers the label on the loop tools: a
// label set on next rides the resume projection, and multi-line or oversized
// labels are rejected.
func TestSessionLabelCarriedAndValidated(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &serve)
	call(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": assembleReport()}, &serve)

	var resumed mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{"session": session}, &resumed)
	if resumed.Label != "" {
		t.Fatalf("no label supplied — the projection carries none, got %q", resumed.Label)
	}

	call(t, cs, "next", map[string]any{
		"session":  session,
		"instance": serve.Instance,
		"report": map[string]any{"chooser": "playback", "choice": "adjust", "userWords": "keep going",
			"fields": map[string]any{"confidence": "medium"}},
		"label": "Oscillation gap capture",
	}, &serve)
	call(t, cs, "resume_session", map[string]any{"session": session}, &resumed)
	if resumed.Label != "Oscillation gap capture" {
		t.Fatalf("the label set on next must ride the resume projection, got %q", resumed.Label)
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

// TestSessionResumeAcrossServers drives a capture to the playback chooser,
// then re-hosts the sessions dir on a fresh server (the restart): the session
// resumes by log replay from a connection that never opened it, serves the
// complete position in full, and completes.
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

	// Restart: same graph and sessions dirs, fresh server and connection.
	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)

	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": sessionID}, &resumed)
	if resumed.Label != "Capture: oscillation gap in integration tests" {
		t.Fatalf("resume briefing should carry the session label, got %q", resumed.Label)
	}
	if resumed.Participant != "Tester" {
		t.Fatalf("resume briefing should carry the participant, got %q", resumed.Participant)
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
	if rehydrated.Collected["anchor"] != fixtureGapID {
		t.Fatalf("resume should project the collected anchor, got %+v", rehydrated.Collected)
	}
	// A reorientation serves the complete position in full: the playback unit
	// with the report evidence that persisted through the log replay, and the
	// framing.
	if !strings.Contains(rehydrated.Instructions, "Play back to the user") {
		t.Fatalf("resume should serve the full unit text, got %q", rehydrated.Instructions)
	}
	if resumed.Framing == "" {
		t.Fatal("resume should serve the framing in full")
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

	// The door always opens a fresh dialogue: a new handle, framing in full.
	fresh := openSession(t, cs2)
	if fresh.Session == sessionID || fresh.Framing == "" {
		t.Fatalf("start_session should open a new session with its framing, got %s (framing %d bytes)", fresh.Session, len(fresh.Framing))
	}
}

// TestResumeProjectsCollectedState covers the honest-surfacing half of the
// re-entry contract (d-cpt-0tm): a resuming agent sees the param and state
// values an instance already collected — even when the current step's unit
// does not render them — while internal trust machinery stays hidden and an
// oversized value is truncated with an explicit notice rather than dropped.
func TestResumeProjectsCollectedState(t *testing.T) {
	// The production per-value cap derives from the serveview budget.
	collectedCap := serveview.Default().Cap(serveview.PartStoreValue).MaxBytes

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

	// A fresh server + connection resumes from the persisted log alone.
	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)

	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": sessionID}, &resumed)

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
	// The production caps derive from the serveview budget.
	valueCap := serveview.Default().Cap(serveview.PartStoreValue).MaxBytes
	instanceCap := serveview.Default().Cap(serveview.PartProduced).MaxBytes

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

	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": sessionID}, &resumed)

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

// TestResumeOmitsWholeInstancesPastTheBudget pins the resume projection's only
// legitimate cut (d-tac-qwc): open instances past the aggregate budget are
// omitted whole — never partially served — and each omission names handle,
// procedure, and step so the pointer to next stays free.
func TestResumeOmitsWholeInstancesPastTheBudget(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var last mcpserver.ServeResult
	for range 10 {
		call(t, cs, "start_procedure", map[string]any{
			"session":   session,
			"canonical": "capture",
			"params":    map[string]any{"anchor": fixtureGapID},
		}, &last)
	}
	sessionID := last.Session

	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)

	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": sessionID}, &resumed)

	// Eleven open instances (shell + ten captures) cannot all fit: the count
	// cap alone guarantees omission.
	if len(resumed.Open) > 8 {
		t.Fatalf("resume projected %d instances, want at most the count cap", len(resumed.Open))
	}
	if len(resumed.Open) == 0 {
		t.Fatal("whole-instance omission must never empty the projection")
	}
	if !strings.Contains(resumed.Instructions, "omitted here for size") {
		t.Fatalf("omission must be named, got %q", resumed.Instructions)
	}
	if !strings.Contains(resumed.Instructions, "capture at ") {
		t.Fatalf("omitted instances must be named by handle, procedure, and step, got %q", resumed.Instructions)
	}
	// Every served instance is whole: its schema is either full or an explicit
	// served-earlier stub, never a truncated fragment.
	for _, open := range resumed.Open {
		if open.Instance == "" || open.Procedure == "" {
			t.Fatalf("served instance must be whole, got %+v", open)
		}
	}
}

// TestAbandonLeavesLogStanding abandons a running instance and checks the
// move leaves the open threads without any implicit cleanup of the log.
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
	if abandoned.Base.OpenThreads != "" {
		t.Fatalf("the abandoned move must not list as an open thread, got %q", abandoned.Base.OpenThreads)
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

	// Restart: the parked draft survives — the session resumes with the move
	// at its step and its seeded state intact.
	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": serve.Session}, &resumed)
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
// one call — no resume, no framing (d-tac-dbk; baseline was six calls +
// ~28KB framing per session). The response names the label and the discarded
// threads, and the session no longer resumes.
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

	// One call, no framing anywhere in the response.
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
		t.Fatalf("a teardown has no dialogue to land, got %+v", down.Base)
	}

	if msg := callExpectError(t, cs2, "resume_session", map[string]any{"session": sessionID}); !strings.Contains(msg, "torn down") {
		t.Fatalf("a torn-down session must not resume, got %q", msg)
	}
	if _, err := os.Stat(filepath.Join(env.sessionsDir, sessionID+".jsonl")); err != nil {
		t.Fatalf("teardown closes the log, never deletes it: %v", err)
	}
}

// TestAbandonSessionByHandle_Cached tears down a session this server holds
// replayed, from a connection that never drove it: the handle is the whole
// authorization, and the connection that opened it finds it ended.
func TestAbandonSessionByHandle_Cached(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "live draft"}, &serve)

	var down mcpserver.AbandonResult
	call(t, connect(t, env.srv), "abandon", map[string]any{"session": session, "reason": "not needed"}, &down)
	if !down.Abandoned || down.Session != session || down.Label != "live draft" {
		t.Fatalf("teardown diverged: %+v", down)
	}
	if len(down.DiscardedThreads) != 1 || !strings.Contains(down.DiscardedThreads[0], "capture at") {
		t.Fatalf("teardown should name the discarded threads, got %v", down.DiscardedThreads)
	}
	if down.Base != nil {
		t.Fatalf("a teardown has no dialogue to land, got %+v", down.Base)
	}

	msg := callExpectError(t, cs, "next", map[string]any{"session": session, "instance": serve.Instance, "report": assembleReport()})
	if !strings.Contains(msg, "torn down") || !strings.Contains(msg, "start_session") {
		t.Fatalf("a move against the torn-down session should refuse naming the way on, got %q", msg)
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
		ID: sdd.SessionID(sessionID), Subject: "tester", Project: "test", Participant: "Tester",
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
	// Instance mode requires the session the move belongs to; an unknown handle
	// is named as such.
	if msg := callExpectError(t, cs, "abandon", map[string]any{"instance": "i_1", "session": "s_x"}); !strings.Contains(msg, "unknown session") {
		t.Fatalf("a move abandon naming an unknown session should name it, got %q", msg)
	}
	// Instance set but session omitted is a work-tool call missing its handle:
	// it names both doors, not the pass-one-of message.
	if msg := callExpectError(t, cs, "abandon", map[string]any{"instance": "i_1"}); !strings.Contains(msg, "start_session") || !strings.Contains(msg, "resume_session") {
		t.Fatalf("a move abandon without a session handle should name both doors, got %q", msg)
	}
	if msg := callExpectError(t, cs, "abandon", map[string]any{"session": "s_nope"}); !strings.Contains(msg, "unknown session") {
		t.Fatalf("unknown session should be named, got %q", msg)
	}
	if msg := callExpectError(t, cs, "abandon", map[string]any{"session": serve.Session, "instance": "i_99"}); !strings.Contains(msg, "not found") {
		t.Fatalf("an unknown instance should be named, got %q", msg)
	}
}

// TestNoHandleRejectionNamesDoorsOnly pins the no-handle rejection: a tool
// called without a session handle names both doors and lists no session —
// handles are issued, never published (d-cpt-aen).
func TestNoHandleRejectionNamesDoorsOnly(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "the open one"}, &serve)

	requireNoListing := func(t *testing.T, tool, msg string) {
		t.Helper()
		if strings.Contains(msg, session) || strings.Contains(msg, "the open one") {
			t.Fatalf("%s without a handle must list no session, got %q", tool, msg)
		}
	}
	for _, cs := range []*mcp.ClientSession{cs, connect(t, env.srv)} {
		msg := callExpectError(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{"x": "y"}})
		if !strings.Contains(msg, "start_session") || !strings.Contains(msg, "resume_session") {
			t.Fatalf("next without a handle should name both doors, got %q", msg)
		}
		requireNoListing(t, "next", msg)

		// resume_session's handle is required by schema: the rejection names the
		// missing argument, never a session to put there.
		msg = callExpectError(t, cs, "resume_session", map[string]any{})
		if !strings.Contains(msg, "session") {
			t.Fatalf("resume_session without a handle should name the missing argument, got %q", msg)
		}
		requireNoListing(t, "resume_session", msg)
	}
}

// TestWorkToolUnknownSessionNamed: every work tool naming a session the store
// does not hold fails naming it — never the no-handle door error.
func TestWorkToolUnknownSessionNamed(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &serve)

	const unknown = "s_not-issued"
	cases := map[string]map[string]any{
		"start_procedure":  {"session": unknown, "canonical": "capture"},
		"next":             {"session": unknown, "instance": serve.Instance, "report": assembleReport()},
		"park":             {"session": unknown, "instance": serve.Instance},
		"bind_branch":      {"session": unknown, "branch": "feature"},
		"stage_attachment": {"session": unknown, "name": "a.md", "content": "x"},
		"abandon":          {"session": unknown, "instance": serve.Instance},
		"resume_session":   {"session": unknown},
		"show":             {"session": unknown, "ids": []string{fixtureGapID}},
	}
	for tool, args := range cases {
		msg := callExpectError(t, cs, tool, args)
		if !strings.Contains(msg, "unknown session") || !strings.Contains(msg, unknown) {
			t.Fatalf("%s naming an unknown session should name it, got %q", tool, msg)
		}
	}
}

// TestNamedResumeOnFreshConnection: resume_session by handle serves the
// session's position on a fresh connection to a fresh server — presenting the
// handle is the whole authorization (d-cpt-aen), and the same handle then
// advances the move.
func TestNamedResumeOnFreshConnection(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"session": session, "canonical": "capture", "label": "resume me elsewhere",
	}, &serve)
	handle := serve.Session

	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": handle}, &resumed)
	if resumed.Session != handle {
		t.Fatalf("named resume should serve %s, got %s", handle, resumed.Session)
	}
	var capServe *mcpserver.ServeResult
	for i := range resumed.Open {
		if resumed.Open[i].Procedure == "capture" {
			capServe = &resumed.Open[i]
		}
	}
	if capServe == nil || capServe.Step != serve.Step {
		t.Fatalf("resume should serve the open move at its step, got %+v", resumed.Open)
	}
	if len(capServe.ReportSchema) == 0 {
		t.Fatalf("resume should serve the running move's report schema, got %+v", capServe)
	}

	call(t, cs2, "next", map[string]any{"session": handle, "instance": capServe.Instance, "report": assembleReport()}, &serve)
	if serve.Step != "playback" {
		t.Fatalf("the handle should advance the move from the new connection, got %q", serve.Step)
	}
}

// TestOneConnectionDrivesTwoSessions: nothing ties a connection to one
// dialogue — two start_session calls mint two handles, and the connection
// drives both by handle with each keeping its own moves.
func TestOneConnectionDrivesTwoSessions(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	a := openSession(t, cs)
	b := openSession(t, cs)
	if a.Session == b.Session {
		t.Fatalf("two start_session calls must mint two handles, both %s", a.Session)
	}
	if b.Framing == "" {
		t.Fatal("a new session serves its framing in full")
	}

	var moveA, moveB mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": a.Session, "canonical": "capture", "label": "A"}, &moveA)
	call(t, cs, "start_procedure", map[string]any{"session": b.Session, "canonical": "catch-up", "label": "B"}, &moveB)

	for _, want := range []struct {
		handle    string
		procedure string
		instance  string
	}{{a.Session, "capture", moveA.Instance}, {b.Session, "catch-up", moveB.Instance}} {
		var resumed mcpserver.ResumeSessionResult
		call(t, cs, "resume_session", map[string]any{"session": want.handle}, &resumed)
		if resumed.Session != want.handle {
			t.Fatalf("resume should serve %s, got %s", want.handle, resumed.Session)
		}
		var procedures []string
		for _, open := range resumed.Open {
			if open.Procedure != "user-dialogue" {
				procedures = append(procedures, open.Procedure)
				if open.Instance != want.instance {
					t.Fatalf("%s should carry its own move %s, got %s", want.handle, want.instance, open.Instance)
				}
			}
		}
		if len(procedures) != 1 || procedures[0] != want.procedure {
			t.Fatalf("%s should carry only its own %s move, got %v", want.handle, want.procedure, procedures)
		}
	}
}

// TestResumeServesPositionInFull pins the reorientation (d-cpt-aen):
// resume_session with the handle re-serves every running move at its current
// step with the schema to continue it and the framing, in full, however often
// it is called.
func TestResumeServesPositionInFull(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

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

	var prevSize int
	for i := range 3 {
		var resumed mcpserver.ResumeSessionResult
		call(t, cs, "resume_session", map[string]any{"session": session}, &resumed)
		if resumed.Session != serve.Session {
			t.Fatalf("resume %d: should re-serve %s, got %s", i, serve.Session, resumed.Session)
		}
		if resumed.Framing == "" {
			t.Fatalf("resume %d: a reorientation serves the framing in full", i)
		}
		var capServe, shellServe *mcpserver.ServeResult
		for j := range resumed.Open {
			switch resumed.Open[j].Procedure {
			case "capture":
				capServe = &resumed.Open[j]
			case "user-dialogue":
				shellServe = &resumed.Open[j]
			}
		}
		if shellServe == nil || !strings.Contains(shellServe.Instructions, "Standing goal") {
			t.Fatalf("resume %d: the shell should re-serve its orientation in full, got %+v", i, shellServe)
		}
		if capServe == nil || capServe.Instance != serve.Instance || capServe.Step != serve.Step {
			t.Fatalf("resume %d: the in-flight move should re-serve at its step, got %+v", i, resumed.Open)
		}
		requireFullReportSchema(t, "resumed capture", capServe.ReportSchema)
		size := jsonSize(t, resumed)
		if i > 0 && size != prevSize {
			t.Fatalf("resume %d: a reorientation is the same complete position every time, got %d bytes after %d", i, size, prevSize)
		}
		prevSize = size
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

// TestServedMemoryIsPerSession pins the served-once memory's home (d-cpt-aen):
// it is derived from the session ledger, so a second connection presenting the
// handle is stubbed for what the session's consumer already holds, a
// reorientation resets it so the complete position serves in full, and
// afterwards the memory accumulates again.
func TestServedMemoryIsPerSession(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	csA := connect(t, env.srv)
	door := openSession(t, csA)
	if door.Framing == "" {
		t.Fatal("precondition: the door serves the framing in full")
	}
	var first mcpserver.ServeResult
	call(t, csA, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &first)
	requireFullReportSchema(t, "first capture", first.ReportSchema)
	if !strings.Contains(first.Instructions, "Existing topics:") {
		t.Fatalf("the first capture serves its unit in full, got %q", first.Instructions)
	}

	// Connection B, same server: what A received is stubbed for B.
	csB := connect(t, env.srv)
	var second mcpserver.ServeResult
	call(t, csB, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &second)
	if second.Framing != "" {
		t.Fatalf("the framing was served to this session already, got it again on another connection: %q", second.Framing)
	}
	requireStubReportSchema(t, "second capture from another connection", second.ReportSchema)
	if !strings.Contains(second.Instructions, "served earlier this session") {
		t.Fatalf("the identical unit should stub, got %q", second.Instructions)
	}

	// B reorients: everything serves in full again — the two identical capture
	// positions dedup against each other within the one call, so the first
	// carries the full schema and unit.
	var resumed mcpserver.ResumeSessionResult
	call(t, csB, "resume_session", map[string]any{"session": door.Session}, &resumed)
	if resumed.Framing == "" {
		t.Fatal("a reorientation serves the framing in full")
	}
	resumedCapture := openServe(t, resumed, "capture")
	requireFullReportSchema(t, "resumed capture", resumedCapture.ReportSchema)
	if !strings.Contains(resumedCapture.Instructions, "Existing topics:") {
		t.Fatalf("a reorientation serves the unit in full, got %q", resumedCapture.Instructions)
	}

	// Then the memory accumulates again, for either connection.
	var third mcpserver.ServeResult
	call(t, csA, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &third)
	if third.Framing != "" {
		t.Fatalf("the framing re-served on resume should stub again, got %q", third.Framing)
	}
	requireStubReportSchema(t, "capture after resume", third.ReportSchema)
}

// TestServedMemorySurvivesServerRestart: the ledger carries the served-once
// memory, so a second server over the same store stubs what an earlier one
// served, without any reorientation in between.
func TestServedMemorySurvivesServerRestart(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)
	var first mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &first)
	requireFullReportSchema(t, "first capture", first.ReportSchema)

	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	var second mcpserver.ServeResult
	call(t, cs2, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &second)
	if second.Framing != "" {
		t.Fatalf("framing served before the restart must stub after it, got %q", second.Framing)
	}
	requireStubReportSchema(t, "capture after restart", second.ReportSchema)
	if !strings.Contains(second.Instructions, "served earlier this session") {
		t.Fatalf("the unit served before the restart must stub after it, got %q", second.Instructions)
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

// TestDoorAndReplayWirePayloads measures the two heaviest automatic MCP
// payloads as JSON wire bytes over the realistic fixture — the door serve,
// and a resume_session reorientation with a running move (dedup reset,
// everything re-served at once). It replaces TestDoorPayloadUnder25KB, whose
// fixture populated none of the scaling lanes and so was structurally blind
// to the breach it existed to catch (s-tac-ayj). The ceilings document the
// measured state and may only shrink as the bounding slices of d-tac-rzi
// land; the door's 25KB contract is currently breached and stays visibly
// named here rather than hidden behind a green fixture.
func TestDoorAndReplayWirePayloads(t *testing.T) {
	const (
		doorWireCeilingBytes   = 40000 // measured ~36KB; contract target is 25000
		replayWireCeilingBytes = 60000 // measured ~53KB with one running catch-up move
	)
	graphDir := filepath.Join(t.TempDir(), "graph")
	// A fixed base just before the test server's fixed clock keeps the
	// fixture inside the recency windows, deterministically.
	base := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	shape := proctest.DefaultShape()
	for _, entry := range proctest.RealisticGraph(base, shape) {
		proctest.WriteEntry(t, graphDir, entry)
	}
	proctest.WriteWIPMarkers(t, graphDir, base, shape)

	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)
	doorSize := jsonSize(t, door)
	t.Logf("door wire payload: %d bytes (contract target 25000)", doorSize)
	if doorSize > doorWireCeilingBytes {
		t.Errorf("door wire payload is %d bytes, over the %d ceiling", doorSize, doorWireCeilingBytes)
	}

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": door.Session, "canonical": "catch-up", "label": "wire measurement"}, &serve)
	var full mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{"session": door.Session}, &full)
	replaySize := jsonSize(t, full)
	t.Logf("resume_session wire payload with one running move: %d bytes", replaySize)
	if replaySize > replayWireCeilingBytes {
		t.Errorf("resume_session wire payload is %d bytes, over the %d ceiling", replaySize, replayWireCeilingBytes)
	}
}

// TestToolResultsCarryTheSpecEnvelope pins the wire shape: every tool result
// carries structuredContent (the machine channel, required with the declared
// output schemas) plus the spec-recommended content text mirror — the channel
// content-projecting hosts (Codex among them, verified live) feed the model.
// Model-context doubling was never observed on a verified host's default
// projection; a lean shape returns as an opt-in only when a both-projecting
// client shows up in real usage.
func TestToolResultsCarryTheSpecEnvelope(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "info", Arguments: map[string]any{"session": session}})
	if err != nil {
		t.Fatal(err)
	}
	if res.StructuredContent == nil {
		t.Fatal("info result must carry structuredContent")
	}
	if len(res.Content) == 0 {
		t.Fatal("info result must carry the content text mirror")
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); !ok || !strings.Contains(tc.Text, "participant") {
		t.Errorf("content must mirror the payload, got %+v", res.Content[0])
	}

	errRes, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "next", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if !errRes.IsError || len(errRes.Content) == 0 {
		t.Errorf("an error result keeps its content text, got %+v", errRes)
	}
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
	id := serve.Produced["entryId"]
	return id
}

// TestFramingLaneDedupAfterWrite pins per-lane framing dedup (I6, AC8): after a
// graph write changes only the recent-moves lane, the next serve re-serves THAT
// lane alone — the stable info block stays stubbed — so the re-serve stays
// under the original. Under whole-block hashing the entire framing would
// re-serve on any write, the reproduced regression.
func TestFramingLaneDedupAfterWrite(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	csB := connect(t, env.srv)
	door := openSession(t, csB)
	if !strings.Contains(door.Framing, "Local participant:") || !strings.Contains(door.Framing, "Recent graph movement") {
		t.Fatalf("the door should serve the full framing, got %q", door.Framing)
	}
	var unchanged mcpserver.ServeResult
	call(t, csB, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &unchanged)
	if unchanged.Framing != "" {
		t.Fatalf("with nothing changed, a later serve carries no framing, got %q", unchanged.Framing)
	}

	// Another session writes an entry — the recent-moves lane now changes for B.
	csA := connect(t, env.srv)
	sessionA := openSession(t, csA).Session
	if id := runCaptureToCompletion(t, csA, sessionA, "churn write"); id == "" {
		t.Fatal("precondition: the capture should produce an entry")
	}

	var afterWrite mcpserver.ServeResult
	call(t, csB, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &afterWrite)
	if !strings.Contains(afterWrite.Framing, "Recent graph movement") {
		t.Fatalf("the changed recent-moves lane must re-serve, got %q", afterWrite.Framing)
	}
	if strings.Contains(afterWrite.Framing, "Local participant:") {
		t.Fatalf("the unchanged info block must stay stubbed (per-lane dedup), got %q", afterWrite.Framing)
	}
	if len(afterWrite.Framing) >= len(door.Framing) {
		t.Fatalf("a post-write re-serve (%d) must stay under the original serve (%d)", len(afterWrite.Framing), len(door.Framing))
	}
}

// TestFramingSectionsDedupIndependently pins the declaration granularity the
// per-lane dedup depends on: the shell declares its overview sections one lane
// each, so a write that changes only the participants section re-serves that
// section alone while the stable aspirations, guiding directives, and focus
// sections stay stubbed. Joined into a single lane all four re-serve on any one
// change, the reproduced regression.
func TestFramingSectionsDedupIndependently(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "graph")
	writeFramingFixture := func(name, frontmatter string) {
		t.Helper()
		path := filepath.Join(graphDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(frontmatter), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeFramingFixture("2026/06/01-100000-d-stg-asp.md", `---
type: decision
layer: strategic
kind: aspiration
confidence: high
summary: An aspiration the framing's aspirations lane ranks, stable across the write this test makes.
---

Body.
`)
	writeFramingFixture("2026/06/01-100100-d-cpt-gui.md", `---
type: decision
layer: conceptual
kind: directive
intent: guiding
confidence: high
summary: A guiding directive the framing's directives lane ranks, stable across the write this test makes.
---

Body.
`)
	writeFramingFixture("2026/06/01-100200-d-tac-foc.md", `---
type: decision
layer: tactical
kind: focus
confidence: high
summary: A focus the framing's focus lane renders, stable across the write this test makes.
---

Body.
`)
	writeFramingFixture("2026/06/01-100300-s-prc-ada.md", `---
type: signal
layer: process
kind: actor
confidence: high
canonical: Ada
summary: An actor the framing's participants lane groups.
---

Body.
`)

	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)
	stable := []string{"Aspirations", "Guiding directives", "## Focus", "## Participants"}
	for _, want := range stable {
		if !strings.Contains(door.Framing, want) {
			t.Fatalf("precondition: the door must serve the %q section, got %q", want, door.Framing)
		}
	}
	var unchanged mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &unchanged)
	if unchanged.Framing != "" {
		t.Fatalf("with nothing changed, a later serve carries no framing, got %q", unchanged.Framing)
	}

	// A second actor changes the participants section and nothing else.
	writeFramingFixture("2026/06/02-100000-s-prc-gra.md", `---
type: signal
layer: process
kind: actor
confidence: high
canonical: Grace
summary: A second actor, written after the door served, so only the participants section changes.
---

Body.
`)

	var afterWrite mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &afterWrite)
	if !strings.Contains(afterWrite.Framing, "Grace") {
		t.Fatalf("the changed participants section must re-serve, got %q", afterWrite.Framing)
	}
	for _, unchanged := range []string{"Aspirations", "Guiding directives", "## Focus"} {
		if strings.Contains(afterWrite.Framing, unchanged) {
			t.Errorf("the unchanged %q section must stay stubbed — its lane is joined with participants, got %q", unchanged, afterWrite.Framing)
		}
	}
}

// TestStubsCarryCompactionBreadcrumb pins the B4 self-trigger: every stub of a
// block the session's consumer already holds — the per-step reminder, the
// report-schema stub, the resume instructions — names the resume_session
// escape with the handle. A bare "served earlier, follow them" is useless to an
// amnesiac.
func TestStubsCarryCompactionBreadcrumb(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "breadcrumb"}, &serve)

	const breadcrumb = "resume_session with this session's handle"
	var second mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &second)
	if !strings.Contains(second.Instructions, "served earlier this session") || !strings.Contains(second.Instructions, breadcrumb) {
		t.Fatalf("a stubbed per-step reminder must carry the compaction breadcrumb, got %q", second.Instructions)
	}
	if stub, _ := second.ReportSchema["served_earlier"].(string); !strings.Contains(stub, breadcrumb) {
		t.Fatalf("a report-schema stub must carry the compaction breadcrumb, got %q", stub)
	}

	var resumed mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{"session": session}, &resumed)
	if !strings.Contains(resumed.Instructions, "resume_session with this handle") {
		t.Fatalf("resume instructions should point a compaction victim at resume_session, got %q", resumed.Instructions)
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
	session := openSession(t, cs).Session

	var page1 mcpserver.ReadAttachmentResult
	call(t, cs, "read_attachment", map[string]any{"session": session, "id": fixtureGapID, "max_bytes": 6}, &page1)
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
		"session": session, "id": fixtureGapID, "name": "notes.md", "offset": page1.NextOffset, "max_bytes": 6,
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
	session := openSession(t, cs).Session

	var res mcpserver.ReadAttachmentResult
	call(t, cs, "read_attachment", map[string]any{"session": session, "id": fixtureGapID}, &res)
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

// TestFreeReads smoke-tests the read tools: free of any move or procedure
// state, yet carrying the session handle like every other tool, since the
// session is what names the project and branch a read runs in (d-tac-1z6).
func TestFreeReads(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	// Without a session there is nothing to read in: the rejection names the doors.
	for name, args := range map[string]map[string]any{
		"info":            {},
		"view":            {"layout": "active:as-counts"},
		"show":            {"ids": []string{fixtureGapID}},
		"search":          {"terms": []string{"oscillation"}},
		"read_attachment": {"id": fixtureGapID},
		"registry":        {"class": "command"},
	} {
		msg := callExpectError(t, cs, name, args)
		if !strings.Contains(msg, "no session handle") || !strings.Contains(msg, "start_session") {
			t.Fatalf("%s without a session must be refused naming the door, got %q", name, msg)
		}
	}

	session := openSession(t, cs).Session

	var info mcpserver.InfoResult
	call(t, cs, "info", map[string]any{"session": session}, &info)
	if info.Participant != "Tester" || info.Search != "text" || info.Project != "test" {
		t.Fatalf("info diverged: %+v", info)
	}

	var view mcpserver.ViewResult
	call(t, cs, "view", map[string]any{"session": session, "layout": "active:as-counts"}, &view)
	if !strings.Contains(view.Sections, "testing/fixture") {
		t.Fatalf("view should list the fixture topic, got %q", view.Sections)
	}

	var show mcpserver.ShowResult
	call(t, cs, "show", map[string]any{"session": session, "ids": []string{fixtureGapID}}, &show)
	if !strings.Contains(show.Entries, fixtureGapID) {
		t.Fatalf("show should render the entry, got %q", show.Entries)
	}

	var search mcpserver.SearchResult
	call(t, cs, "search", map[string]any{"session": session, "terms": []string{"oscillation"}}, &search)
	if !strings.Contains(search.Results, fixtureGapID) {
		t.Fatalf("search should find the fixture gap, got %q", search.Results)
	}

	// The bare session ID names the attached session too — a handle carried
	// from before the project prefix existed keeps working.
	call(t, cs, "show", map[string]any{"session": session, "ids": []string{fixtureGapID}}, &show)
	if !strings.Contains(show.Entries, fixtureGapID) {
		t.Fatalf("show with the bare session ID should render the entry, got %q", show.Entries)
	}

	var reg mcpserver.RegistryResult
	call(t, cs, "registry", map[string]any{"session": session, "class": "command"}, &reg)
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

	// A named resume of an unknown handle fails on its own terms — never the
	// no-handle door error.
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

	// Knocking again opens a fresh dialogue under a new handle, orientation in
	// full; the first session is untouched.
	other := openSession(t, cs)
	if other.Session == session || !strings.Contains(other.Instructions, "Standing goal") || other.Framing == "" {
		t.Fatalf("a second start_session should open a new session served in full, got %s: %q", other.Session, other.Instructions)
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

// TestShellConcludeLeavesOpenThreadsBehind pins the un-gated conclude end to end
// (d-tac-k4q): a session with an open move ends on the user's one answer, the
// response names that thread specifically as left behind, the session stops being
// open work — so nothing offers a thread no served path reaches — and its log
// stays readable for inspection.
func TestShellConcludeLeavesOpenThreadsBehind(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	shell := openSession(t, cs)
	session := shell.Session

	var capture mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "half-done capture"}, &capture)

	var serve mcpserver.ServeResult
	call(t, cs, "next", map[string]any{"session": session, "instance": shell.Instance, "report": map[string]any{
		"chooser": "junction", "choice": "conclude", "userWords": "wrap it up, leave the rest",
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("conclude with an open move should end the session, got %s at %q", serve.Status, serve.Step)
	}

	// The listing stays per-thread and specific — never a count — and is framed as
	// what is being dropped rather than as continuations on offer.
	for _, want := range []string{capture.Instance, "capture at ", "leaves behind"} {
		if !strings.Contains(serve.OpenThreads, want) {
			t.Fatalf("the conclude response should name the abandoned thread specifically; %q missing %q", serve.OpenThreads, want)
		}
	}
	if !strings.Contains(serve.Instructions, "start_session") {
		t.Fatalf("the conclude response should state the way on, got %q", serve.Instructions)
	}

	// The terminal record is durable, so collection can reclaim it after retention.
	stored, err := env.sessions.Load(t.Context(), sdd.SessionID(session))
	if err != nil {
		t.Fatal(err)
	}
	if stored.Metadata.Ended == nil || stored.Metadata.Ended.Act != sdd.SessionConcluded {
		t.Fatalf("conclude should record a terminal conclude, got %+v", stored.Metadata.Ended)
	}

	// Ended-ness is the authority: the dropped thread is still a running instance
	// in the log, yet the session is no longer reachable as open work.
	if msg := callExpectError(t, cs, "resume_session", map[string]any{"session": session}); !strings.Contains(msg, "concluded") || !strings.Contains(msg, "start_session") {
		t.Fatalf("a concluded session must not resume, and the refusal names the way on, got %q", msg)
	}
	msg := callExpectError(t, cs, "start_procedure", map[string]any{"canonical": "capture"})
	if strings.Contains(msg, session) {
		t.Fatalf("the no-handle rejection must not offer a concluded session, got %q", msg)
	}

	// The log itself stays readable for inspection until retention expires.
	events, err := readSessionLog(t, env.sessionsDir, session)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("a concluded session's log should stay readable")
	}
}

// TestDoorAfterConcludeOpensFresh is the s-tac-3be regression over MCP: the door
// called on a connection whose session just concluded mints a NEW handle instead
// of re-serving the spent one, a move against the concluded handle is refused
// naming that path, and the fix stays bounded by d-cpt-0tm — the new session,
// still running, resumes by handle at its pending step with the schema to
// continue it.
func TestDoorAfterConcludeOpensFresh(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	shell := openSession(t, cs)
	spent := shell.Session

	var serve mcpserver.ServeResult
	call(t, cs, "next", map[string]any{"session": spent, "instance": shell.Instance, "report": map[string]any{
		"chooser": "junction", "choice": "conclude", "userWords": "that's it for today",
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("conclude should complete the shell, got %s at %q", serve.Status, serve.Step)
	}
	for _, want := range []string{"finished", "start_session", "new handle"} {
		if !strings.Contains(serve.Instructions, want) {
			t.Fatalf("the conclude response %q is missing %q (finished / the way on)", serve.Instructions, want)
		}
	}

	// A move against the concluded handle is refused with that same way on,
	// rather than silently reviving the completed session.
	msg := callExpectError(t, cs, "start_procedure", map[string]any{"session": spent, "canonical": "capture"})
	if !strings.Contains(msg, "start_session") || !strings.Contains(msg, "concluded") {
		t.Fatalf("a move against a concluded session should refuse naming the new-session path, got %q", msg)
	}

	// The door opens fresh: a new handle, a running shell at its junction.
	fresh := openSession(t, cs)
	if fresh.Session == spent {
		t.Fatalf("the door re-served the concluded session %s — it must mint a new handle", spent)
	}
	if fresh.Status != "running" || fresh.Step != "junction" {
		t.Fatalf("the fresh door serve should be a running junction, got %s at %q", fresh.Status, fresh.Step)
	}

	// d-cpt-0tm's bound: door-opens-fresh never widens into re-initializing a
	// running session. The new session's move resumes by handle where it stands.
	var move mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": fresh.Session, "canonical": "capture"}, &move)
	call(t, cs, "next", map[string]any{"session": fresh.Session, "instance": move.Instance, "report": assembleReport()}, &move)
	if move.Step != "playback" {
		t.Fatalf("the move should stand at playback before the resume, got %q", move.Step)
	}
	var resumed mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{"session": fresh.Session}, &resumed)
	var projected *mcpserver.ServeResult
	for i := range resumed.Open {
		if resumed.Open[i].Instance == move.Instance {
			projected = &resumed.Open[i]
		}
	}
	if projected == nil {
		t.Fatalf("resuming a running session must project its open move, got %+v", resumed.Open)
	}
	if projected.Step != "playback" || projected.Status != "running" || len(projected.ReportSchema) == 0 {
		t.Fatalf("the projected move should come back running at playback with a report schema, got %s at %q (schema %v)", projected.Status, projected.Step, projected.ReportSchema)
	}
}

// TestConnectionEventsEndNothing pins d-cpt-rw7 and d-cpt-aen at the real call
// sites: a dialogue ends only by a participant act. Shutting the server down or
// dropping a connection ends nothing — the session resumes by handle from a new
// connection with its shell still running — while a teardown by handle records
// the terminal abandon.
func TestConnectionEventsEndNothing(t *testing.T) {
	end := func(t *testing.T, env testEnv, session string) *sdd.SessionEnd {
		t.Helper()
		stored, err := env.sessions.Load(t.Context(), sdd.SessionID(session))
		if err != nil {
			t.Fatal(err)
		}
		return stored.Metadata.Ended
	}
	startMove := func(t *testing.T, cs *mcp.ClientSession, session string) mcpserver.ServeResult {
		t.Helper()
		var serve mcpserver.ServeResult
		call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture", "label": "work"}, &serve)
		return serve
	}
	requireRunning := func(t *testing.T, cs *mcp.ClientSession, session string, move mcpserver.ServeResult) {
		t.Helper()
		var resumed mcpserver.ResumeSessionResult
		call(t, cs, "resume_session", map[string]any{"session": session}, &resumed)
		var shell, capture bool
		for _, open := range resumed.Open {
			shell = shell || open.Procedure == "user-dialogue" && open.Status == "running"
			capture = capture || open.Instance == move.Instance && open.Step == move.Step
		}
		if !shell || !capture {
			t.Fatalf("the session should resume with its shell running and its move at %s, got %+v", move.Step, resumed.Open)
		}
	}

	t.Run("abandon by handle records the terminal abandon", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		csA := connect(t, env.srv)
		a := openSession(t, csA).Session
		startMove(t, csA, a)
		var torn mcpserver.AbandonResult
		call(t, connect(t, env.srv), "abandon", map[string]any{"session": a}, &torn)
		got := end(t, env, a)
		if got == nil || got.Act != sdd.SessionAbandoned {
			t.Fatalf("teardown by handle = %+v, want a terminal abandon", got)
		}
	})

	t.Run("shutdown ends nothing", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		cs := connect(t, env.srv)
		a := openSession(t, cs).Session
		move := startMove(t, cs, a)
		if err := env.srv.Shutdown(t.Context()); err != nil {
			t.Fatalf("shutdown: %v", err)
		}
		if got := end(t, env, a); got != nil {
			t.Fatalf("shutdown ended the session: %+v", got)
		}
		env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
		requireRunning(t, connect(t, env2.srv), a, move)
	})

	t.Run("disconnect ends nothing", func(t *testing.T) {
		env := newTestServer(t, nil, "", "")
		cs := connect(t, env.srv)
		a := openSession(t, cs).Session
		move := startMove(t, cs, a)
		if err := cs.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if got := end(t, env, a); got != nil {
			t.Fatalf("disconnect ended the session: %+v", got)
		}
		requireRunning(t, connect(t, env.srv), a, move)
	})
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
	for _, want := range []string{"Open loops", "Open and warm", "Pre-flight verdict oscillation"} {
		if !strings.Contains(serve.Instructions, want) {
			t.Fatalf("compose unit should carry %q from the injected lanes, got %q", want, serve.Instructions)
		}
	}
	// A lane that matched nothing contributes nothing — the fixture has no done
	// signals, so the recent-done lane is absent rather than a bare header.
	if strings.Contains(serve.Instructions, "Recent done") {
		t.Fatalf("an empty lane should not serve its header, got %q", serve.Instructions)
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
