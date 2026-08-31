package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/meta"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
	mcpserver "github.com/networkteam/sdd/pkg/mcpapp"
)

func serveCmd() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Run the workflow MCP server: dialogue-loop tools over stdio or HTTP",
		Flags: append(embeddingFlags(),
			&cli.StringFlag{
				Name:  "transport",
				Value: "stdio",
				Usage: "transport: stdio or http",
			},
			&cli.StringFlag{
				Name:  "addr",
				Value: "127.0.0.1:8765",
				Usage: "HTTP listen address (transport=http only)",
			},
			&cli.StringFlag{
				Name:    "auth-token",
				Usage:   "bearer token required on every HTTP request (transport=http only)",
				Sources: cli.EnvVars("SDD_SERVE_TOKEN"),
			},
		),
		Action: withWriteGate(func(ctx context.Context, cmd *cli.Command) error {
			dir, err := resolveGraphDir(cmd)
			if err != nil {
				return err
			}
			sddDir, err := resolveSDDDir()
			if err != nil {
				return err
			}
			runner, err := newRunner(cmd, "")
			if err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			reg, _, err := defaultRepos()
			if err != nil {
				return err
			}
			application, project, identity, err := buildLocalApplication(ctx, cmd, dir, sddDir, cfg, reg, runner)
			if err != nil {
				return err
			}
			retention, err := model.ResolveSessionRetention(cfg)
			if err != nil {
				return err
			}
			collectSessions(ctx, application, project, identity, retention)

			transport := cmd.String("transport")
			srv, err := mcpserver.New(mcpserver.Options{
				Application:   application,
				Project:       project,
				LocalIdentity: identity,
				LocalClient:   transport == "stdio",
				LocalAttachmentPath: func(entryID, filename string) (string, error) {
					attachDir, pathErr := sdd.AttachmentDirRelPath(entryID)
					if pathErr != nil {
						return "", pathErr
					}
					return filepath.Abs(filepath.Join(dir, attachDir, filename))
				},
				Version: version,
			})
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()

			switch transport {
			case "stdio":
				return srv.RunStdio(ctx)
			case "http":
				token := cmd.String("auth-token")
				if token == "" {
					return fmt.Errorf("transport=http requires --auth-token (or SDD_SERVE_TOKEN) — the write path must not be open to anyone who can reach the address")
				}
				addr := cmd.String("addr")
				fmt.Fprintf(os.Stderr, "sdd serve: listening on http://%s (bearer token required)\n", addr)
				return runLocalHTTP(ctx, addr, token, srv)
			default:
				return fmt.Errorf("invalid transport %q: use stdio or http", cmd.String("transport"))
			}
		}),
	}
}

func runLocalHTTP(ctx context.Context, addr, token string, app *mcpserver.Server) error {
	httpServer := &http.Server{
		Addr:    addr,
		Handler: localBearerAuth(token, app.Handler()),
		BaseContext: func(net.Listener) context.Context {
			return context.WithoutCancel(ctx)
		},
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()

	select {
	case <-ctx.Done():
		return shutdownLocalHTTP(httpServer, app)
	case err := <-errCh:
		shutdownErr := shutdownMCPApp(app)
		if errors.Is(err, http.ErrServerClosed) {
			return shutdownErr
		}
		return errors.Join(err, shutdownErr)
	}
}

func shutdownLocalHTTP(httpServer *http.Server, app *mcpserver.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make(chan error, 2)
	go func() { results <- httpServer.Shutdown(ctx) }()
	go func() { results <- app.Shutdown(ctx) }()

	var errs []error
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				errs = append(errs, err)
			}
		case <-ctx.Done():
			errs = append(errs, context.Cause(ctx))
			if err := httpServer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("force-closing HTTP transport: %w", err))
			}
			return errors.Join(errs...)
		}
	}
	return errors.Join(errs...)
}

func shutdownMCPApp(app *mcpserver.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return app.Shutdown(ctx)
}

