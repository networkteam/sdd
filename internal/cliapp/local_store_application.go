package cliapp

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/networkteam/sdd/internal/repos"
	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

// buildLocalStoreApplication composes the application over the local checkout
// for commands that inspect session state without running language models.
func buildLocalStoreApplication(ctx context.Context, cmd *cli.Command) (*sdd.Application, sdd.ProjectID, sdd.RequestIdentity, error) {
	graphDir, err := resolveGraphDir(cmd)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	sddDir, err := resolveSDDDir()
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	if cfg == nil || cfg.DefaultBranch == "" {
		return nil, "", sdd.RequestIdentity{}, fmt.Errorf("default_branch is required in .sdd/config.yaml")
	}
	locations, err := repos.DefaultLocations()
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
	targets, err := localadapter.NewRepositoryTargets(project, filepath.Dir(sddDir), locations.ConfigPath)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: project, DisplayName: filepath.Base(filepath.Dir(sddDir))}, DefaultBranch: cfg.DefaultBranch,
		Graph: graph, Targets: targets,
		LLM: pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
			return pkgllm.Result{}, fmt.Errorf("session inspection does not execute language models")
		}),
	})
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	access := &localRuntimeAccess{project: project, participant: cfg.Participant, runtime: runtime, dependencies: map[sdd.RepoID]*sdd.ProjectRuntime{}}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: access, Sessions: sessions, StagedBlobs: blobs})
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	identity := sdd.RequestIdentity{Subject: "local"}
	if _, err := application.Info(ctx, identity, project, sdd.InfoRequest{}); err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	return application, project, identity, nil
}
