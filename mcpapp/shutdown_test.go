package mcpapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestShutdownClosesTrackedSessionsAndRejectsNewOnes(t *testing.T) {
	server := newLifecycleTestServer()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "shutdown-client", Version: "test"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = clientSession.Close() }()

	waited := make(chan error, 1)
	go func() { waited <- serverSession.Wait() }()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("ServerSession.Wait remained blocked after Shutdown")
	}

	newServerTransport, _ := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), newServerTransport); !errors.Is(err, ErrServerClosing) {
		t.Fatalf("Connect after Shutdown = %v, want ErrServerClosing", err)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "http://example.test/mcp", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("new HTTP session after Shutdown returned %d, want 503", recorder.Code)
	}
}

func TestShutdownContextBoundsInFlightSessionDrain(t *testing.T) {
	server := newLifecycleTestServer()
	entered := make(chan struct{})
	release := make(chan struct{})
	mcp.AddTool(server.mcp, &mcp.Tool{Name: "block"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		close(entered)
		<-release
		return &mcp.CallToolResult{}, struct{}{}, nil
	})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "blocking-client", Version: "test"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	callDone := make(chan error, 1)
	go func() {
		_, callErr := clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "block"})
		callDone <- callErr
	}()
	<-entered

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := server.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded Shutdown = %v, want context deadline", err)
	}
	close(release)
	select {
	case <-callDone:
	case <-time.After(time.Second):
		t.Fatal("in-flight request did not finish after release")
	}
	drainCtx, drainCancel := context.WithTimeout(t.Context(), time.Second)
	defer drainCancel()
	if err := server.Shutdown(drainCtx); err != nil {
		t.Fatalf("Shutdown after in-flight request completed: %v", err)
	}
}

func newLifecycleTestServer() *Server {
	server := &Server{sessions: newSessionStore(), servedBlocks: map[*mcp.ServerSession]map[[32]byte]bool{}}
	server.mcp = mcp.NewServer(&mcp.Implementation{Name: "lifecycle-test", Version: "test"}, nil)
	server.mcp.AddReceivingMiddleware(server.trackSessionMiddleware)
	return server
}
