package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
)

// errNoRepos surfaces a repo operation on a handler constructed without the
// connected-repos manager — a wiring gap, never a user state (fail loud).
var errNoRepos = fmt.Errorf("connected-repos support is not wired (handler built without Repos)")

// RepoAdd executes a RepoAddCmd with its two writes: clone the target,
// establish its canonical identity from the repo_id it declares in its
// committed config, register the connection in the user-global config (the
// per-user resolution), and declare the dependency in the current repo's
// committed .sdd/config.yaml (the portable record of what this graph
// needs). When the clone URL itself derives an identity (ssh/https forms),
// the declared value must match it — that cross-check catches pointing at a
// fork or mirror under the wrong name. A URL with no host/path form (a
// local path, an offline mirror) skips the cross-check and trusts the
// declared identity, which is canonical by design. The clone runs before
// registration so a failed verification leaves the config untouched.
// Re-running against an already-connected URL skips the clone and only
// ensures the declaration — the upgrade path for connections made before
// dependencies existed.
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

	// Already connected under this URL: the resolution half is done — only
	// ensure the committed declaration. Errors when that too is already in
	// place, so a true no-op still reads as "nothing to do".
	if existing, ok := cfg.ConnectedByURL(cmd.CloneURL); ok {
		declared, err := h.declareDependency(existing.RepoID, cmd)
		if err != nil {
			return err
		}
		if !declared {
			return fmt.Errorf("repo %q is already connected and declared", existing.RepoID)
		}
		return nil
	}

	// Past the already-connected no-op: real work follows. Report the stages
	// so the footer shows connecting → cloning rather than a stuck spinner.
	emitPhase(cmd.OnPhase, model.PhaseConnecting)

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
	emitPhase(cmd.OnPhase, model.PhaseCloning)
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
	if err := h.connectRepo(repo); err != nil {
		return err
	}
	if _, err := h.declareDependency(declared, cmd); err != nil {
		return err
	}
	if cmd.OnAdded != nil {
		cmd.OnAdded(declared, cacheDir)
	}
	return nil
}

// declareDependency ensures repoID is listed in the committed dependencies
// of the current repo's .sdd/config.yaml, using the comment-preserving
// sequence upsert. Reports whether the declaration was added (false: it was
// already there). Outside an sdd repo it is a silent no-op — registering a
// connection without a dependent graph is legitimate. A graph never depends
// on itself.
func (h *Handler) connectRepo(repo repos.ConnectedRepo) error {
	file, err := h.globalConfigFile()
	if err != nil {
		return err
	}
	return file.patch(func(existing []byte) ([]byte, error) {
		cfg, err := repos.ParseGlobalConfig(existing)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.path, err)
		}
		if err := cfg.AddRepo(repo); err != nil {
			return nil, err
		}
		return model.SetYAMLValue(existing, "repos", cfg.Repos)
	})
}

func (h *Handler) declareDependency(repoID string, cmd *command.RepoAddCmd) (bool, error) {
	if h.sddDir == "" {
		return false, nil
	}
	file, err := h.repoConfigFile()
	if err != nil {
		return false, err
	}
	declared := false
	err = file.patch(func(existing []byte) ([]byte, error) {
		cfgFile, err := model.ParseConfig(existing)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.path, err)
		}
		if cfgFile.RepoID == repoID {
			return nil, fmt.Errorf("%q is this repo's own identity — a graph cannot depend on itself", repoID)
		}
		if slices.Contains(cfgFile.Dependencies, repoID) {
			return nil, nil
		}
		patched, err := model.SetYAMLSequence(existing, "dependencies", append(cfgFile.Dependencies, repoID))
		if err != nil {
			return nil, fmt.Errorf("declaring dependency in %s: %w", file.path, err)
		}
		declared = true
		return patched, nil
	})
	if err != nil {
		return false, err
	}
	if !declared {
		if cmd.OnDeclared != nil {
			cmd.OnDeclared(repoID, true)
		}
		return false, nil
	}
	// Auto-commit the declaration, consistent with sdd init/new/summarize —
	// the committed dependencies list is a shared, go.mod-style record other
	// clones read to know what to connect, so it must not linger uncommitted.
	if h.committer != nil {
		if err := h.committer.Commit(fmt.Sprintf("sdd: repo add %s", repoID), file.path); err != nil {
			return false, fmt.Errorf("committing dependency declaration: %w", err)
		}
	}
	if cmd.OnDeclared != nil {
		cmd.OnDeclared(repoID, false)
	}
	return true, nil
}