func localBearerAuth(token string, next http.Handler) http.Handler {
	expect := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(expect)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type localRuntimeAccess struct {
	project      sdd.ProjectID
	participant  string
	runtime      *sdd.ProjectRuntime
	dependencies map[string]*sdd.ProjectRuntime
}

func (a *localRuntimeAccess) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	if identity.Subject == "" {
		return sdd.Principal{}, &sdd.ApplicationError{Code: sdd.ErrorAuthenticationRequired, Message: "request identity is required"}
	}
	return sdd.Principal{Subject: identity.Subject, Participant: a.participant}, nil
}

func (a *localRuntimeAccess) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	return sdd.ProjectList{Projects: []sdd.ProjectSummary{{ProjectRef: a.runtime.Project(), CanRead: true, CanWrite: true, State: sdd.ProjectReady}}}, nil
}

func (a *localRuntimeAccess) ResolveProject(_ context.Context, _ sdd.Principal, project sdd.ProjectID, _ sdd.Access) (*sdd.ProjectRuntime, error) {
	if project != a.project {
		return nil, &sdd.ApplicationError{Code: sdd.ErrorProjectUnavailable, Message: "project unavailable"}
	}
	return a.runtime, nil
}

func (a *localRuntimeAccess) ResolveDependency(_ context.Context, _ sdd.Principal, _ sdd.ProjectID, dependency string) (*sdd.ProjectRuntime, error) {
	runtime := a.dependencies[dependency]
	if runtime == nil {
		return nil, &sdd.ApplicationError{Code: sdd.ErrorProjectUnavailable, Message: "dependency unavailable"}
	}
	return runtime, nil
}

