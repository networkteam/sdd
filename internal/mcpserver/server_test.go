package mcpserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

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
		Config:          &model.PerRepoConfig{BaseConfig: model.BaseConfig{Participant: "Tester"}},
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

// languageFinder builds a finder whose config carries the given locale.
func languageFinder(lang string) *finders.Finder {
	return finders.New(finders.Options{
		PreflightRunner: stubRunner{},
		Config:          &model.PerRepoConfig{BaseConfig: model.BaseConfig{Participant: "Tester"}, Language: lang},
	})
}

// TestVocabularyBlockForNonEnglishGraphs serves the bundled translation
// table exactly once per connection when the graph language is non-English —
// locale rendering's engine-surface home (d-tac-dbk, s-tac-fgy).
func TestVocabularyBlockForNonEnglishGraphs(t *testing.T) {
	env := newTestServer(t, nil, "", "", func(o *mcpserver.Options) {
		o.Finder = languageFinder("de")
	})
	cs := connect(t, env.srv)
	door := openSession(t, cs)
	if !strings.Contains(door.Vocabulary, "Vokabular") {
		t.Fatalf("a German graph's first serve should carry the vocabulary table, got %q", door.Vocabulary)
	}
	if !strings.Contains(door.Instructions, "Language: de") {
		t.Fatalf("the shell orientation should state the locale, got %q", door.Instructions)
	}

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture"}, &serve)
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
	env := newTestServer(t, nil, "", "", func(o *mcpserver.Options) {
		o.Finder = languageFinder("fr")
	})
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
		"abandon", "info", "list_sessions", "next", "park", "read_attachment",
		"registry", "resume_session", "search", "show", "stage_attachment",
		"start_procedure", "start_session", "view",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tool surface diverges from spec:\n got %v\nwant %v", got, want)
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

// TestCaptureProcedureLoop drives the full capture spine over MCP: batch
// report, playback chooser with served-instruction memory, staged
// attachment materialized by the write gate, summary verification, and the
// produced entry ID.
func TestCaptureProcedureLoop(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	openSession(t, cs)

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

	// Adjust bounces through assemble and back to playback. The body changed,
	// so the re-rendered unit is different bytes — served in full again: a
	// confirm must never bind to an unseen description (content-hash dedup,
	// no per-step memory; d-tac-dbk).
	const tightened = "Test capture entry, tightened: the fixture oscillation gap also " +
		"shows up in integration tests, verifying the engine write path end to end."
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
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
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "adjust", "userWords": "no, keep it",
		"fields": map[string]any{"body": tightened},
	}}, &serve)
	if serve.Step != "playback" {
		t.Fatalf("no-op adjust should cascade back to playback, got %q", serve.Step)
	}
	if strings.Contains(serve.Instructions, "Play back to the user") || !strings.Contains(serve.Instructions, "served earlier this session") {
		t.Fatalf("an unchanged playback re-serve should stub to the reminder, got %q", serve.Instructions)
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
	openSession(t, cs)

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

	// A draft ref'ing an entry never served in full holds the assemble gate —
	// the rejection names exactly the un-inspected ID (d-tac-dbk).
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": assembleReport()}, &serve)
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
	openSession(t, cs)

	// A known anchor passed as a start input seeds the anchor state and
	// auto-advances the resolver straight to scope (the uniform anchor
	// contract — no separate resolver turn for a known entry).
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
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
	call(t, cs, "next", map[string]any{"instance": evalInstance, "report": map[string]any{
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
	call(t, cs, "next", map[string]any{"instance": evalInstance, "report": map[string]any{
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
	call(t, cs, "next", map[string]any{"instance": cap.Instance, "report": map[string]any{
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
	call(t, cs, "next", map[string]any{"instance": cap.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "yes, capture it",
	}}, &cap)
	if cap.Step != "verifySummary" {
		t.Fatalf("confirm should write and reach verifySummary, got %q status %s", cap.Step, cap.Status)
	}

	// Capture #4: verify the summary — completes.
	captureCalls++
	call(t, cs, "next", map[string]any{"instance": cap.Instance, "report": map[string]any{
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
	openSession(t, cs)

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
	openSession(t, cs)

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture"}, &serve)

	// A live-bound session never lists on its own connection; the descriptor
	// is observed the way another participant would see it — a fresh server
	// over the same sessions dir.
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
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": assembleReport()}, &serve)
	listed = descriptor()
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
	listed = descriptor()
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
	openSession(t, cs)

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
	call(t, cs2, "resume_session", map[string]any{"session": sessionID}, &resumed)
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

	// Re-knocking on the door clears the connection's served-once memory —
	// the orientation re-serves in full, on demand.
	if reshell := openSession(t, cs2); reshell.Framing == "" {
		t.Fatal("a repeated start_session should re-serve the framing in full")
	}
}

// TestAbandonLeavesLogStanding abandons a running instance and checks the
// session drops off the open list without any implicit cleanup of its log.
func TestAbandonLeavesLogStanding(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	openSession(t, cs)

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture"}, &serve)

	var abandoned mcpserver.AbandonResult
	call(t, cs, "abandon", map[string]any{"instance": serve.Instance, "reason": "test teardown"}, &abandoned)
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
	openSession(t, cs)

	// A mid-dialogue capture-worthy item: start the capture, seed what is
	// known through the normal start params, and park it.
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"canonical": "capture",
		"params":    map[string]any{"anchor": fixtureGapID, "body": "A draft noted for later."},
		"label":     "noted for later",
	}, &serve)

	var parked mcpserver.ParkResult
	call(t, cs, "park", map[string]any{"instance": serve.Instance, "note": "user wants this after the main work"}, &parked)
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
	if msg := callExpectError(t, cs2, "park", map[string]any{"instance": shellServe.Instance}); !strings.Contains(msg, "park is for moves") {
		t.Fatalf("parking the shell should be refused, got %q", msg)
	}
	if msg := callExpectError(t, cs2, "park", map[string]any{"instance": "i_99"}); !strings.Contains(msg, "not found") {
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
	openSession(t, cs)

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
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
	openSession(t, cs)

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture", "label": "parked draft"}, &serve)
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
	lines := []string{
		`{"v":1,"ts":"2026-07-06T09:00:00Z","session":"` + sessionID + `","seq":1,"event":"session_meta","data":{"participant":"Tester"}}`,
		`{"v":1,"ts":"2026-07-06T09:00:01Z","session":"` + sessionID + `","seq":2,"instance":"i_1","event":"started","data":{"procedure":"implementation","step":"work"}}`,
		`{"v":1,"ts":"2026-07-06T09:00:02Z","session":"` + sessionID + `","seq":3,"instance":"i_1","event":"op_result","data":{"step":"setup","fn":"wipStart","writes":{"wipMarker":"wip-123"}}}`,
	}
	if err := os.MkdirAll(env.sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(env.sessionsDir, sessionID+".jsonl")
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
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

	if msg := callExpectError(t, cs, "abandon", map[string]any{}); !strings.Contains(msg, "exactly one") {
		t.Fatalf("neither instance nor session should be rejected, got %q", msg)
	}
	if msg := callExpectError(t, cs, "abandon", map[string]any{"instance": "i_1", "session": "s_x"}); !strings.Contains(msg, "exactly one") {
		t.Fatalf("both instance and session should be rejected, got %q", msg)
	}
	if msg := callExpectError(t, cs, "abandon", map[string]any{"session": serve.Session}); !strings.Contains(msg, "own junction") {
		t.Fatalf("tearing down the bound session should point at conclude, got %q", msg)
	}
	if msg := callExpectError(t, cs, "abandon", map[string]any{"session": "s_nope"}); !strings.Contains(msg, "unknown session") {
		t.Fatalf("unknown session should be named, got %q", msg)
	}

	// A session live on another connection refuses teardown.
	cs2 := connect(t, env.srv)
	other := openSession(t, cs2)
	if msg := callExpectError(t, cs, "abandon", map[string]any{"session": other.Session}); !strings.Contains(msg, "live on another connection") {
		t.Fatalf("live session should refuse teardown, got %q", msg)
	}
}

// TestUnboundRejectionInlinesParkedSessions pins the discovery half of the
// teardown flow: a stateful call with no session bound is rejected with the
// parked-sessions list inline (handle + label), so the agent's next call can
// already be the resume or the teardown.
func TestUnboundRejectionInlinesParkedSessions(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	openSession(t, cs)
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture", "label": "the parked one"}, &serve)

	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	cs2 := connect(t, env2.srv)
	msg := callExpectError(t, cs2, "next", map[string]any{"instance": "i_2", "report": map[string]any{"x": "y"}})
	if !strings.Contains(msg, "start_session is the door") {
		t.Fatalf("unbound rejection should point at the door, got %q", msg)
	}
	if !strings.Contains(msg, serve.Session) || !strings.Contains(msg, "the parked one") {
		t.Fatalf("unbound rejection should inline the parked session (handle + label), got %q", msg)
	}
}

// TestChooserSequenceValidation exercises the trust property over MCP: a
// chooser cannot be answered before it is pending.
func TestChooserSequenceValidation(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	openSession(t, cs)

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
	openSession(t, cs)

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
	// The pending chooser names itself by step id — the value a chooser answer
	// must supply, so it appears in the payload it answers (s-tac-keb).
	if serve.PendingChooser.Chooser != "moves" {
		t.Fatalf("pending_chooser must name its step id, got %q", serve.PendingChooser.Chooser)
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

// TestDoorGatingAndShellLifecycle pins the d-cpt-h99 session architecture:
// one door, gated loop tools, the shell's resident junction, auto-parented
// moves, the nested landing when a move ends, and the conclude path.
func TestDoorGatingAndShellLifecycle(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)

	// Every loop tool rejects without a session, pointing at the door.
	for tool, args := range map[string]map[string]any{
		"start_procedure":  {"canonical": "capture"},
		"next":             {"instance": "i_1", "report": map[string]any{"x": "y"}},
		"abandon":          {"instance": "i_1"},
		"list_sessions":    {},
		"resume_session":   {"session": "s_nope"},
		"stage_attachment": {"name": "a.md", "content": "x"},
	} {
		if msg := callExpectError(t, cs, tool, args); !strings.Contains(msg, "start_session") {
			t.Fatalf("%s without a session should point at the door, got %q", tool, msg)
		}
	}

	// The door serves the shell's orientation: standing goal, session info,
	// and the live move enumeration (shells excluded from it).
	shell := openSession(t, cs)
	if shell.Goal != "dialogue freely; start a move when something crystallizes" {
		t.Fatalf("shell junction should carry the standing goal, got %q", shell.Goal)
	}
	if !strings.Contains(shell.Instructions, "Participant: Tester") {
		t.Fatalf("opening serve should carry the session info header, got %q", shell.Instructions)
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
	if msg := callExpectError(t, cs, "start_procedure", map[string]any{"canonical": "user-dialogue"}); !strings.Contains(msg, "start_session") {
		t.Fatalf("starting a shell as a move should point at the door, got %q", msg)
	}

	// Moves auto-parent to the shell instance.
	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture"}, &serve)
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
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": assembleReport()}, &serve)
	if serve.OpenThreads != "" {
		t.Fatalf("mid-procedure serve must not carry open threads, got %q", serve.OpenThreads)
	}

	// A move that ends lands back on the shell junction — nested serve.
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "capture it",
	}}, &serve)
	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
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
	if msg := callExpectError(t, cs, "abandon", map[string]any{"instance": shell.Instance}); !strings.Contains(msg, "conclude") {
		t.Fatalf("abandoning the shell should point at conclude, got %q", msg)
	}

	// Conclude with nothing open ends the session directly.
	call(t, cs, "next", map[string]any{"instance": shell.Instance, "report": map[string]any{
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

	var capture mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "capture"}, &capture)

	// Conclude with an open capture: the quiescence gate holds and routes to
	// the threads step, whose serve carries the open work.
	var serve mcpserver.ServeResult
	call(t, cs, "next", map[string]any{"instance": shell.Instance, "report": map[string]any{
		"chooser": "junction", "choice": "conclude", "userWords": "wrap it up",
	}}, &serve)
	if serve.Step != "threads" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("conclude with open moves should reach the threads chooser, got %s at %q", serve.Status, serve.Step)
	}
	if !strings.Contains(serve.OpenThreads, capture.Instance) {
		t.Fatalf("the threads serve should list the open capture, got %q", serve.OpenThreads)
	}

	// Park keeps everything and returns to the resident junction.
	call(t, cs, "next", map[string]any{"instance": shell.Instance, "report": map[string]any{
		"chooser": "threads", "choice": "park", "userWords": "keep it for later",
	}}, &serve)
	if serve.Step != "junction" || serve.Status != "running" {
		t.Fatalf("park should return to the junction, got %s at %q", serve.Status, serve.Step)
	}

	// Settle the thread (abandon it), then conclude for real.
	var ab mcpserver.AbandonResult
	call(t, cs, "abandon", map[string]any{"instance": capture.Instance, "reason": "not needed"}, &ab)
	call(t, cs, "next", map[string]any{"instance": shell.Instance, "report": map[string]any{
		"chooser": "junction", "choice": "conclude", "userWords": "done now",
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("conclude after settling threads should complete the shell, got %s at %q", serve.Status, serve.Step)
	}
}

// TestParkedSessionsAcrossConnections pins the leave rule and the parked
// discrimination: the door surfaces parked dialogues as open threads,
// live-bound sessions neither list nor resume, and switching away from a
// quiescent session auto-ends it.
func TestParkedSessionsAcrossConnections(t *testing.T) {
	env := newTestServer(t, nil, "", "")

	// Server 1, connection A: park a capture (stays live-bound here).
	csA := connect(t, env.srv)
	shellA := openSession(t, csA)
	var serveA mcpserver.ServeResult
	call(t, csA, "start_procedure", map[string]any{"canonical": "capture", "label": "parked capture A"}, &serveA)
	sessionA := serveA.Session

	// Connection B on the same server: A is live-bound — it must not appear
	// as an open thread, must not list, and must not resume.
	csB := connect(t, env.srv)
	shellB := openSession(t, csB)
	if strings.Contains(shellB.OpenThreads, sessionA) {
		t.Fatalf("a live-bound session must not surface as an open thread, got %q", shellB.OpenThreads)
	}
	var listed mcpserver.ListSessionsResult
	call(t, csB, "list_sessions", map[string]any{}, &listed)
	if len(listed.Sessions) != 0 {
		t.Fatalf("live-bound sessions must not list, got %+v", listed.Sessions)
	}
	if msg := callExpectError(t, csB, "resume_session", map[string]any{"session": sessionA}); !strings.Contains(msg, "live") {
		t.Fatalf("resuming a live-bound session should refuse, got %q", msg)
	}
	_ = shellA

	// A fresh server over the same sessions dir sees A as parked: the door
	// surfaces it as an open thread with the intro text, and resuming it
	// auto-ends the quiescent session the connection came from.
	env2 := newTestServer(t, nil, env.graphDir, env.sessionsDir)
	csC := connect(t, env2.srv)
	shellC := openSession(t, csC)
	if !strings.Contains(shellC.OpenThreads, sessionA) || !strings.Contains(shellC.OpenThreads, "parked capture A") {
		t.Fatalf("the door should surface the parked dialogue, got %q", shellC.OpenThreads)
	}
	if !strings.Contains(shellC.OpenThreads, "never as an obligation") {
		t.Fatalf("the first block should carry the intro instruction, got %q", shellC.OpenThreads)
	}

	var resumed mcpserver.ResumeSessionResult
	call(t, csC, "resume_session", map[string]any{"session": sessionA}, &resumed)
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
	f, err := os.Open(filepath.Join(dir, session+".jsonl"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return engine.ReadEvents(f)
}

// TestEmbeddedCatchupProcedure drives the shipped catch-up base entry over
// MCP with the production viewLayout query: the multi-section lane layout
// parses and renders against the real graph, the briefing report reaches the
// junction, and the user's pick completes the check-in.
func TestEmbeddedCatchupProcedure(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	openSession(t, cs)

	var serve mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"canonical": "catch-up"}, &serve)
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

	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
		"briefing": "**One open gap.**\n\n1. Decide the oscillation gap (`s-tac-aaa`).\n\n**What do you want to move forward?**",
	}}, &serve)
	if serve.Step != "junction" || serve.PendingChooser == nil || serve.PendingChooser.Kind != "user" {
		t.Fatalf("briefing should reach the junction user chooser, got %q", serve.Step)
	}

	call(t, cs, "next", map[string]any{"instance": serve.Instance, "report": map[string]any{
		"chooser": "junction", "choice": "pursue", "userWords": "the oscillation gap",
		"fields": map[string]any{"selectedThread": "decide the oscillation gap"},
	}}, &serve)
	if serve.Status != "completed" {
		t.Fatalf("pursue should complete the check-in, got %s at %q", serve.Status, serve.Step)
	}
}
