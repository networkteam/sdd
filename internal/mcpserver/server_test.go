package mcpserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

const fixtureGapID = "20260601-100000-s-tac-aaa"

// stubRunner satisfies llm.Runner for finder construction; the read paths
// the server exercises never invoke it.
type stubRunner struct{}

func (stubRunner) Run(context.Context, llm.Request) (*llm.RunResult, error) {
	return nil, fmt.Errorf("no llm in tests")
}

// fakeReader delegates reads to the real finder but answers pre-flight with
// canned findings, so capture outcomes are deterministic without an LLM.
type fakeReader struct {
	*finders.Finder
	findings []query.Finding
}

func (f fakeReader) Preflight(context.Context, query.PreflightQuery) (*query.PreflightResult, error) {
	return &query.PreflightResult{Findings: f.findings}, nil
}

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
		"2026/06/01-110000-d-tac-bbb.md": `---
type: decision
layer: tactical
kind: directive
participants:
    - Tester
confidence: medium
refs:
    - id: 20260601-100000-s-tac-aaa
      kind: addresses
      desc: commits to a convergence criterion for the oscillation
summary: Commits to adding a convergence criterion to pre-flight verdicts.
---

Commits to adding a convergence criterion to pre-flight verdicts, addressing the oscillation gap (20260601-100000-s-tac-aaa).
`,
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
	return graphDir
}