func buildLocalApplication(ctx context.Context, cmd *cli.Command, graphDir, sddDir string, cfg *model.PerRepoConfig, registry *repos.Registry, runner pkgllm.Runner) (*sdd.Application, sdd.ProjectID, sdd.RequestIdentity, error) {
	displayName := filepath.Base(filepath.Dir(sddDir))
	participant := ""
	language := ""
	var dependencies []string
	if cfg != nil {
		participant = cfg.Participant
		language = cfg.Language
		dependencies = append(dependencies, cfg.Dependencies...)
	}
	locations, err := repos.DefaultLocations()
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	stableRepoRoot, err := git.StableRepoRoot(filepath.Dir(sddDir))
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	storeLocations, err := resolveSessionLocations(sddDir, cfg, locations)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	project := sessionStoreProject(cfg)
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: project, GraphDir: graphDir})
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	sessions, err := localadapter.NewFilesystemSessionStore(storeLocations...)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStore(storeLocations...)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	localEmbedder, err := buildEmbedder(cmd)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	crossEmbedder, err := crossRepoEmbedder(cmd)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	// The machine-global persistent store the CLI also builds and reads: one
	// store per (repo-key, embedding fingerprint) under the cache root. The
	// base repo key matches the CLI exactly (see cmd search resolveIndexStore);
	// the fingerprint selects the final StoreDir inside the adapter at request
	// time, so a lazy embedder that reveals its dimensionality only on first
	// use still routes correctly. No process-local memory store in production.
	cacheRoot := registry.CacheRoot()
	baseRepoKey := persistentIndexRepoKey(cfg, stableRepoRoot)
	baseIndex := localadapter.NewPersistentSearchIndexStore(project, cacheRoot, baseRepoKey)
	var embeddings sdd.EmbeddingExecutor
	if localEmbedder != nil {
		embeddings = publicEmbeddingExecutor(localEmbedder)
	}
	targets, err := newLocalMutationTargets(project, filepath.Dir(sddDir))
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	if cfg == nil || cfg.DefaultBranch == "" {
		return nil, "", sdd.RequestIdentity{}, fmt.Errorf("default_branch is required in .sdd/config.yaml before serving mutation tools")
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: project, DisplayName: displayName}, DefaultBranch: cfg.DefaultBranch, Language: language,
		Dependencies: dependencies, Graph: graph, Targets: targets, Branches: targets,
		Recovery: sdd.RecoveryAuthorizerFunc(func(_ context.Context, request sdd.RecoveryAccessRequest) error {
			if request.Actor.Subject != request.OriginalSubject {
				return &sdd.ApplicationError{Code: sdd.ErrorWriteDenied, Message: "cross-principal recovery is not authorized by the local runtime"}
			}
			return nil
		}),
		Sessions: sessions, StagedBlobs: blobs, Embeddings: embeddings, SearchIndex: optionalSearchIndex(embeddings, baseIndex),
		LLM: runner,
	})
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	access := &localRuntimeAccess{project: project, participant: participant, runtime: runtime, dependencies: map[string]*sdd.ProjectRuntime{}}
	for _, dependency := range dependencies {
		cacheDir, cacheErr := registry.CacheDir(dependency)
		if cacheErr != nil {
			return nil, "", sdd.RequestIdentity{}, cacheErr
		}
		memberGraph, graphErr := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: sdd.ProjectID(dependency), GraphDir: filepath.Join(cacheDir, model.DefaultGraphDir)})
		if graphErr != nil {
			return nil, "", sdd.RequestIdentity{}, graphErr
		}
		// Connected stores are keyed by the connected repo's ID (the existing
		// connected-repository storage contract) and exclude embedded entries,
		// so binary-shipped base facts embed once per machine in the base store,
		// not once per connected repo.
		memberEmbedder := optionalEmbeddingExecutor(crossEmbedder)
		memberIndex := localadapter.NewPersistentSearchIndexStore(sdd.ProjectID(dependency), cacheRoot, dependency)
		member, runtimeErr := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
			Project: sdd.ProjectRef{ID: sdd.ProjectID(dependency), DisplayName: dependency}, Graph: memberGraph,
			Sessions: sessions, StagedBlobs: blobs, Embeddings: memberEmbedder, SearchIndex: optionalSearchIndex(memberEmbedder, memberIndex), LLM: runner,
			ExcludeEmbeddedFromIndex: true,
		})
		if runtimeErr != nil {
			return nil, "", sdd.RequestIdentity{}, runtimeErr
		}
		access.dependencies[dependency] = member
	}
	application, err := sdd.NewApplication(access)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	identity := sdd.RequestIdentity{Subject: "local"}
	if _, err := application.Info(ctx, identity, project, sdd.InfoRequest{}); err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	return application, project, identity, nil
}

// collectSessions runs one reclamation pass at startup. This is the trigger
// rather than a command because the store is being opened anyway and collection
// involves no procedure and no served instruction, so it stays host-neutral by
// construction.
//
// CollectSessions itself returns its error, so a composition calling it on a
// schedule can act on one. This adapter cannot: over stdio a server that exits
// leaves the agent with a dead pipe and no way to act, which is worse than the
// unreclaimed disk it would be reporting. So startup logs and serves.
func collectSessions(
	ctx context.Context,
	application *sdd.Application,
	project sdd.ProjectID,
	identity sdd.RequestIdentity,
	retention time.Duration,
) {
	result, err := application.CollectSessions(ctx, identity, project, sdd.CollectSessionsCmd{
		Retention: retention,
	})
	if err != nil {
		slogutils.FromContext(ctx).Warn("session collection did not complete", "err", err)
		return
	}
	if len(result.RemovedSessions) > 0 || len(result.RemovedStaged) > 0 || result.DrainedIntents > 0 {
		slogutils.FromContext(ctx).Info("collected session scaffolding",
			"sessions", len(result.RemovedSessions),
			"staged_sessions", len(result.RemovedStaged),
			"drained_intents", result.DrainedIntents,
			"skipped", len(result.Skipped),
		)
	}
}

