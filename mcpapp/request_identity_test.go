package mcpapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	sdd "github.com/networkteam/sdd/application"
	localadapter "github.com/networkteam/sdd/local"
)

type tokenTransport struct {
	mu    sync.RWMutex
	token string
	base  http.RoundTripper
	last  int
}

func (t *tokenTransport) set(token string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.token = token
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.RLock()
	token := t.token
	t.mu.RUnlock()
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+token)
	response, err := t.base.RoundTrip(clone)
	if response != nil {
		t.mu.Lock()
		t.last = response.StatusCode
		t.mu.Unlock()
	}
	return response, err
}

func (t *tokenTransport) lastStatus() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.last
}

func TestStreamableHTTPUsesCurrentRequestIdentity(t *testing.T) {
	type observation struct {
		lane     string
		identity requestIdentity
	}
	var (
		mu           sync.Mutex
		observations []observation
	)

	server := mcp.NewServer(&mcp.Implementation{Name: "identity-spike", Version: "test"}, nil)
	for _, lane := range []string{"project_resolution", "engine_query", "mutation_authorization"} {
		lane := lane
		mcp.AddTool(server, &mcp.Tool{Name: lane}, func(_ context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			mu.Lock()
			observations = append(observations, observation{lane: lane, identity: identityFromRequest(req)})
			mu.Unlock()
			return &mcp.CallToolResult{}, struct{}{}, nil
		})
	}

	verifier := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		info := &auth.TokenInfo{Expiration: time.Now().Add(time.Hour)}
		switch token {
		case "christopher-read":
			info.UserID = "christopher"
			info.Scopes = []string{"project:read"}
			info.Extra = map[string]any{"sentinel": "read-request"}
		case "christopher-write":
			info.UserID = "christopher"
			info.Scopes = []string{"project:read", "project:write"}
			info.Extra = map[string]any{"sentinel": "write-request"}
		case "mallory":
			info.UserID = "mallory"
			info.Scopes = []string{"project:write"}
		default:
			return nil, auth.ErrInvalidToken
		}
		return info, nil
	}
	handler := auth.RequireBearerToken(verifier, nil)(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{JSONResponse: true}))
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	roundTripper := &tokenTransport{token: "christopher-read", base: http.DefaultTransport}
	client := mcp.NewClient(&mcp.Implementation{Name: "identity-spike-client", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             httpServer.URL,
		HTTPClient:           &http.Client{Transport: roundTripper},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	call := func(name string) {
		t.Helper()
		if _, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name}); err != nil {
			t.Fatalf("calling %s: %v", name, err)
		}
	}
	call("project_resolution")
	roundTripper.set("christopher-write")
	call("engine_query")
	call("mutation_authorization")

	mu.Lock()
	got := append([]observation(nil), observations...)
	mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("got %d identity observations, want 3", len(got))
	}
	if got[0].identity.Subject != "christopher" || len(got[0].identity.Scopes) != 1 || got[0].identity.Attributes["sentinel"] != "read-request" {
		t.Fatalf("project resolution did not receive initialization request identity: %+v", got[0])
	}
	for _, observation := range got[1:] {
		if observation.identity.Subject != "christopher" || len(observation.identity.Scopes) != 2 || observation.identity.Attributes["sentinel"] != "write-request" {
			t.Fatalf("%s reused stale identity: %+v", observation.lane, observation.identity)
		}
	}

	roundTripper.set("mallory")
	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "engine_query"})
	if err == nil {
		t.Fatal("changed user should be rejected on the existing MCP session")
	}
	if got := roundTripper.lastStatus(); got != http.StatusForbidden {
		t.Fatalf("changed user returned HTTP %d, want 403: %v", got, err)
	}
}

type observingAccess struct {
	runtime       *sdd.ProjectRuntime
	mu            sync.Mutex
	seen          []sdd.RequestIdentity
	currentScopes map[string][]string
}

func (a *observingAccess) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	a.mu.Lock()
	a.seen = append(a.seen, identity)
	if a.currentScopes == nil {
		a.currentScopes = map[string][]string{}
	}
	a.currentScopes[identity.Subject] = append([]string(nil), identity.Scopes...)
	a.mu.Unlock()
	return sdd.Principal{Subject: identity.Subject, Participant: "Christopher"}, nil
}

func (a *observingAccess) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	return sdd.ProjectList{Projects: []sdd.ProjectSummary{{ProjectRef: a.runtime.Project(), CanRead: true, CanWrite: true, State: sdd.ProjectReady}}}, nil
}

func (a *observingAccess) ResolveProject(_ context.Context, principal sdd.Principal, _ sdd.ProjectID, access sdd.Access) (*sdd.ProjectRuntime, error) {
	a.mu.Lock()
	scopes := append([]string(nil), a.currentScopes[principal.Subject]...)
	a.mu.Unlock()
	want := "project:read"
	code := sdd.ErrorReadDenied
	if access == sdd.AccessWrite {
		want = "project:write"
		code = sdd.ErrorWriteDenied
	}
	for _, scope := range scopes {
		if scope == want {
			return a.runtime, nil
		}
	}
	return nil, &sdd.ApplicationError{Code: code, Message: "scope denied"}
}

