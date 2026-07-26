package local

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	app "github.com/networkteam/sdd/application"
)

const (
	// SessionRelocationTombstone is retained in each abandoned session root.
	SessionRelocationTombstone = ".relocated"
	// SessionRelocationManifest is durable only while publication or source
	// cleanup is incomplete.
	SessionRelocationManifest = ".relocation-manifest.json"
	// SessionRelocationLock serializes relocation planning, publication,
	// source cleanup, and transition completion across processes.
	SessionRelocationLock = ".relocation.lock"
	// SessionRelocationTempDir contains publication temps and is never payload.
	SessionRelocationTempDir = ".relocation-tmp"
	// SessionRelocationQuarantineDir durably holds source payloads while
	// target naming is revalidated after the destructive half of cleanup.
	SessionRelocationQuarantineDir = ".relocation-quarantine"

	sessionRelocationManifestVersion = 2
)

type RelocationItem struct {
	Source      string
	Destination string
}

type SessionStoreRelocationSourceKind string

const (
	SessionStoreRelocationSourceInTree    SessionStoreRelocationSourceKind = "in_tree"
	SessionStoreRelocationSourceOldGlobal SessionStoreRelocationSourceKind = "old_global"
)

type SessionStoreRelocationSource struct {
	Kind           SessionStoreRelocationSourceKind `json:"kind"`
	Sessions       string                           `json:"sessions"`
	StagedBlobs    string                           `json:"staged_blobs"`
	WriteTombstone bool                             `json:"write_tombstone"`
}

type InTreeSessionSourceAuthorizer func(
	ctx context.Context,
	stableRepoAuthority string,
	sessions string,
	stagedBlobs string,
) error

type FilesystemSessionStoreRelocatorOptions struct {
	Sources                   []SessionStoreRelocationSource
	AuthorizedOldGlobalSource *SessionStoreRelocationSource
	TrustedStateRoot          string
	StableRepoAuthority       string
	AuthorizeInTreeSource     InTreeSessionSourceAuthorizer
	TargetSessions            string
	TargetBlobs               string
	TargetProject             app.ProjectID
	Transformer               app.SessionIdentityTransformer
	Transition                *SessionIdentityTransition
}

type SessionRelocationTombstoneRecord struct {
	Version           int           `json:"version"`
	TargetProject     app.ProjectID `json:"target_project"`
	TargetSessions    string        `json:"target_sessions"`
	TargetStagedBlobs string        `json:"target_staged_blobs"`
	RelocatedAt       time.Time     `json:"relocated_at"`
}

type relocationPayloadState string

const (
	relocationPayloadPlanned     relocationPayloadState = "planned"
	relocationPayloadPublished   relocationPayloadState = "published"
	relocationPayloadQuarantined relocationPayloadState = "source_quarantined"
	relocationPayloadDeleted     relocationPayloadState = "source_deleted"
)

type relocationManifestPayload struct {
	Source      string                 `json:"source"`
	Destination string                 `json:"destination"`
	SessionLog  bool                   `json:"session_log,omitempty"`
	Digest      string                 `json:"digest"`
	Size        int64                  `json:"size"`
	Mode        uint32                 `json:"mode"`
	State       relocationPayloadState `json:"state"`
	Quarantine  string                 `json:"quarantine"`
	Restore     string                 `json:"restore"`
}

type relocationManifest struct {
	Version             int                            `json:"version"`
	TransformerVersion  uint32                         `json:"transformer_version"`
	TargetProject       app.ProjectID                  `json:"target_project"`
	TargetSessions      string                         `json:"target_sessions"`
	TargetBlobs         string                         `json:"target_blobs"`
	StableRepoAuthority string                         `json:"stable_repo_authority"`
	Sources             []SessionStoreRelocationSource `json:"sources"`
	Payloads            []relocationManifestPayload    `json:"payloads"`
	UpdatedAt           time.Time                      `json:"updated_at"`
}

// FilesystemSessionStoreRelocator performs one acknowledged offline aggregate
// move. The manifest makes publication and cleanup idempotently resumable.
type FilesystemSessionStoreRelocator struct {
	options       FilesystemSessionStoreRelocatorOptions
	now           func() time.Time
	targets       *relocationTargetRoots
	transition    *trustedDirectoryChain
	activeSources []SessionStoreRelocationSource
	pinnedSources []pinnedRelocationSource

	// Crash-injection seams used only by package tests. Panics deliberately
	// model process death, bypassing ordinary rollback.
	afterPublish                    func()
	afterTempSync                   func()
	afterSourceDelete               func()
	beforeIrreversible              func(stage string)
	afterSourceQuarantine           func()
	afterSourceQuarantineRename     func()
	afterQuarantineRemove           func()
	afterRestoreTempSync            func()
	afterRestoreLink                func()
	beforeSourceRestoreLink         func()
	afterSourceRestoreLink          func()
	afterTargetRollbackQuarantine   func(string)
	afterTargetRollbackRestoreLink  func()
	beforeTargetRollbackFinalUnlink func()
	syncPublishedTargetDirectory    func(*os.Root, string) error
	beforeTransitionWrite           func()
	beforeSourcePrune               func(path string)
}

type pinnedSourceCategory struct {
	path        string
	parent      *os.Root
	ownsParent  bool
	base        string
	root        *os.Root
	identity    fs.FileInfo
	chain       *trustedDirectoryChain
	absent      bool
	strictChain bool
}

type pinnedRelocationSource struct {
	source          SessionStoreRelocationSource
	sessions        pinnedSourceCategory
	blobs           pinnedSourceCategory
	immutableAbsent bool
}

type relocationTargetRoots struct {
	authority             *trustedStateAuthority
	sessionsCategoryChain *trustedDirectoryChain
	blobsCategoryChain    *trustedDirectoryChain
	sessionsStoreChain    *trustedDirectoryChain
	blobsStoreChain       *trustedDirectoryChain
	sessionsCategory      *os.Root
	blobsCategory         *os.Root
	sessions              *os.Root
	blobs                 *os.Root
}

func NewFilesystemSessionStoreRelocator(options FilesystemSessionStoreRelocatorOptions) (*FilesystemSessionStoreRelocator, error) {
	if options.TrustedStateRoot == "" ||
		options.TargetSessions == "" || options.TargetBlobs == "" ||
		options.TargetProject == "" || options.Transformer == nil || options.StableRepoAuthority == "" {
		return nil, fmt.Errorf("sdd: trusted category roots, relocation targets, stable repository authority, target project, and identity transformer are required")
	}
	if !pathAtOrInside(filepath.Join(options.TrustedStateRoot, "sessions"), options.TargetSessions) ||
		!pathAtOrInside(filepath.Join(options.TrustedStateRoot, "staged-blobs"), options.TargetBlobs) {
		return nil, fmt.Errorf("sdd: relocation targets must be confined to categories under the trusted state root")
	}
	if options.AuthorizedOldGlobalSource != nil {
		if err := validateAuthorizedOldGlobalSource(
			options.TrustedStateRoot, *options.AuthorizedOldGlobalSource,
		); err != nil {
			return nil, err
		}
	}
	for _, source := range options.Sources {
		switch source.Kind {
		case SessionStoreRelocationSourceInTree:
			if options.AuthorizeInTreeSource == nil {
				return nil, fmt.Errorf("sdd: in-tree relocation sources require an authorizer")
			}
		case SessionStoreRelocationSourceOldGlobal:
			if options.AuthorizedOldGlobalSource == nil ||
				!sameRelocationSource(source, *options.AuthorizedOldGlobalSource) {
				return nil, fmt.Errorf(
					"sdd: old_global relocation source does not match the explicitly authorized confined source: %s",
					source.Sessions,
				)
			}
		default:
			return nil, fmt.Errorf("sdd: unsupported relocation source kind %q", source.Kind)
		}
		if source.Sessions == "" || source.StagedBlobs == "" {
			return nil, fmt.Errorf("sdd: every relocation source requires session and staged-blob directories")
		}
		if source.Sessions == options.TargetSessions || source.StagedBlobs == options.TargetBlobs {
			return nil, fmt.Errorf("sdd: relocation source and target stores must differ")
		}
	}
	return &FilesystemSessionStoreRelocator{options: options, now: time.Now}, nil
}

func validateAuthorizedOldGlobalSource(
	trustedStateRoot string,
	source SessionStoreRelocationSource,
) error {
	if source.Kind != SessionStoreRelocationSourceOldGlobal {
		return fmt.Errorf("sdd: authorized old-global source has kind %q", source.Kind)
	}
	var mapping string
	for _, item := range []struct {
		category string
		path     string
	}{
		{category: "sessions", path: source.Sessions},
		{category: "staged-blobs", path: source.StagedBlobs},
	} {
		categoryRoot := filepath.Join(trustedStateRoot, item.category)
		relative, err := filepath.Rel(categoryRoot, item.path)
		if err != nil || !filepath.IsLocal(relative) || relative == "." {
			return fmt.Errorf(
				"sdd: authorized old-global %s source escapes the trusted state category",
				item.category,
			)
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) != 2 || parts[0] != "local" || len(parts[1]) != 12 {
			return fmt.Errorf(
				"sdd: authorized old-global %s source must use local/<12-hex> mapping",
				item.category,
			)
		}
		if _, err := hex.DecodeString(parts[1]); err != nil {
			return fmt.Errorf(
				"sdd: authorized old-global %s source has an invalid local hash",
				item.category,
			)
		}
		if mapping == "" {
			mapping = filepath.ToSlash(relative)
		} else if mapping != filepath.ToSlash(relative) {
			return fmt.Errorf("sdd: authorized old-global session and blob sources use different local mappings")
		}
	}
	return nil
}

func (r *FilesystemSessionStoreRelocator) EnsurePending(context.Context) (resultErr error) {
	if r.options.Transition == nil || r.options.Transition.State != SessionIdentityTransitionPending {
		return nil
	}
	targets, err := openRelocationTargetRoots(
		r.options.TrustedStateRoot, r.options.TargetSessions, r.options.TargetBlobs, true,
	)
	if err != nil {
		return err
	}
	r.targets = targets
	defer func() {
		resultErr = errors.Join(resultErr, r.closeTransitionRoot())
		resultErr = errors.Join(resultErr, targets.close())
		r.targets = nil
	}()
	relocationLock, err := r.acquireRelocationLock(targets)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, relocationLock.Unlock())
	}()
	return r.ensurePendingLocked()
}

func (r *FilesystemSessionStoreRelocator) acquireRelocationLock(targets *relocationTargetRoots) (*pinnedFileLock, error) {
	lockFile, err := targets.sessions.OpenFile(
		SessionRelocationLock, os.O_CREATE|os.O_RDWR, 0o600,
	)
	if err != nil {
		return nil, fmt.Errorf("opening pinned session relocation lock: %w", err)
	}
	relocationLock, err := tryPinnedFileLock(lockFile)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("acquiring session relocation lock: %w", err), lockFile.Close())
	}
	if !relocationLock.locked {
		_ = relocationLock.Unlock()
		return nil, fmt.Errorf("another session-store relocation is already in progress for %s", r.options.TargetSessions)
	}
	return relocationLock, nil
}

func (r *FilesystemSessionStoreRelocator) ensurePendingLocked() error {
	expected := r.options.Transition
	if expected == nil || expected.State != SessionIdentityTransitionPending {
		return nil
	}
	root, closeRoot, err := r.openTransitionRoot(true)
	if err != nil {
		return err
	}
	defer func() { _ = closeRoot() }()
	current, err := readSessionIdentityTransitionRoot(root, expected.OldSessions)
	if err != nil {
		return err
	}
	if err := r.revalidateBeforeTransitionWrite(); err != nil {
		return err
	}
	if current == nil {
		if err := r.revalidateBeforeTransitionWrite(); err != nil {
			return err
		}
		return writeSessionIdentityTransitionRoot(root, *expected)
	}
	if !sameTransitionIdentity(*current, *expected) {
		return fmt.Errorf("existing session identity transition does not match the requested transition")
	}
	switch current.State {
	case SessionIdentityTransitionPending:
		r.options.Transition = current
		return nil
	case SessionIdentityTransitionCutover, SessionIdentityTransitionCompleted:
		r.options.Transition = current
		return nil
	default:
		return fmt.Errorf("unsupported session identity transition state %q", current.State)
	}
}

