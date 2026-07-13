package mcpapp

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"sync"

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

	lifecycleMu  sync.Mutex
	closing      bool
	shutdownCtx  context.Context
	watchers     sync.WaitGroup
	lifecycleErr []error
}

// ErrServerClosing reports that a connection or request arrived after
// graceful shutdown began.
var ErrServerClosing = errors.New("mcpapp: server is shutting down")

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
	s.mcp.AddReceivingMiddleware(s.trackSessionMiddleware)
	s.registerTools()
	return s, nil
}

// RunStdio serves a single local connection over stdin/stdout until the
// transport closes or ctx is cancelled, then drains its tracked lifecycle
// before returning.
func (s *Server) RunStdio(ctx context.Context) error {
	runErr := s.mcp.Run(ctx, &mcp.StdioTransport{})
	return errors.Join(runErr, s.Shutdown(context.Background()))
}

func (s *Server) trackSessionMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if session, ok := req.GetSession().(*mcp.ServerSession); ok {
			if err := s.watchDisconnect(session); err != nil {
				return nil, err
			}
		}
		return next(ctx, method, req)
	}
}

// watchDisconnect tracks every protocol connection once and applies the leave
// rule when it closes. The lifecycle mutex serializes WaitGroup.Add with the
// transition to closing, so Shutdown never races a newly admitted watcher.
func (s *Server) watchDisconnect(ms *mcp.ServerSession) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.closing {
		return ErrServerClosing
	}
	if !s.sessions.markWatched(ms) {
		return nil
	}
	s.watchers.Add(1)
	go func() {
		defer s.watchers.Done()
		// Close is the authoritative, surfaced shutdown operation. Wait only
		// coordinates its completion and commonly returns the close reason.
		_ = ms.Wait()
		if err := s.handleDisconnect(ms, s.disconnectContext()); err != nil {
			s.recordLifecycleError(fmt.Errorf("leaving disconnected MCP session %s: %w", ms.ID(), err))
		}
	}()
	return nil
}

// handleDisconnect unbinds the connection and applies the leave rule to
// whatever session it held. Idempotent — the watcher and the stdio sweep
// may both fire.
func (s *Server) handleDisconnect(ms *mcp.ServerSession, ctx context.Context) error {
	s.forgetConnection(ms)
	return s.leaveSession(ctx, s.sessions.unbind(ms))
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

// Handler returns the shared Streamable HTTP application without choosing an
// authentication protocol. External compositions must place authenticated
// middleware in front of it and populate the SDK's current-request TokenInfo.
func (s *Server) Handler() http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.mcp
	}, &mcp.StreamableHTTPOptions{
		// Hosts may mount this handler behind a tunnel or reverse proxy that
		// preserves a public Host while forwarding to localhost. Authentication
		// is deliberately host-owned and mandatory for such deployments.
		DisableLocalhostProtection: true,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.isClosing() && r.Header.Get("Mcp-Session-Id") == "" {
			http.Error(w, ErrServerClosing.Error(), http.StatusServiceUnavailable)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
}

// Connect attaches the server to an arbitrary transport (in-memory in
// tests). The returned session ends when the client disconnects.
func (s *Server) Connect(ctx context.Context, t mcp.Transport) (*mcp.ServerSession, error) {
	if s.isClosing() {
		return nil, ErrServerClosing
	}
	session, err := s.mcp.Connect(ctx, t, nil)
	if err != nil {
		return nil, err
	}
	if err := s.watchDisconnect(session); err != nil {
		return nil, errors.Join(err, session.Close())
	}
	return session, nil
}

// Disconnect synchronously applies the session leave rule before closing a
// protocol connection. Hosts with an explicit connection lifecycle should
// prefer it over relying on the asynchronous transport watcher.
func (s *Server) Disconnect(ctx context.Context, session *mcp.ServerSession) error {
	s.forgetConnection(session)
	return errors.Join(s.leaveSession(ctx, s.sessions.unbind(session)), session.Close())
}

// Shutdown stops admitting sessions, closes every tracked MCP connection in
// parallel, and waits for disconnect cleanup. The context bounds the wait;
// a host that reaches its deadline should force-close its HTTP transport,
// which unblocks the remaining session watchers.
func (s *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("mcpapp: shutdown context is required")
	}
	s.lifecycleMu.Lock()
	s.closing = true
	s.shutdownCtx = ctx
	s.lifecycleMu.Unlock()

	for _, session := range s.sessions.connections() {
		session := session
		go func() {
			if err := session.Close(); err != nil {
				s.recordLifecycleError(fmt.Errorf("closing MCP session %s: %w", session.ID(), err))
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		s.watchers.Wait()
		close(done)
	}()
	select {
	case <-done:
		return errors.Join(s.lifecycleErrors()...)
	case <-ctx.Done():
		return errors.Join(append([]error{context.Cause(ctx)}, s.lifecycleErrors()...)...)
	}
}

func (s *Server) isClosing() bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.closing
}

func (s *Server) disconnectContext() context.Context {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.shutdownCtx != nil {
		return s.shutdownCtx
	}
	return context.Background()
}

func (s *Server) recordLifecycleError(err error) {
	if err == nil {
		return
	}
	s.lifecycleMu.Lock()
	s.lifecycleErr = append(s.lifecycleErr, err)
	s.lifecycleMu.Unlock()
}

func (s *Server) lifecycleErrors() []error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return append([]error(nil), s.lifecycleErr...)
}

func toolError(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
