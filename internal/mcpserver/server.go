// Package mcpserver hosts the stateful workflow MCP server: dialogue-loop
// tools that return graph data plus just-in-time instructions, with a
// per-session state machine gating writes (capture requires a prior
// grounding call). The server is deliberately not an agent — the connecting
// client supplies all LLM reasoning; this layer only serves the right data
// and instructions at the right moment and enforces the write gate
// (experiment directive 20260609-234656-d-cpt-afn).
//
// CQRS conformance: tools are pure dispatch — queries go to the finder,
// the capture command goes to the handler. The only state owned here is
// the session map, which is protocol state, not domain state.
package mcpserver

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/query"
)

// Searcher runs a search against the graph. *finders.SearchFinder satisfies
// this; the serve command may wrap it to lazy-fill the vector index before
// querying.
type Searcher interface {
	Search(ctx context.Context, q query.SearchQuery) (*query.SearchResult, error)
}

// Options configures a workflow MCP server.
type Options struct {
	Handler  *handlers.Handler // write path: capture dispatches NewEntryCmd here
	Finder   *finders.Finder   // read path: graph load, info, view, show
	Searcher Searcher          // ground retrieval; nil degrades ground to topics-only
	// VectorSearch selects phrase (vector/semantic) retrieval for ground
	// calls. False falls back to text-term matching on the topic words.
	VectorSearch bool
	GraphDir     string
	Version      string
}

// Server wires the MCP protocol surface to the SDD read and write layers.
type Server struct {
	mcp      *mcp.Server
	handler  *handlers.Handler
	finder   *finders.Finder
	searcher Searcher
	vector   bool
	graphDir string
	sessions *sessionStore
}

// New constructs the server and registers the four dialogue-loop tools.
func New(opts Options) (*Server, error) {
	if opts.Handler == nil || opts.Finder == nil {
		return nil, errors.New("mcpserver: Handler and Finder are required")
	}
	if opts.GraphDir == "" {
		return nil, errors.New("mcpserver: GraphDir is required")
	}
	s := &Server{
		handler:  opts.Handler,
		finder:   opts.Finder,
		searcher: opts.Searcher,
		vector:   opts.VectorSearch,
		graphDir: opts.GraphDir,
		sessions: newSessionStore(),
	}
	s.mcp = mcp.NewServer(&mcp.Implementation{
		Name:    "sdd",
		Version: opts.Version,
	}, &mcp.ServerOptions{
		Instructions: serverInstructions,
	})
	s.registerTools()
	return s, nil
}

// RunStdio serves a single local connection over stdin/stdout until the
// transport closes or ctx is cancelled.
func (s *Server) RunStdio(ctx context.Context) error {
	return s.mcp.Run(ctx, &mcp.StdioTransport{})
}

// RunHTTP serves streamable HTTP at addr until ctx is cancelled. authToken
// must be non-empty: every request needs `Authorization: Bearer <token>`.
// The write path would otherwise be open to anyone who can reach the
// address — the evaluation setup tunnels it to the public internet.
func (s *Server) RunHTTP(ctx context.Context, addr, authToken string) error {
	if authToken == "" {
		return errors.New("mcpserver: HTTP transport requires an auth token")
	}
	httpServer := &http.Server{
		Addr:    addr,
		Handler: s.httpHandler(authToken),
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

// HTTPHandler exposes the bearer-guarded MCP handler for tests that drive
// the server through an httptest.Server instead of a real listener.
func (s *Server) HTTPHandler(authToken string) http.Handler {
	return s.httpHandler(authToken)
}

func (s *Server) httpHandler(authToken string) http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.mcp
	}, &mcp.StreamableHTTPOptions{
		// The evaluation setup reaches this server through a tunnel (ngrok)
		// that forwards to localhost while preserving the public Host
		// header — the SDK's rebinding protection would 403 every such
		// request. The mandatory bearer token already refuses anything a
		// rebound browser request could send, so the Host check adds no
		// protection here.
		DisableLocalhostProtection: true,
	})
	return bearerAuth(authToken, mcpHandler)
}

// Connect attaches the server to an arbitrary transport (in-memory in
// tests). The returned session ends when the client disconnects.
func (s *Server) Connect(ctx context.Context, t mcp.Transport) (*mcp.ServerSession, error) {
	return s.mcp.Connect(ctx, t, nil)
}

func bearerAuth(token string, next http.Handler) http.Handler {
	expect := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if subtle.ConstantTimeCompare([]byte(got), []byte(expect)) != 1 {
			slog.Default().Debug("mcpserver: rejected request without valid bearer token", "remote", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func toolError(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
