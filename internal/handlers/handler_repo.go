package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
)

// errNoRepos surfaces a repo operation on a handler constructed without the
// connected-repos manager — a wiring gap, never a user state (fail loud).
var errNoRepos = fmt.Errorf("connected-repos support is not wired (handler built without Repos)")

// RepoAdd executes a RepoAddCmd: clone the target, establish its canonical
// identity from the repo_id it declares in its committed config, and
// register the connection in the user-global config. When the clone URL
// itself derives an identity (ssh/https forms), the declared value must
// match it — that cross-check catches pointing at a fork or mirror under
// the wrong name. A URL with no host/path form (a local path, an offline
// mirror) skips the cross-check and trusts the declared identity, which is
// canonical by design. The clone runs before registration so a failed
// verification leaves the config untouched.
func (h *Handler) RepoAdd(ctx context.Context, cmd *command.RepoAddCmd) error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}
	if h.repos == nil {
		return errNoRepos
	}

	cfg, err := h.repos.Registry().Load()
	if err != nil {
		return err
	}

	// Clone into a staging location under the cache root first — the
	// canonical identity (and so the final cache path) is only known once
	// the clone's declared config is readable.
	root := h.repos.Registry().CacheRoot()
	staging, err := os.MkdirTemp(root, ".adding-*")
	if err != nil {
		if mkErr := os.MkdirAll(root, 0o755); mkErr != nil {
			return fmt.Errorf("creating cache root: %w", mkErr)
		}
		if staging, err = os.MkdirTemp(root, ".adding-*"); err != nil {
			return err
		}
	}
	defer func() { _ = os.RemoveAll(staging) }()
	cloneDir := filepath.Join(staging, "clone")
	repo := repos.ConnectedRepo{CloneURL: cmd.CloneURL}
	if _, err := h.repos.EnsureCloned(ctx, repo, cloneDir); err != nil {
		return err
	}

	declared, err := repos.DeclaredRepoID(cloneDir)
	if err != nil {
		return err
	}
	if err := model.ValidateRepoID(declared); err != nil {
		return fmt.Errorf("repo at %s declares invalid repo_id %q: %w", cmd.CloneURL, declared, err)
	}
	if derived, derr := model.DeriveRepoID(cmd.CloneURL); derr == nil && derived != declared {
		return fmt.Errorf("repo at %s declares repo_id %q, but its clone URL derives %q — the declared identity is canonical; check the URL", cmd.CloneURL, declared, derived)
	}
	if _, exists := cfg.Connected(declared); exists {
		return fmt.Errorf("repo %q is already connected", declared)
	}

	cacheDir, err := h.repos.Registry().CacheDir(declared)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("clearing stale cache at %s: %w", cacheDir, err)
	}
	if err := os.Rename(cloneDir, cacheDir); err != nil {
		return fmt.Errorf("moving clone into cache: %w", err)
	}

	repo.RepoID = declared
	if err := cfg.AddRepo(repo); err != nil {
		return err
	}
	if err := h.repos.Save(cfg); err != nil {
		return err
	}
	if cmd.OnAdded != nil {
		cmd.OnAdded(declared, cacheDir)
	}
	return nil
}

// RepoRemove executes a RepoRemoveCmd, dropping the connection from the
// user-global config. The cache stays on disk.
func (h *Handler) RepoRemove(ctx context.Context, cmd *command.RepoRemoveCmd) error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}
	if h.repos == nil {
		return errNoRepos
	}
	cfg, err := h.repos.Registry().Load()
	if err != nil {
		return err
	}
	if !cfg.RemoveRepo(cmd.RepoID) {
		return fmt.Errorf("repo %q is not connected", cmd.RepoID)
	}
	if err := h.repos.Save(cfg); err != nil {
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
	if h.repos == nil {
		return errNoRepos
	}
	cfg, err := h.repos.Registry().Load()
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
		cacheDir, err := h.repos.Registry().CacheDir(repoID)
		if err != nil {
			return err
		}
		cloned, err := h.repos.EnsureCloned(ctx, repo, cacheDir)
		if err != nil {
			return err
		}
		if !cloned {
			if err := h.repos.ForcePull(ctx, cacheDir); err != nil {
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

	if h.repos == nil {
		return graph
	}
	cfg, err := h.repos.Registry().Load()
	if err != nil {
		logger.Warn("fetch-on-miss: loading global config failed", "err", err)
		return graph
	}
	fetched := false
	for _, repoID := range stale {
		if _, ok := cfg.Connected(repoID); !ok {
			continue
		}
		cacheDir, err := h.repos.Registry().CacheDir(repoID)
		if err != nil {
			continue
		}
		if err := h.repos.ForcePull(ctx, cacheDir); err != nil {
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

// PrepareCrossRepoSearch runs the side-effect half of a cross-graph
// search, mirroring how the local lazy-fill precedes the search finder:
// resolve the query's repo selection, bring those caches up to date, and —
// when vector mode is in play — lazy-fill each repo's index with the
// shared embedder, excluding embedded entries. Entries indexed under a
// different fingerprint re-embed here rather than excluding the repo, so
// one vector space holds across every selected index. The finder read
// (finders.MultiSearch) then runs pure.
func (h *Handler) PrepareCrossRepoSearch(ctx context.Context, q query.SearchQuery, embedder llm.Embedder) error {
	if h.repos == nil {
		if q.AllRepos || len(q.Repos) > 0 {
			return errNoRepos
		}
		return nil
	}
	repoIDs, err := h.repos.Registry().SelectRepoIDs(q.Repos, q.AllRepos)
	if err != nil || len(repoIDs) == 0 {
		return err
	}
	if _, err := h.EnsureReposFresh(ctx, repoIDs); err != nil {
		return err
	}
	if q.Phrase == "" || embedder == nil {
		return nil
	}
	for _, repoID := range repoIDs {
		cacheDir, err := h.repos.Registry().CacheDir(repoID)
		if err != nil {
			return err
		}
		if !repos.IsCloned(cacheDir) {
			continue // unconnected or clone failed — the finder reports the skip
		}
		graphDir, err := repos.GraphDir(cacheDir)
		if err != nil {
			return err
		}
		idxDir := repos.IndexDir(cacheDir)
		store, err := index.Open(idxDir)
		if err != nil {
			return fmt.Errorf("opening index for %s: %w", repoID, err)
		}
		ih := NewIndexHandler(IndexHandlerOptions{
			GraphDir:        graphDir,
			IndexDir:        idxDir,
			Embedder:        embedder,
			IndexStore:      store,
			Reader:          h.reader,
			ExcludeEmbedded: true,
		})
		if err := ih.LazyFill(ctx, &command.LazyFillIndexCmd{}); err != nil {
			return fmt.Errorf("indexing %s: %w", repoID, err)
		}
	}
	return nil
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
	if h.repos == nil {
		return false, errNoRepos
	}
	cfg, err := h.repos.Registry().Load()
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
		cacheDir, err := h.repos.Registry().CacheDir(repoID)
		if err != nil {
			return changed, err
		}
		cloned, err := h.repos.EnsureCloned(ctx, repo, cacheDir)
		if err != nil {
			return changed, err
		}
		if cloned {
			changed = true
			continue
		}
		pulled, err := h.repos.CooldownPull(ctx, cacheDir, repos.DefaultPullCooldown)
		if err != nil {
			return changed, err
		}
		changed = changed || pulled
	}
	return changed, nil
}