func (r *FilesystemSessionStoreRelocator) revalidateBeforeTransitionWrite() error {
	if r.beforeTransitionWrite != nil {
		r.beforeTransitionWrite()
	}
	if err := r.targets.revalidateConfiguredPaths(
		r.options.TargetSessions, r.options.TargetBlobs,
	); err != nil {
		return err
	}
	if r.transition != nil {
		if err := r.transition.revalidate(); err != nil {
			return fmt.Errorf("session identity transition root was rebound: %w", err)
		}
	}
	return r.revalidatePinnedSources()
}

func (r *FilesystemSessionStoreRelocator) openTransitionRoot(create bool) (*os.Root, func() error, error) {
	expected := r.options.Transition
	if expected == nil {
		return nil, func() error { return nil }, fmt.Errorf("session identity transition is not configured")
	}
	if category, err := r.pinnedCategory(expected.OldSessions); err == nil && !category.absent && category.root != nil {
		if err := revalidatePinnedCategoryIdentity(category); err != nil {
			return nil, func() error { return nil }, err
		}
		return category.root, func() error { return nil }, nil
	}
	relative, err := filepath.Rel(filepath.Join(r.options.TrustedStateRoot, "sessions"), expected.OldSessions)
	if err != nil || !filepath.IsLocal(relative) {
		return nil, func() error { return nil }, fmt.Errorf(
			"session identity transition root escapes trusted sessions category: %s",
			expected.OldSessions,
		)
	}
	if relative == "." {
		return r.targets.sessionsCategory, func() error { return nil }, nil
	}
	if r.transition == nil {
		chain, err := openTrustedRelativeDirectoryChain(
			r.targets.sessionsCategory, relative, create, false, 0o755,
			expected.OldSessions,
		)
		if err != nil {
			return nil, func() error { return nil }, err
		}
		r.transition = chain
	} else if err := r.transition.revalidate(); err != nil {
		return nil, func() error { return nil }, err
	}
	return r.transition.root(), func() error { return nil }, nil
}

func (r *FilesystemSessionStoreRelocator) closeTransitionRoot() error {
	if r.transition == nil {
		return nil
	}
	err := r.transition.close()
	r.transition = nil
	return err
}

func (r *FilesystemSessionStoreRelocator) revalidateBeforeIrreversible(stage string) error {
	if r.beforeIrreversible != nil {
		r.beforeIrreversible(stage)
	}
	if err := r.targets.revalidateConfiguredPaths(
		r.options.TargetSessions, r.options.TargetBlobs,
	); err != nil {
		return err
	}
	if r.transition != nil {
		if err := r.transition.revalidate(); err != nil {
			return fmt.Errorf("session identity transition root was rebound: %w", err)
		}
	}
	return r.revalidatePinnedSources()
}

func sameTransitionIdentity(left, right SessionIdentityTransition) bool {
	return left.Version == right.Version &&
		left.OldKey == right.OldKey &&
		left.NewKey == right.NewKey &&
		left.OldSessions == right.OldSessions &&
		left.OldBlobs == right.OldBlobs &&
		left.TargetProject == right.TargetProject
}

// SessionStoreMaterial lists durable payload only. The top-level relocation
// controls and top-level <session>.jsonl.lock files are metadata/noise;
// nested .lock attachments are payload and move with their session.
func SessionStoreMaterial(sessionsDir, blobsDir string) ([]string, error) {
	sessions, err := storePayloadFiles(sessionsDir, true)
	if err != nil {
		return nil, err
	}
	blobs, err := storePayloadFiles(blobsDir, false)
	if err != nil {
		return nil, err
	}
	material := append(sessions, blobs...)
	sort.Strings(material)
	return material, nil
}

func storePayloadFiles(root string, sessionRoot bool) ([]string, error) {
	info, err := os.Lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading session-store directory %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("session-store path %s is not a directory", root)
	}
	var files []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filepath.Dir(path) == root &&
			(filepath.Base(path) == SessionRelocationTempDir ||
				filepath.Base(path) == SessionRelocationQuarantineDir ||
				filepath.Base(path) == legacyMigrationControlDir) {
			if !entry.IsDir() {
				return fmt.Errorf("session relocation temp control %s is not a directory", path)
			}
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		if sessionRoot && isSessionStoreControl(root, path) {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("session store contains non-regular control file %s (%s)", path, info.Mode().Type())
			}
			if filepath.Base(path) == SessionRelocationTombstone {
				if _, err := ReadSessionRelocationTombstone(root); err != nil {
					return err
				}
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("session store contains non-regular payload %s (%s)", path, info.Mode().Type())
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning session store %s: %w", root, err)
	}
	sort.Strings(files)
	return files, nil
}

func isSessionStoreControl(root, path string) bool {
	if filepath.Dir(path) != root {
		return false
	}
	switch filepath.Base(path) {
	case SessionRelocationTombstone, SessionRelocationManifest, SessionRelocationLock, SessionIdentityTransitionMarker:
		return true
	}
	return isTopLevelSessionLock(root, path)
}

func isTopLevelSessionLock(root, path string) bool {
	if filepath.Dir(path) != root {
		return false
	}
	base := filepath.Base(path)
	return len(base) > len(".jsonl.lock") && strings.HasSuffix(base, ".jsonl.lock")
}

func (r *FilesystemSessionStoreRelocator) Relocate(ctx context.Context, onMoved, onSkipped func(source, destination string)) (resultErr error) {
	targets, err := openRelocationTargetRoots(
		r.options.TrustedStateRoot, r.options.TargetSessions, r.options.TargetBlobs, true,
	)
	if err != nil {
		return err
	}
	r.targets = targets
	defer func() {
		resultErr = errors.Join(resultErr, r.closeTransitionRoot())
		resultErr = errors.Join(resultErr, r.closePinnedSources())
		resultErr = errors.Join(resultErr, targets.close())
		r.targets = nil
		r.activeSources = nil
	}()
	relocationLock, err := r.acquireRelocationLock(targets)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, relocationLock.Unlock())
	}()
	if err := targets.cleanTemps(); err != nil {
		return fmt.Errorf("cleaning interrupted relocation temps: %w", err)
	}
	manifestPath := filepath.Join(r.options.TargetSessions, SessionRelocationManifest)
	manifest, resumed, err := r.loadOrCreateManifest(ctx, manifestPath, onSkipped)
	if err != nil {
		return err
	}
	r.activeSources = append([]SessionStoreRelocationSource(nil), manifest.Sources...)
	if err := r.cleanRelocationSourceTemps(); err != nil {
		return fmt.Errorf("cleaning originating relocation temps: %w", err)
	}
	if err := r.ensurePendingLocked(); err != nil {
		return fmt.Errorf("establishing session identity transition: %w", err)
	}

	publishedState := make([]bool, len(manifest.Payloads))
	var collisions []RelocationItem
	for _, payload := range manifest.Payloads {
		if err := r.reconcileTargetRollbackControl(payload); err != nil {
			return err
		}
	}
	for index, payload := range manifest.Payloads {
		if sourceErr := r.verifySource(payload); sourceErr != nil {
			return r.rollback(manifestPath, manifest, nil, sourceErr, resumed)
		}
		published, verifyErr := r.verifyPublished(payload)
		if verifyErr == nil {
			publishedState[index] = published
			continue
		}
		var collision *SessionStoreCollisionError
		if errors.As(verifyErr, &collision) {
			collisions = append(collisions, collision.Collisions...)
			continue
		}
		return r.rollback(manifestPath, manifest, nil, verifyErr, resumed)
	}
	if len(collisions) > 0 {
		for _, collision := range collisions {
			if onSkipped != nil {
				onSkipped(collision.Source, collision.Destination)
			}
		}
		return r.rollback(
			manifestPath, manifest, nil,
			&SessionStoreCollisionError{Collisions: collisions}, resumed,
		)
	}

	newlyPublished := make([]int, 0, len(manifest.Payloads))
	for index := range manifest.Payloads {
		if err := ctx.Err(); err != nil {
			return r.rollback(manifestPath, manifest, newlyPublished, err, resumed)
		}
		payload := &manifest.Payloads[index]
		if !publishedState[index] {
			if err := r.publish(*payload); err != nil {
				if errors.Is(err, fs.ErrExist) && onSkipped != nil {
					onSkipped(payload.Source, payload.Destination)
				}
				return r.rollback(manifestPath, manifest, newlyPublished, err, resumed)
			}
			newlyPublished = append(newlyPublished, index)
			if r.afterPublish != nil {
				r.afterPublish()
			}
		}
		if payload.State == relocationPayloadPlanned {
			payload.State = relocationPayloadPublished
			if err := r.writeManifest(manifestPath, manifest); err != nil {
				return r.rollback(manifestPath, manifest, newlyPublished, err, resumed)
			}
		}
	}

	if err := r.revalidateBeforeIrreversible("source_tombstones"); err != nil {
		return fmt.Errorf("revalidating relocation roots before tombstones: %w", err)
	}
	if err := r.writeSourceTombstones(manifest.Sources); err != nil {
		return fmt.Errorf("writing session relocation tombstones: %w", err)
	}
	if err := r.revalidateBeforeIrreversible("cutover"); err != nil {
		return fmt.Errorf("revalidating relocation roots before cutover: %w", err)
	}
	if err := r.writeTransitionState(SessionIdentityTransitionCutover); err != nil {
		return fmt.Errorf("recording session identity cutover: %w", err)
	}

	for index := range manifest.Payloads {
		payload := &manifest.Payloads[index]
		if payload.State == relocationPayloadDeleted {
			if err := r.removePayloadRestore(*payload); err != nil {
				return err
			}
			continue
		}
		if err := r.revalidateBeforeIrreversible("source_delete"); err != nil {
			return fmt.Errorf("revalidating relocation roots before source deletion: %w", err)
		}
		if err := r.quarantineVerifiedSource(*payload); err != nil {
			return fmt.Errorf("quarantining relocated source payload %s: %w", payload.Source, err)
		}
		if payload.State != relocationPayloadQuarantined {
			payload.State = relocationPayloadQuarantined
			if err := r.writeManifest(manifestPath, manifest); err != nil {
				return err
			}
		}
		if err := r.finalizeQuarantinedSource(*payload); err != nil {
			payload.State = relocationPayloadPublished
			manifestErr := r.writeManifest(manifestPath, manifest)
			return errors.Join(
				fmt.Errorf("finalizing relocated source payload %s: %w", payload.Source, err),
				manifestErr,
			)
		}
		if r.afterSourceDelete != nil {
			r.afterSourceDelete()
		}
		payload.State = relocationPayloadDeleted
		if err := r.writeManifest(manifestPath, manifest); err != nil {
			return err
		}
		if err := r.removePayloadRestore(*payload); err != nil {
			return err
		}
		if onMoved != nil {
			onMoved(payload.Source, payload.Destination)
		}
	}
	if err := r.revalidateBeforeIrreversible("source_cleanup"); err != nil {
		return fmt.Errorf("revalidating relocation roots before source cleanup: %w", err)
	}
	if err := r.cleanupPinnedSources(); err != nil {
		return err
	}
	if err := r.revalidateBeforeIrreversible("completed"); err != nil {
		return fmt.Errorf("revalidating relocation roots before transition completion: %w", err)
	}
	if err := r.writeTransitionState(SessionIdentityTransitionCompleted); err != nil {
		return fmt.Errorf("completing session identity transition: %w", err)
	}
	if err := targets.sessions.Remove(SessionRelocationManifest); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := targets.cleanTemps(); err != nil {
		return err
	}
	return syncRootDirectory(targets.sessions, ".")
}

