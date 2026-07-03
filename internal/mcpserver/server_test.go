package mcpserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/mcpserver"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

const (
	fixtureGapID = "20260601-100000-s-tac-aaa"

	// embeddedCaptureID is the base capture procedure shipped inside the
	// binary (internal/baseprocedures/entries). The fixture procedure
	// supersedes it, exercising the project-head-wins fork rule on the
	// production resolution path.
	embeddedCaptureID = "20260703-094500-d-prc-cap"
)

// stubRunner satisfies llm.Runner for finder construction; the paths the
// server exercises never invoke it (pre-flight is stubbed, summaries skip
// without a runner).
type stubRunner struct{}

func (stubRunner) Run(context.Context, llm.Request) (*llm.RunResult, error) {
	return nil, fmt.Errorf("no llm in tests")
}

// fakeReader delegates reads to the real finder but answers pre-flight with
// canned findings, so write-gate outcomes are deterministic without an LLM.
type fakeReader struct {
	*finders.Finder
	findings []query.Finding
}

func (f fakeReader) Preflight(context.Context, query.PreflightQuery) (*query.PreflightResult, error) {
	return &query.PreflightResult{Findings: f.findings}, nil
}

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
}

// newTestServer builds a server over a fixture graph with deterministic
// pre-flight findings and text-mode search. Passing existing dirs re-hosts
// them on a fresh server (the restart scenario); mutate tweaks Options.
func newTestServer(t *testing.T, findings []query.Finding, graphDir, sessionsDir string, mutate ...func(*mcpserver.Options)) testEnv {
	t.Helper()
	if graphDir == "" {
		graphDir = writeFixtureGraph(t)
	}
	if sessionsDir == "" {
		sessionsDir = filepath.Join(t.TempDir(), "sessions")
	}

	finder := finders.New(finders.Options{
		PreflightRunner: stubRunner{},
		Config:          &model.Config{Participant: "Tester"},
	})
	handler := handlers.New(handlers.Options{
		GraphDir: graphDir,
		SDDDir:   filepath.Dir(graphDir),
		Reader:   fakeReader{Finder: finder, findings: findings},
	})

	opts := mcpserver.Options{
		Handler:      handler,
		Finder:       finder,
		Searcher:     finders.NewSearchFinder(finders.SearchFinderOptions{GraphDir: graphDir}),
		VectorSearch: false,
		GraphDir:     graphDir,
		SessionsDir:  sessionsDir,
		Version:      "test",
	}
	for _, m := range mutate {
		m(&opts)
	}
	srv, err := mcpserver.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return testEnv{srv: srv, graphDir: graphDir, sessionsDir: sessionsDir}
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
		"abandon", "info", "list_sessions", "next", "read_attachment",
		"registry", "resume_session", "search", "show", "stage_attachment",
		"start_procedure", "view",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tool surface diverges from spec:\n got %v\nwant %v", got, want)
	}
}