// newTestServer builds a server over a fixture graph with deterministic
// pre-flight findings and text-mode search.
func newTestServer(t *testing.T, findings []query.Finding) (*mcpserver.Server, string) {
	t.Helper()
	graphDir := writeFixtureGraph(t)

	finder := finders.New(finders.Options{
		PreflightRunner: stubRunner{},
		Config:          &model.Config{},
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
		Version:      "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, graphDir
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

func captureArgs(token string) map[string]any {
	return map[string]any{
		"session_token": token,
		"type":          "s",
		"layer":         "tac",
		"kind":          "gap",
		"description": "Test capture entry: the fixture oscillation gap also shows up in integration tests. " +
			"This entry exists to verify the capture path end to end.",
		"refs": []map[string]any{
			{"id": fixtureGapID, "kind": "related", "desc": "the fixture gap this test entry sits beside"},
		},
		"topics":       []string{"testing/fixture"},
		"confidence":   "low",
		"participants": []string{"Tester"},
	}
}

func TestDialogueLoopHappyPath(t *testing.T) {
	srv, graphDir := newTestServer(t, []query.Finding{
		{Severity: query.SeverityLow, Category: "style", Observation: "fixture finding"},
	})
	cs := connect(t, srv)

	var open mcpserver.OpenSessionResult
	call(t, cs, "sdd_open_session", map[string]any{}, &open)
	if open.SessionToken == "" {
		t.Fatal("open_session returned empty token")
	}
	if open.Instructions == "" {
		t.Error("open_session returned no instructions")
	}
	for _, want := range []string{"Recent done", "Active and hot", "Open and warm"} {
		if !strings.Contains(open.Briefing, want) {
			t.Errorf("briefing misses section %q:\n%s", want, open.Briefing)
		}
	}

	var ground mcpserver.GroundResult
	call(t, cs, "sdd_ground", map[string]any{
		"session_token": open.SessionToken,
		"topic":         "oscillation",
	}, &ground)
	if !strings.Contains(ground.RelatedEntries, fixtureGapID) {
		t.Errorf("ground did not find the fixture gap:\n%s", ground.RelatedEntries)
	}
	if !strings.Contains(ground.TopicsInUse, "testing/fixture") {
		t.Errorf("ground topics miss the fixture label:\n%s", ground.TopicsInUse)
	}

	var captured mcpserver.CaptureResult
	call(t, cs, "sdd_capture", captureArgs(open.SessionToken), &captured)
	if !captured.Created || captured.Blocked {
		t.Fatalf("capture: want created, got %+v", captured)
	}
	if captured.ID == "" || captured.Path == "" {
		t.Fatalf("capture returned empty id/path: %+v", captured)
	}
	if _, err := os.Stat(captured.Path); err != nil {
		t.Fatalf("captured entry not on disk: %v", err)
	}
	if !strings.HasPrefix(captured.Path, graphDir) {
		t.Errorf("entry path %q outside graph dir %q", captured.Path, graphDir)
	}
	if len(captured.Findings) != 1 || captured.Findings[0].Severity != "low" {
		t.Errorf("capture findings not passed through: %+v", captured.Findings)
	}

	var shown mcpserver.ShowEntryResult
	call(t, cs, "sdd_show_entry", map[string]any{"ids": []string{captured.ID}}, &shown)
	if !strings.Contains(shown.Entries, "Test capture entry") {
		t.Errorf("show_entry misses the captured body:\n%s", shown.Entries)
	}
	if !strings.Contains(shown.Entries, fixtureGapID) {
		t.Errorf("show_entry misses the upstream ref:\n%s", shown.Entries)
	}
}

func TestCaptureRequiresGrounding(t *testing.T) {
	srv, graphDir := newTestServer(t, nil)
	cs := connect(t, srv)

	var open mcpserver.OpenSessionResult
	call(t, cs, "sdd_open_session", map[string]any{}, &open)

	var captured mcpserver.CaptureResult
	call(t, cs, "sdd_capture", captureArgs(open.SessionToken), &captured)
	if captured.Created || !captured.Blocked {
		t.Fatalf("capture without grounding: want blocked, got %+v", captured)
	}
	if len(captured.Findings) != 1 || captured.Findings[0].Category != "grounding-gate" {
		t.Fatalf("want grounding-gate finding, got %+v", captured.Findings)
	}
	assertNoNewEntries(t, graphDir)
}

func TestCaptureBlockedOnHighFinding(t *testing.T) {
	srv, graphDir := newTestServer(t, []query.Finding{
		{Severity: query.SeverityHigh, Category: "entry-quality", Observation: "fixture blocker"},
	})
	cs := connect(t, srv)

	var open mcpserver.OpenSessionResult
	call(t, cs, "sdd_open_session", map[string]any{}, &open)
	var ground mcpserver.GroundResult
	call(t, cs, "sdd_ground", map[string]any{"session_token": open.SessionToken, "topic": "oscillation"}, &ground)

	var captured mcpserver.CaptureResult
	call(t, cs, "sdd_capture", captureArgs(open.SessionToken), &captured)
	if captured.Created || !captured.Blocked {
		t.Fatalf("capture with high finding: want blocked, got %+v", captured)
	}
	if len(captured.Findings) != 1 || captured.Findings[0].Severity != "high" {
		t.Fatalf("want the high finding, got %+v", captured.Findings)
	}
	if !strings.Contains(captured.Instructions, "revise") {
		t.Errorf("blocked instructions should direct revision: %s", captured.Instructions)
	}
	assertNoNewEntries(t, graphDir)

	// skip_preflight is honored once the user directs it: same args pass.
	args := captureArgs(open.SessionToken)
	args["skip_preflight"] = true
	var retried mcpserver.CaptureResult
	call(t, cs, "sdd_capture", args, &retried)
	if !retried.Created {
		t.Fatalf("capture with skip_preflight: want created, got %+v", retried)
	}
}

func TestUnknownSessionToken(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	cs := connect(t, srv)

	msg := callExpectError(t, cs, "sdd_ground", map[string]any{
		"session_token": "bogus",
		"topic":         "anything",
	})
	if !strings.Contains(msg, "sdd_open_session") {
		t.Errorf("ground error should direct to open_session: %s", msg)
	}

	msg = callExpectError(t, cs, "sdd_capture", captureArgs("bogus"))
	if !strings.Contains(msg, "sdd_open_session") {
		t.Errorf("capture error should direct to open_session: %s", msg)
	}
}

func TestHTTPRequiresBearerToken(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	ts := httptest.NewServer(srv.HTTPHandler("sekrit"))
	t.Cleanup(ts.Close)

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`

	post := func(auth string) int {
		req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(initBody))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		return res.StatusCode
	}

	if got := post(""); got != http.StatusUnauthorized {
		t.Errorf("no token: want 401, got %d", got)
	}
	if got := post("Bearer wrong"); got != http.StatusUnauthorized {
		t.Errorf("wrong token: want 401, got %d", got)
	}
	if got := post("Bearer sekrit"); got == http.StatusUnauthorized {
		t.Error("valid token: got 401")
	}

	// The full client handshake works through the auth middleware.
	authed := &http.Client{Transport: authRoundTripper{token: "sekrit"}}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: authed,
	}, nil)
	if err != nil {
		t.Fatalf("authorized client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	var open mcpserver.OpenSessionResult
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "sdd_open_session", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("open_session over HTTP: %v", err)
	}
	if res.IsError {
		t.Fatalf("open_session over HTTP: %s", contentText(res))
	}
	decodeStructured(t, res, &open)
	if open.SessionToken == "" {
		t.Error("open_session over HTTP returned empty token")
	}
}

type authRoundTripper struct {
	token string
}

func (rt authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+rt.token)
	return http.DefaultTransport.RoundTrip(req)
}

// assertNoNewEntries verifies the fixture graph still holds exactly the two
// seeded entries — a refused capture must leave no file behind.
func assertNoNewEntries(t *testing.T, graphDir string) {
	t.Helper()
	count := 0
	err := filepath.WalkDir(graphDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("graph holds %d entries, want the 2 fixtures", count)
	}
}