func (r *FilesystemSessionStoreRelocator) pinSources(
	sources []SessionStoreRelocationSource,
	allowFullyAbsent bool,
) error {
	if len(r.pinnedSources) != 0 {
		return fmt.Errorf("relocation sources are already pinned")
	}
	for _, source := range sources {
		r.pinnedSources = append(r.pinnedSources, pinnedRelocationSource{source: source})
		pinned := &r.pinnedSources[len(r.pinnedSources)-1]
		for _, item := range []struct {
			path     string
			category *pinnedSourceCategory
			name     string
		}{
			{path: source.Sessions, category: &pinned.sessions, name: "sessions"},
			{path: source.StagedBlobs, category: &pinned.blobs, name: "staged-blobs"},
		} {
			item.category.path = item.path
			if source.Kind == SessionStoreRelocationSourceOldGlobal {
				if err := r.pinOldGlobalCategory(item.name, item.path, item.category); err != nil {
					_ = r.closePinnedSources()
					return err
				}
				continue
			}
			chain, err := openTrustedAbsoluteDirectoryChain(
				item.path, false, true, 0o755,
			)
			if err != nil {
				_ = r.closePinnedSources()
				return fmt.Errorf("pinning in-tree relocation source %s: %w", item.path, err)
			}
			item.category.chain = chain
			item.category.strictChain = true
			item.category.absent = !chain.complete
			item.category.root = chain.root()
			if item.category.root != nil {
				item.category.identity, err = item.category.root.Stat(".")
				if err != nil {
					_ = r.closePinnedSources()
					return err
				}
			}
		}
		pinned.immutableAbsent = pinned.sessions.absent && pinned.blobs.absent
		if pinned.immutableAbsent && !allowFullyAbsent {
			_ = r.closePinnedSources()
			return fmt.Errorf("new relocation source is absent: %s", source.Sessions)
		}
	}
	return nil
}

func (r *FilesystemSessionStoreRelocator) pinOldGlobalCategory(
	categoryName string,
	path string,
	result *pinnedSourceCategory,
) error {
	if r.targets == nil {
		return fmt.Errorf("pinning old-global source without trusted state authority")
	}
	var category *os.Root
	switch categoryName {
	case "sessions":
		category = r.targets.sessionsCategory
	case "staged-blobs":
		category = r.targets.blobsCategory
	default:
		return fmt.Errorf("unsupported old-global source category %q", categoryName)
	}
	categoryPath := filepath.Join(r.options.TrustedStateRoot, categoryName)
	relative, err := filepath.Rel(categoryPath, path)
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		return fmt.Errorf("old-global source escapes trusted %s category: %s", categoryName, path)
	}
	result.parent = category
	result.base = relative
	result.strictChain = true
	chain, err := openTrustedRelativeDirectoryChain(
		category, relative, false, true, 0o755, path,
	)
	if err != nil {
		return fmt.Errorf("pinning old-global source %s: %w", path, err)
	}
	result.chain = chain
	result.absent = !chain.complete
	result.root = chain.root()
	if result.root != nil {
		result.identity, err = result.root.Stat(".")
		if err != nil {
			_ = chain.close()
			return err
		}
	}
	return nil
}

func (r *FilesystemSessionStoreRelocator) closePinnedSources() error {
	var errs []error
	for index := range r.pinnedSources {
		for _, category := range []*pinnedSourceCategory{
			&r.pinnedSources[index].sessions, &r.pinnedSources[index].blobs,
		} {
			if category.chain != nil {
				errs = append(errs, category.chain.close())
				category.chain = nil
				category.root = nil
			} else if category.root != nil {
				errs = append(errs, category.root.Close())
				category.root = nil
			}
			if category.parent != nil && category.ownsParent {
				errs = append(errs, category.parent.Close())
			}
			category.parent = nil
		}
	}
	r.pinnedSources = nil
	return errors.Join(errs...)
}

func (r *FilesystemSessionStoreRelocator) revalidatePinnedSources() error {
	for index := range r.pinnedSources {
		source := &r.pinnedSources[index]
		for _, category := range []*pinnedSourceCategory{&source.sessions, &source.blobs} {
			if err := revalidatePinnedCategoryIdentity(category); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *FilesystemSessionStoreRelocator) cleanRelocationSourceTemps() error {
	var errs []error
	for index := range r.pinnedSources {
		source := &r.pinnedSources[index]
		if source.immutableAbsent {
			continue
		}
		for _, category := range []*pinnedSourceCategory{&source.sessions, &source.blobs} {
			if category.absent {
				continue
			}
			removeErr := category.root.RemoveAll(SessionRelocationTempDir)
			if removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
				errs = append(errs, removeErr)
			}
		}
	}
	return errors.Join(errs...)
}

func (r *FilesystemSessionStoreRelocator) loadOrCreateManifest(ctx context.Context, path string, onSkipped func(string, string)) (*relocationManifest, bool, error) {
	manifest, err := r.readRelocationManifest()
	if err != nil {
		return nil, false, err
	}
	if manifest != nil {
		if err := r.validateManifest(ctx, *manifest); err != nil {
			return nil, true, err
		}
		return manifest, true, nil
	}
	if err := r.pinSources(r.options.Sources, false); err != nil {
		return nil, false, err
	}
	if err := r.authorizePinnedSources(ctx, r.options.Sources, r.options.StableRepoAuthority); err != nil {
		return nil, false, err
	}
	payloads, duplicateCollisions, err := r.planPayloads(r.options.Sources)
	if err != nil {
		return nil, false, err
	}
	var collisions []RelocationItem
	collisions = append(collisions, duplicateCollisions...)
	for _, payload := range payloads {
		targetRoot, targetRelative, rootErr := r.targetRoot(payload.Destination)
		if rootErr != nil {
			return nil, false, rootErr
		}
		if _, err := targetRoot.Lstat(targetRelative); err == nil {
			collisions = append(collisions, RelocationItem{Source: payload.Source, Destination: payload.Destination})
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, false, fmt.Errorf("checking relocation target %s: %w", payload.Destination, err)
		}
	}
	if len(collisions) > 0 {
		for _, collision := range collisions {
			if onSkipped != nil {
				onSkipped(collision.Source, collision.Destination)
			}
		}
		return nil, false, &SessionStoreCollisionError{Collisions: collisions}
	}
	manifest = &relocationManifest{
		Version: sessionRelocationManifestVersion, TransformerVersion: r.options.Transformer.Version(),
		TargetProject: r.options.TargetProject, TargetSessions: r.options.TargetSessions, TargetBlobs: r.options.TargetBlobs,
		StableRepoAuthority: r.options.StableRepoAuthority,
		Sources:             append([]SessionStoreRelocationSource(nil), r.options.Sources...), Payloads: payloads,
	}
	if err := r.writeManifest(path, manifest); err != nil {
		return nil, false, err
	}
	return manifest, false, nil
}

func (r *FilesystemSessionStoreRelocator) validateManifest(ctx context.Context, manifest relocationManifest) error {
	if manifest.Version != sessionRelocationManifestVersion {
		return fmt.Errorf("unsupported session relocation manifest version %d", manifest.Version)
	}
	if manifest.TransformerVersion != r.options.Transformer.Version() {
		return fmt.Errorf("session relocation manifest transformer version %d does not match current version %d", manifest.TransformerVersion, r.options.Transformer.Version())
	}
	if manifest.TargetProject != r.options.TargetProject ||
		manifest.TargetSessions != r.options.TargetSessions || manifest.TargetBlobs != r.options.TargetBlobs {
		return fmt.Errorf("session relocation manifest targets a different store")
	}
	if filepath.Clean(manifest.StableRepoAuthority) != filepath.Clean(r.options.StableRepoAuthority) {
		return fmt.Errorf(
			"session relocation manifest belongs to a different stable repository authority: manifest=%s current=%s",
			manifest.StableRepoAuthority, r.options.StableRepoAuthority,
		)
	}
	if err := r.pinSources(manifest.Sources, true); err != nil {
		return err
	}
	if err := r.authorizeManifestSources(ctx, manifest); err != nil {
		return err
	}
	freshPayloads, duplicateCollisions, err := r.planPayloads(manifest.Sources)
	if err != nil {
		return fmt.Errorf("validating current relocation plan: %w", err)
	}
	if len(duplicateCollisions) > 0 {
		return &SessionStoreCollisionError{Collisions: duplicateCollisions}
	}
	freshBySource := make(map[string]relocationManifestPayload, len(freshPayloads))
	for _, payload := range freshPayloads {
		freshBySource[payload.Source] = payload
	}
	for _, currentSource := range r.options.Sources {
		found := false
		for _, persistedSource := range manifest.Sources {
			if sameRelocationSource(currentSource, persistedSource) {
				found = true
				break
			}
		}
		if !found && (currentSource.WriteTombstone || relocationSourceExists(currentSource)) {
			return fmt.Errorf(
				"current relocation plan contains an unmanifested %s source: %s",
				currentSource.Kind, currentSource.Sessions,
			)
		}
	}

	destinations := map[string]bool{}
	sources := map[string]bool{}
	for _, payload := range manifest.Payloads {
		if payload.Source == "" || payload.Destination == "" || payload.Size < 0 {
			return fmt.Errorf("session relocation manifest contains an incomplete payload")
		}
		decoded, err := hex.DecodeString(payload.Digest)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("session relocation manifest contains an invalid payload digest")
		}
		switch payload.State {
		case relocationPayloadPlanned, relocationPayloadPublished,
			relocationPayloadQuarantined, relocationPayloadDeleted:
		default:
			return fmt.Errorf("session relocation manifest contains invalid payload state %q", payload.State)
		}
		if !validRelocationRecoveryName(payload.Quarantine, ".source") ||
			!validRelocationRecoveryName(payload.Restore, ".restore") {
			return fmt.Errorf("session relocation manifest contains invalid recovery controls for %s", payload.Source)
		}
		if destinations[payload.Destination] {
			return fmt.Errorf("session relocation manifest contains duplicate destination %s", payload.Destination)
		}
		destinations[payload.Destination] = true
		if sources[payload.Source] {
			return fmt.Errorf("session relocation manifest contains duplicate source %s", payload.Source)
		}
		sources[payload.Source] = true

		expectedDestination, expectedSessionLog, mappingErr := r.payloadMapping(manifest.Sources, payload.Source)
		if mappingErr != nil {
			return fmt.Errorf("session relocation manifest source is not in the current plan: %w", mappingErr)
		}
		if payload.Destination != expectedDestination || payload.SessionLog != expectedSessionLog {
			return fmt.Errorf(
				"session relocation manifest payload mapping differs from the current plan: source=%s destination=%s session_log=%t",
				payload.Source, payload.Destination, payload.SessionLog,
			)
		}
		if fresh, exists := freshBySource[payload.Source]; exists {
			if payload.State == relocationPayloadDeleted {
				return fmt.Errorf("session relocation manifest marks a present source as deleted: %s", payload.Source)
			}
			if !sameRelocationPayloadPlan(payload, fresh) {
				return fmt.Errorf("session relocation manifest payload differs from the fresh current plan: %s", payload.Source)
			}
			delete(freshBySource, payload.Source)
		} else {
			targetRoot, targetRelative, rootErr := r.targetRoot(payload.Destination)
			if rootErr != nil {
				return rootErr
			}
			targetInfo, statErr := targetRoot.Lstat(targetRelative)
			if statErr != nil {
				return fmt.Errorf("manifested source %s is absent and its target cannot be validated: %w", payload.Source, statErr)
			}
			if !targetInfo.Mode().IsRegular() {
				return fmt.Errorf("manifested relocation target %s is not a regular file", payload.Destination)
			}
			if uint32(targetInfo.Mode().Perm()) != payload.Mode {
				return fmt.Errorf("manifested relocation target mode differs from the current manifest for %s", payload.Destination)
			}
		}
	}
	if len(freshBySource) > 0 {
		unmanifested := make([]string, 0, len(freshBySource))
		for source := range freshBySource {
			unmanifested = append(unmanifested, source)
		}
		sort.Strings(unmanifested)
		return fmt.Errorf("current relocation sources contain unmanifested payloads: %s", strings.Join(unmanifested, ", "))
	}
	return nil
}

