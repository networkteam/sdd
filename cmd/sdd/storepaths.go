package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
	localadapter "github.com/networkteam/sdd/local"
)

type localStorePaths struct {
	// Sessions/StagedBlobs/RepoKey are the currently routed store. During a
	// bounded pending identity transition these remain the old global key.
	Sessions    string
	StagedBlobs string
	RepoKey     string

	DesiredSessions     string
	DesiredBlobs        string
	DesiredKey          string
	OldSessions         string
	OldBlobs            string
	OldKey              string
	StableRepoAuthority string

	Transition           *localadapter.SessionIdentityTransition
	PendingIdentity      bool
	Interrupted          bool
	InTreeMaterial       []string
	InTreeLegacySessions []string
	OldMaterial          []string
	OldLegacySessions    []string
	InTreeTombstone      *localadapter.SessionRelocationTombstoneRecord
	OldTombstone         *localadapter.SessionRelocationTombstoneRecord
	// InTreeTombstoneNeedsUpdate means a prior identity-less relocation
	// points at Old* and must be advanced to Desired* in the aggregate move.
	InTreeTombstoneNeedsUpdate bool
}

func relocationSourcesForInit(
	paths localStorePaths,
	sddDir string,
) ([]localadapter.SessionStoreRelocationSource, localadapter.SessionStoreRelocationSource) {
	authorizedOld := localadapter.SessionStoreRelocationSource{
		Kind:     localadapter.SessionStoreRelocationSourceOldGlobal,
		Sessions: paths.OldSessions, StagedBlobs: paths.OldBlobs, WriteTombstone: true,
	}
	var sources []localadapter.SessionStoreRelocationSource
	if len(paths.InTreeMaterial) > 0 || len(paths.InTreeLegacySessions) > 0 ||
		paths.InTreeTombstoneNeedsUpdate {
		sources = append(sources, localadapter.SessionStoreRelocationSource{
			Kind:           localadapter.SessionStoreRelocationSourceInTree,
			Sessions:       filepath.Join(sddDir, "sessions"),
			StagedBlobs:    filepath.Join(sddDir, "staged-blobs"),
			WriteTombstone: true,
		})
	}
	if paths.DesiredKey != paths.OldKey &&
		(len(paths.OldMaterial) > 0 || len(paths.OldLegacySessions) > 0 || paths.OldTombstone != nil) {
		sources = append(sources, authorizedOld)
	}
	return sources, authorizedOld
}

func sessionStoreIdentity(existing *model.PerRepoConfig, remoteURL string) (*model.PerRepoConfig, sdd.ProjectID) {
	cfg := &model.PerRepoConfig{}
	if existing != nil {
		*cfg = *existing
	}
	if cfg.RepoID == "" {
		if repoID, err := model.DeriveRepoID(remoteURL); err == nil {
			cfg.RepoID = repoID
		}
	}
	project := sdd.ProjectID("local")
	if cfg.RepoID != "" {
		project = sdd.ProjectID(cfg.RepoID)
	}
	return cfg, project
}

