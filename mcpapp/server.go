package mcpapp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/networkteam/sdd"
)

// Options configures a workflow MCP server.
type Options struct {
	// Application is the protocol-neutral SDD runtime. Project selects the
	// immutable base project for this MCP application.
	Application *sdd.Application
	Project     sdd.ProjectID
	// LocalIdentity supplies the identity for a trusted composition whose
	// transport authenticates every request but cannot populate MCP TokenInfo
	// (the local stdio and static-bearer wrappers use this seam).
	LocalIdentity sdd.RequestIdentity

	// LocalClient marks the connecting client as sharing this filesystem
	// (stdio transport). Local clients get absolute paths in read results
	// (read_attachment) so they can read files directly instead of paging.
	LocalClient bool
	// LocalAttachmentPath optionally adds the local-only path hint to
	// read_attachment results. Canonical attachment reads remain path-free.
	LocalAttachmentPath func(entryID, filename string) (string, error)
	Version             string
}

// Server wires the MCP protocol surface to the engine and the SDD read and
// write layers.
type Server struct {
	mcp                 *mcp.Server
	app                 *sdd.Application
	project             sdd.ProjectID
	localIdentity       sdd.RequestIdentity
	local               bool
	localAttachmentPath func(string, string) (string, error)
	version             string
	sessions            *sessionStore

	// servedBlocks is the served-once memory: per connection, the content
	// hashes of rendered blocks (instruction units, framing, the open-threads
	// intro) already served in full. Keyed to the connection — not the
	// session binding — so a same-connection resume never re-pays orientation
	// while a fresh consumer always gets full text (d-tac-dbk, s-tac-w3v).
	servedMu     sync.Mutex
	servedBlocks map[*mcp.ServerSession]map[[sha256.Size]byte]bool
}

// New constructs the server and registers the workflow tool surface.
func New(opts Options) (*Server, error) {
	if opts.Application == nil {
		return nil, errors.New("mcpapp: Application is required")
	}
	if opts.Project == "" {
		return nil, errors.New("mcpapp: Project is required")
	}
	s := &Server{
		app:                 opts.Application,
		project:             opts.Project,
		localIdentity:       opts.LocalIdentity,
		local:               opts.LocalClient,
		localAttachmentPath: opts.LocalAttachmentPath,
		version:             opts.Version,
		sessions:            newSessionStore(),
		servedBlocks:        map[*mcp.ServerSession]map[[sha256.Size]byte]bool{},
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
// transport closes or ctx is cancelled. The post-run sweep applies the
// leave rule synchronously — the per-connection watcher goroutine would
// race process exit on stdio.
func (s *Server) RunStdio(ctx context.Context) error {
	err := s.mcp.Run(ctx, &mcp.StdioTransport{})
	for _, ms := range s.sessions.connections() {
		s.handleDisconnect(ms)
	}
	return err
}

// watchDisconnect spawns (once per connection) a goroutine applying the
// leave rule when the client goes away: a quiescent session auto-ends — a
// closed tab leaves no corpse — while open moves park for resume.
func (s *Server) watchDisconnect(ms *mcp.ServerSession) {
	if !s.sessions.markWatched(ms) {
		return
	}
	go func() {
		_ = ms.Wait()
		s.handleDisconnect(ms)
	}()
}

// handleDisconnect unbinds the connection and applies the leave rule to
// whatever session it held. Idempotent — the watcher and the stdio sweep
// may both fire.
func (s *Server) handleDisconnect(ms *mcp.ServerSession) {
	s.forgetConnection(ms)
	if err := s.leaveSession(context.Background(), s.sessions.unbind(ms)); err != nil {
		slog.Default().Error("mcpserver: leaving disconnected session", "error", err)
	}
}

// servedBefore reports whether these exact block bytes were already served
// on this connection, recording them when not. Served-once memory keys to
// the connection and dedups by content hash over the rendered bytes
// (post-template, post-injection): identical bytes are stubbed or omitted by
// the caller, changed content always serves in full — no semantic skip rules
// (d-tac-dbk).
func (s *Server) servedBefore(ms *mcp.ServerSession, block string) bool {
	if ms == nil || block == "" {
		return false
	}
	sum := sha256.Sum256([]byte(block))
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	blocks := s.servedBlocks[ms]
	if blocks == nil {
		blocks = map[[sha256.Size]byte]bool{}
		s.servedBlocks[ms] = blocks
	}
	if blocks[sum] {
		return true
	}
	blocks[sum] = true
	return false
}

// forgetConnection drops a connection's served-block memory: on disconnect
// (the entry would leak), and on a repeated start_session — the door
// re-serves the full orientation on demand.
func (s *Server) forgetConnection(ms *mcp.ServerSession) {
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	delete(s.servedBlocks, ms)
}

// leaveSession applies the leave rule to a session a connection stepped
// away from (disconnect or resume-switch): still bound elsewhere → live,
// untouched; open moves → parked, resumable; quiescent (shell only) →
// auto-ended, since un-logged free dialogue leaves nothing to resume.
func (s *Server) leaveSession(ctx context.Context, ss *shellSession) error {
	if ss == nil {
		return nil
	}
	if s.sessions.liveIDs()[ss.id] {
		return nil
	}
	if err := ss.root.Leave(ctx, ss.rootIdentity); err != nil {
		return err
	}
	if len(ss.root.OpenInstances()) == 0 {
		s.sessions.drop(ss.id)
	}
	return nil
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
	return bearerAuth(authToken, s.Handler())
}

// Handler returns the shared Streamable HTTP application without choosing an
// authentication protocol. External compositions must place authenticated
// middleware in front of it and populate the SDK's current-request TokenInfo.
func (s *Server) Handler() http.Handler {
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
	return mcpHandler
}

// Connect attaches the server to an arbitrary transport (in-memory in
// tests). The returned session ends when the client disconnects.
func (s *Server) Connect(ctx context.Context, t mcp.Transport) (*mcp.ServerSession, error) {
	return s.mcp.Connect(ctx, t, nil)
}

// Disconnect synchronously applies the session leave rule before closing a
// protocol connection. Hosts with an explicit connection lifecycle should
// prefer it over relying on the asynchronous transport watcher.
func (s *Server) Disconnect(ctx context.Context, session *mcp.ServerSession) error {
	s.forgetConnection(session)
	if err := s.leaveSession(ctx, s.sessions.unbind(session)); err != nil {
		return err
	}
	return session.Close()
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