func (r *FilesystemSessionStoreRelocator) authorizeManifestSources(ctx context.Context, manifest relocationManifest) error {
	seenKinds := map[SessionStoreRelocationSourceKind]bool{}
	for _, source := range manifest.Sources {
		if seenKinds[source.Kind] {
			return fmt.Errorf("session relocation manifest contains duplicate %s sources", source.Kind)
		}
		seenKinds[source.Kind] = true
		switch source.Kind {
		case SessionStoreRelocationSourceOldGlobal:
			matched := false
			for _, current := range r.options.Sources {
				if current.Kind == SessionStoreRelocationSourceOldGlobal && sameRelocationSource(source, current) {
					matched = true
					break
				}
			}
			if !matched && r.options.AuthorizedOldGlobalSource != nil &&
				sameRelocationSource(source, *r.options.AuthorizedOldGlobalSource) {
				matched = true
			}
			if !matched {
				return fmt.Errorf(
					"session relocation manifest old_global source does not match currently derived confined paths: %s",
					source.Sessions,
				)
			}
		case SessionStoreRelocationSourceInTree:
		default:
			return fmt.Errorf("session relocation manifest contains unsupported source kind %q", source.Kind)
		}
	}
	return r.authorizePinnedSources(ctx, manifest.Sources, manifest.StableRepoAuthority)
}

func (r *FilesystemSessionStoreRelocator) authorizePinnedSources(
	ctx context.Context,
	sources []SessionStoreRelocationSource,
	authority string,
) error {
	for _, source := range sources {
		if source.Kind != SessionStoreRelocationSourceInTree {
			continue
		}
		pinned, err := r.pinnedRelocationSource(source)
		if err != nil {
			return err
		}
		if pinned.immutableAbsent {
			continue
		}
		if r.options.AuthorizeInTreeSource == nil {
			return fmt.Errorf("session relocation has a present in-tree source but no authorizer is configured")
		}
		if err := revalidatePinnedSourceIdentity(pinned); err != nil {
			return err
		}
		if err := r.options.AuthorizeInTreeSource(
			ctx, authority, source.Sessions, source.StagedBlobs,
		); err != nil {
			return fmt.Errorf("authorizing pinned in-tree relocation source %s: %w", source.Sessions, err)
		}
		if err := revalidatePinnedSourceIdentity(pinned); err != nil {
			return fmt.Errorf("revalidating in-tree source after authorization: %w", err)
		}
	}
	return nil
}

func (r *FilesystemSessionStoreRelocator) pinnedRelocationSource(
	source SessionStoreRelocationSource,
) (*pinnedRelocationSource, error) {
	for index := range r.pinnedSources {
		if sameRelocationSource(r.pinnedSources[index].source, source) {
			return &r.pinnedSources[index], nil
		}
	}
	return nil, fmt.Errorf("relocation source is not pinned: %s", source.Sessions)
}

func revalidatePinnedSourceIdentity(source *pinnedRelocationSource) error {
	for _, category := range []*pinnedSourceCategory{&source.sessions, &source.blobs} {
		if err := revalidatePinnedCategoryIdentity(category); err != nil {
			return err
		}
	}
	return nil
}

func revalidatePinnedCategoryIdentity(category *pinnedSourceCategory) error {
	if category.strictChain {
		if err := category.chain.revalidate(); err != nil {
			return fmt.Errorf("relocation source component was rebound: %s: %w", category.path, err)
		}
		return nil
	}
	if category.absent {
		if category.parent != nil {
			if _, err := category.parent.Lstat(category.base); !errors.Is(err, fs.ErrNotExist) {
				if err == nil {
					return fmt.Errorf("previously absent relocation source reappeared: %s", category.path)
				}
				return err
			}
			return nil
		}
		if _, err := os.Lstat(category.path); !errors.Is(err, fs.ErrNotExist) {
			if err == nil {
				return fmt.Errorf("previously absent relocation source reappeared: %s", category.path)
			}
			return err
		}
		return nil
	}
	configured, err := category.parent.Lstat(category.base)
	if err != nil || configured.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(category.identity, configured) {
		return fmt.Errorf("configured relocation source was rebound: %s", category.path)
	}
	return nil
}

func sameRelocationSource(left, right SessionStoreRelocationSource) bool {
	return left.Kind == right.Kind &&
		filepath.Clean(left.Sessions) == filepath.Clean(right.Sessions) &&
		filepath.Clean(left.StagedBlobs) == filepath.Clean(right.StagedBlobs) &&
		left.WriteTombstone == right.WriteTombstone
}

func relocationSourceExists(source SessionStoreRelocationSource) bool {
	for _, root := range []string{source.Sessions, source.StagedBlobs} {
		if _, err := os.Lstat(root); err == nil {
			return true
		}
	}
	return false
}

func sameRelocationPayloadPlan(manifest, fresh relocationManifestPayload) bool {
	return manifest.Source == fresh.Source &&
		manifest.Destination == fresh.Destination &&
		manifest.SessionLog == fresh.SessionLog &&
		manifest.Digest == fresh.Digest &&
		manifest.Size == fresh.Size &&
		manifest.Mode == fresh.Mode &&
		manifest.Quarantine == fresh.Quarantine &&
		manifest.Restore == fresh.Restore
}

func validRelocationRecoveryName(name, suffix string) bool {
	return filepath.IsLocal(name) &&
		filepath.Dir(name) == SessionRelocationQuarantineDir &&
		strings.HasSuffix(filepath.Base(name), suffix)
}

func (r *FilesystemSessionStoreRelocator) payloadMapping(sources []SessionStoreRelocationSource, sourcePath string) (string, bool, error) {
	for _, source := range sources {
		for _, pair := range []struct {
			root        string
			target      string
			sessionRoot bool
		}{
			{root: source.Sessions, target: r.options.TargetSessions, sessionRoot: true},
			{root: source.StagedBlobs, target: r.options.TargetBlobs},
		} {
			if !pathInside(pair.root, sourcePath) {
				continue
			}
			rel, err := filepath.Rel(pair.root, sourcePath)
			if err != nil {
				return "", false, err
			}
			return filepath.Join(pair.target, rel),
				pair.sessionRoot && filepath.Dir(rel) == "." && strings.HasSuffix(rel, ".jsonl"),
				nil
		}
	}
	return "", false, fmt.Errorf("%s is outside authorized originating sources %v", sourcePath, sources)
}

func pathInside(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && rel != "." && rel != ".." && !filepath.IsAbs(rel) &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func pathAtOrInside(root, path string) bool {
	return filepath.Clean(root) == filepath.Clean(path) || pathInside(root, path)
}

func openRelocationTargetRoots(
	trustedStatePath string,
	sessionsPath string,
	blobsPath string,
	create bool,
) (*relocationTargetRoots, error) {
	sessionsCategoryPath := filepath.Join(trustedStatePath, "sessions")
	blobsCategoryPath := filepath.Join(trustedStatePath, "staged-blobs")
	sessionsRelative, err := filepath.Rel(sessionsCategoryPath, sessionsPath)
	if err != nil {
		return nil, err
	}
	blobsRelative, err := filepath.Rel(blobsCategoryPath, blobsPath)
	if err != nil {
		return nil, err
	}
	if !filepath.IsLocal(sessionsRelative) || !filepath.IsLocal(blobsRelative) {
		return nil, fmt.Errorf("relocation targets escape trusted state categories")
	}
	authority, err := openTrustedStateAuthority(trustedStatePath, create)
	if err != nil {
		return nil, err
	}
	openStore := func(
		categoryName string,
		storeRelative string,
		display string,
	) (*trustedDirectoryChain, *trustedDirectoryChain, error) {
		category, err := openTrustedRelativeDirectoryChain(
			authority.state, categoryName, create, false, 0o700,
			filepath.Join(trustedStatePath, categoryName),
		)
		if err != nil {
			return nil, nil, err
		}
		var store *trustedDirectoryChain
		if storeRelative != "." {
			store, err = openTrustedRelativeDirectoryChain(
				category.root(), storeRelative, create, false, 0o755, display,
			)
			if err != nil {
				return nil, nil, errors.Join(err, category.close())
			}
		}
		return category, store, nil
	}
	sessionsCategory, sessionStore, err := openStore(
		"sessions", sessionsRelative, sessionsPath,
	)
	if err != nil {
		return nil, errors.Join(err, authority.close())
	}
	blobsCategory, blobStore, err := openStore(
		"staged-blobs", blobsRelative, blobsPath,
	)
	if err != nil {
		return nil, errors.Join(
			err, sessionStore.close(), sessionsCategory.close(), authority.close(),
		)
	}
	return &relocationTargetRoots{
		authority:             authority,
		sessionsCategoryChain: sessionsCategory, blobsCategoryChain: blobsCategory,
		sessionsStoreChain: sessionStore, blobsStoreChain: blobStore,
		sessionsCategory: sessionsCategory.root(), blobsCategory: blobsCategory.root(),
		sessions: trustedStoreChainRoot(sessionsCategory, sessionStore),
		blobs:    trustedStoreChainRoot(blobsCategory, blobStore),
	}, nil
}

func trustedStoreChainRoot(
	category *trustedDirectoryChain,
	store *trustedDirectoryChain,
) *os.Root {
	if store != nil {
		return store.root()
	}
	return category.root()
}

func (r *relocationTargetRoots) close() error {
	return errors.Join(
		closeTrustedDirectoryChain(r.sessionsStoreChain), closeTrustedDirectoryChain(r.blobsStoreChain),
		r.sessionsCategoryChain.close(), r.blobsCategoryChain.close(),
		r.authority.close(),
	)
}

func closeTrustedDirectoryChain(chain *trustedDirectoryChain) error {
	if chain == nil {
		return nil
	}
	return chain.close()
}

func (r *relocationTargetRoots) revalidateConfiguredPaths(sessionsPath, blobsPath string) error {
	if err := r.authority.revalidate(); err != nil {
		return err
	}
	for _, target := range []struct {
		configured string
		category   *trustedDirectoryChain
		store      *trustedDirectoryChain
	}{
		{
			configured: sessionsPath, category: r.sessionsCategoryChain, store: r.sessionsStoreChain,
		},
		{
			configured: blobsPath, category: r.blobsCategoryChain, store: r.blobsStoreChain,
		},
	} {
		if err := target.category.revalidate(); err != nil {
			return fmt.Errorf("configured state category was rebound during migration: %s", target.configured)
		}
		if target.store != nil {
			if err := target.store.revalidate(); err != nil {
				return fmt.Errorf("configured relocation target was rebound during migration: %s", target.configured)
			}
		}
	}
	return nil
}

func (r *relocationTargetRoots) cleanTemps() error {
	var errs []error
	for _, root := range []*os.Root{r.sessions, r.blobs} {
		if err := root.RemoveAll(SessionRelocationTempDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *FilesystemSessionStoreRelocator) targetRoot(path string) (*os.Root, string, error) {
	if r.targets == nil {
		return nil, "", fmt.Errorf("relocation target roots are not open")
	}
	for _, target := range []struct {
		path string
		root *os.Root
	}{
		{path: r.options.TargetSessions, root: r.targets.sessions},
		{path: r.options.TargetBlobs, root: r.targets.blobs},
	} {
		if !pathInside(target.path, path) {
			continue
		}
		rel, err := filepath.Rel(target.path, path)
		if err != nil || !filepath.IsLocal(rel) {
			return nil, "", fmt.Errorf("unsafe relocation target path %s", path)
		}
		return target.root, rel, nil
	}
	return nil, "", fmt.Errorf("relocation target escapes configured category roots: %s", path)
}

func relocationTempName() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return filepath.Join(SessionRelocationTempDir, hex.EncodeToString(token[:])), nil
}

func (r *FilesystemSessionStoreRelocator) readRelocationManifest() (*relocationManifest, error) {
	encoded, present, err := readRootedRegularControlFile(
		r.targets.sessions, SessionRelocationManifest,
	)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}
	var manifest relocationManifest
	if err := decodeStrictJSON(encoded, &manifest); err != nil {
		return nil, fmt.Errorf("decoding session relocation manifest: %w", err)
	}
	return &manifest, nil
}

func (r *FilesystemSessionStoreRelocator) writeManifest(path string, manifest *relocationManifest) error {
	manifest.UpdatedAt = r.now().UTC().Round(0)
	if err := writeJSONAtomicRoot(r.targets.sessions, SessionRelocationManifest, manifest); err != nil {
		return fmt.Errorf("writing session relocation manifest %s: %w", path, err)
	}
	return nil
}

func writeJSONAtomicRoot(root *os.Root, name string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := root.MkdirAll(SessionRelocationTempDir, 0o700); err != nil {
		return err
	}
	tempName, err := relocationTempName()
	if err != nil {
		return err
	}
	temp, err := root.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(tempName) }()
	if _, err := temp.Write(encoded); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := root.Rename(tempName, name); err != nil {
		return err
	}
	return syncRootDirectory(root, filepath.Dir(name))
}