// TestCaptureProcedureLoop drives the full capture spine over MCP: batch
// report, playback chooser with served-instruction memory, staged
// attachment materialized by the write gate, summary verification, and the
// produced entry ID.
func TestCaptureProcedureLoop(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	var staged mcpserver.StageAttachmentResult
	call(t, cs, "stage_attachment", map[string]any{
		"name":    "evidence.md",
		"content": "# Evidence\n\nStaged before the capture ran.",
	}, &staged)
	if staged.Handle != "evidence.md" {
		t.Fatalf("unexpected handle %q", staged.Handle)
	}

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture"}, &serve)
	if serve.Step != "assemble" || serve.Status != "running" {
		t.Fatalf("expected running at assemble, got %s at %q", serve.Status, serve.Step)
	}
	if serve.Framing == "" || !strings.Contains(serve.Framing, "Local participant: Tester") {
		t.Fatalf("first serve should carry the session framing, got %q", serve.Framing)
	}
	if !strings.Contains(serve.Instructions, "Existing topics:") {
		t.Fatalf("assemble unit should render with the viewLayout injection, got %q", serve.Instructions)
	}
	if serve.ReportSchema == nil {
		t.Fatal("serve should carry a report schema")
	}

	report := assembleReport()
	report["attachments"] = []string{staged.Handle}
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": report}, &serve)
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

	// Adjust bounces through assemble and back to playback: the second
	// playback serve must be the one-line reminder, not the full unit.
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "adjust", "userWords": "tighten the first sentence",
		"fields": map[string]any{"body": "Test capture entry, tightened: the fixture oscillation gap also " +
			"shows up in integration tests, verifying the engine write path end to end."},
	}}, &serve)
	if serve.Step != "playback" {
		t.Fatalf("adjust should cascade back to playback, got %q", serve.Step)
	}
	if strings.Contains(serve.Instructions, "Play back to the user") || !strings.Contains(serve.Instructions, "served earlier this session") {
		t.Fatalf("second playback serve should be a reminder, got %q", serve.Instructions)
	}

	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "yes, capture it",
	}}, &serve)
	if serve.Step != "verifySummary" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "agent" {
		t.Fatalf("confirm should write and reach verifySummary, got step %q status %s (%s)", serve.Step, serve.Status, serve.Instructions)
	}

	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
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

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture"}, &serve)
	if serve.Step != "assemble" || serve.Status != "running" {
		t.Fatalf("expected running at assemble, got %s at %q", serve.Status, serve.Step)
	}
	if !strings.Contains(serve.Instructions, "Ground before you draft") {
		t.Fatalf("expected the embedded assemble unit, got %q", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, "testing/fixture") {
		t.Fatalf("assemble unit should render the injected topic counts, got %q", serve.Instructions)
	}

	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": assembleReport()}, &serve)
	if serve.Step != "playback" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("expected pending user chooser at playback, got step %q", serve.Step)
	}
	if !strings.Contains(serve.Instructions, "Test capture entry") {
		t.Fatalf("playback unit should render the drafted body verbatim, got %q", serve.Instructions)
	}

	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "yes, capture it",
	}}, &serve)
	if serve.Step != "verifySummary" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "agent" {
		t.Fatalf("confirm should write and reach verifySummary, got step %q status %s (%s)", serve.Step, serve.Status, serve.Instructions)
	}

	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
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

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture"}, &serve)
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": assembleReport()}, &serve)
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "capture it",
	}}, &serve)
	if serve.Step != "reviseOrOverride" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("high findings should route to the reviseOrOverride user chooser, got %q", serve.Step)
	}

	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
		"chooser": "reviseOrOverride", "choice": "override", "userWords": "override it — the finding is wrong",
	}}, &serve)
	if serve.Step != "verifySummary" {
		t.Fatalf("override should re-run the write with pre-flight skipped, got %q (%s)", serve.Step, serve.Instructions)
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

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture"}, &serve)

	var listed mcpserver.ListSessionsResult
	call(t, cs, "list_sessions", map[string]any{}, &listed)
	if len(listed.Sessions) != 1 || listed.Sessions[0].Label != "" {
		t.Fatalf("nothing drafted and no label supplied — label must be blank, got %+v", listed.Sessions)
	}

	// A drafted body backfills the label from its first line.
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": assembleReport()}, &serve)
	call(t, cs, "list_sessions", map[string]any{}, &listed)
	if !strings.HasPrefix(listed.Sessions[0].Label, "Test capture entry") {
		t.Fatalf("label should fall back to the drafted body's first line, got %q", listed.Sessions[0].Label)
	}

	// An explicit label wins over the derived fallback.
	call(t, cs, "next", map[string]any{
		"instance": serve.Instance,
		"report": map[string]any{"chooser": "playback", "choice": "adjust", "userWords": "keep going",
			"fields": map[string]any{"confidence": "medium"}},
		"label": "Oscillation gap capture",
	}, &serve)
	call(t, cs, "list_sessions", map[string]any{}, &listed)
	if listed.Sessions[0].Label != "Oscillation gap capture" {
		t.Fatalf("explicit label must win over the fallback, got %q", listed.Sessions[0].Label)
	}

	// Validation: multi-line and oversized labels are rejected.
	if msg := callExpectError(t, cs, "next", map[string]any{
		"instance": serve.Instance, "report": map[string]any{"confidence": "low"},
		"label": "two\nlines",
	}); !strings.Contains(msg, "single line") {
		t.Fatalf("multi-line label should be rejected, got %q", msg)
	}
	if msg := callExpectError(t, cs, "next", map[string]any{
		"instance": serve.Instance, "report": map[string]any{"confidence": "low"},
		"label": strings.Repeat("x", 200),
	}); !strings.Contains(msg, "120") {
		t.Fatalf("oversized label should be rejected, got %q", msg)
	}
}

