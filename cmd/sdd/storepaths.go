package main

import (
	"fmt"
	"path/filepath"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
	localadapter "github.com/networkteam/sdd/local"
)

func sessionStoreProject(cfg *model.PerRepoConfig) sdd.ProjectID {
	if repoID := repoIDOf(cfg); repoID != "" {
		return sdd.ProjectID(repoID)
	}
	return "local"
}

// resolveSessionLocations names every store location sessions may be found in,
// most current first. Nothing is moved between them: a session is read from and
// appended to wherever it already lies.
func resolveSessionLocations(
	sddDir string,
	cfg *model.PerRepoConfig,
	locations repos.Locations,
) ([]localadapter.StoreLocation, error) {
	repoID := repoIDOf(cfg)
	if repoID != "" {
		if err := model.ValidateRepoID(repoID); err != nil {
			return nil, fmt.Errorf("invalid repo_id for session store: %w", err)
		}
	}
	stableRoot, err := git.StableRepoRoot(filepath.Dir(sddDir))
	if err != nil {
		return nil, err
	}
	return localadapter.SessionLocations(locations.StateRoot, sddDir, repoID, stableRoot)
}

// persistentIndexRepoKey keys the machine-global vector index. It follows the
// repository identity directly, independent of where sessions are found.
func persistentIndexRepoKey(cfg *model.PerRepoConfig, stableRoot string) string {
	return index.RepoKey(repoIDOf(cfg), stableRoot)
}