// resolveLocalStorePaths is the one composition-root definition of routing.
// Old-global payload starts the bounded old-key→repo-ID hold; its persisted
// pending marker maintains that hold after the payload disappears mid-move.
// In-tree payload alone never changes repo-ID runtime routing.
func resolveLocalStorePaths(sddDir string, cfg *model.PerRepoConfig, locations repos.Locations) (localStorePaths, error) {
	if !filepath.IsAbs(locations.StateRoot) {
		return localStorePaths{}, fmt.Errorf("XDG state root must be absolute: %q", locations.StateRoot)
	}
	repoID := repoIDOf(cfg)
	if repoID != "" {
		if err := model.ValidateRepoID(repoID); err != nil {
			return localStorePaths{}, fmt.Errorf("invalid repo_id for session store: %w", err)
		}
	}
	stableRoot, err := git.StableRepoRoot(filepath.Dir(sddDir))
	if err != nil {
		return localStorePaths{}, err
	}
	oldKey := index.RepoKey("", stableRoot)
	desiredKey := index.RepoKey(repoID, stableRoot)
	oldSessions, err := confinedStatePath(locations.StateRoot, "sessions", oldKey)
	if err != nil {
		return localStorePaths{}, err
	}
	oldBlobs, err := confinedStatePath(locations.StateRoot, "staged-blobs", oldKey)
	if err != nil {
		return localStorePaths{}, err
	}
	desiredSessions, err := confinedStatePath(locations.StateRoot, "sessions", desiredKey)
	if err != nil {
		return localStorePaths{}, err
	}
	desiredBlobs, err := confinedStatePath(locations.StateRoot, "staged-blobs", desiredKey)
	if err != nil {
		return localStorePaths{}, err
	}
	inTree, err := localadapter.SessionStoreMaterial(
		filepath.Join(sddDir, "sessions"), filepath.Join(sddDir, "staged-blobs"),
	)
	if err != nil {
		return localStorePaths{}, fmt.Errorf("inspecting in-tree session store: %w", err)
	}
	inTreeMigrator, err := localadapter.NewFilesystemLegacySessionMigrator(
		filepath.Join(sddDir, "sessions"), filepath.Join(sddDir, "staged-blobs"),
		"local", "local",
	)
	if err != nil {
		return localStorePaths{}, fmt.Errorf("inspecting in-tree legacy migrations: %w", err)
	}
	inTreeLegacySessions, err := inTreeMigrator.ListPendingLegacySessions(context.Background())
	inTreeCloseErr := inTreeMigrator.Close()
	if err := errors.Join(err, inTreeCloseErr); err != nil {
		return localStorePaths{}, fmt.Errorf("inspecting in-tree legacy migrations: %w", err)
	}
	oldMaterial, err := localadapter.SessionStoreMaterial(oldSessions, oldBlobs)
	if err != nil {
		return localStorePaths{}, fmt.Errorf("inspecting identity-less global session store: %w", err)
	}
	var oldLegacySessions []string
	if repoID != "" && desiredKey != oldKey {
		oldMigrator, err := localadapter.NewFilesystemLegacySessionMigratorAtStateRoot(
			locations.StateRoot, oldSessions, oldBlobs, "local", "local",
		)
		if err != nil {
			return localStorePaths{}, fmt.Errorf("inspecting identity-less legacy migrations: %w", err)
		}
		oldLegacySessions, err = oldMigrator.ListPendingLegacySessions(context.Background())
		closeErr := oldMigrator.Close()
		if err := errors.Join(err, closeErr); err != nil {
			return localStorePaths{}, fmt.Errorf("inspecting identity-less legacy migrations: %w", err)
		}
	}
	inTreeTombstone, err := localadapter.ReadSessionRelocationTombstone(filepath.Join(sddDir, "sessions"))
	if err != nil {
		return localStorePaths{}, err
	}
	oldTombstone, err := localadapter.ReadSessionRelocationTombstone(oldSessions)
	if err != nil {
		return localStorePaths{}, err
	}
	inTreeTombstoneNeedsUpdate := false
	if repoID == "" || desiredKey == oldKey {
		if inTreeTombstone != nil && !tombstoneTargets(
			inTreeTombstone, "local", desiredSessions, desiredBlobs,
		) {
			return localStorePaths{}, fmt.Errorf("in-tree session relocation tombstone does not match current local routing")
		}
		if oldTombstone != nil && !tombstoneTargets(
			oldTombstone, "local", desiredSessions, desiredBlobs,
		) {
			return localStorePaths{}, fmt.Errorf("global session relocation tombstone does not match current local routing")
		}
	} else {
		if inTreeTombstone != nil {
			switch {
			case tombstoneTargets(inTreeTombstone, sdd.ProjectID(repoID), desiredSessions, desiredBlobs):
			case tombstoneTargets(inTreeTombstone, "local", oldSessions, oldBlobs):
				inTreeTombstoneNeedsUpdate = true
			default:
				return localStorePaths{}, fmt.Errorf(
					"in-tree session relocation tombstone targets a different repository identity or store",
				)
			}
		}
		if oldTombstone != nil && !tombstoneTargets(
			oldTombstone, sdd.ProjectID(repoID), desiredSessions, desiredBlobs,
		) {
			return localStorePaths{}, fmt.Errorf(
				"identity-less global session relocation tombstone targets a different repository identity or store",
			)
		}
	}
	transition, err := localadapter.ReadSessionIdentityTransition(oldSessions)
	if err != nil {
		return localStorePaths{}, err
	}
	if transition != nil {
		if transition.OldKey != oldKey || transition.OldSessions != oldSessions || transition.OldBlobs != oldBlobs ||
			transition.NewKey != desiredKey || transition.TargetProject != sdd.ProjectID(repoID) ||
			transition.CurrentSessions == "" || transition.CurrentBlobs == "" {
			return localStorePaths{}, fmt.Errorf("session identity transition marker does not match current repository identity")
		}
		switch transition.State {
		case localadapter.SessionIdentityTransitionPending:
			if transition.CurrentSessions != oldSessions || transition.CurrentBlobs != oldBlobs {
				return localStorePaths{}, fmt.Errorf("pending session identity transition does not route to its old store")
			}
		case localadapter.SessionIdentityTransitionCutover, localadapter.SessionIdentityTransitionCompleted:
			if transition.CurrentSessions != desiredSessions || transition.CurrentBlobs != desiredBlobs {
				return localStorePaths{}, fmt.Errorf("completed session identity transition does not route to its current store")
			}
		}
	}
	_, manifestErr := os.Lstat(filepath.Join(desiredSessions, localadapter.SessionRelocationManifest))
	interrupted := manifestErr == nil
	if manifestErr != nil && !errors.Is(manifestErr, fs.ErrNotExist) {
		return localStorePaths{}, manifestErr
	}

	result := localStorePaths{
		Sessions: desiredSessions, StagedBlobs: desiredBlobs, RepoKey: desiredKey,
		DesiredSessions: desiredSessions, DesiredBlobs: desiredBlobs, DesiredKey: desiredKey,
		OldSessions: oldSessions, OldBlobs: oldBlobs, OldKey: oldKey,
		StableRepoAuthority: stableRoot,
		Transition:          transition, Interrupted: interrupted,
		InTreeMaterial: inTree, InTreeLegacySessions: inTreeLegacySessions,
		OldMaterial: oldMaterial, OldLegacySessions: oldLegacySessions,
		InTreeTombstone: inTreeTombstone, OldTombstone: oldTombstone,
		InTreeTombstoneNeedsUpdate: inTreeTombstoneNeedsUpdate,
	}
	if repoID != "" && desiredKey != oldKey {
		switch {
		case transition != nil &&
			(transition.State == localadapter.SessionIdentityTransitionCutover ||
				transition.State == localadapter.SessionIdentityTransitionCompleted):
			// Cutover is monotonic. Reappearing old/in-tree material is
			// explicit recovery input, never permission to route new runtime
			// sessions back to the abandoned identity.
		case len(inTree) > 0 || len(inTreeLegacySessions) > 0 ||
			len(oldMaterial) > 0 || len(oldLegacySessions) > 0 ||
			inTreeTombstoneNeedsUpdate:
			result.PendingIdentity = true
		case transition != nil && transition.State == localadapter.SessionIdentityTransitionPending:
			result.PendingIdentity = true
		}
		if result.PendingIdentity {
			if transition == nil {
				transition = &localadapter.SessionIdentityTransition{
					Version: localadapter.SessionIdentityTransitionVersion,
					State:   localadapter.SessionIdentityTransitionPending,
					OldKey:  oldKey, NewKey: desiredKey,
					OldSessions: oldSessions, OldBlobs: oldBlobs,
					CurrentSessions: oldSessions, CurrentBlobs: oldBlobs,
					TargetProject: sdd.ProjectID(repoID),
				}
				result.Transition = transition
			}
			result.Sessions, result.StagedBlobs, result.RepoKey = oldSessions, oldBlobs, oldKey
		}
	}
	return result, nil
}