func (r *FilesystemSessionStoreRelocator) planPayloads(sources []SessionStoreRelocationSource) ([]relocationManifestPayload, []RelocationItem, error) {
	previousSources := r.activeSources
	r.activeSources = sources
	defer func() { r.activeSources = previousSources }()

	var payloads []relocationManifestPayload
	destinations := map[string]string{}
	var collisions []RelocationItem
	for _, source := range sources {
		for _, pair := range []struct {
			root        string
			target      string
			sessionRoot bool
		}{
			{root: source.Sessions, target: r.options.TargetSessions, sessionRoot: true},
			{root: source.StagedBlobs, target: r.options.TargetBlobs},
		} {
			category, err := r.pinnedCategory(pair.root)
			if err != nil {
				return nil, nil, err
			}
			files, err := storePayloadFilesPinned(*category, pair.sessionRoot)
			if err != nil {
				return nil, nil, err
			}
			for _, filename := range files {
				rel, err := filepath.Rel(pair.root, filename)
				if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					return nil, nil, fmt.Errorf("session-store payload escapes source root: %s", filename)
				}
				destination := filepath.Join(pair.target, rel)
				if previous, exists := destinations[destination]; exists {
					collisions = append(collisions,
						RelocationItem{Source: previous, Destination: destination},
						RelocationItem{Source: filename, Destination: destination},
					)
					continue
				}
				destinations[destination] = filename
				relative, err := filepath.Rel(pair.root, filename)
				if err != nil || !filepath.IsLocal(relative) {
					return nil, nil, fmt.Errorf("unsafe relocation source path %s", filename)
				}
				info, err := category.root.Lstat(relative)
				if err != nil {
					return nil, nil, err
				}
				payload := relocationManifestPayload{
					Source: filename, Destination: destination,
					SessionLog: pair.sessionRoot && filepath.Dir(rel) == "." && strings.HasSuffix(rel, ".jsonl"),
					Mode:       uint32(info.Mode().Perm()), State: relocationPayloadPlanned,
				}
				payload.Quarantine, payload.Restore = relocationRecoveryNames(filename, destination)
				digest, size, err := r.payloadDigest(payload)
				if err != nil {
					return nil, nil, err
				}
				payload.Digest, payload.Size = digest, size
				payloads = append(payloads, payload)
			}
		}
	}
	sort.Slice(payloads, func(i, j int) bool { return payloads[i].Destination < payloads[j].Destination })
	return payloads, collisions, nil
}

func (r *FilesystemSessionStoreRelocator) pinnedCategory(path string) (*pinnedSourceCategory, error) {
	for index := range r.pinnedSources {
		for _, category := range []*pinnedSourceCategory{
			&r.pinnedSources[index].sessions, &r.pinnedSources[index].blobs,
		} {
			if filepath.Clean(category.path) == filepath.Clean(path) {
				return category, nil
			}
		}
	}
	return nil, fmt.Errorf("relocation source category is not pinned: %s", path)
}

