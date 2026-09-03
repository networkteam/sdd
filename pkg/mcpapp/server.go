package mcpapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	sdd "github.com/networkteam/sdd/pkg/application"
)

// Options configures a workflow MCP server.
type Options struct {
	// Application is the protocol-neutral SDD runtime. The server pins no
	// project: start_session selects one inside the dialogue, a sole
	// accessible project is inferred, and every other tool reaches its
	// project through the session it names (d-tac-1z6, d-cpt-yjc).
	Application *sdd.Application
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
	localIdentity       sdd.RequestIdentity
	local               bool
	localAttachmentPath func(string, string) (string, error)
	version             string
	// sessions caches loaded sessions by ID. The transport holds no state
	// about a dialogue: nothing is keyed to a connection, and no connection
	// event acts on a session (d-cpt-aen).
	sessions *sessionCache

	lifecycleMu sync.Mutex
	closing     bool
}

// ErrServerClosing reports that a connection or request arrived after
// graceful shutdown began.
var ErrServerClosing = errors.New("mcpapp: server is shutting down")

// New constructs the server and registers the workflow tool surface.
func New(opts Options) (*Server, error) {
	if opts.Application == nil {
		return nil, errors.New("mcpapp: Application is required")
	}
	s := &Server{
		app:                 opts.Application,
		localIdentity:       opts.LocalIdentity,
		local:               opts.LocalClient,
		localAttachmentPath: opts.LocalAttachmentPath,
		version:             opts.Version,
		sessions:            newSessionCache(),
	}
	s.mcp = mcp.NewServer(&mcp.Implementation{
		Name:    "sdd",
		Version: opts.Version,
	}, &mcp.ServerOptions{
		Instructions: serverInstructions,
	})
	s.mcp.AddReceivingMiddleware(s.refuseWhileClosing)
	s.registerTools()
	return s, nil
}

// RunStdio serves a single local connection over stdin/stdout until the
// transport closes or ctx is cancelled.
func (s *Server) RunStdio(ctx context.Context) error {
	runErr := s.mcp.Run(ctx, &mcp.StdioTransport{})
	return errors.Join(runErr, s.Shutdown(context.Background()))
}

func (s *Server) refuseWhileClosing(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if s.isClosing() {
			return nil, ErrServerClosing
		}
		return next(ctx, method, req)
	}
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
	return s.mcp.Connect(ctx, t, nil)
}

// Shutdown stops admitting requests and closes every live MCP connection in
// parallel. Closing a connection touches no session — a dialogue ends only by
// a participant act (d-cpt-aen) — so the context bounds only the close wait; a
// host that reaches its deadline should force-close its HTTP transport.
func (s *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("mcpapp: shutdown context is required")
	}
	s.lifecycleMu.Lock()
	s.closing = true
	s.lifecycleMu.Unlock()

	var (
		wg    sync.WaitGroup
		errMu sync.Mutex
		errs  []error
	)
	for session := range s.mcp.Sessions() {
		wg.Add(1)
		go func(session *mcp.ServerSession) {
			defer wg.Done()
			if err := session.Close(); err != nil {
				errMu.Lock()
				errs = append(errs, fmt.Errorf("closing MCP session %s: %w", session.ID(), err))
				errMu.Unlock()
			}
		}(session)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		errMu.Lock()
		defer errMu.Unlock()
		return errors.Join(errs...)
	case <-ctx.Done():
		errMu.Lock()
		defer errMu.Unlock()
		return errors.Join(append([]error{context.Cause(ctx)}, errs...)...)
	}
}

func (s *Server) isClosing() bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.closing
}

func toolError(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
