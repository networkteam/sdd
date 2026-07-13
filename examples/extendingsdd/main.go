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

	"github.com/networkteam/sdd"
	"github.com/networkteam/sdd/mcpapp"
)

type externalAccess struct{ runtime *sdd.ProjectRuntime }

func (r externalAccess) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	if identity.Subject == "" {
		return sdd.Principal{}, &sdd.ApplicationError{Code: sdd.ErrorAuthenticationRequired, Message: "identity required"}
	}
	return sdd.Principal{Subject: identity.Subject, Participant: identity.Subject}, nil
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

func main() {
	root := env("SDD_EXAMPLE_DATA", ".example-sdd")
	graph, err := sdd.NewFilesystemGraphStore(sdd.FilesystemGraphStoreOptions{Project: "example", GraphDir: filepath.Join(root, "graph")})
	check(err)
	sessions, err := sdd.NewFilesystemSessionStore(filepath.Join(root, "sessions"))
	check(err)
	blobs, err := sdd.NewFilesystemStagedBlobStore(filepath.Join(root, "staged-blobs"))
	check(err)
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example", DisplayName: "External example"}, Graph: graph, Sessions: sessions, StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) {
				return sdd.LLMResult{}, errors.New("configure an LLM executor for writes")
			},
		},
	})
	check(err)
	application, err := sdd.NewApplication(externalAccess{runtime: runtime})
	check(err)
	server, err := mcpapp.New(mcpapp.Options{Application: application, Project: "example", Version: "example"})
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
