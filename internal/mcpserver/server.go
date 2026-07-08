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
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/networkteam/sdd/internal/bundledskills"
	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
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
	// Repos is the pure read surface over the connected repos, used to
	// resolve cross-repo selections on read tools. Nil disables cross-repo
	// selection (repo-selecting calls fail loud).
	Repos *repos.Registry
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
	repos    *repos.Registry
	sessions *sessionStore
	// docsRegistry answers the registry tool: function docs are identical
	// across per-session registries, so one throwaway instance serves them.
	docsRegistry *engine.Registry

	// servedBlocks is the served-once memory: per connection, the content
	// hashes of rendered blocks (instruction units, framing, the open-threads
	// intro) already served in full. Keyed to the connection — not the
	// session binding — so a same-connection resume never re-pays orientation
	// while a fresh consumer always gets full text (d-tac-dbk, s-tac-w3v).
	servedMu     sync.Mutex
	servedBlocks map[*mcp.ServerSession]map[[sha256.Size]byte]bool

	// vocabulary is the translation data block for non-English graphs,
	// rendered once at construction (config and bundle are static per
	// process) and served once per connection.
	vocabulary string
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
		handler:      opts.Handler,
		finder:       opts.Finder,
		searcher:     opts.Searcher,
		vector:       opts.VectorSearch,
		graphDir:     opts.GraphDir,
		local:        opts.LocalClient,
		version:      opts.Version,
		repos:        opts.Repos,
		sessions:     newSessionStore(opts.SessionsDir),
		servedBlocks: map[*mcp.ServerSession]map[[sha256.Size]byte]bool{},
	}
	docsRegistry, err := s.buildRegistry(&shellSession{id: "docs"})
	if err != nil {
		return nil, err
	}
	s.docsRegistry = docsRegistry
	s.vocabulary = buildVocabularyBlock(s.finder)
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
	s.leaveSession(s.sessions.unbind(ms))
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

// buildVocabularyBlock renders the translation data block for non-English
// graphs from the bundled vocabulary reference — the single source the skill
// render ships (d-cpt-chi), giving locale rendering its engine-surface home
// (s-tac-fgy). English (or no) locale serves nothing. A configured locale
// without a bundled reference serves an explicit note instead of silently
// dropping the commitment.
func buildVocabularyBlock(finder *finders.Finder) string {
	info, err := finder.Info(query.InfoQuery{})
	if err != nil || info.Language == "" {
		return ""
	}
	locale := strings.ToLower(info.Language)
	base := locale
	if i := strings.IndexAny(base, "-_"); i >= 0 {
		base = base[:i]
	}
	if base == "en" {
		return ""
	}
	for _, candidate := range []string{locale, base} {
		body, err := bundledskills.ReadReference("sdd", "references/vocabulary-"+candidate+".md")
		if err == nil {
			return strings.TrimSpace(string(body))
		}
		if candidate == base {
			break // locale == base when unqualified; avoid a duplicate probe
		}
	}
	return fmt.Sprintf("(configured graph language %q has no bundled vocabulary reference — render user-facing terms in English canonical form; adding references/vocabulary-%s.md is a framework-level contribution)", info.Language, base)
}

// leaveSession applies the leave rule to a session a connection stepped
// away from (disconnect or resume-switch): still bound elsewhere → live,
// untouched; open moves → parked, resumable; quiescent (shell only) →
// auto-ended, since un-logged free dialogue leaves nothing to resume.
func (s *Server) leaveSession(ss *shellSession) {
	if ss == nil {
		return
	}
	if s.sessions.liveIDs()[ss.id] {
		return
	}
	if !sessionQuiescent(ss) {
		return
	}
	if ss.sess != nil && ss.shellInstance != "" {
		if inst, ok := ss.sess.Instance(ss.shellInstance); ok && inst.Status == engine.StatusRunning {
			_ = ss.sess.Abandon(ss.shellInstance, "auto-concluded: session left with no open work")
		}
	}
	ss.close()
	s.sessions.drop(ss.id)
}

// sessionQuiescent reports whether nothing but the session shell is
// running — the state in which leaving a session ends it.
func sessionQuiescent(ss *shellSession) bool {
	if ss.sess == nil {
		return true
	}
	for _, inst := range ss.sess.Instances() {
		if inst.ID != ss.shellInstance && inst.Status == engine.StatusRunning {
			return false
		}
	}
	return true
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
