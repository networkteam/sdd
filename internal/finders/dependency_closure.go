package finders

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/networkteam/sdd/internal/meta"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
)

// DependencyClosure lists the repos the local repo reaches through declared
// dependencies, as they stand in the connected-repo caches. Freshening those
// caches is the caller's side effect: a refreshed cache can declare more.
func (f *Finder) DependencyClosure(ctx context.Context, _ query.DependencyClosureQuery) ([]model.RepoID, error) {
	if f.cfg == nil {
		return nil, nil
	}
	r := repoDependencies{local: f.cfg.RepoID, declared: f.cfg.Dependencies, repos: f.repos}
	var closure []model.RepoID
	for repoID, err := range model.DependencyClosure(ctx, f.cfg.RepoID, r) {
		if err != nil {
			return nil, err
		}
		closure = append(closure, repoID)
	}
	return closure, nil
}

// repoDependencies resolves a declared dependency to the connected repo of
// the same ID: the local repo answers from its per-repo config, a connected
// repo from the committed config in its cache, read as the local composition
// reads a dependency project's. A repo that is not connected or not cached
// declares nothing, so it stays in the closure with nothing reached behind it.
type repoDependencies struct {
	local    model.RepoID
	declared []model.RepoID
	repos    *repos.Registry
}

func (r repoDependencies) Dependencies(_ context.Context, repoID model.RepoID) ([]model.RepoID, error) {
	if repoID == r.local {
		return r.declared, nil
	}
	if r.repos == nil {
		return nil, nil
	}
	connected, err := r.repos.Load()
	if err != nil {
		return nil, err
	}
	if _, ok := connected.Connected(repoID); !ok {
		return nil, nil
	}
	cacheDir, err := r.repos.CacheDir(repoID)
	if err != nil {
		return nil, err
	}
	cfg, err := meta.ReadCommittedConfigFS(os.DirFS(cacheDir))
	if errors.Is(err, meta.ErrNotAnSDDProject) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config of dependency %s: %w", repoID, err)
	}
	return cfg.Dependencies, nil
}