func TestSessionResumeAcrossServers(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"canonical": "capture",
		"params":    map[string]any{"anchor": fixtureGapID},
		"label":     "Capture: something about oscillation",
	}, &serve)
	// The label update rides an ordinary next call as the subject sharpens.
	call(t, cs, "next", map[string]any{
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
		t.Fatalf("open instance descriptor diverged: %+v", desc.Open)
	}

	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": sessionID}, &resumed)
	if resumed.Label != "Capture: oscillation gap in integration tests" {
		t.Fatalf("resume briefing should carry the session label, got %q", resumed.Label)
	}
	if len(resumed.Open) != 1 {
		t.Fatalf("expected one running instance after resume, got %+v", resumed.Open)
	}
	rehydrated := resumed.Open[0]
	if rehydrated.Step != "playback" || rehydrated.PendingChooser == nil {
		t.Fatalf("resume should rehydrate the pending chooser at playback, got %+v", rehydrated)
	}
	// Served-instruction memory reset: the new consumer gets full text, and
	// the report evidence persisted through the log replay.
	if !strings.Contains(rehydrated.Instructions, "Play back to the user") {
		t.Fatalf("resume should serve the full unit text again, got %q", rehydrated.Instructions)
	}
	if resumed.Framing == "" {
		t.Fatal("resume should carry the session framing for the new consumer")
	}

	call(t, cs2, "next", map[string]any{"instance": rehydrated.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "capture it",
	}}, &serve)
	call(t, cs2, "next", map[string]any{"instance": rehydrated.Instance, "report": map[string]any{
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
}

// TestAbandonLeavesLogStanding abandons a running instance and checks the
// session drops off the open list without any implicit cleanup of its log.
func TestAbandonLeavesLogStanding(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture"}, &serve)

	var abandoned mcpserver.AbandonResult
	call(t, cs, "abandon", map[string]any{"instance": serve.Instance, "reason": "test teardown"}, &abandoned)
	if !abandoned.Abandoned {
		t.Fatal("abandon should confirm")
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

// TestChooserSequenceValidation exercises the trust property over MCP: a
// chooser cannot be answered before it is pending.
func TestChooserSequenceValidation(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture"}, &serve)

	msg := callExpectError(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
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

// TestFreeReads smoke-tests the ungated read tools.
func TestFreeReads(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	var info mcpserver.InfoResult
	call(t, cs, "info", map[string]any{}, &info)
	if info.Participant != "Tester" || info.Search != "text" {
		t.Fatalf("info diverged: %+v", info)
	}

	var view mcpserver.ViewResult
	call(t, cs, "view", map[string]any{"layout": "active:as-counts"}, &view)
	if !strings.Contains(view.Sections, "testing/fixture") {
		t.Fatalf("view should list the fixture topic, got %q", view.Sections)
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

// TestHTTPTransportAuth keeps the bearer guard covered after the surface
// rewrite.
func TestHTTPTransportAuth(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	hs := httptest.NewServer(env.srv.HTTPHandler("secret-token"))
	defer hs.Close()

	res, err := http.Get(hs.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request should 401, got %d", res.StatusCode)
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

	// Engage: anchor step stalls a made-up anchor, accepts the fixture gap.
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "engage"}, &serve)
	if serve.Step != "anchor" || serve.Status != "running" {
		t.Fatalf("expected running at anchor, got %s at %q", serve.Status, serve.Step)
	}
	engageInstance := serve.Instance

	call(t, cs, "next", map[string]any{"instance": engageInstance, "report": map[string]any{
		"anchor": "20260601-110000-s-tac-zzz",
	}}, &serve)
	if serve.Step != "anchor" || !strings.Contains(serve.Instructions, "does not resolve") {
		t.Fatalf("unresolved anchor should hold the gate naming it, got step %q: %q", serve.Step, serve.Instructions)
	}

	call(t, cs, "next", map[string]any{"instance": engageInstance, "report": map[string]any{
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

	call(t, cs, "next", map[string]any{"instance": engageInstance, "report": map[string]any{
		"brief":       "Narrative: open gap, no downstream activity; needs a decision.",
		"widenReport": "searched oscillation and pre-flight angles; chain already saturates",
	}}, &serve)
	if serve.Step != "moves" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("brief should reach the moves user junction, got %q", serve.Step)
	}

	call(t, cs, "next", map[string]any{"instance": engageInstance, "report": map[string]any{
		"chooser": "moves", "choice": "move", "userWords": "let's explore around it first",
		"fields": map[string]any{"selectedMove": "explore the neighborhood"},
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("move pick should complete the engagement, got %s at %q", serve.Status, serve.Step)
	}

	// Explore as a sub-move of the finished engage, parent-linked.
	call(t, cs, "start_procedure", map[string]any{
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
	if !strings.Contains(serve.Instructions, "overview: what surrounds the oscillation gap") {
		t.Fatalf("inspect unit should carry the goal verbatim, got %q", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, "contradictory findings across runs on identical input") {
		t.Fatalf("inspect unit should serve the target chains, got %q", serve.Instructions)
	}
	exploreInstance := serve.Instance

	call(t, cs, "next", map[string]any{"instance": exploreInstance, "report": map[string]any{
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

// TestJunctionOpenThreads pins the d-tac-nqo property: the open-threads block
// appears exactly at junctions — session entry, terminal serves, abandon,
// resume — never on mid-procedure serves, with the base instruction once and
// the one-line reminder after.
func TestJunctionOpenThreads(t *testing.T) {
	env := newTestServer(t, nil, "", "")

	// Dialogue A parks a capture at assemble.
	csA := connect(t, env.srv)
	var serveA mcpserver.ServeResult
	call(t, csA, "start_procedure", map[string]any{"canonical": "capture", "label": "parked capture A"}, &serveA)
	sessionA := serveA.Session

	// Dialogue B enters fresh: the session-entry junction surfaces A.
	csB := connect(t, env.srv)
	var serve mcpserver.ServeResult
	call(t, csB, "start_procedure", map[string]any{"canonical": "capture"}, &serve)
	if !strings.Contains(serve.OpenThreads, sessionA) || !strings.Contains(serve.OpenThreads, "parked capture A") {
		t.Fatalf("session entry should surface dialogue A, got %q", serve.OpenThreads)
	}
	if !strings.Contains(serve.OpenThreads, "never as an obligation") {
		t.Fatalf("first block should carry the base instruction, got %q", serve.OpenThreads)
	}

	// Mid-procedure serves carry no block — by construction.
	call(t, csB, "next", map[string]any{"instance": serve.Instance, "report": assembleReport()}, &serve)
	if serve.OpenThreads != "" {
		t.Fatalf("mid-procedure serve must not carry open threads, got %q", serve.OpenThreads)
	}
	call(t, csB, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "capture it",
	}}, &serve)
	if serve.OpenThreads != "" {
		t.Fatalf("chooser serve must not carry open threads, got %q", serve.OpenThreads)
	}

	// Completion is a junction: block present, now as the one-line reminder.
	call(t, csB, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
		"chooser": "verifySummary", "choice": "faithful", "userWords": "",
		"fields": map[string]any{"fidelityNote": "matches"},
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("expected completion, got %s at %q", serve.Status, serve.Step)
	}
	if !strings.Contains(serve.OpenThreads, sessionA) {
		t.Fatalf("completion junction should surface dialogue A, got %q", serve.OpenThreads)
	}
	if strings.Contains(serve.OpenThreads, "never as an obligation") || !strings.Contains(serve.OpenThreads, "offer continuations as before") {
		t.Fatalf("later blocks should be the reminder, got %q", serve.OpenThreads)
	}

	// A second start in the same bound session is not a session entry: no block.
	call(t, csB, "start_procedure", map[string]any{"canonical": "capture"}, &serve)
	if serve.OpenThreads != "" {
		t.Fatalf("non-entry start must not carry open threads, got %q", serve.OpenThreads)
	}

	// Abandon is a junction.
	var ab mcpserver.AbandonResult
	call(t, csB, "abandon", map[string]any{"instance": serve.Instance, "reason": "test teardown"}, &ab)
	if !strings.Contains(ab.OpenThreads, sessionA) {
		t.Fatalf("abandon junction should surface dialogue A, got %q", ab.OpenThreads)
	}

	// Resume is a junction listing *other* dialogues; B has nothing running,
	// so resuming A from B's connection yields no block — own threads live in
	// open_instances, not the block.
	var res mcpserver.ResumeSessionResult
	call(t, csB, "resume_session", map[string]any{"session": sessionA}, &res)
	if len(res.Open) != 1 {
		t.Fatalf("resume should serve A's open instance, got %d", len(res.Open))
	}
	if res.OpenThreads != "" {
		t.Fatalf("resume block lists other dialogues only; B is closed, got %q", res.OpenThreads)
	}
}

// readSessionLog reads a session's JSONL event log from the sessions dir.
func readSessionLog(t *testing.T, dir, session string) ([]engine.Event, error) {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, session+".jsonl"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return engine.ReadEvents(f)
}