func storePayloadFilesPinned(category pinnedSourceCategory, sessionRoot bool) ([]string, error) {
	if category.absent {
		return nil, nil
	}
	var files []string
	err := fs.WalkDir(category.root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		if filepath.Dir(path) == "." &&
			(filepath.Base(path) == SessionRelocationTempDir ||
				filepath.Base(path) == SessionRelocationQuarantineDir ||
				filepath.Base(path) == legacyMigrationControlDir) {
			if !entry.IsDir() {
				return fmt.Errorf("session relocation temp control %s is not a directory", filepath.Join(category.path, path))
			}
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		absolute := filepath.Join(category.path, path)
		if sessionRoot && isSessionStoreControl(category.path, absolute) {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("session store contains non-regular control file %s (%s)", absolute, info.Mode().Type())
			}
			if filepath.Base(path) == SessionRelocationTombstone {
				tombstone, err := readSessionRelocationTombstoneRoot(category.root, category.path)
				if err != nil {
					return err
				}
				if tombstone == nil {
					return fmt.Errorf("session relocation tombstone disappeared while scanning")
				}
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("session store contains non-regular payload %s (%s)", absolute, info.Mode().Type())
		}
		files = append(files, absolute)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning pinned session store %s: %w", category.path, err)
	}
	sort.Strings(files)
	return files, nil
}

func relocationRecoveryNames(source, destination string) (string, string) {
	sum := sha256.Sum256([]byte(source + "\x00" + destination))
	token := hex.EncodeToString(sum[:16])
	return filepath.Join(SessionRelocationQuarantineDir, token+".source"),
		filepath.Join(SessionRelocationQuarantineDir, token+".restore")
}

func (r *FilesystemSessionStoreRelocator) payloadDigest(payload relocationManifestPayload) (string, int64, error) {
	hash := sha256.New()
	count := &countingWriter{Writer: hash}
	if err := r.writePayload(payload, count); err != nil {
		return "", 0, fmt.Errorf("digesting relocation payload %s: %w", payload.Source, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), count.n, nil
}

type countingWriter struct {
	io.Writer
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.n += int64(n)
	return n, err
}

func (r *FilesystemSessionStoreRelocator) writePayload(payload relocationManifestPayload, destination io.Writer) error {
	source, err := r.openSource(payload.Source)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	return r.writePayloadFromHandle(payload, source, destination)
}

func (r *FilesystemSessionStoreRelocator) writePayloadFromHandle(
	payload relocationManifestPayload,
	source *os.File,
	destination io.Writer,
) error {
	if payload.SessionLog {
		return r.copySessionLog(source, destination)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, copyErr := io.Copy(destination, source)
	return copyErr
}

func (r *FilesystemSessionStoreRelocator) copySessionLog(input *os.File, destination io.Writer) error {
	format, err := classifySessionHandle(input)
	if err != nil {
		return err
	}
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if format != sessionFormatCurrent {
		_, err = io.Copy(destination, input)
		return err
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var line sessionLine
		if err := decodeStrictJSON(scanner.Bytes(), &line); err != nil {
			return fmt.Errorf("decoding current session line %d: %w", lineNumber, err)
		}
		if line.Metadata != nil && line.Metadata.CodecVersion != app.SessionCodecVersion {
			return fmt.Errorf(
				"decoding current session line %d: unsupported metadata codec version %d",
				lineNumber, line.Metadata.CodecVersion,
			)
		}
		for _, event := range line.Events {
			if event.CodecVersion != app.SessionCodecVersion {
				return fmt.Errorf(
					"decoding current session line %d: unsupported event codec version %d",
					lineNumber, event.CodecVersion,
				)
			}
		}
		rewritten, err := r.options.Transformer.RewriteProjectIdentity(r.options.TargetProject, app.SessionIdentityEnvelope{
			Metadata: line.Metadata, Events: line.Events,
		})
		if err != nil {
			return fmt.Errorf("rewriting session identity on line %d: %w", lineNumber, err)
		}
		line.Metadata, line.Events = rewritten.Metadata, rewritten.Events
		encoded, err := json.Marshal(line)
		if err != nil {
			return err
		}
		if _, err := destination.Write(append(encoded, '\n')); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (r *FilesystemSessionStoreRelocator) sourceRoot(path string) (*pinnedSourceCategory, string, error) {
	for index := range r.pinnedSources {
		for _, category := range []*pinnedSourceCategory{
			&r.pinnedSources[index].sessions, &r.pinnedSources[index].blobs,
		} {
			if !pathInside(category.path, path) {
				continue
			}
			relative, err := filepath.Rel(category.path, path)
			if err != nil || !filepath.IsLocal(relative) {
				return nil, "", fmt.Errorf("unsafe relocation source path %s", path)
			}
			return category, relative, nil
		}
	}
	return nil, "", fmt.Errorf("relocation source escapes authorized category roots: %s", path)
}

func (r *FilesystemSessionStoreRelocator) openSource(path string) (*os.File, error) {
	category, relative, err := r.sourceRoot(path)
	if err != nil {
		return nil, err
	}
	if category.absent || category.root == nil {
		return nil, fs.ErrNotExist
	}
	return openRootedRegular(category.root, relative)
}

func decodeStrictJSON(encoded []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func ReadSessionRelocationTombstone(sessionsDir string) (*SessionRelocationTombstoneRecord, error) {
	path := filepath.Join(sessionsDir, SessionRelocationTombstone)
	encoded, present, err := readRegularControlFile(
		sessionsDir, SessionRelocationTombstone,
	)
	if err != nil {
		return nil, fmt.Errorf("reading session relocation tombstone %s: %w", path, err)
	}
	if !present {
		return nil, nil
	}
	return decodeSessionRelocationTombstone(encoded, path)
}

func readSessionRelocationTombstoneRoot(
	root *os.Root,
	displayRoot string,
) (*SessionRelocationTombstoneRecord, error) {
	path := filepath.Join(displayRoot, SessionRelocationTombstone)
	encoded, present, err := readRootedRegularControlFile(
		root, SessionRelocationTombstone,
	)
	if err != nil {
		return nil, fmt.Errorf("reading session relocation tombstone %s: %w", path, err)
	}
	if !present {
		return nil, nil
	}
	return decodeSessionRelocationTombstone(encoded, path)
}

func decodeSessionRelocationTombstone(
	encoded []byte,
	path string,
) (*SessionRelocationTombstoneRecord, error) {
	var tombstone SessionRelocationTombstoneRecord
	if err := decodeStrictJSON(encoded, &tombstone); err != nil {
		return nil, fmt.Errorf("decoding session relocation tombstone %s: %w", path, err)
	}
	if tombstone.Version != 1 && tombstone.Version != 2 {
		return nil, fmt.Errorf("unsupported session relocation tombstone version %d in %s", tombstone.Version, path)
	}
	if tombstone.TargetProject == "" ||
		!filepath.IsAbs(tombstone.TargetSessions) ||
		!filepath.IsAbs(tombstone.TargetStagedBlobs) ||
		tombstone.RelocatedAt.IsZero() {
		return nil, fmt.Errorf("session relocation tombstone %s contains invalid targets or relocation time", path)
	}
	return &tombstone, nil
}

func (r *FilesystemSessionStoreRelocator) verifyPublished(payload relocationManifestPayload) (bool, error) {
	root, relative, err := r.targetRoot(payload.Destination)
	if err != nil {
		return false, err
	}
	info, err := root.Lstat(relative)
	if errors.Is(err, fs.ErrNotExist) {
		if payload.State != relocationPayloadPlanned {
			return false, fmt.Errorf("published relocation target disappeared: %s", payload.Destination)
		}
		sourceCategory, sourceRelative, mappingErr := r.sourceRoot(payload.Source)
		if mappingErr != nil {
			return false, mappingErr
		}
		if sourceCategory.absent || sourceCategory.root == nil {
			return false, fmt.Errorf("relocation payload has neither source nor target: %s", payload.Source)
		}
		sourceInfo, sourceErr := sourceCategory.root.Lstat(sourceRelative)
		if sourceErr != nil {
			return false, fmt.Errorf("relocation payload has neither source nor target: %s", payload.Source)
		}
		if !sourceInfo.Mode().IsRegular() {
			return false, fmt.Errorf("relocation source %s is no longer a regular file", payload.Source)
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("relocation target %s is not a regular file", payload.Destination)
	}
	if uint32(info.Mode().Perm()) != payload.Mode {
		return false, fmt.Errorf(
			"relocation target mode changed after publication: %s has %04o, want %04o",
			payload.Destination, info.Mode().Perm(), payload.Mode,
		)
	}
	digest, size, err := rootedFileDigest(root, relative)
	if err != nil {
		return false, err
	}
	if digest != payload.Digest || size != payload.Size {
		return false, &SessionStoreCollisionError{Collisions: []RelocationItem{{
			Source: payload.Source, Destination: payload.Destination,
		}}}
	}
	return true, nil
}

func (r *FilesystemSessionStoreRelocator) verifySource(payload relocationManifestPayload) error {
	category, relative, mappingErr := r.sourceRoot(payload.Source)
	if mappingErr != nil {
		return mappingErr
	}
	if category.absent || category.root == nil {
		if payload.State != relocationPayloadDeleted {
			return fmt.Errorf("pre-deletion relocation payload has no recoverable source artifact: %s", payload.Source)
		}
		return nil
	}
	var file *os.File
	var err error
	for _, candidate := range []string{payload.Quarantine, payload.Restore, relative} {
		file, err = openRootedRegular(category.root, candidate)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if file == nil {
		if payload.State != relocationPayloadDeleted {
			return fmt.Errorf("pre-deletion relocation payload has no recoverable source artifact: %s", payload.Source)
		}
		return nil
	}
	defer func() { _ = file.Close() }()
	return r.verifyPayloadHandle(payload, file)
}

func (r *FilesystemSessionStoreRelocator) verifyPayloadHandle(
	payload relocationManifestPayload,
	file *os.File,
) error {
	hash := sha256.New()
	count := &countingWriter{Writer: hash}
	if err := r.writePayloadFromHandle(payload, file, count); err != nil {
		return err
	}
	digest, size := hex.EncodeToString(hash.Sum(nil)), count.n
	if digest != payload.Digest || size != payload.Size {
		return fmt.Errorf("relocation source changed after manifest creation: %s", payload.Source)
	}
	return nil
}

func (r *FilesystemSessionStoreRelocator) quarantineVerifiedSource(
	payload relocationManifestPayload,
) error {
	category, relative, err := r.sourceRoot(payload.Source)
	if err != nil {
		return err
	}
	if category.absent || category.root == nil {
		return fmt.Errorf("relocation source category disappeared before deletion: %s", payload.Source)
	}
	if err := category.root.MkdirAll(SessionRelocationQuarantineDir, 0o700); err != nil {
		return err
	}
	if _, err := category.root.Lstat(payload.Quarantine); errors.Is(err, fs.ErrNotExist) {
		source, openErr := openRootedRegular(category.root, relative)
		if errors.Is(openErr, fs.ErrNotExist) {
			restore, restoreErr := openRootedRegular(category.root, payload.Restore)
			if restoreErr != nil {
				if errors.Is(restoreErr, fs.ErrNotExist) {
					return fmt.Errorf("relocation source has no original, quarantine, or restore artifact: %s", payload.Source)
				}
				return restoreErr
			}
			verifyErr := r.verifyPayloadHandle(payload, restore)
			closeErr := restore.Close()
			if err := errors.Join(verifyErr, closeErr); err != nil {
				return err
			}
			if err := category.root.Link(payload.Restore, payload.Quarantine); err != nil {
				return fmt.Errorf("recreating relocation quarantine from verified restore: %w", err)
			}
			if err := syncRootDirectory(category.root, SessionRelocationQuarantineDir); err != nil {
				return err
			}
			openErr = nil
		}
		if openErr != nil {
			return openErr
		}
		if source != nil {
			defer func() { _ = source.Close() }()
			sourceInfo, statErr := source.Stat()
			if statErr != nil {
				return statErr
			}
			if err := r.verifyPayloadHandle(payload, source); err != nil {
				return err
			}
			if err := category.root.Rename(relative, payload.Quarantine); err != nil {
				return err
			}
			if r.afterSourceQuarantineRename != nil {
				r.afterSourceQuarantineRename()
			}
			quarantinedInfo, err := category.root.Lstat(payload.Quarantine)
			if err != nil || !os.SameFile(sourceInfo, quarantinedInfo) {
				restoreErr := r.restoreRelocationSourceName(category.root, relative, payload.Quarantine)
				if err == nil {
					err = fmt.Errorf("quarantined name does not identify the verified source")
				}
				return errors.Join(err, restoreErr)
			}
			if err := syncRootDirectory(category.root, filepath.Dir(relative)); err != nil {
				return err
			}
			if r.afterSourceQuarantine != nil {
				r.afterSourceQuarantine()
			}
		}
	} else if err != nil {
		return err
	} else {
		quarantineInfo, quarantineErr := category.root.Lstat(payload.Quarantine)
		originalInfo, originalErr := category.root.Lstat(relative)
		if errors.Join(quarantineErr, originalErr) == nil &&
			quarantineInfo.Mode().IsRegular() && originalInfo.Mode().IsRegular() &&
			os.SameFile(quarantineInfo, originalInfo) {
			if err := category.root.Remove(relative); err != nil {
				return err
			}
			if err := syncRootDirectory(category.root, filepath.Dir(relative)); err != nil {
				return err
			}
		} else if originalErr != nil && !errors.Is(originalErr, fs.ErrNotExist) {
			return originalErr
		}
	}
	quarantined, err := openRootedRegular(category.root, payload.Quarantine)
	if err != nil {
		return err
	}
	defer func() { _ = quarantined.Close() }()
	if err := r.verifyPayloadHandle(payload, quarantined); err != nil {
		return err
	}
	return r.ensurePayloadRestore(category.root, payload)
}

func (r *FilesystemSessionStoreRelocator) ensurePayloadRestore(
	root *os.Root,
	payload relocationManifestPayload,
) error {
	if existing, err := openRootedRegular(root, payload.Restore); err == nil {
		verifyErr := r.verifyPayloadHandle(payload, existing)
		closeErr := existing.Close()
		if errors.Join(verifyErr, closeErr) == nil {
			return reconcilePayloadRestoreTemp(root, payload)
		}
		quarantine, quarantineErr := openRootedRegular(root, payload.Quarantine)
		if quarantineErr != nil {
			return errors.Join(verifyErr, closeErr, quarantineErr)
		}
		quarantineVerifyErr := r.verifyPayloadHandle(payload, quarantine)
		quarantineCloseErr := quarantine.Close()
		if err := errors.Join(quarantineVerifyErr, quarantineCloseErr); err != nil {
			return errors.Join(verifyErr, closeErr, err)
		}
		if err := reconcilePayloadRestoreTemp(root, payload); err != nil {
			return errors.Join(verifyErr, closeErr, err)
		}
		if err := root.Remove(payload.Restore); err != nil {
			return errors.Join(verifyErr, closeErr, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	source, err := openRootedRegular(root, payload.Quarantine)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	tempName := payload.Restore + ".tmp"
	if info, err := root.Lstat(tempName); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("relocation restore temp is not regular: %s", tempName)
		}
		if err := root.Remove(tempName); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	restore, err := root.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fs.FileMode(payload.Mode))
	if err != nil {
		return err
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		_ = restore.Close()
		return err
	}
	_, copyErr := io.Copy(restore, source)
	closeErr := errors.Join(restore.Sync(), restore.Close())
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = root.Remove(tempName)
		return err
	}
	if r.afterRestoreTempSync != nil {
		r.afterRestoreTempSync()
	}
	if err := root.Link(tempName, payload.Restore); err != nil {
		_ = root.Remove(tempName)
		if errors.Is(err, fs.ErrExist) {
			existing, openErr := openRootedRegular(root, payload.Restore)
			if openErr != nil {
				return openErr
			}
			verifyErr := r.verifyPayloadHandle(payload, existing)
			return errors.Join(verifyErr, existing.Close())
		}
		return fmt.Errorf("publishing relocation restore without replacement: %w", err)
	}
	if r.afterRestoreLink != nil {
		r.afterRestoreLink()
	}
	return reconcilePayloadRestoreTemp(root, payload)
}

func reconcilePayloadRestoreTemp(
	root *os.Root,
	payload relocationManifestPayload,
) error {
	tempName := payload.Restore + ".tmp"
	tempInfo, err := root.Lstat(tempName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !tempInfo.Mode().IsRegular() {
		return fmt.Errorf("relocation restore temp is not regular: %s", tempName)
	}
	restoreInfo, err := root.Lstat(payload.Restore)
	if err != nil {
		return fmt.Errorf("relocation restore temp exists without final restore %s: %w", payload.Restore, err)
	}
	if !restoreInfo.Mode().IsRegular() {
		return fmt.Errorf("relocation restore control is not regular: %s", payload.Restore)
	}
	if !os.SameFile(tempInfo, restoreInfo) {
		return fmt.Errorf("relocation restore temp differs from final restore: %s", payload.Restore)
	}
	if err := root.Remove(tempName); err != nil {
		return err
	}
	return syncRootDirectory(root, SessionRelocationQuarantineDir)
}

func (r *FilesystemSessionStoreRelocator) finalizeQuarantinedSource(
	payload relocationManifestPayload,
) error {
	category, relative, err := r.sourceRoot(payload.Source)
	if err != nil {
		return err
	}
	if category.absent || category.root == nil {
		return nil
	}
	if err := category.root.Remove(payload.Quarantine); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := syncRootDirectory(category.root, SessionRelocationQuarantineDir); err != nil {
		return err
	}
	if r.afterQuarantineRemove != nil {
		r.afterQuarantineRemove()
	}
	postErr := r.revalidateBeforeIrreversible("post_source_delete")
	if postErr == nil {
		_, postErr = r.verifyPublished(payload)
	}
	if postErr == nil {
		return nil
	}
	restoreErr := r.restoreRelocationSourceName(category.root, relative, payload.Restore)
	return errors.Join(postErr, restoreErr)
}

func (r *FilesystemSessionStoreRelocator) restoreRelocationSourceName(
	root *os.Root,
	original string,
	restore string,
) error {
	return restoreRelocationSourceNameWithHooks(
		root, original, restore,
		r.beforeSourceRestoreLink, r.afterSourceRestoreLink,
	)
}

func restoreRelocationSourceName(root *os.Root, original, restore string) error {
	return restoreRelocationSourceNameWithHooks(root, original, restore, nil, nil)
}

func restoreRelocationSourceNameWithHooks(
	root *os.Root,
	original string,
	restore string,
	beforeLink func(),
	afterLink func(),
) error {
	restoreInfo, err := root.Lstat(restore)
	if err != nil || !restoreInfo.Mode().IsRegular() {
		if err == nil {
			err = fmt.Errorf("restore control is not regular")
		}
		return err
	}
	reconcileVisible := func() error {
		originalInfo, err := root.Lstat(original)
		if err != nil {
			return err
		}
		if !originalInfo.Mode().IsRegular() || !os.SameFile(restoreInfo, originalInfo) {
			return fmt.Errorf(
				"source restoration collision: visible source differs from deterministic restore control",
			)
		}
		return syncRootDirectory(root, filepath.Dir(original))
	}
	if _, err := root.Lstat(original); err == nil {
		return reconcileVisible()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if beforeLink != nil {
		beforeLink()
	}
	if err := root.Link(restore, original); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return reconcileVisible()
		}
		return err
	}
	if afterLink != nil {
		afterLink()
	}
	return reconcileVisible()
}

func (r *FilesystemSessionStoreRelocator) removePayloadRestore(
	payload relocationManifestPayload,
) error {
	category, _, err := r.sourceRoot(payload.Source)
	if err != nil {
		return err
	}
	if category.absent || category.root == nil {
		return nil
	}
	restore, err := openRootedRegular(category.root, payload.Restore)
	if errors.Is(err, fs.ErrNotExist) {
		if _, tempErr := category.root.Lstat(payload.Restore + ".tmp"); !errors.Is(tempErr, fs.ErrNotExist) {
			if tempErr == nil {
				return fmt.Errorf("relocation restore temp exists without final restore: %s", payload.Restore)
			}
			return tempErr
		}
		return nil
	}
	if err != nil {
		return err
	}
	verifyErr := r.verifyPayloadHandle(payload, restore)
	if err := errors.Join(verifyErr, restore.Close()); err != nil {
		return err
	}
	if err := reconcilePayloadRestoreTemp(category.root, payload); err != nil {
		return err
	}
	if err := category.root.Remove(payload.Restore); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncRootDirectory(category.root, SessionRelocationQuarantineDir)
}

func rootedFileDigest(root *os.Root, path string) (string, int64, error) {
	file, err := openRootedRegular(root, path)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(hash, file)
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), n, nil
}

func (r *FilesystemSessionStoreRelocator) publish(payload relocationManifestPayload) error {
	root, relative, err := r.targetRoot(payload.Destination)
	if err != nil {
		return err
	}
	destinationDir := filepath.Dir(relative)
	if err := root.MkdirAll(destinationDir, 0o755); err != nil {
		return err
	}
	if err := root.MkdirAll(SessionRelocationTempDir, 0o700); err != nil {
		return err
	}
	tempName, err := relocationTempName()
	if err != nil {
		return err
	}
	temp, err := root.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(tempName) }()
	hash := sha256.New()
	count := &countingWriter{Writer: io.MultiWriter(temp, hash)}
	if err := r.writePayload(payload, count); err != nil {
		_ = temp.Close()
		return err
	}
	if count.n != payload.Size || hex.EncodeToString(hash.Sum(nil)) != payload.Digest {
		_ = temp.Close()
		return fmt.Errorf("relocation source changed after manifest creation: %s", payload.Source)
	}
	if err := temp.Chmod(fs.FileMode(payload.Mode)); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if r.afterTempSync != nil {
		r.afterTempSync()
	}
	if err := r.targets.revalidateConfiguredPaths(
		r.options.TargetSessions, r.options.TargetBlobs,
	); err != nil {
		return err
	}
	if err := r.revalidatePinnedSources(); err != nil {
		return err
	}
	if err := root.Link(tempName, relative); err != nil {
		if errors.Is(err, fs.ErrExist) {
			err = fmt.Errorf("%w: target already exists", fs.ErrExist)
		} else {
			err = fmt.Errorf("atomic confined no-replace publication is unsupported: %w", err)
		}
		return fmt.Errorf("publishing relocated payload %s: %w", payload.Destination, err)
	}
	syncTargetDirectory := syncRootDirectory
	if r.syncPublishedTargetDirectory != nil {
		syncTargetDirectory = r.syncPublishedTargetDirectory
	}
	if err := syncTargetDirectory(root, destinationDir); err != nil {
		tempInfo, statErr := root.Lstat(tempName)
		if statErr != nil {
			return &publishedTargetCleanupError{
				err: errors.Join(err, statErr), preserveManifest: true,
			}
		}
		if rollbackErr := r.rollbackPublishedPayloadOwned(payload, tempInfo); rollbackErr != nil {
			return &publishedTargetCleanupError{
				err: errors.Join(err, rollbackErr), preserveManifest: true,
			}
		}
		return err
	}
	return nil
}

type publishedTargetCleanupError struct {
	err              error
	preserveManifest bool
}

type publishedTargetOwnershipError struct {
	message string
}

func (e *publishedTargetOwnershipError) Error() string {
	return e.message
}

func (e *publishedTargetCleanupError) Error() string {
	return e.err.Error()
}

func (e *publishedTargetCleanupError) Unwrap() error {
	return e.err
}

// publishNoClobber uses a hard link from a durable temp in the target
// directory. That operation is genuinely atomic and no-replace. Filesystems
// without it fail loud; reservation+rename is not equivalent and is rejected.
func publishNoClobber(tempName, destination string) error {
	return publishNoClobberWithLink(tempName, destination, os.Link)
}

func publishNoClobberWithLink(tempName, destination string, link func(string, string) error) error {
	if err := link(tempName, destination); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: target already exists", fs.ErrExist)
		}
		return fmt.Errorf("atomic no-replace publication is unsupported: %w", err)
	}
	if err := syncDirectory(filepath.Dir(destination)); err != nil {
		removeErr := os.Remove(destination)
		syncErr := syncDirectory(filepath.Dir(destination))
		return errors.Join(err, removeErr, syncErr)
	}
	return nil
}

func (r *FilesystemSessionStoreRelocator) rollback(manifestPath string, manifest *relocationManifest, published []int, cause error, resumed bool) error {
	var rollbackErrs []error
	for _, payload := range manifest.Payloads {
		category, _, sourceErr := r.sourceRoot(payload.Source)
		if sourceErr != nil {
			rollbackErrs = append(rollbackErrs, sourceErr)
			continue
		}
		if category.absent || category.root == nil {
			continue
		}
		if err := reconcilePayloadRestoreTemp(category.root, payload); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf(
				"reconciling restore controls for %s: %w", payload.Source, err,
			))
		}
	}
	for index := len(published) - 1; index >= 0; index-- {
		payload := &manifest.Payloads[published[index]]
		if err := r.rollbackPublishedPayload(*payload); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("rolling back %s: %w", payload.Destination, err))
			continue
		}
		payload.State = relocationPayloadPlanned
	}
	var cleanupPending *publishedTargetCleanupError
	preserveManifest := errors.As(cause, &cleanupPending) && cleanupPending.preserveManifest
	if !resumed && len(rollbackErrs) == 0 && !preserveManifest {
		if err := r.targets.sessions.Remove(SessionRelocationManifest); err != nil && !errors.Is(err, fs.ErrNotExist) {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("removing rollback manifest: %w", err))
		} else if err := syncRootDirectory(r.targets.sessions, "."); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("syncing rollback manifest removal: %w", err))
		}
	} else if err := r.writeManifest(manifestPath, manifest); err != nil {
		rollbackErrs = append(rollbackErrs, fmt.Errorf("persisting rollback manifest: %w", err))
	}
	return errors.Join(append([]error{cause}, rollbackErrs...)...)
}

