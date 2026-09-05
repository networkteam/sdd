// extendingsdd shows an external package mounting SDD's shared MCP
// application behind composition-owned bearer authentication.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
	"github.com/networkteam/sdd/pkg/mcpapp"
)

type externalAccess struct{ runtime *sdd.ProjectRuntime }

func (r externalAccess) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	if identity.Subject == "" {
		return sdd.Principal{}, &sdd.ApplicationError{Code: sdd.ErrorAuthenticationRequired, Message: "identity required"}
	}
	return sdd.Principal{Subject: identity.Subject}, nil
}

// ResolveParticipant is where a composition applies a per-project participant
// name; this example appears under the account's subject everywhere.
func (r externalAccess) ResolveParticipant(_ context.Context, principal sdd.Principal, _ sdd.ProjectID) (string, error) {
	return principal.Subject, nil
}

func (r externalAccess) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	return sdd.ProjectList{Projects: []sdd.ProjectSummary{{ProjectRef: r.runtime.Project(), CanRead: true, CanWrite: true, State: sdd.ProjectReady}}}, nil
}

func (r externalAccess) ResolveProject(_ context.Context, _ sdd.Principal, project sdd.ProjectID, _ sdd.Access) (*sdd.ProjectRuntime, error) {
	if project != r.runtime.Project().ID {
		return nil, &sdd.ApplicationError{Code: sdd.ErrorProjectUnavailable, Message: "project unavailable"}
	}
	return r.runtime, nil
}

func (r externalAccess) ResolveDependency(context.Context, sdd.Principal, sdd.ProjectID, string) (*sdd.ProjectRuntime, error) {
	return nil, &sdd.ApplicationError{Code: sdd.ErrorProjectUnavailable, Message: "dependency unavailable"}
}

// AuthorizeSession is where a composition decides who may continue a
// dialogue; this example keeps SDD's owner-only default.
func (r externalAccess) AuthorizeSession(ctx context.Context, request sdd.SessionAccessRequest) error {
	return sdd.OwnerOnly(ctx, request)
}

func main() {
	root := env("SDD_EXAMPLE_DATA", ".example-sdd")
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: filepath.Join(root, "graph")})
	check(err)
	sessions, err := localadapter.NewFilesystemSessionStoreAt(filepath.Join(root, "sessions"))
	check(err)
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(filepath.Join(root, "staged-blobs"))
	check(err)
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example", DisplayName: "External example"}, Graph: graph,
		LLM: llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) {
			return llm.Result{}, &llm.Error{
				Identity: llm.Identity{Provider: "example", Model: "stub"},
				Err:      errors.New("configure an LLM runner for writes"),
			}
		}),
	})
	check(err)
	// Sessions and staged blobs belong to the composition, not to a project:
	// one store serves every project the principal can reach (d-cpt-yjc).
	application, err := sdd.NewApplication(sdd.ApplicationOptions{
		Access: externalAccess{runtime: runtime}, Sessions: sessions, StagedBlobs: blobs,
	})
	check(err)
	// The wrapper serves every project the principal can reach, and with one
	// accessible project start_session infers it.
	server, err := mcpapp.New(mcpapp.Options{SearchSyncMode: sdd.SearchSyncAll, Application: application, Version: "example"})
	check(err)

	token := env("SDD_EXAMPLE_TOKEN", "development-token")
	verify := func(_ context.Context, presented string, _ *http.Request) (*auth.TokenInfo, error) {
		if presented != token {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{UserID: "example-user", Scopes: []string{"project:read", "project:write"}, Expiration: time.Now().Add(time.Hour)}, nil
	}
	handler := auth.RequireBearerToken(verify, nil)(server.Handler())
	addr := env("SDD_EXAMPLE_ADDR", "127.0.0.1:8765")
	log.Printf("external SDD MCP application listening on http://%s", addr)
	httpServer := &http.Server{Addr: addr, Handler: handler}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		appErr := server.Shutdown(shutdownCtx)
		httpErr := httpServer.Shutdown(shutdownCtx)
		if shutdownCtx.Err() != nil {
			check(errors.Join(appErr, httpErr, httpServer.Close()))
		}
		check(errors.Join(appErr, httpErr))
	case err := <-errCh:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		check(errors.Join(err, server.Shutdown(shutdownCtx)))
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
