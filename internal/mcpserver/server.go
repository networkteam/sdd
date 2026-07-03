// Package mcpserver is the workflow engine's MCP shell (v1 plan
// 20260702-220449-d-tac-ry0, slice 3): the loop tools (start_procedure,
// next, abandon) drive procedure instances through the engine, sessions
// persist as append-only JSONL logs under the sessions dir (list_sessions /
// resume_session), stage_attachment fills the session scratch, and the read
// tools (search, view, show, read_attachment, info, registry) are free and
// never gated. Graph writes exist only as procedure transitions — there is
// no direct write tool (enforcement scoped by surface ownership, s-cpt-1dz).
//
// The server is deliberately not an agent — the connecting client supplies
// all LLM reasoning; this layer serves instructions, validates reports and
// chooser sequence through the engine, and owns the side-effect
// dependencies the engine registry's shell functions need.
//
// CQRS conformance: reads go to finders, writes dispatch handler commands
// from inside registry command closures. State owned here is protocol and
// session-lifecycle state, not domain state.
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

	"github.com/networkteam/sdd/internal/engine"
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
	Handler  *handlers.Handler // write path: registry commands dispatch here
	Finder   *finders.Finder   // read path: graph load, info, view, show
	Searcher Searcher          // search tool; nil disables the search tool's retrieval
	// VectorSearch enables phrase (vector/semantic) retrieval on the search
	// tool. False limits it to text-term matching.
	VectorSearch bool
	GraphDir     string
	// SessionsDir holds the per-participant append-only JSONL session logs
	// and the per-session attachment staging scratch. Local, gitignored.
	SessionsDir string
	// LocalClient marks the connecting client as sharing this filesystem
	// (stdio transport). Local clients get absolute paths in read results
	// (read_attachment) so they can read files directly instead of paging.
	LocalClient bool
	Version     string
}

// Server wires the MCP protocol surface to the engine and the SDD read and
// write layers.
type Server struct {
	mcp      *mcp.Server
	handler  *handlers.Handler
	finder   *finders.Finder
	searcher Searcher
	vector   bool
	graphDir string
	local    bool
	version  string
	sessions *sessionStore
	// docsRegistry answers the registry tool: function docs are identical
	// across per-session registries, so one throwaway instance serves them.
	docsRegistry *engine.Registry
}

// New constructs the server and registers the workflow tool surface.
func New(opts Options) (*Server, error) {
	if opts.Handler == nil || opts.Finder == nil {
		return nil, errors.New("mcpserver: Handler and Finder are required")
	}
	if opts.GraphDir == "" {
		return nil, errors.New("mcpserver: GraphDir is required")
	}
	if opts.SessionsDir == "" {
		return nil, errors.New("mcpserver: SessionsDir is required")
	}
	s := &Server{
		handler:  opts.Handler,
		finder:   opts.Finder,
		searcher: opts.Searcher,
		vector:   opts.VectorSearch,
		graphDir: opts.GraphDir,
		local:    opts.LocalClient,
		version:  opts.Version,
		sessions: newSessionStore(opts.SessionsDir),
	}
	docsRegistry, err := s.buildRegistry(&shellSession{id: "docs"})
	if err != nil {
		return nil, err
	}
	s.docsRegistry = docsRegistry
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