func relocationTargetRollbackName(payload relocationManifestPayload) string {
	sum := sha256.Sum256([]byte(payload.Destination))
	return filepath.Join(
		SessionRelocationQuarantineDir,
		hex.EncodeToString(sum[:])+".target-rollback",
	)
}

func verifyPublishedPayloadHandle(payload relocationManifestPayload, file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || uint32(info.Mode().Perm()) != payload.Mode {
		return fmt.Errorf("published relocation target changed before rollback")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return err
	}
	if size != payload.Size || hex.EncodeToString(hash.Sum(nil)) != payload.Digest {
		return fmt.Errorf("published relocation target changed before rollback")
	}
	return nil
}

func (r *FilesystemSessionStoreRelocator) restoreRelocationTargetName(
	root *os.Root,
	quarantine string,
	visible string,
) error {
	quarantineInfo, err := root.Lstat(quarantine)
	if err != nil {
		return err
	}
	visibleInfo, err := root.Lstat(visible)
	if errors.Is(err, fs.ErrNotExist) {
		if err := root.Link(quarantine, visible); err != nil {
			return err
		}
		if r.afterTargetRollbackRestoreLink != nil {
			r.afterTargetRollbackRestoreLink()
		}
		visibleInfo, err = root.Lstat(visible)
	}
	if err != nil {
		return err
	}
	currentQuarantineInfo, quarantineErr := root.Lstat(quarantine)
	if quarantineErr != nil {
		return quarantineErr
	}
	if !quarantineInfo.Mode().IsRegular() || !currentQuarantineInfo.Mode().IsRegular() ||
		!visibleInfo.Mode().IsRegular() ||
		!os.SameFile(quarantineInfo, currentQuarantineInfo) ||
		!os.SameFile(currentQuarantineInfo, visibleInfo) {
		return fmt.Errorf("relocation rollback target collides with a changed visible name")
	}
	if err := syncRootDirectory(root, filepath.Dir(visible)); err != nil {
		return err
	}
	if err := root.Remove(quarantine); err != nil {
		return err
	}
	return syncRootDirectory(root, filepath.Dir(quarantine))
}

func (r *FilesystemSessionStoreRelocator) rollbackPublishedPayload(
	payload relocationManifestPayload,
) error {
	return r.rollbackPublishedPayloadOwned(payload, nil)
}

func (r *FilesystemSessionStoreRelocator) rollbackPublishedPayloadOwned(
	payload relocationManifestPayload,
	expected fs.FileInfo,
) error {
	return r.rollbackPublishedPayloadOwnedImpl(payload, expected, true)
}

