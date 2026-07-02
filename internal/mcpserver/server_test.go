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

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/mcpserver"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

const (
	fixtureGapID       = "20260601-100000-s-tac-aaa"
	fixtureProcedureID = "20260601-120000-d-prc-cap"
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
// stored as a normal graph entry so canonical resolution and spec load run
// the production path.
const captureProcedure = `---
type: decision
layer: process
kind: procedure
canonical: capture
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

func writeFixtureGraph(t *testing.T) string {
	t.Helper()
	graphDir := filepath.Join(t.TempDir(), "graph")

	entries := map[string]string{
		"2026/06/01-100000-s-tac-aaa.md": `---
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
`,
		"2026/06/01-120000-d-prc-cap.md": captureProcedure,
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
// them on a fresh server (the restart scenario).
func newTestServer(t *testing.T, findings []query.Finding, graphDir, sessionsDir string) testEnv {
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

	srv, err := mcpserver.New(mcpserver.Options{
		Handler:      handler,
		Finder:       finder,
		Searcher:     finders.NewSearchFinder(finders.SearchFinderOptions{GraphDir: graphDir}),
		VectorSearch: false,
		GraphDir:     graphDir,
		SessionsDir:  sessionsDir,
		Version:      "test",
	})
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
func TestSessionResumeAcrossServers(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"canonical": "capture",
		"params":    map[string]any{"anchor": fixtureGapID},
	}, &serve)
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": assembleReport()}, &serve)
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
	if len(desc.Open) != 1 || desc.Open[0].Procedure != "capture" || desc.Open[0].Step != "playback" {
		t.Fatalf("open instance descriptor diverged: %+v", desc.Open)
	}

	var resumed mcpserver.ResumeSessionResult
	call(t, cs2, "resume_session", map[string]any{"session": sessionID}, &resumed)
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

// TestReadAttachmentPaging pages through a fixture attachment.
func TestReadAttachmentPaging(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	var page1 mcpserver.ReadAttachmentResult
	call(t, cs, "read_attachment", map[string]any{"id": fixtureGapID, "max_bytes": 6}, &page1)
	if page1.Name != "notes.md" || page1.Content != "012345" || !page1.More {
		t.Fatalf("first page diverged: %+v", page1)
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