// RepoRemove executes a RepoRemoveCmd: it drops repoID from the committed
// dependencies of the current repo's .sdd/config.yaml and commits that
// change. Project-scoped and reference-safety-guarded — it refuses when any
// local entry still holds a cross-repo ref into the target (dropping the
// declaration would strand those refs and retroactively break
// resolve-or-block), unless Force is set, in which case the stranded refs are
// named through OnStranded before the removal proceeds. The per-user global
// connection and its cache are left untouched — machine-level teardown is a
// separate command surface, out of scope here.
func (h *Handler) RepoRemove(ctx context.Context, cmd *command.RepoRemoveCmd) error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}
	if h.sddDir == "" {
		return fmt.Errorf("`sdd repo remove` is project-scoped — run it inside an sdd repo to drop a declared dependency")
	}
	slogutils.FromContext(ctx).Debug("removing declared dependency", "repo", cmd.RepoID, "force", cmd.Force)

	file, err := h.repoConfigFile()
	if err != nil {
		return err
	}
	data, err := file.read()
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("no .sdd/config.yaml here — nothing to remove")
	}
	cfgFile, err := model.ParseConfig(data)
	if err != nil {
		return fmt.Errorf("%s: %w", file.path, err)
	}
	if !slices.Contains(cfgFile.Dependencies, cmd.RepoID) {
		return fmt.Errorf("repo %q is not a declared dependency in .sdd/config.yaml", cmd.RepoID)
	}

	// Ref-safety guard: a local entry referencing the target would be
	// orphaned by dropping the declaration. Refuse unless forced, and name
	// the refs either way so the block (or the override) is never silent.
	stranded, err := h.strandedRefs(cmd.RepoID)
	if err != nil {
		return err
	}
	if len(stranded) > 0 {
		if !cmd.Force {
			return fmt.Errorf("refusing to remove %q: %d local %s still reference it — removal would strand:\n%s\nre-run with --force to remove anyway",
				cmd.RepoID, len(stranded), pluralEntries(len(stranded)), formatStranded(stranded))
		}
		if cmd.OnStranded != nil {
			cmd.OnStranded(cmd.RepoID, stranded)
		}
	}

	remaining := slices.DeleteFunc(slices.Clone(cfgFile.Dependencies), func(d string) bool { return d == cmd.RepoID })
	if err := file.patch(func(existing []byte) ([]byte, error) {
		patched, err := model.SetYAMLSequence(existing, "dependencies", remaining)
		if err != nil {
			return nil, fmt.Errorf("removing dependency from %s: %w", file.path, err)
		}
		return patched, nil
	}); err != nil {
		return err
	}
	if h.committer != nil {
		if err := h.committer.Commit(fmt.Sprintf("sdd: repo remove %s", cmd.RepoID), file.path); err != nil {
			return fmt.Errorf("committing dependency removal: %w", err)
		}
	}
	if cmd.OnRemoved != nil {
		cmd.OnRemoved(cmd.RepoID)
	}
	return nil
}

// strandedRefs scans the current graph for local entries that hold a
// cross-repo reference into repoID — the refs a `repo remove` of that
// dependency would orphan. Cross-repo lifecycle effects (closes, supersedes)
// are within-graph only, so only Refs can point across the boundary.
func (h *Handler) strandedRefs(repoID string) ([]command.StrandedRef, error) {
	if h.reader == nil || h.graphDir == "" {
		return nil, nil
	}
	graph, err := h.reader.CurrentGraph(h.graphDir)
	if err != nil {
		return nil, fmt.Errorf("loading graph for ref-safety check: %w", err)
	}
	var out []command.StrandedRef
	for _, e := range graph.Entries {
		for _, r := range e.Refs {
			if target, _, ok := model.SplitCrossRepoID(r.ID); ok && target == repoID {
				out = append(out, command.StrandedRef{EntryID: e.ID, RefID: r.ID, Kind: string(r.Kind)})
			}
		}
	}
	return out, nil
}

// formatStranded renders stranded refs as an indented, aligned block for the
// refusal message and the forced-removal warning.
func formatStranded(stranded []command.StrandedRef) string {
	var b strings.Builder
	for _, s := range stranded {
		fmt.Fprintf(&b, "  %s  %s  %s\n", s.EntryID, s.Kind, s.RefID)
	}
	return strings.TrimRight(b.String(), "\n")
}

