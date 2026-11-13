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

// Options configures SDD's workflow tools.
type Options struct {
	// Application is the protocol-neutral SDD runtime. The server pins no
	// project: start_session selects one inside the dialogue, a sole
	// accessible project is inferred, and every other tool reaches its
	// project through the session it names (d-tac-1z6, d-cpt-yjc).
	Application *sdd.Application
	// SearchSyncMode is chosen by the host for every MCP search.
	SearchSyncMode sdd.SearchSyncMode
	// LocalIdentity supplies the identity for a trusted composition whose
	// transport authenticates every request but cannot populate MCP TokenInfo
	// (the local stdio and static-bearer wrappers use this seam).
	LocalIdentity sdd.RequestIdentity

	// LocalClient marks the connecting client as sharing this filesystem
	// (stdio transport). Local clients get absolute paths in read results
	// (read_attachment) so they can read files directly instead of paging.
	LocalClient bool
	// Version is the SDD version the info tool reports, and the server
	// implementation version under New.
	Version string
}

// toolSet carries what SDD's tool handlers need, independent of the server
// they are registered on.
type toolSet struct {
	app            *sdd.Application
	searchSyncMode sdd.SearchSyncMode
	localIdentity  sdd.RequestIdentity
	local          bool
	version        string
	// sessions caches loaded sessions by ID. The transport holds no state
	// about a dialogue: nothing is keyed to a connection, and no connection
	// event acts on a session (d-cpt-aen).
	sessions *sessionCache
}

func newToolSet(opts Options) (*toolSet, error) {
	if !opts.SearchSyncMode.Valid() {
		return nil, errors.New("mcpapp: SearchSyncMode is required: none, local, or all")
	}
	if opts.Application == nil {
		return nil, errors.New("mcpapp: Application is required")
	}
	return &toolSet{
		app:            opts.Application,
		searchSyncMode: opts.SearchSyncMode,
		localIdentity:  opts.LocalIdentity,
		local:          opts.LocalClient,
		version:        opts.Version,
		sessions:       newSessionCache(),
	}, nil
}

// Register adds SDD's workflow tools to a server the host constructs, beside
// any tools of the host's own (d-tac-d3f). The host owns that server: its
// implementation name and version, its instructions, composed around
// [Instructions], its transport and handler, its authentication, and its
// lifecycle. SDD's tools derive nothing from the connection, so any transport
// serves them.
//
// The MCP SDK replaces a tool registered under an existing name without error:
// a host tool must not reuse one of SDD's tool names.
func Register(server *mcp.Server, opts Options) error {
	if server == nil {
		return errors.New("mcpapp: server is required")
	}
	tools, err := newToolSet(opts)
	if err != nil {
		return err
	}
	tools.register(server)
	return nil
}

// Server is an MCP server carrying SDD's tools alone, for compositions that
// add no tools of their own. It owns the transport and the lifecycle.
type Server struct {
	mcp *mcp.Server

	lifecycleMu sync.Mutex
	closing     bool
}

// ErrServerClosing reports that a connection or request arrived after
// graceful shutdown began.
var ErrServerClosing = errors.New("mcpapp: server is shutting down")

// New constructs a server named sdd and registers SDD's tools on it.
func New(opts Options) (*Server, error) {
	tools, err := newToolSet(opts)
	if err != nil {
		return nil, err
	}
	s := &Server{
		mcp: mcp.NewServer(&mcp.Implementation{
			Name:    "sdd",
			Version: opts.Version,
		}, &mcp.ServerOptions{
			Instructions: Instructions,
		}),
	}
	s.mcp.AddReceivingMiddleware(s.refuseWhileClosing)
	tools.register(s.mcp)
	return s, nil
}

// RunStdio serves a single local connection over stdin/stdout until the
// transport closes or ctx is cancelled.
func (s *Server) RunStdio(ctx context.Context) error {
	return s.Run(ctx, &mcp.StdioTransport{})
}

// Run serves the supplied transport and shuts down the server when it ends.
func (s *Server) Run(ctx context.Context, transport mcp.Transport) error {
	runErr := s.mcp.Run(ctx, transport)
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

// Handler returns the stateless Streamable HTTP application without choosing an
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
		Stateless:                  true,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.isClosing() {
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