func (r *FilesystemSessionStoreRelocator) rollbackPublishedPayloadOwnedImpl(
	payload relocationManifestPayload,
	expected fs.FileInfo,
	acquireSessionLock bool,
) (resultErr error) {
	root, relative, err := r.targetRoot(payload.Destination)
	if err != nil {
		return err
	}
	var sessionLock *pinnedFileLock
	if acquireSessionLock && filepath.Dir(relative) == "." && strings.HasSuffix(relative, ".jsonl") {
		lockFile, err := root.OpenFile(relative+".lock", os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return err
		}
		sessionLock, err = tryPinnedFileLock(lockFile)
		if err != nil {
			return errors.Join(err, lockFile.Close())
		}
		if !sessionLock.locked {
			_ = sessionLock.Unlock()
			return fmt.Errorf("session target is locked by an active writer: %s", payload.Destination)
		}
		defer func() {
			resultErr = errors.Join(resultErr, sessionLock.Unlock())
		}()
	}
	quarantine := relocationTargetRollbackName(payload)
	quarantinePresent, err := rootedRegularPresence(root, quarantine)
	if err != nil {
		return err
	}
	visiblePresent, err := rootedRegularPresence(root, relative)
	if err != nil {
		return err
	}
	if quarantinePresent {
		if expected != nil {
			quarantineInfo, err := root.Lstat(quarantine)
			if err != nil || !os.SameFile(expected, quarantineInfo) {
				if err != nil {
					return err
				}
				return &publishedTargetOwnershipError{
					message: "published relocation target identity changed before rollback",
				}
			}
		}
		if visiblePresent {
			quarantineInfo, quarantineErr := root.Lstat(quarantine)
			visibleInfo, visibleErr := root.Lstat(relative)
			if err := errors.Join(quarantineErr, visibleErr); err != nil {
				return err
			}
			if !os.SameFile(quarantineInfo, visibleInfo) {
				return fmt.Errorf("relocation rollback quarantine collides with a visible target")
			}
			file, err := openRootedRegular(root, quarantine)
			if err != nil {
				return err
			}
			opened, statErr := file.Stat()
			verifyErr := verifyPublishedPayloadHandle(payload, file)
			closeErr := file.Close()
			if err := errors.Join(statErr, closeErr); err != nil {
				return err
			}
			if err := syncRootDirectory(root, filepath.Dir(relative)); err != nil {
				return err
			}
			if r.beforeTargetRollbackFinalUnlink != nil {
				r.beforeTargetRollbackFinalUnlink()
			}
			currentQuarantine, currentErr := root.Lstat(quarantine)
			if currentErr != nil || !os.SameFile(opened, currentQuarantine) {
				if currentErr != nil {
					return currentErr
				}
				return &publishedTargetOwnershipError{
					message: "published relocation target identity changed before rollback unlink",
				}
			}
			if err := root.Remove(quarantine); err != nil {
				return err
			}
			syncErr := syncRootDirectory(root, filepath.Dir(quarantine))
			if verifyErr != nil {
				return errors.Join(verifyErr, syncErr)
			}
			if syncErr != nil {
				return syncErr
			}
			ownedIdentity := expected
			if ownedIdentity == nil {
				ownedIdentity = opened
			}
			return r.rollbackPublishedPayloadOwnedImpl(payload, ownedIdentity, false)
		}
		file, err := openRootedRegular(root, quarantine)
		if err != nil {
			return err
		}
		opened, statErr := file.Stat()
		verifyErr := verifyPublishedPayloadHandle(payload, file)
		closeErr := file.Close()
		if err := errors.Join(statErr, verifyErr, closeErr); err != nil {
			if verifyErr == nil {
				return err
			}
			restoreErr := r.restoreRelocationTargetName(root, quarantine, relative)
			return errors.Join(err, restoreErr)
		}
		if r.beforeTargetRollbackFinalUnlink != nil {
			r.beforeTargetRollbackFinalUnlink()
		}
		currentQuarantine, currentErr := root.Lstat(quarantine)
		if currentErr != nil || !os.SameFile(opened, currentQuarantine) {
			if currentErr != nil {
				return currentErr
			}
			restoreErr := r.restoreRelocationTargetName(root, quarantine, relative)
			return errors.Join(
				&publishedTargetOwnershipError{
					message: "published relocation target identity changed before rollback unlink",
				},
				restoreErr,
			)
		}
		if err := root.Remove(quarantine); err != nil {
			return err
		}
		return syncRootDirectory(root, filepath.Dir(quarantine))
	}
	if !visiblePresent {
		return nil
	}
	if err := root.MkdirAll(SessionRelocationQuarantineDir, 0o700); err != nil {
		return err
	}
	file, err := openRootedRegular(root, relative)
	if err != nil {
		return err
	}
	opened, statErr := file.Stat()
	verifyErr := verifyPublishedPayloadHandle(payload, file)
	if expected != nil && statErr == nil && !os.SameFile(expected, opened) {
		verifyErr = errors.Join(
			verifyErr,
			&publishedTargetOwnershipError{
				message: "published relocation target identity changed before rollback",
			},
		)
	}
	if err := errors.Join(statErr, verifyErr); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := root.Rename(relative, quarantine); err != nil {
		return errors.Join(err, file.Close())
	}
	if r.afterTargetRollbackQuarantine != nil {
		r.afterTargetRollbackQuarantine(quarantine)
	}
	moved, movedErr := root.Lstat(quarantine)
	if err := errors.Join(movedErr); err != nil || !os.SameFile(opened, moved) {
		restoreErr := r.restoreRelocationTargetName(root, quarantine, relative)
		if err == nil {
			err = fmt.Errorf("relocation target changed while entering rollback quarantine")
		}
		return errors.Join(err, file.Close(), restoreErr)
	}
	if err := errors.Join(
		syncRootDirectory(root, filepath.Dir(relative)),
		syncRootDirectory(root, filepath.Dir(quarantine)),
	); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := verifyPublishedPayloadHandle(payload, file); err != nil {
		return errors.Join(
			err,
			file.Close(),
			r.restoreRelocationTargetName(root, quarantine, relative),
		)
	}
	if err := file.Close(); err != nil {
		return err
	}
	if r.beforeTargetRollbackFinalUnlink != nil {
		r.beforeTargetRollbackFinalUnlink()
	}
	currentQuarantine, currentErr := root.Lstat(quarantine)
	if currentErr != nil || !os.SameFile(opened, currentQuarantine) {
		if currentErr != nil {
			return currentErr
		}
		restoreErr := r.restoreRelocationTargetName(root, quarantine, relative)
		return errors.Join(
			&publishedTargetOwnershipError{
				message: "published relocation target identity changed before rollback unlink",
			},
			restoreErr,
		)
	}
	if err := root.Remove(quarantine); err != nil {
		return err
	}
	return syncRootDirectory(root, filepath.Dir(quarantine))
}

func (r *FilesystemSessionStoreRelocator) reconcileTargetRollbackControl(
	payload relocationManifestPayload,
) error {
	root, _, err := r.targetRoot(payload.Destination)
	if err != nil {
		return err
	}
	present, err := rootedRegularPresence(root, relocationTargetRollbackName(payload))
	if err != nil || !present {
		return err
	}
	return r.rollbackPublishedPayload(payload)
}

func (r *FilesystemSessionStoreRelocator) writeSourceTombstones(sources []SessionStoreRelocationSource) error {
	tombstone := SessionRelocationTombstoneRecord{
		Version: 2, TargetProject: r.options.TargetProject,
		TargetSessions: r.options.TargetSessions, TargetStagedBlobs: r.options.TargetBlobs,
		RelocatedAt: r.now().UTC().Round(0),
	}
	for _, source := range sources {
		if !source.WriteTombstone {
			continue
		}
		category, err := r.pinnedCategory(source.Sessions)
		if err != nil {
			return err
		}
		if category.absent || category.root == nil {
			continue
		}
		if err := revalidatePinnedCategoryIdentity(category); err != nil {
			return err
		}
		if err := writeJSONAtomicRoot(category.root, SessionRelocationTombstone, tombstone); err != nil {
			return err
		}
	}
	return nil
}

func (r *FilesystemSessionStoreRelocator) writeTransitionState(state SessionIdentityTransitionState) error {
	if r.options.Transition == nil {
		return nil
	}
	expected := *r.options.Transition
	root, closeRoot, err := r.openTransitionRoot(false)
	if err != nil {
		return err
	}
	defer func() { _ = closeRoot() }()
	current, err := readSessionIdentityTransitionRoot(root, expected.OldSessions)
	if err != nil {
		return err
	}
	if err := r.revalidateBeforeTransitionWrite(); err != nil {
		return err
	}
	if current != nil {
		if !sameTransitionIdentity(*current, expected) {
			return fmt.Errorf("existing session identity transition does not match the active relocation")
		}
		switch current.State {
		case SessionIdentityTransitionCompleted:
			r.options.Transition = current
			return nil
		case SessionIdentityTransitionCutover:
			if state == SessionIdentityTransitionCutover {
				r.options.Transition = current
				return nil
			}
		case SessionIdentityTransitionPending:
		default:
			return fmt.Errorf("unsupported session identity transition state %q", current.State)
		}
		expected = *current
	}
	if expected.State == SessionIdentityTransitionCompleted {
		return nil
	}
	expected.State = state
	expected.CurrentSessions = r.options.TargetSessions
	expected.CurrentBlobs = r.options.TargetBlobs
	if err := r.revalidateBeforeTransitionWrite(); err != nil {
		return err
	}
	if err := writeSessionIdentityTransitionRoot(root, expected); err != nil {
		return err
	}
	r.options.Transition = &expected
	return nil
}

func (r *FilesystemSessionStoreRelocator) cleanupPinnedSources() error {
	for index := range r.pinnedSources {
		source := &r.pinnedSources[index]
		if source.immutableAbsent {
			continue
		}
		if err := removePinnedSessionLockFiles(&source.sessions); err != nil {
			return err
		}
		if err := prunePinnedCategory(&source.sessions, source.source.WriteTombstone); err != nil {
			return err
		}
		if r.beforeSourcePrune != nil {
			r.beforeSourcePrune(source.blobs.path)
		}
		if err := prunePinnedCategory(&source.blobs, false); err != nil {
			return err
		}
	}
	return nil
}

func removePinnedSessionLockFiles(category *pinnedSourceCategory) error {
	if category.absent || category.root == nil {
		return nil
	}
	if err := revalidatePinnedCategoryIdentity(category); err != nil {
		return err
	}
	entries, err := fs.ReadDir(category.root.FS(), ".")
	if err != nil {
		return err
	}
	removed := false
	for _, entry := range entries {
		path := filepath.Join(category.path, entry.Name())
		if entry.IsDir() || !isTopLevelSessionLock(category.path, path) {
			continue
		}
		if err := revalidatePinnedCategoryIdentity(category); err != nil {
			return err
		}
		if err := category.root.Remove(entry.Name()); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		removed = true
	}
	if removed {
		return syncRootDirectory(category.root, ".")
	}
	return nil
}

func prunePinnedCategory(category *pinnedSourceCategory, keepRoot bool) error {
	if category.absent || category.root == nil {
		return nil
	}
	if err := revalidatePinnedCategoryIdentity(category); err != nil {
		return err
	}
	var dirs []string
	err := fs.WalkDir(category.root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && path != "." {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, dir := range dirs {
		if err := revalidatePinnedCategoryIdentity(category); err != nil {
			return err
		}
		if err := category.root.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			entries, readErr := fs.ReadDir(category.root.FS(), dir)
			if readErr == nil && len(entries) > 0 {
				continue
			}
			return err
		}
	}
	if keepRoot {
		return nil
	}
	if err := revalidatePinnedCategoryIdentity(category); err != nil {
		return fmt.Errorf("relocation source root was replaced before pruning: %w", err)
	}
	if category.chain != nil {
		if err := category.chain.removeVerifiedLeaf(); err != nil {
			return err
		}
		category.root = nil
		category.absent = true
		return nil
	} else {
		if err := category.root.Close(); err != nil {
			return err
		}
		category.root = nil
		pathInfo, err := category.parent.Lstat(category.base)
		if err != nil {
			return err
		}
		if !os.SameFile(category.identity, pathInfo) {
			return fmt.Errorf("relocation source root was replaced before removal: %s", category.path)
		}
	}
	if err := category.parent.Remove(category.base); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	category.absent = true
	return syncRootDirectory(category.parent, ".")
}

type SessionStoreCollisionError struct {
	Collisions []RelocationItem
}

func (e *SessionStoreCollisionError) Error() string {
	return fmt.Sprintf("session-store relocation found %d target payload collision(s); no new source payload was moved", len(e.Collisions))
}

func pruneEmptyDirectories(root string, keepRoot bool) error {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, dir := range dirs {
		if keepRoot && dir == root {
			continue
		}
		if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			if entries, readErr := os.ReadDir(dir); readErr == nil && len(entries) > 0 {
				continue
			}
			return err
		}
	}
	return nil
}

// RelocateSessionStore is retained for the single-directory compatibility
// tests. It follows the same preflight-all and hard-link no-replace rules.
func RelocateSessionStore(sourceDir, targetDir string) (moved, skipped []string, err error) {
	files, err := storePayloadFiles(sourceDir, true)
	if err != nil {
		return nil, nil, err
	}
	for _, source := range files {
		rel, err := filepath.Rel(sourceDir, source)
		if err != nil {
			return nil, nil, err
		}
		destination := filepath.Join(targetDir, rel)
		if _, err := os.Lstat(destination); err == nil {
			skipped = append(skipped, rel)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
	}
	if len(skipped) > 0 {
		collisions := make([]RelocationItem, 0, len(skipped))
		for _, rel := range skipped {
			collisions = append(collisions, RelocationItem{
				Source: filepath.Join(sourceDir, rel), Destination: filepath.Join(targetDir, rel),
			})
		}
		return nil, skipped, &SessionStoreCollisionError{Collisions: collisions}
	}
	var published []string
	for _, source := range files {
		rel, _ := filepath.Rel(sourceDir, source)
		destination := filepath.Join(targetDir, rel)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return nil, nil, err
		}
		input, err := os.Open(source)
		if err != nil {
			return nil, nil, err
		}
		info, err := input.Stat()
		if err != nil {
			_ = input.Close()
			return nil, nil, err
		}
		temp, err := os.CreateTemp(filepath.Dir(destination), ".sdd-relocate-*")
		if err != nil {
			_ = input.Close()
			return nil, nil, err
		}
		tempName := temp.Name()
		_, copyErr := io.Copy(temp, input)
		err = errors.Join(copyErr, input.Close(), temp.Chmod(info.Mode().Perm()), temp.Sync(), temp.Close())
		if err == nil {
			err = publishNoClobber(tempName, destination)
		}
		_ = os.Remove(tempName)
		if err != nil {
			var rollback []error
			for _, path := range published {
				rollback = append(rollback, os.Remove(path))
			}
			return nil, nil, errors.Join(append([]error{err}, rollback...)...)
		}
		published = append(published, destination)
	}
	for _, source := range files {
		if err := os.Remove(source); err != nil {
			return moved, nil, err
		}
		rel, _ := filepath.Rel(sourceDir, source)
		moved = append(moved, rel)
	}
	return moved, nil, pruneEmptyDirectories(sourceDir, false)
}
