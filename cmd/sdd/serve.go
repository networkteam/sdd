package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/urfave/cli/v3"

	sdd "github.com/networkteam/sdd"
	gitadapter "github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
	mcpserver "github.com/networkteam/sdd/mcpapp"
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
			runner, err := newRunner(cmd)
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

			transport := cmd.String("transport")
			srv, err := mcpserver.New(mcpserver.Options{
				Application:   application,
				Project:       project,
				LocalIdentity: identity,
				LocalClient:   transport == "stdio",
				LocalAttachmentPath: func(entryID, filename string) (string, error) {
					attachDir, pathErr := model.AttachDirRelPath(entryID)
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
				return srv.RunHTTP(ctx, addr, token)
			default:
				return fmt.Errorf("invalid transport %q: use stdio or http", cmd.String("transport"))
			}
		}),
	}
}

type localRuntimeAccess struct {
	project      sdd.ProjectID
	runtime      *sdd.ProjectRuntime
	dependencies map[string]*sdd.ProjectRuntime
}

func (a *localRuntimeAccess) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	if identity.Subject == "" {
		return sdd.Principal{}, &sdd.ApplicationError{Code: sdd.ErrorAuthenticationRequired, Message: "request identity is required"}
	}
	participant, _ := identity.Attributes["participant"].(string)
	return sdd.Principal{Subject: identity.Subject, Participant: participant}, nil
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

func buildLocalApplication(ctx context.Context, cmd *cli.Command, graphDir, sddDir string, cfg *model.PerRepoConfig, registry *repos.Registry, runner llm.Runner) (*sdd.Application, sdd.ProjectID, sdd.RequestIdentity, error) {
	project := sdd.ProjectID("local")
	displayName := filepath.Base(filepath.Dir(sddDir))
	participant := ""
	language := ""
	var dependencies []string
	if cfg != nil {
		if cfg.RepoID != "" {
			project = sdd.ProjectID(cfg.RepoID)
		}
		participant = cfg.Participant
		language = cfg.Language
		dependencies = append(dependencies, cfg.Dependencies...)
	}
	graph, err := sdd.NewFilesystemGraphStore(sdd.FilesystemGraphStoreOptions{Project: project, GraphDir: graphDir})
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	sessions, err := sdd.NewFilesystemSessionStore(filepath.Join(sddDir, "sessions"))
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	blobs, err := sdd.NewFilesystemStagedBlobStore(filepath.Join(sddDir, "staged-blobs"))
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	executor := sdd.LLMExecutorFuncs{
		CapabilitiesFunc: func(context.Context) ([]string, error) { return []string{"json-schema"}, nil },
		ExecuteFunc: func(ctx context.Context, request sdd.LLMRequest) (sdd.LLMResult, error) {
			result, err := runner.Run(ctx, llm.Request{SystemPrompt: request.SystemPrompt, UserPrompt: request.Prompt})
			if err != nil {
				return sdd.LLMResult{}, err
			}
			out := sdd.LLMResult{Output: []byte(result.Text), ExecutorFingerprint: "local", FinishReason: "completed"}
			if result.Meta != nil {
				out.Usage.InputTokens = int64(result.Meta.InputTokens)
				out.Usage.OutputTokens = int64(result.Meta.OutputTokens)
				if result.Meta.Provider != "" {
					out.ExecutorFingerprint = result.Meta.Provider
				}
			}
			return out, nil
		},
	}
	localEmbedder, err := buildEmbedder(cmd)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	crossEmbedder, err := crossRepoEmbedder(cmd)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	indexStore := sdd.NewMemorySearchIndexStore()
	var embeddings sdd.EmbeddingExecutor
	if localEmbedder != nil {
		embeddings = publicEmbeddingExecutor(localEmbedder)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: project, DisplayName: displayName}, Participant: participant, Language: language,
		Dependencies: dependencies, Graph: graph, Sessions: sessions, StagedBlobs: blobs, Embeddings: embeddings, SearchIndex: optionalSearchIndex(embeddings, indexStore), LLM: executor,
		Finalizers: []sdd.MutationFinalizer{localGitFinalizer{graphDir: graphDir, git: gitadapter.CLI{}}},
	})
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	access := &localRuntimeAccess{project: project, runtime: runtime, dependencies: map[string]*sdd.ProjectRuntime{}}
	for _, dependency := range dependencies {
		cacheDir, cacheErr := registry.CacheDir(dependency)
		if cacheErr != nil {
			return nil, "", sdd.RequestIdentity{}, cacheErr
		}
		memberGraph, graphErr := sdd.NewFilesystemGraphStore(sdd.FilesystemGraphStoreOptions{Project: sdd.ProjectID(dependency), GraphDir: filepath.Join(cacheDir, model.DefaultGraphDir)})
		if graphErr != nil {
			return nil, "", sdd.RequestIdentity{}, graphErr
		}
		member, runtimeErr := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
			Project: sdd.ProjectRef{ID: sdd.ProjectID(dependency), DisplayName: dependency}, Graph: memberGraph,
			Sessions: sessions, StagedBlobs: blobs, Embeddings: optionalEmbeddingExecutor(crossEmbedder), SearchIndex: optionalSearchIndex(optionalEmbeddingExecutor(crossEmbedder), indexStore), LLM: executor,
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
	identity := sdd.RequestIdentity{Subject: "local", Attributes: map[string]any{"participant": participant}}
	if _, err := application.Info(ctx, identity, project, sdd.InfoRequest{}); err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	return application, project, identity, nil
}

type localGitFinalizer struct {
	graphDir string
	git      localGit
}

type localGit interface {
	Commit(string, ...string) error
	HasCommitMessage(context.Context, string) (bool, error)
}

func (localGitFinalizer) Name() string { return "git" }

func (f localGitFinalizer) Finalize(ctx context.Context, mutation sdd.AppliedMutation) error {
	trailer := "SDD-Mutation: " + mutation.BatchID
	committed, err := f.git.HasCommitMessage(ctx, trailer)
	if err != nil {
		return err
	}
	if committed {
		return nil
	}
	seen := map[string]bool{}
	var paths []string
	appendPath := func(logical string) {
		if logical == "" || seen[logical] {
			return
		}
		seen[logical] = true
		paths = append(paths, filepath.Join(f.graphDir, filepath.FromSlash(logical)))
	}
	for _, change := range mutation.Batch.Changes {
		appendPath(change.LogicalPath)
	}
	for _, attachment := range mutation.Batch.Attachments {
		appendPath(attachment.LogicalPath)
	}
	if len(paths) == 0 {
		return fmt.Errorf("git finalizer: mutation %s has no paths", mutation.BatchID)
	}
	message := mutation.Batch.Message
	if message == "" {
		message = "sdd: apply " + mutation.BatchID
	}
	return f.git.Commit(message+"\n\n"+trailer, paths...)
}

func publicEmbeddingExecutor(embedder llm.Embedder) sdd.EmbeddingExecutor {
	return sdd.EmbeddingExecutorFuncs{
		SpecFunc: func(context.Context) (sdd.EmbeddingSpec, error) {
			return sdd.EmbeddingSpec{Fingerprint: embedder.Fingerprint(), Dimensions: embedder.Dimensions()}, nil
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
