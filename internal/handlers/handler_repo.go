package handlers

import (
	"context"
	"fmt"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
)

// RepoAdd executes a RepoAddCmd: clone the target into its cache location,
// verify the repo declares the identity the URL derives, and register the
// connection in the user-global config. The clone runs before registration
// so a failed verification leaves the config untouched.
func (h *Handler) RepoAdd(ctx context.Context, cmd *command.RepoAddCmd) error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}

	repoID, err := model.DeriveRepoID(cmd.CloneURL)
	if err != nil {
		return fmt.Errorf("deriving repo identity: %w", err)
	}

	cfg, err := repos.LoadConfig()
	if err != nil {
		return err
	}
	if _, exists := cfg.Connected(repoID); exists {
		return fmt.Errorf("repo %q is already connected", repoID)
	}

	cacheDir, err := repos.CacheDir(repoID)
	if err != nil {
		return err
	}
	repo := repos.ConnectedRepo{RepoID: repoID, CloneURL: cmd.CloneURL}
	if _, err := repos.EnsureCloned(ctx, repo, cacheDir); err != nil {
		return err
	}

	// The connection is only valid when the target itself declares the
	// identity we register it under — that keeps repo_id canonical across
	// every user rather than a local nickname.
	declared, err := repos.DeclaredRepoID(cacheDir)
	if err != nil {
		return err
	}
	if declared != repoID {
		return fmt.Errorf("repo at %s declares repo_id %q, but its clone URL derives %q — the declared identity is canonical; check the URL", cmd.CloneURL, declared, repoID)
	}

	if err := cfg.AddRepo(repo); err != nil {
		return err
	}
	if err := repos.SaveConfig(cfg); err != nil {
		return err
	}
	if cmd.OnAdded != nil {
		cmd.OnAdded(repoID, cacheDir)
	}
	return nil
}

// RepoRemove executes a RepoRemoveCmd, dropping the connection from the
// user-global config. The cache stays on disk.
func (h *Handler) RepoRemove(ctx context.Context, cmd *command.RepoRemoveCmd) error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}
	cfg, err := repos.LoadConfig()
	if err != nil {
		return err
	}
	if !cfg.RemoveRepo(cmd.RepoID) {
		return fmt.Errorf("repo %q is not connected", cmd.RepoID)
	}
	if err := repos.SaveConfig(cfg); err != nil {
		return err
	}
	if cmd.OnRemoved != nil {
		cmd.OnRemoved(cmd.RepoID)
	}
	return nil
}

// RepoSync executes a RepoSyncCmd: force-pull the named connected repos'
// caches (all of them when none are named), cloning lazily where absent.
func (h *Handler) RepoSync(ctx context.Context, cmd *command.RepoSyncCmd) error {
	cfg, err := repos.LoadConfig()
	if err != nil {
		return err
	}

	targets := cmd.RepoIDs
	if len(targets) == 0 {
		for _, r := range cfg.Repos {
			targets = append(targets, r.RepoID)
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("no connected repos — add one with `sdd repo add <clone-url>`")
	}

	for _, repoID := range targets {
		repo, ok := cfg.Connected(repoID)
		if !ok {
			return fmt.Errorf("repo %q is not connected", repoID)
		}
		cacheDir, err := repos.CacheDir(repoID)
		if err != nil {
			return err
		}
		cloned, err := repos.EnsureCloned(ctx, repo, cacheDir)
		if err != nil {
			return err
		}
		if !cloned {
			if err := repos.ForcePull(ctx, cacheDir); err != nil {
				return err
			}
		}
		if cmd.OnSynced != nil {
			cmd.OnSynced(repoID)
		}
	}
	return nil
}

// freshenReferencedRepos refreshes the caches of every repo a ref set
// points into (lazy clone + cooldown pull) so capture-time resolution reads
// live state. Unconnected repos are skipped — resolve-or-block reports them.
func (h *Handler) freshenReferencedRepos(ctx context.Context, refs []model.Ref) error {
	repoIDs := referencedRepoIDs(refs)
	if len(repoIDs) == 0 {
		return nil
	}
	_, err := h.EnsureReposFresh(ctx, repoIDs)
	return err
}

// fetchOnMiss force-pulls the cache of any connected repo whose referenced
// backward-class target is absent from the loaded member graph, then
// reloads the graph once so pre-flight judges the freshest state. A target
// that is still absent after the fetch is genuinely missing — that block
// stands. Forward-class refs are exempt from resolve-or-block, so they
// trigger no fetch.
func (h *Handler) fetchOnMiss(ctx context.Context, graph *model.Graph, refs []model.Ref) *model.Graph {
	logger := slogutils.FromContext(ctx)
	var stale []string
	for _, r := range refs {
		repoID, entryID, ok := model.SplitCrossRepoID(r.ID)
		if !ok || model.IsForwardClassRefKind(r.Kind) {
			continue
		}
		member, err := graph.MemberGraph(repoID)
		if err != nil || member == nil {
			continue // unconnected or unloadable — resolve-or-block owns the finding
		}
		if _, found := member.ByID[entryID]; !found {
			stale = append(stale, repoID)
		}
	}
	if len(stale) == 0 {
		return graph
	}

	cfg, err := repos.LoadConfig()
	if err != nil {
		logger.Warn("fetch-on-miss: loading global config failed", "err", err)
		return graph
	}
	fetched := false
	for _, repoID := range stale {
		if _, ok := cfg.Connected(repoID); !ok {
			continue
		}
		cacheDir, err := repos.CacheDir(repoID)
		if err != nil {
			continue
		}
		if err := repos.ForcePull(ctx, cacheDir); err != nil {
			logger.Warn("fetch-on-miss pull failed", "repo", repoID, "err", err)
			continue
		}
		fetched = true
	}
	if !fetched {
		return graph
	}
	reloaded, err := h.reader.CurrentGraph(h.graphDir)
	if err != nil {
		logger.Warn("fetch-on-miss reload failed", "err", err)
		return graph
	}
	return reloaded
}

// referencedRepoIDs collects the distinct repo IDs a ref set points into.
func referencedRepoIDs(refs []model.Ref) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range refs {
		if repoID, _, ok := model.SplitCrossRepoID(r.ID); ok && !seen[repoID] {
			seen[repoID] = true
			out = append(out, repoID)
		}
	}
	return out
}

// EnsureReposFresh brings the caches of the given connected repos up to
// date for a read: lazy clone when absent, cooldown-gated pull otherwise.
// Unconnected repo IDs are skipped — the read side renders them unresolved,
// which is that state's honest surface. Reports whether any cache changed
// so a long-lived caller can invalidate its GraphSource.
func (h *Handler) EnsureReposFresh(ctx context.Context, repoIDs []string) (changed bool, err error) {
	if len(repoIDs) == 0 {
		return false, nil
	}
	cfg, err := repos.LoadConfig()
	if err != nil {
		return false, err
	}
	logger := slogutils.FromContext(ctx)
	for _, repoID := range repoIDs {
		repo, ok := cfg.Connected(repoID)
		if !ok {
			logger.Debug("repo not connected; skipping cache refresh", "repo", repoID)
			continue
		}
		cacheDir, err := repos.CacheDir(repoID)
		if err != nil {
			return changed, err
		}
		cloned, err := repos.EnsureCloned(ctx, repo, cacheDir)
		if err != nil {
			return changed, err
		}
		if cloned {
			changed = true
			continue
		}
		pulled, err := repos.CooldownPull(ctx, cacheDir, repos.DefaultPullCooldown)
		if err != nil {
			return changed, err
		}
		changed = changed || pulled
	}
	return changed, nil
}