func tombstoneTargets(
	tombstone *localadapter.SessionRelocationTombstoneRecord,
	project sdd.ProjectID,
	sessions string,
	blobs string,
) bool {
	return tombstone.TargetProject == project &&
		filepath.Clean(tombstone.TargetSessions) == filepath.Clean(sessions) &&
		filepath.Clean(tombstone.TargetStagedBlobs) == filepath.Clean(blobs)
}

func confinedStatePath(stateRoot, category, key string) (string, error) {
	if category == "" || key == "" {
		return "", fmt.Errorf("session store category and repo key are required")
	}
	if category == "." || category == ".." || filepath.Base(category) != category || strings.ContainsAny(category, `/\`) {
		return "", fmt.Errorf("invalid session store category %q", category)
	}
	for _, segment := range strings.Split(filepath.ToSlash(key), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid session store repo key %q", key)
		}
	}
	categoryRoot := filepath.Clean(filepath.Join(stateRoot, category))
	target := filepath.Clean(filepath.Join(categoryRoot, filepath.FromSlash(key)))
	rel, err := filepath.Rel(categoryRoot, target)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("session store path escapes %s category: %q", category, key)
	}
	return target, nil
}

func routedSessionProject(cfg *model.PerRepoConfig, paths localStorePaths) sdd.ProjectID {
	if paths.PendingIdentity {
		return "local"
	}
	if repoID := repoIDOf(cfg); repoID != "" {
		return sdd.ProjectID(repoID)
	}
	return "local"
}

func persistentIndexRepoKey(paths localStorePaths) string {
	return paths.DesiredKey
}

func currentSessionRelocationNotice(sddDir string, locations repos.Locations) (string, error) {
	cfg, err := resolveConfigAt(sddDir)
	if err != nil {
		return "", fmt.Errorf("reloading current configuration for session relocation notice: %w", err)
	}
	return sessionRelocationNotice(sddDir, cfg, locations)
}

func sessionRelocationNotice(sddDir string, cfg *model.PerRepoConfig, locations repos.Locations) (string, error) {
	paths, err := resolveLocalStorePaths(sddDir, cfg, locations)
	if err != nil {
		return "", err
	}
	switch {
	case paths.Interrupted:
		return "Session relocation recovery required: an acknowledged offline migration was interrupted. Stop running `sdd serve` processes, restart agent sessions, then run `sdd init --migrate-sessions` to resume manifest-driven cleanup.", nil
	case paths.PendingIdentity:
		return fmt.Sprintf(
			"Session identity transition pending: routing remains on %s until the acknowledged offline move to %s completes. Stop running `sdd serve` processes, restart agent sessions, then run `sdd init --migrate-sessions`.",
			paths.OldKey, paths.DesiredKey,
		), nil
	case len(paths.InTreeMaterial) > 0 || len(paths.InTreeLegacySessions) > 0:
		count := len(paths.InTreeMaterial) + len(paths.InTreeLegacySessions)
		return fmt.Sprintf(
			"Session relocation required: %d in-tree session payload or pending migration transaction(s) remain under %s. Stop running `sdd serve` processes, restart agent sessions, then run `sdd init --migrate-sessions`; this server does not merge those leftovers.",
			count, sddDir,
		), nil
	case paths.Transition != nil &&
		(paths.Transition.State == localadapter.SessionIdentityTransitionCutover ||
			paths.Transition.State == localadapter.SessionIdentityTransitionCompleted) &&
		(len(paths.OldMaterial) > 0 || len(paths.OldLegacySessions) > 0):
		return fmt.Sprintf(
			"Abandoned session state reappeared under old store %s after identity cutover. The current server continues using %s; stop old servers and run `sdd init --migrate-sessions` for explicit recovery.",
			paths.OldKey, paths.DesiredKey,
		), nil
	default:
		return "", nil
	}
}