func newLocalMutationTargets(project sdd.ProjectID, serverCheckout string) (*localadapter.GitWorktreeAcquirer, error) {
	return localadapter.NewGitWorktreeAcquirer(localadapter.GitWorktreeAcquirerOptions{
		Project: project, ServerCheckout: serverCheckout,
		Factory: func(_ context.Context, checkout string, target sdd.MutationTarget) (sdd.GraphStore, []sdd.MutationFinalizer, func() error, error) {
			targetCfg, cfgErr := resolveConfigAt(filepath.Join(checkout, model.SDDDirName))
			if cfgErr != nil {
				return nil, nil, nil, fmt.Errorf("loading mutation target config for %s: %w", target.Branch, cfgErr)
			}
			if targetCfg == nil {
				return nil, nil, nil, fmt.Errorf("mutation target checkout %q does not contain project %s", checkout, project)
			}
			targetProject := sdd.ProjectID(targetCfg.RepoID)
			if targetProject == "" {
				targetProject = "local"
			}
			if targetProject != project {
				return nil, nil, nil, fmt.Errorf("mutation target checkout %q does not contain project %s", checkout, project)
			}
			targetGraphDir := meta.ResolveGraphDir(checkout, targetCfg)
			targetGraph, graphErr := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: project, GraphDir: targetGraphDir})
			if graphErr != nil {
				return nil, nil, nil, graphErr
			}
			graphDirRel := targetCfg.GraphDir
			if graphDirRel == "" {
				graphDirRel = model.DefaultGraphDir
			}
			return targetGraph, []sdd.MutationFinalizer{localadapter.GitFinalizer{Checkout: checkout, GraphDir: graphDirRel, Branch: target.Branch}}, func() error { return nil }, nil
		},
	})
}

func publicEmbeddingExecutor(embedder llm.Embedder) sdd.EmbeddingExecutor {
	return sdd.EmbeddingExecutorFuncs{
		SpecFunc: func(context.Context) (sdd.EmbeddingSpec, error) {
			return sdd.EmbeddingSpec{Fingerprint: embedder.Fingerprint()}, nil
		},
		EmbedFunc: func(ctx context.Context, inputs []sdd.EmbeddingInput) ([]sdd.EmbeddingVector, error) {
			result := make([]sdd.EmbeddingVector, len(inputs))
			for start := 0; start < len(inputs); {
				purpose := inputs[start].Purpose
				end := start + 1
				for end < len(inputs) && inputs[end].Purpose == purpose {
					end++
				}
				texts := make([]string, end-start)
				for index := start; index < end; index++ {
					texts[index-start] = inputs[index].Text
				}
				var vectors [][]float32
				var err error
				if purpose == sdd.EmbeddingQuery {
					vectors, err = embedder.EmbedQueries(ctx, texts)
				} else {
					vectors, err = embedder.EmbedDocuments(ctx, texts)
				}
				if err != nil {
					return nil, err
				}
				if len(vectors) != len(texts) {
					return nil, fmt.Errorf("embedding provider returned %d vectors for %d inputs", len(vectors), len(texts))
				}
				for index := start; index < end; index++ {
					result[index] = sdd.EmbeddingVector{ID: inputs[index].ID, Values: vectors[index-start]}
				}
				start = end
			}
			return result, nil
		},
	}
}

func optionalEmbeddingExecutor(embedder llm.Embedder) sdd.EmbeddingExecutor {
	if embedder == nil {
		return nil
	}
	return publicEmbeddingExecutor(embedder)
}

func optionalSearchIndex(embeddings sdd.EmbeddingExecutor, index sdd.SearchIndexStore) sdd.SearchIndexStore {
	if embeddings == nil {
		return nil
	}
	return index
}

// repoIDOf returns the committed repo_id, or "" when unconfigured — the input
// to index.RepoKey, which hashes the repo root under the "local" namespace
// when there is no declared ID.
func repoIDOf(cfg *model.PerRepoConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.RepoID
}