func (a *observingAccess) ResolveDependency(context.Context, sdd.Principal, sdd.ProjectID, string) (*sdd.ProjectRuntime, error) {
	return nil, &sdd.ApplicationError{Code: sdd.ErrorProjectUnavailable, Message: "dependency unavailable"}
}

func TestStatefulHTTPWorkflowResolvesIdentityPerRequest(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "graph")
	fixturePath := filepath.Join(graphDir, "2026", "07", "13-120000-s-tac-idt.md")
	if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixturePath, []byte(`---
type: signal
kind: gap
layer: tactical
confidence: high
summary: Identity mutation test anchor.
---

The HTTP identity test anchors its real mutation here.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "identity-test", GraphDir: graphDir})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := localadapter.NewFilesystemSessionStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStore(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "identity-test", DisplayName: "Identity test"}, DefaultBranch: "main",
		Graph: graph, Sessions: sessions, StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) {
				return sdd.LLMResult{Output: []byte(`{"findings":[]}`), ExecutorFingerprint: "test"}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	access := &observingAccess{runtime: runtime}
	application, err := sdd.NewApplication(access)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Options{Application: application, Project: "identity-test", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	verifier := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		info := &auth.TokenInfo{Expiration: time.Now().Add(time.Hour)}
		switch token {
		case "read":
			info.UserID = "christopher"
			info.Scopes = []string{"project:read"}
			info.Extra = map[string]any{"sentinel": "read-request"}
		case "write":
			info.UserID = "christopher"
			info.Scopes = []string{"project:read", "project:write"}
			info.Extra = map[string]any{"sentinel": "write-request"}
		case "mallory":
			info.UserID = "mallory"
			info.Scopes = []string{"project:read", "project:write"}
		default:
			return nil, auth.ErrInvalidToken
		}
		return info, nil
	}
	httpServer := httptest.NewServer(auth.RequireBearerToken(verifier, nil)(server.Handler()))
	defer httpServer.Close()

	transport := &tokenTransport{token: "read", base: http.DefaultTransport}
	client := mcp.NewClient(&mcp.Implementation{Name: "identity-client", Version: "test"}, nil)
	clientSession, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: httpServer.URL, HTTPClient: &http.Client{Transport: transport}, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = clientSession.Close() }()
	var door ServeResult
	callIdentityTool(t, clientSession, "start_session", map[string]any{}, &door)
	session := door.Session
	var shown ShowResult
	callIdentityTool(t, clientSession, "show", map[string]any{"ids": []string{"20260713-120000-s-tac-idt"}}, &shown)
	if !strings.Contains(shown.Entries, "anchors its real mutation") {
		t.Fatalf("identity test anchor was not served through the real application: %q", shown.Entries)
	}
	transport.set("write")
	var capture ServeResult
	callIdentityTool(t, clientSession, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &capture)
	callIdentityTool(t, clientSession, "next", map[string]any{"session": session, "instance": capture.Instance, "report": map[string]any{
		"body":        "The real HTTP identity path authorizes this durable mutation using the current write-bearing request.",
		"entryKind":   "gap",
		"layer":       "tactical",
		"refs":        []map[string]any{{"id": "20260713-120000-s-tac-idt", "kind": "related", "desc": "the identity test anchor"}},
		"topics":      []string{"testing/identity"},
		"confidence":  "high",
		"widenReport": "inspected the identity test anchor before writing the mutation",
	}}, &capture)
	if capture.Step != "playback" {
		t.Fatalf("capture assemble reached %q, want playback; missing=%v instructions=%q", capture.Step, capture.Missing, capture.Instructions)
	}
	callIdentityTool(t, clientSession, "next", map[string]any{"session": session, "instance": capture.Instance, "report": map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "confirm the identity-authorized mutation",
	}}, &capture)
	if capture.Step != "verifySummary" {
		t.Fatalf("capture mutation reached %q, want verifySummary", capture.Step)
	}
	var search SearchResult
	callIdentityTool(t, clientSession, "search", map[string]any{"terms": []string{"durable", "mutation"}, "max_citations": 1}, &search)
	if !strings.Contains(search.Results, "real HTTP identity path") {
		t.Fatalf("durable mutation not visible through the application: %q", search.Results)
	}

	access.mu.Lock()
	seen := append([]sdd.RequestIdentity(nil), access.seen...)
	access.mu.Unlock()
	var readSeen, writeSeen bool
	for _, identity := range seen {
		sentinel, _ := identity.Attributes["sentinel"].(string)
		readSeen = readSeen || sentinel == "read-request" && len(identity.Scopes) == 1
		writeSeen = writeSeen || sentinel == "write-request" && len(identity.Scopes) == 2
	}
	if !readSeen || !writeSeen {
		t.Fatalf("root application identities did not follow requests: %+v", seen)
	}

	transport.set("mallory")
	_, err = clientSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "search", Arguments: map[string]any{"terms": []string{"identity"}}})
	if err == nil {
		t.Fatal("changed user should be rejected on the existing real SDD session")
	}
	if got := transport.lastStatus(); got != http.StatusForbidden {
		t.Fatalf("changed user returned HTTP %d, want 403: %v", got, err)
	}
}

func callIdentityTool[T any](t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any, out *T) {
	t.Helper()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s returned a tool error: %+v", name, result.Content)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decoding %s result: %v", name, err)
	}
}
