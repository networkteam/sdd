package mcpapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