func pluralEntries(n int) string {
	if n == 1 {
		return "entry"
	}
	return "entries"
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
	_, err := h.EnsureReposFresh(ctx, command.EnsureReposFreshCmd{RepoIDs: repoIDs})
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
// when vector mode is in play — lazy-fill each repo's index with the shared
// embedder. The finder read (finders.MultiSearch) then runs pure. fill is
// optional: the CLI passes progress callbacks so the member builds render
// through the output coordinator; the MCP server passes nil (non-TTY slog).
func (h *Handler) PrepareCrossRepoSearch(ctx context.Context, q query.SearchQuery, embedder IndexEmbedder, fill *command.BuildConnectedIndexesCmd) error {
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
	// Text-only search needs fresh caches but no embedding; the vector path
	// freshens and fills in one pass through BuildConnectedIndexes.
	if q.Phrase == "" || embedder.Embedder == nil {
		fresh := command.EnsureReposFreshCmd{RepoIDs: repoIDs}
		if fill != nil {
			fresh.OnPhase = fill.OnPhase
		}
		_, err := h.EnsureReposFresh(ctx, fresh)
		return err
	}
	return h.BuildConnectedIndexes(ctx, repoIDs, embedder, fill)
}

// BuildConnectedIndexes freshens the given connected repos' caches and fills
// each member's index under the shared embedder, excluding embedded entries.
// Entries indexed under a different fingerprint re-embed here rather than
// excluding the repo, so one vector space holds across every selected index.
// It is the shared spine for eager pre-indexing (`sdd index --repo`) and the
// fill half of a cross-repo search's prepare step. fill's callbacks report
// progress per repo and aggregate across them; a nil fill runs quiet. When
// fill.Force is set (only `sdd index --repo/--all-repos --force`), each member
// store is fully rebuilt rather than lazily reconciled, repairing stale or
// corrupt connected indexes.
func (h *Handler) BuildConnectedIndexes(ctx context.Context, repoIDs []string, embedder IndexEmbedder, fill *command.BuildConnectedIndexesCmd) error {
	if h.repos == nil {
		return errNoRepos
	}
	if len(repoIDs) == 0 {
		return nil
	}
	fresh := command.EnsureReposFreshCmd{RepoIDs: repoIDs}
	if fill != nil {
		fresh.OnPhase = fill.OnPhase
	}
	if _, err := h.EnsureReposFresh(ctx, fresh); err != nil {
		return err
	}
	if embedder.Embedder == nil {
		return nil
	}
	if fill == nil {
		fill = &command.BuildConnectedIndexesCmd{}
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
		// One machine-global store per (repo-id, fingerprint): the same
		// index the repo's own checkout would use, embedded once. The handler
		// loads and locks the store itself at write time (index.WriteStore).
		idxDir := index.StoreDir(h.repos.Registry().CacheRoot(), repoID, embedder.Fingerprint())
		ih := NewIndexHandler(IndexHandlerOptions{
			GraphDir:        graphDir,
			IndexDir:        idxDir,
			Embedder:        embedder,
			Reader:          h.reader,
			ExcludeEmbedded: true,
		})
		if fill.OnRepoStart != nil {
			fill.OnRepoStart(repoID)
		}
		// Force rebuilds the whole member store (repairing a stale or corrupt
		// index); the default lazy reconcile only touches what changed.
		if fill.Force {
			if err := ih.Build(ctx, &command.BuildIndexCmd{
				Force:          true,
				OnPlanned:      fill.OnPlanned,
				OnBatchStart:   fill.OnBatchStart,
				OnEntryIndexed: fill.OnEntryIndexed,
			}); err != nil {
				return fmt.Errorf("indexing %s: %w", repoID, err)
			}
			continue
		}
		if err := ih.LazyFill(ctx, &command.LazyFillIndexCmd{
			OnPlanned:      fill.OnPlanned,
			OnBatchStart:   fill.OnBatchStart,
			OnEntryIndexed: fill.OnEntryIndexed,
		}); err != nil {
			return fmt.Errorf("indexing %s: %w", repoID, err)
		}
	}
	return nil
}

// EnsureReposFresh brings the caches of the named connected repos up to date
// for a read: lazy clone when absent, cooldown-gated pull otherwise.
// Unconnected repo IDs are skipped — the read side renders them unresolved,
// which is that state's honest surface. Reports whether any cache changed so a
// long-lived caller can invalidate its GraphSource. cmd.OnPhase (optional)
// mirrors RepoAdd — connecting → cloning for a clone, syncing for a due pull,
// nothing for a fresh-cache no-op — so a text-only cross-repo read shows a
// phase-true label instead of "indexing".
func (h *Handler) EnsureReposFresh(ctx context.Context, cmd command.EnsureReposFreshCmd) (changed bool, err error) {
	if len(cmd.RepoIDs) == 0 {
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
	for _, repoID := range cmd.RepoIDs {
		repo, ok := cfg.Connected(repoID)
		if !ok {
			logger.Debug("repo not connected; skipping cache refresh", "repo", repoID)
			continue
		}
		cacheDir, err := h.repos.Registry().CacheDir(repoID)
		if err != nil {
			return changed, err
		}
		switch {
		case !repos.IsCloned(cacheDir):
			emitPhase(cmd.OnPhase, model.PhaseConnecting)
			emitPhase(cmd.OnPhase, model.PhaseCloning)
		case repos.PullDue(cacheDir, repos.DefaultPullCooldown):
			emitPhase(cmd.OnPhase, model.PhaseSyncing)
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

// emitPhase fires an optional phase callback, guarding the nil case at the one
// point phases cross the CQRS boundary (handlers report phases as plain
// callback data; internal/cliout stays out of these layers).
func emitPhase(onPhase func(model.Phase), phase model.Phase) {
	if onPhase != nil {
		onPhase(phase)
	}
}
