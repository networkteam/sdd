package local

import (
	"bufio"
	"bytes"
	"context"
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
	"sync"
	"time"

	app "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/engine"
)

// FilesystemLegacySessionMigrator converts the event-only session logs used
// before v0.16 into the current filesystem session envelope. Migration is an
// explicit maintenance operation; FilesystemSessionStore never invokes it
// while serving runtime reads.
type FilesystemLegacySessionMigrator struct {
	sessions         *FilesystemSessionStore
	blobs            *FilesystemStagedBlobStore
	subject          string
	project          app.ProjectID
	trustedStateRoot string
	roots            *relocationTargetRoots
	mu               sync.Mutex
	closed           bool

	beforeMutation              func()
	afterSourceVerified         func()
	afterSourceQuarantine       func()
	afterQuarantineRemove       func()
	afterBlobPlanPersisted      func()
	beforeBlobProofWrite        func()
	duringBlobStage             func()
	afterBlobProofPrepared      func()
	beforeSessionProofWrite     func()
	duringSessionProofWrite     func()
	afterSessionProofPrepared   func()
	afterBlobDataPublished      func()
	afterBlobMetadataPublished  func()
	afterBlobProofRemoved       func()
	afterBlobRollbackPair       func()
	beforeBlobFinalQuarantine   func()
	afterSessionPublished       func()
	beforeLiveSessionQuarantine func()
	afterLiveSessionQuarantine  func()
	afterQuarantineRestoreLink  func()
	afterSessionProofRemoved    func()
	afterSessionRollbackProof   func()
	afterCommittedJournal       func()
	renameSessionProof          func(*os.Root, string, string) error
	removeQuarantine            func(*os.Root, string) error
	removeLegacyStaging         func(*os.Root, string) error
}

const legacyMigrationControlDir = ".legacy-migration"

type legacyMigrationJournal struct {
	Version int               `json:"version"`
	State   string            `json:"state"`
	Session legacySessionPlan `json:"session"`
	Blobs   []legacyBlobPlan  `json:"blobs"`
}

const (
	legacyMigrationPrepared  = "prepared"
	legacyMigrationRollback  = "rolling_back"
	legacyMigrationCommitted = "aggregate_committed"
)

type legacySessionPlan struct {
	Proof  string `json:"proof"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type legacyBlobPlan struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	Digest    string    `json:"digest"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

type legacyStagedInput struct {
	plan legacyBlobPlan
	data []byte
}

func legacyBlobPlanCreatedAt(metadata app.SessionMetadata) time.Time {
	if metadata.UpdatedAt.IsZero() {
		return time.Unix(0, 0).UTC()
	}
	return metadata.UpdatedAt.UTC().Round(0)
}

func legacySessionProofName(id app.SessionID) string {
	return filepath.Join(legacyMigrationControlDir, string(id)+".prepared")
}

func legacySessionProofCleanupName(id app.SessionID) string {
	return filepath.Join(legacyMigrationControlDir, string(id)+".prepared-cleanup")
}

func legacyPublishedRollbackName(id app.SessionID) string {
	return filepath.Join(legacyMigrationControlDir, string(id)+".published-rollback")
}

func validateLegacySessionPlan(id app.SessionID, plan legacySessionPlan) error {
	if plan.Proof != legacySessionProofName(id) {
		return fmt.Errorf("legacy migration journal has invalid prepared-session proof path")
	}
	decoded, err := hex.DecodeString(plan.Digest)
	if err != nil || len(decoded) != sha256.Size || plan.Size <= 0 {
		return fmt.Errorf("legacy migration journal has invalid prepared-session identity")
	}
	return nil
}

func (m *FilesystemLegacySessionMigrator) prepareLegacySessionProof(
	id app.SessionID,
	metadata app.SessionMetadata,
	events []app.StoredEvent,
) (legacySessionPlan, error) {
	proof := legacySessionProofName(id)
	encoded, err := json.Marshal(sessionLine{
		Version: 1, Metadata: &metadata, Events: events,
	})
	if err != nil {
		return legacySessionPlan{}, err
	}
	encoded = append(encoded, '\n')
	plan := legacySessionPlan{
		Proof: proof, Digest: sha256Digest(encoded).Value, Size: int64(len(encoded)),
	}
	if err := reconcilePreparedRootedFile(
		m.roots.sessions,
		proof,
		encoded,
		m.beforeSessionProofWrite,
		m.duringSessionProofWrite,
	); err != nil {
		return legacySessionPlan{}, err
	}
	if err := verifyLegacySessionProof(m.roots.sessions, id, plan); err != nil {
		return legacySessionPlan{}, err
	}
	if m.afterSessionProofPrepared != nil {
		m.afterSessionProofPrepared()
	}
	return plan, nil
}

func removeRootedFileIfSame(root *os.Root, path string, created fs.FileInfo) error {
	info, err := root.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !os.SameFile(info, created) {
		return fmt.Errorf("refusing to clean replaced migration preparation: %s", path)
	}
	return root.Remove(path)
}

func reconcilePreparedRootedFile(
	root *os.Root,
	path string,
	expected []byte,
	beforeWrite func(),
	duringWrite func(),
) (err error) {
	file, openErr := openRootedRegular(root, path)
	switch {
	case openErr == nil:
		info, statErr := file.Stat()
		raw, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if err := errors.Join(statErr, readErr, closeErr); err != nil {
			return err
		}
		if bytes.Equal(raw, expected) {
			return nil
		}
		if len(raw) >= len(expected) || !bytes.Equal(raw, expected[:len(raw)]) {
			return fmt.Errorf("conflicting nonmatching migration proof: %s", path)
		}
		if err := removeRootedFileIfSame(root, path, info); err != nil {
			return err
		}
		if err := syncRootDirectory(root, filepath.Dir(path)); err != nil {
			return err
		}
	case errors.Is(openErr, fs.ErrNotExist):
	default:
		return openErr
	}

	file, err = root.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	createdInfo, err := file.Stat()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(
				err,
				removeRootedFileIfSame(root, path, createdInfo),
				syncRootDirectory(root, filepath.Dir(path)),
			)
		}
	}()
	if beforeWrite != nil {
		beforeWrite()
	}
	split := len(expected) / 2
	if written, writeErr := file.Write(expected[:split]); writeErr != nil || written != split {
		return errors.Join(writeErr, io.ErrShortWrite)
	}
	if duringWrite != nil {
		duringWrite()
	}
	if written, writeErr := file.Write(expected[split:]); writeErr != nil ||
		written != len(expected)-split {
		return errors.Join(writeErr, io.ErrShortWrite)
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return err
	}
	atName, err := root.Lstat(path)
	if err != nil || !atName.Mode().IsRegular() || !os.SameFile(createdInfo, atName) {
		if err == nil {
			err = fmt.Errorf("prepared migration proof was replaced while writing: %s", path)
		}
		return err
	}
	return syncRootDirectory(root, filepath.Dir(path))
}

func verifyCurrentSessionCodec(
	root *os.Root,
	path string,
	id app.SessionID,
) error {
	file, err := openRootedRegular(root, path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		if len(strings.TrimSpace(scanner.Text())) == 0 {
			continue
		}
		var line sessionLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return err
		}
		if line.Version != 1 || line.Metadata == nil ||
			line.Metadata.CodecVersion != app.SessionCodecVersion ||
			line.Metadata.ID != id {
			return fmt.Errorf("prepared session does not use the current session codec")
		}
		for _, event := range line.Events {
			if event.CodecVersion != app.SessionCodecVersion {
				return fmt.Errorf("prepared session event does not use the current session codec")
			}
		}
		return nil
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("prepared session is empty")
}

func verifyLegacySessionProof(
	root *os.Root,
	id app.SessionID,
	plan legacySessionPlan,
) error {
	if err := validateLegacySessionPlan(id, plan); err != nil {
		return err
	}
	return verifyLegacySessionFile(root, plan.Proof, id, plan)
}

func verifyLegacySessionFile(
	root *os.Root,
	path string,
	id app.SessionID,
	plan legacySessionPlan,
) error {
	digest, size, err := rootedFileDigest(root, path)
	if err != nil {
		return err
	}
	if digest != plan.Digest || size != plan.Size {
		return fmt.Errorf("prepared session proof differs from migration journal")
	}
	return verifyCurrentSessionCodec(root, path, id)
}

func verifyCommittedLegacySession(
	root *os.Root,
	name string,
	id app.SessionID,
	plan legacySessionPlan,
) error {
	if err := validateLegacySessionPlan(id, plan); err != nil {
		return err
	}
	return verifyLegacySessionFile(root, name, id, plan)
}

func verifyPublishedLegacySession(
	root *os.Root,
	name string,
	id app.SessionID,
	plan legacySessionPlan,
) error {
	if err := verifyLegacySessionProof(root, id, plan); err != nil {
		return err
	}
	proofInfo, err := root.Lstat(plan.Proof)
	if err != nil {
		return err
	}
	liveInfo, err := root.Lstat(name)
	if err != nil || !proofInfo.Mode().IsRegular() || !liveInfo.Mode().IsRegular() ||
		!os.SameFile(proofInfo, liveInfo) {
		return fmt.Errorf("live current session lacks prepared-session ownership proof")
	}
	digest, size, err := rootedFileDigest(root, name)
	if err != nil {
		return err
	}
	if digest != plan.Digest || size != plan.Size {
		return fmt.Errorf("live current session differs from prepared-session journal identity")
	}
	return verifyCurrentSessionCodec(root, name, id)
}

func (m *FilesystemLegacySessionMigrator) rollbackLegacySessionProof(
	name string,
	id app.SessionID,
	plan legacySessionPlan,
	allowMissing bool,
) error {
	root := m.roots.sessions
	cleanup := legacySessionProofCleanupName(id)
	proofPresent, err := rootedRegularPresence(root, plan.Proof)
	if err != nil {
		return err
	}
	cleanupPresent, err := rootedRegularPresence(root, cleanup)
	if err != nil {
		return err
	}
	if proofPresent && cleanupPresent {
		proofInfo, proofErr := root.Lstat(plan.Proof)
		cleanupInfo, cleanupErr := root.Lstat(cleanup)
		if err := errors.Join(proofErr, cleanupErr); err != nil {
			return err
		}
		if !os.SameFile(proofInfo, cleanupInfo) {
			return fmt.Errorf("prepared-session proof and cleanup quarantine both exist")
		}
		if err := root.Remove(cleanup); err != nil {
			return err
		}
		if err := syncRootDirectory(root, legacyMigrationControlDir); err != nil {
			return err
		}
		cleanupPresent = false
	}
	if cleanupPresent {
		if !allowMissing {
			return fmt.Errorf("prepared-session cleanup quarantine exists before rollback state")
		}
		if err := verifyLegacySessionFile(root, cleanup, id, plan); err != nil {
			return fmt.Errorf("refusing to clean unverified prepared-session cleanup quarantine: %w", err)
		}
		if err := root.Remove(cleanup); err != nil {
			return err
		}
		if err := syncRootDirectory(root, legacyMigrationControlDir); err != nil {
			return err
		}
		if m.afterSessionRollbackProof != nil {
			m.afterSessionRollbackProof()
		}
		return nil
	}
	if !proofPresent {
		if allowMissing {
			return nil
		}
		return fmt.Errorf("prepared-session proof is missing before rollback state")
	}
	if err := verifyLegacySessionProof(root, id, plan); err != nil {
		return fmt.Errorf("refusing to clean unverified prepared-session proof: %w", err)
	}
	proofInfo, err := root.Lstat(plan.Proof)
	if err != nil {
		return err
	}
	if liveInfo, liveErr := root.Lstat(name); liveErr == nil &&
		liveInfo.Mode().IsRegular() && os.SameFile(proofInfo, liveInfo) {
		return fmt.Errorf("refusing to remove prepared-session proof while it is still published")
	} else if liveErr != nil && !errors.Is(liveErr, fs.ErrNotExist) {
		return liveErr
	}
	if err := root.Rename(plan.Proof, cleanup); err != nil {
		return err
	}
	if err := verifyLegacySessionFile(root, cleanup, id, plan); err != nil {
		restoreErr := m.restoreQuarantinedVisibleName(root, cleanup, plan.Proof)
		return errors.Join(err, restoreErr)
	}
	if err := root.Remove(cleanup); err != nil {
		return err
	}
	if err := syncRootDirectory(root, legacyMigrationControlDir); err != nil {
		return err
	}
	if m.afterSessionRollbackProof != nil {
		m.afterSessionRollbackProof()
	}
	return nil
}

func rootedRegularPresence(root *os.Root, path string) (bool, error) {
	info, err := root.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("migration control is not regular: %s", path)
	}
	return true, nil
}

func rootedFileMatchesAny(root *os.Root, path string, candidates ...string) (bool, error) {
	info, err := root.Lstat(path)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("migration live name is not regular: %s", path)
	}
	for _, candidate := range candidates {
		candidateInfo, candidateErr := root.Lstat(candidate)
		if errors.Is(candidateErr, fs.ErrNotExist) {
			continue
		}
		if candidateErr != nil {
			return false, candidateErr
		}
		if !candidateInfo.Mode().IsRegular() {
			return false, fmt.Errorf("migration control is not regular: %s", candidate)
		}
		if os.SameFile(info, candidateInfo) {
			return true, nil
		}
	}
	return false, nil
}

func (m *FilesystemLegacySessionMigrator) quarantinePublishedLegacySession(
	name string,
	id app.SessionID,
	plan legacySessionPlan,
) error {
	root := m.roots.sessions
	rollback := legacyPublishedRollbackName(id)
	rollbackPresent, err := rootedRegularPresence(root, rollback)
	if err != nil {
		return err
	}
	livePresent, err := rootedRegularPresence(root, name)
	if err != nil {
		return err
	}
	if rollbackPresent {
		if livePresent {
			rollbackInfo, rollbackErr := root.Lstat(rollback)
			liveInfo, liveErr := root.Lstat(name)
			if err := errors.Join(rollbackErr, liveErr); err != nil {
				return err
			}
			if !os.SameFile(rollbackInfo, liveInfo) {
				return fmt.Errorf("published-session rollback control collides with the live name")
			}
			verifyErr := verifyLegacySessionFile(root, rollback, id, plan)
			proofInfo, proofErr := root.Lstat(plan.Proof)
			owned := proofErr == nil && os.SameFile(proofInfo, rollbackInfo)
			if err := root.Remove(rollback); err != nil {
				return err
			}
			if err := errors.Join(
				syncRootDirectory(root, legacyMigrationControlDir),
				syncRootDirectory(root, "."),
			); err != nil {
				return err
			}
			if verifyErr != nil {
				return fmt.Errorf("restored foreign published-session rollback control: %w", verifyErr)
			}
			if !owned {
				if proofErr != nil {
					return proofErr
				}
				return fmt.Errorf("restored published-session rollback control without prepared proof ownership")
			}
			if err := root.Remove(name); err != nil {
				return err
			}
			return syncRootDirectory(root, ".")
		}
		if err := verifyLegacySessionFile(root, rollback, id, plan); err != nil {
			restoreErr := m.restoreQuarantinedVisibleName(root, rollback, name)
			return errors.Join(
				fmt.Errorf("restored foreign published-session rollback control: %w", err),
				restoreErr,
			)
		}
		proofInfo, err := root.Lstat(plan.Proof)
		if err != nil {
			return err
		}
		rollbackInfo, err := root.Lstat(rollback)
		if err != nil || !os.SameFile(proofInfo, rollbackInfo) {
			return fmt.Errorf("published-session rollback control lacks prepared proof ownership")
		}
		if err := root.Remove(rollback); err != nil {
			return err
		}
		return errors.Join(
			syncRootDirectory(root, legacyMigrationControlDir),
			syncRootDirectory(root, "."),
		)
	}
	if !livePresent {
		return nil
	}
	if err := verifyPublishedLegacySession(root, name, id, plan); err != nil {
		return err
	}
	proofInfo, err := root.Lstat(plan.Proof)
	if err != nil {
		return err
	}
	if m.beforeLiveSessionQuarantine != nil {
		m.beforeLiveSessionQuarantine()
	}
	if err := root.Rename(name, rollback); err != nil {
		return err
	}
	if m.afterLiveSessionQuarantine != nil {
		m.afterLiveSessionQuarantine()
	}
	rollbackInfo, err := root.Lstat(rollback)
	if err != nil || !os.SameFile(proofInfo, rollbackInfo) {
		restoreErr := m.restoreQuarantinedVisibleName(root, rollback, name)
		return errors.Join(
			fmt.Errorf("live session changed while entering rollback quarantine"),
			restoreErr,
		)
	}
	if err := verifyLegacySessionFile(root, rollback, id, plan); err != nil {
		restoreErr := m.restoreQuarantinedVisibleName(root, rollback, name)
		return errors.Join(err, restoreErr)
	}
	if err := root.Remove(rollback); err != nil {
		return err
	}
	return errors.Join(
		syncRootDirectory(root, legacyMigrationControlDir),
		syncRootDirectory(root, "."),
	)
}

func (m *FilesystemLegacySessionMigrator) ensureCommittedLegacySession(
	name string,
	id app.SessionID,
	plan legacySessionPlan,
) (string, error) {
	root := m.roots.sessions
	proof := plan.Proof
	cleanup := legacySessionProofCleanupName(id)
	proofPresent, err := rootedRegularPresence(root, proof)
	if err != nil {
		return "", err
	}
	cleanupPresent, err := rootedRegularPresence(root, cleanup)
	if err != nil {
		return "", err
	}
	if proofPresent && cleanupPresent {
		proofInfo, proofErr := root.Lstat(proof)
		cleanupInfo, cleanupErr := root.Lstat(cleanup)
		if err := errors.Join(proofErr, cleanupErr); err != nil {
			return "", err
		}
		if !os.SameFile(proofInfo, cleanupInfo) {
			return "", fmt.Errorf("prepared-session proof and cleanup quarantine both exist")
		}
		if err := root.Remove(cleanup); err != nil {
			return "", err
		}
		if err := syncRootDirectory(root, legacyMigrationControlDir); err != nil {
			return "", err
		}
		cleanupPresent = false
	}
	proofPath := ""
	switch {
	case proofPresent:
		proofPath = proof
	case cleanupPresent:
		proofPath = cleanup
	}
	livePresent, err := rootedRegularPresence(root, name)
	if err != nil {
		return "", err
	}
	if !livePresent {
		if proofPath == "" {
			return "", fmt.Errorf("committed legacy migration lacks both live session and prepared proof")
		}
		if err := verifyLegacySessionFile(root, proofPath, id, plan); err != nil {
			return "", err
		}
		if err := root.Link(proofPath, name); err != nil {
			return "", fmt.Errorf("republishing committed legacy session without replacement: %w", err)
		}
		if err := syncRootDirectory(root, "."); err != nil {
			return "", err
		}
	}
	if proofPath == "" {
		if err := verifyCommittedLegacySession(root, name, id, plan); err != nil {
			return "", err
		}
		return "", nil
	}
	if err := verifyLegacySessionFile(root, proofPath, id, plan); err != nil {
		return "", err
	}
	proofInfo, err := root.Lstat(proofPath)
	if err != nil {
		return "", err
	}
	liveInfo, err := root.Lstat(name)
	if err != nil || !proofInfo.Mode().IsRegular() || !liveInfo.Mode().IsRegular() ||
		!os.SameFile(proofInfo, liveInfo) {
		return "", fmt.Errorf("committed live session lacks prepared-session ownership proof")
	}
	if err := verifyCommittedLegacySession(root, name, id, plan); err != nil {
		return "", err
	}
	return proofPath, nil
}

func (m *FilesystemLegacySessionMigrator) finalizeCommittedLegacyMigration(
	name string,
	id app.SessionID,
	journal legacyMigrationJournal,
) error {
	proofPath, err := m.ensureCommittedLegacySession(name, id, journal.Session)
	if err != nil {
		return err
	}
	cleanup := legacySessionProofCleanupName(id)
	if proofPath == journal.Session.Proof {
		renameProof := m.roots.sessions.Rename
		if m.renameSessionProof != nil {
			renameProof = func(oldName, newName string) error {
				return m.renameSessionProof(m.roots.sessions, oldName, newName)
			}
		}
		if err := renameProof(journal.Session.Proof, cleanup); err != nil {
			return err
		}
		proofPath = cleanup
	}
	if proofPath == cleanup {
		if _, err := m.ensureCommittedLegacySession(name, id, journal.Session); err != nil {
			restoreErr := m.restoreQuarantinedVisibleName(
				m.roots.sessions, cleanup, journal.Session.Proof,
			)
			return errors.Join(err, restoreErr)
		}
		if err := m.roots.sessions.Remove(cleanup); err != nil {
			return err
		}
		if err := syncRootDirectory(m.roots.sessions, legacyMigrationControlDir); err != nil {
			return err
		}
		if m.afterSessionProofRemoved != nil {
			m.afterSessionProofRemoved()
		}
	}
	if err := verifyCommittedLegacySession(
		m.roots.sessions, name, id, journal.Session,
	); err != nil {
		return err
	}
	return nil
}

func restoreLegacyOriginalFromControls(
	root *os.Root,
	name string,
	quarantineName string,
	restoreName string,
) error {
	quarantinePresent, err := rootedRegularPresence(root, quarantineName)
	if err != nil {
		return err
	}
	restorePresent, err := rootedRegularPresence(root, restoreName)
	if err != nil {
		return err
	}
	if !quarantinePresent && !restorePresent {
		return nil
	}
	source := quarantineName
	if restorePresent {
		source = restoreName
	}
	if quarantinePresent && restorePresent {
		quarantineInfo, err := root.Lstat(quarantineName)
		if err != nil {
			return err
		}
		restoreInfo, err := root.Lstat(restoreName)
		if err != nil {
			return err
		}
		if !os.SameFile(quarantineInfo, restoreInfo) {
			return fmt.Errorf("legacy migration source controls have different identities")
		}
	}
	sourceInfo, err := root.Lstat(source)
	if err != nil {
		return err
	}
	liveInfo, liveErr := root.Lstat(name)
	switch {
	case errors.Is(liveErr, fs.ErrNotExist):
		if err := root.Link(source, name); err != nil {
			return err
		}
	case liveErr != nil:
		return liveErr
	case !liveInfo.Mode().IsRegular() || !os.SameFile(sourceInfo, liveInfo):
		return fmt.Errorf("refusing to replace a foreign live session while restoring legacy controls")
	}
	if quarantinePresent {
		if err := root.Remove(quarantineName); err != nil {
			return err
		}
	}
	if restorePresent {
		if err := root.Remove(restoreName); err != nil {
			return err
		}
	}
	return errors.Join(
		syncRootDirectory(root, legacyMigrationControlDir),
		syncRootDirectory(root, "."),
	)
}

func NewFilesystemLegacySessionMigrator(sessionsDir, blobsDir, subject string, project app.ProjectID) (*FilesystemLegacySessionMigrator, error) {
	stateRoot, err := inferTrustedStateRoot(sessionsDir, blobsDir)
	if err != nil {
		return nil, fmt.Errorf("sdd: isolated legacy migrator paths: %w", err)
	}
	return NewFilesystemLegacySessionMigratorAtStateRoot(
		stateRoot, sessionsDir, blobsDir, subject, project,
	)
}

func NewFilesystemLegacySessionMigratorAtStateRoot(
	trustedStateRoot string,
	sessionsDir string,
	blobsDir string,
	subject string,
	project app.ProjectID,
) (*FilesystemLegacySessionMigrator, error) {
	if sessionsDir == "" || blobsDir == "" || subject == "" || project == "" {
		return nil, fmt.Errorf("sdd: migration session directory, blob directory, subject, and project are required")
	}
	if !pathAtOrInside(filepath.Join(trustedStateRoot, "sessions"), sessionsDir) ||
		!pathAtOrInside(filepath.Join(trustedStateRoot, "staged-blobs"), blobsDir) {
		return nil, fmt.Errorf("sdd: legacy migration stores must be confined to the trusted state root")
	}
	sessions := &FilesystemSessionStore{dir: sessionsDir}
	blobs := &FilesystemStagedBlobStore{dir: blobsDir}
	return &FilesystemLegacySessionMigrator{
		sessions: sessions, blobs: blobs, subject: subject, project: project,
		trustedStateRoot: trustedStateRoot,
	}, nil
}

func inferTrustedStateRoot(sessionsDir, blobsDir string) (string, error) {
	sessions := filepath.Clean(sessionsDir)
	blobs := filepath.Clean(blobsDir)
	for current := sessions; ; current = filepath.Dir(current) {
		if filepath.Base(current) == "sessions" {
			candidate := filepath.Dir(current)
			if pathAtOrInside(filepath.Join(candidate, "staged-blobs"), blobs) {
				return candidate, nil
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return "", fmt.Errorf("use sibling sessions and staged-blobs categories")
}

func (m *FilesystemLegacySessionMigrator) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	if m.roots == nil {
		return nil
	}
	err := m.roots.close()
	m.roots = nil
	return err
}

func (m *FilesystemLegacySessionMigrator) revalidate() error {
	if m.closed || m.roots == nil {
		return fmt.Errorf("legacy session migrator is closed")
	}
	return m.roots.revalidateConfiguredPaths(m.sessions.dir, m.blobs.dir)
}

func (m *FilesystemLegacySessionMigrator) ensureMutationRoots() error {
	if m.closed {
		return fmt.Errorf("legacy session migrator is closed")
	}
	if m.roots != nil {
		return m.revalidate()
	}
	roots, err := openRelocationTargetRoots(
		m.trustedStateRoot, m.sessions.dir, m.blobs.dir, true,
	)
	if err != nil {
		return err
	}
	m.roots = roots
	return nil
}

func (m *FilesystemLegacySessionMigrator) prepareMutation() error {
	if m.beforeMutation != nil {
		m.beforeMutation()
		m.beforeMutation = nil
	}
	return m.revalidate()
}

// ListLegacySessions returns absolute paths for every non-current JSONL record.
// Unreadable legacy records are included so an accepted migration reports the
// exact affected path instead of hiding them.
func (m *FilesystemLegacySessionMigrator) ListLegacySessions(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("legacy session migrator is closed")
	}
	root, err := openTrustedStoreRoot(
		m.trustedStateRoot, "sessions", m.sessions.dir, false,
	)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.close() }()
	entries, err := fs.ReadDir(root.store.FS(), ".")
	if err != nil {
		return nil, err
	}
	pathSet := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(m.sessions.dir, entry.Name())
		file, err := openRootedRegular(root.store, entry.Name())
		if err != nil {
			return nil, err
		}
		format, classifyErr := classifySessionHandle(file)
		closeErr := file.Close()
		if err := errors.Join(classifyErr, closeErr); err != nil {
			return nil, err
		}
		if format == sessionFormatLegacy {
			pathSet[path] = struct{}{}
		}
	}
	controlPaths, err := m.listPendingLegacySessionsRoot(root.store)
	if err != nil {
		return nil, err
	}
	for _, path := range controlPaths {
		pathSet[path] = struct{}{}
	}
	paths := make([]string, 0, len(pathSet))
	for path := range pathSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

// ListPendingLegacySessions returns the sessions named by application-owned
// migration controls, including transactions whose live name is temporarily
// absent. It does not interpret controls as relocation payload.
func (m *FilesystemLegacySessionMigrator) ListPendingLegacySessions(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("legacy session migrator is closed")
	}
	root, err := openTrustedStoreRoot(
		m.trustedStateRoot, "sessions", m.sessions.dir, false,
	)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.close() }()
	return m.listPendingLegacySessionsRoot(root.store)
}

func (m *FilesystemLegacySessionMigrator) listPendingLegacySessionsRoot(root *os.Root) ([]string, error) {
	controlInfo, err := root.Lstat(legacyMigrationControlDir)
	if err == nil {
		if !controlInfo.IsDir() {
			return nil, fmt.Errorf("legacy migration control path is not a directory")
		}
		controls, err := fs.ReadDir(root.FS(), legacyMigrationControlDir)
		if err != nil {
			return nil, err
		}
		pathSet := make(map[string]struct{})
		for _, entry := range controls {
			id, recognized := legacyMigrationControlSessionID(entry.Name())
			if !recognized {
				continue
			}
			controlName := filepath.Join(legacyMigrationControlDir, entry.Name())
			file, err := openRootedRegular(root, controlName)
			if err != nil {
				return nil, err
			}
			if err := file.Close(); err != nil {
				return nil, err
			}
			if _, err := m.sessions.filename(id); err != nil {
				return nil, fmt.Errorf("invalid legacy migration control %s: %w", controlName, err)
			}
			pathSet[filepath.Join(m.sessions.dir, string(id)+".jsonl")] = struct{}{}
		}
		paths := make([]string, 0, len(pathSet))
		for path := range pathSet {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		return paths, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return nil, nil
}

func legacyMigrationControlSessionID(name string) (app.SessionID, bool) {
	for _, suffix := range []string{
		".published-rollback",
		".prepared-cleanup",
		".original",
		".prepared",
		".restore",
		".json",
	} {
		if strings.HasSuffix(name, suffix) {
			id := strings.TrimSuffix(name, suffix)
			return app.SessionID(id), id != ""
		}
	}
	return "", false
}

// MigrateLegacySession replaces one legacy log atomically. A current record is
// not rewritten, but any legacy staging directory left by an earlier migration
// is removed. Any staged blobs created before replacement are removed again if
// conversion fails, leaving the legacy log and staging directory untouched for
// inspection and retry.
func (m *FilesystemLegacySessionMigrator) MigrateLegacySession(_ context.Context, path string) (err error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absDir, err := filepath.Abs(m.sessions.dir)
	if err != nil {
		return err
	}
	if filepath.Dir(absPath) != absDir || !strings.HasSuffix(filepath.Base(absPath), ".jsonl") {
		return fmt.Errorf("sdd: legacy session path is outside the session directory")
	}
	id := app.SessionID(strings.TrimSuffix(filepath.Base(absPath), ".jsonl"))
	filename, err := m.sessions.filename(id)
	if err != nil {
		return err
	}
	name := filepath.Base(filename)

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureMutationRoots(); err != nil {
		return err
	}
	lockFile, err := m.roots.sessions.OpenFile(name+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	lock, err := tryPinnedFileLock(lockFile)
	if err != nil {
		return errors.Join(err, lockFile.Close())
	}
	if !lock.locked {
		_ = lock.Unlock()
		return fmt.Errorf("legacy session %s is locked by another process", id)
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	recoveredCurrent, err := m.recoverLegacyMigration(id, name)
	if err != nil {
		return err
	}
	if recoveredCurrent {
		return m.completeLegacyMigration(
			filepath.Join(legacyMigrationControlDir, string(id)+".json"),
			filepath.Base(filepath.Join(m.sessions.dir, string(id)+"-staging")),
		)
	}

	input, err := openRootedRegular(m.roots.sessions, name)
	if err != nil {
		return err
	}
	format, err := classifySessionHandle(input)
	if err != nil {
		_ = input.Close()
		return err
	}
	stagingDir := filepath.Join(m.sessions.dir, string(id)+"-staging")
	stagingRelative := filepath.Base(stagingDir)
	if format == sessionFormatCurrent {
		_ = input.Close()
		if err := m.prepareMutation(); err != nil {
			return err
		}
		return m.removeLegacyStagingDir(stagingRelative)
	}

	legacy, err := readLegacyEventsHandle(input, id)
	if err != nil {
		_ = input.Close()
		return err
	}
	metadata := legacyMetadata(id, m.subject, m.project, legacy.events)
	owner := app.BlobOwner{Subject: m.subject, Session: id}
	journalName := filepath.Join(legacyMigrationControlDir, string(id)+".json")
	quarantineName := filepath.Join(legacyMigrationControlDir, string(id)+".original")
	restoreName := filepath.Join(legacyMigrationControlDir, string(id)+".restore")
	journal := legacyMigrationJournal{Version: 4, State: legacyMigrationPrepared}
	journalWritten := false
	preparedBlobs := make([]legacyBlobPlan, 0)
	sessionProofPrepared := false
	sessionPublished := false
	aggregateCommitted := false
	if err := m.roots.sessions.MkdirAll(legacyMigrationControlDir, 0o700); err != nil {
		_ = input.Close()
		return err
	}
	defer func() {
		// A panic models process death: named err remains nil and durable
		// recovery controls must remain for the next invocation.
		if err == nil || aggregateCommitted {
			return
		}
		if sessionPublished {
			if quarantineErr := m.quarantinePublishedLegacySession(
				name, id, journal.Session,
			); quarantineErr != nil {
				err = errors.Join(err, fmt.Errorf(
					"retaining migration controls because the live session is not migration-owned: %w",
					quarantineErr,
				))
				return
			}
			sessionPublished = false
		}
		if restoreErr := restoreLegacyOriginalFromControls(
			m.roots.sessions, name, quarantineName, restoreName,
		); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf(
				"retaining migration controls because the legacy session could not be restored: %w",
				restoreErr,
			))
			return
		}
		if journalWritten && journal.State == legacyMigrationPrepared {
			if verifyErr := m.verifyLegacyBlobRollbackPlans(owner, journal.Blobs); verifyErr != nil {
				err = errors.Join(err, fmt.Errorf(
					"retaining migration controls because blob rollback ownership is incomplete: %w",
					verifyErr,
				))
				return
			}
			if verifyErr := verifyLegacySessionProof(
				m.roots.sessions, id, journal.Session,
			); verifyErr != nil {
				err = errors.Join(err, fmt.Errorf(
					"retaining migration controls because session rollback ownership is incomplete: %w",
					verifyErr,
				))
				return
			}
			journal.State = legacyMigrationRollback
			if stateErr := writeJSONAtomicRoot(
				m.roots.sessions, journalName, journal,
			); stateErr != nil {
				err = errors.Join(err, fmt.Errorf(
					"recording legacy migration rollback state: %w", stateErr,
				))
				return
			}
		}
		var cleanupErrs []error
		for _, blob := range preparedBlobs {
			cleanupErrs = append(cleanupErrs, m.rollbackLegacyBlobPlan(
				owner, blob, journalWritten && journal.State == legacyMigrationRollback,
			))
		}
		if cleanupErr := errors.Join(cleanupErrs...); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("cleaning failed legacy staged blobs: %w", cleanupErr))
			return
		}
		if sessionProofPrepared {
			if cleanupErr := m.rollbackLegacySessionProof(
				name, id, journal.Session,
				journalWritten && journal.State == legacyMigrationRollback,
			); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf(
					"cleaning failed prepared-session proof: %w", cleanupErr,
				))
				return
			}
		}
		if journalWritten {
			if removeErr := m.roots.sessions.Remove(journalName); removeErr != nil &&
				!errors.Is(removeErr, fs.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("retaining legacy migration journal: %w", removeErr))
			} else if syncErr := syncRootDirectory(
				m.roots.sessions, legacyMigrationControlDir,
			); syncErr != nil {
				err = errors.Join(err, syncErr)
			}
		}
	}()

	storedEvents := make([]app.StoredEvent, 0, len(legacy.events))
	for _, payload := range legacy.payloads {
		storedEvents = append(storedEvents, app.StoredEvent{
			CodecVersion: app.SessionCodecVersion,
			Code:         app.WorkflowEventCode,
			Payload:      payload,
		})
	}

	if err := m.prepareMutation(); err != nil {
		_ = input.Close()
		return err
	}
	var stagingRoot *os.Root
	stagingRoot, err = m.roots.sessions.OpenRoot(stagingRelative)
	if errors.Is(err, fs.ErrNotExist) {
		stagingRoot = nil
	} else if err != nil {
		_ = input.Close()
		return err
	}
	if stagingRoot != nil {
		defer func() { err = errors.Join(err, stagingRoot.Close()) }()
	}
	var staged []fs.DirEntry
	if stagingRoot != nil {
		staged, err = fs.ReadDir(stagingRoot.FS(), ".")
		if err != nil {
			_ = input.Close()
			return err
		}
	}
	stagedInputs := make([]legacyStagedInput, 0, len(staged))
	for _, entry := range staged {
		if entry.IsDir() {
			_ = input.Close()
			return fmt.Errorf("legacy staging entry %s is a directory", filepath.Join(stagingDir, entry.Name()))
		}
		file, openErr := openRootedRegular(stagingRoot, entry.Name())
		if openErr != nil {
			_ = input.Close()
			return openErr
		}
		data, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			_ = input.Close()
			return err
		}
		plan := legacyBlobPlan{
			ID:        deterministicLegacyBlobID(owner, entry.Name(), data),
			Filename:  entry.Name(),
			Digest:    sha256Digest(data).Value,
			Size:      int64(len(data)),
			CreatedAt: legacyBlobPlanCreatedAt(metadata),
		}
		journal.Blobs = append(journal.Blobs, plan)
		stagedInputs = append(stagedInputs, legacyStagedInput{plan: plan, data: data})
	}
	if err := m.ensureLegacyBlobTargetsAbsent(owner, journal.Blobs); err != nil {
		_ = input.Close()
		return err
	}
	for _, stagedInput := range stagedInputs {
		if err := m.prepareRootedBlob(owner, stagedInput.plan, stagedInput.data); err != nil {
			_ = input.Close()
			return err
		}
		preparedBlobs = append(preparedBlobs, stagedInput.plan)
		payload, marshalErr := json.Marshal(map[string]string{
			"handle":  stagedInput.plan.Filename,
			"blob_id": stagedInput.plan.ID,
		})
		if marshalErr != nil {
			_ = input.Close()
			return marshalErr
		}
		storedEvents = append(storedEvents, app.StoredEvent{
			CodecVersion: app.SessionCodecVersion,
			Code:         "workflow_staged_blob",
			Payload:      payload,
		})
	}
	journal.Session, err = m.prepareLegacySessionProof(id, metadata, storedEvents)
	if err != nil {
		_ = input.Close()
		return err
	}
	sessionProofPrepared = true
	if err := m.revalidate(); err != nil {
		_ = input.Close()
		return err
	}
	if err := writeJSONAtomicRoot(m.roots.sessions, journalName, journal); err != nil {
		_ = input.Close()
		return err
	}
	journalWritten = true
	if m.afterBlobPlanPersisted != nil {
		m.afterBlobPlanPersisted()
	}
	for _, stagedInput := range stagedInputs {
		if stageErr := m.publishRootedBlob(owner, stagedInput.plan); stageErr != nil {
			_ = input.Close()
			return stageErr
		}
	}
	sourceInfo, err := input.Stat()
	if err != nil {
		_ = input.Close()
		return err
	}
	pathInfo, err := m.roots.sessions.Lstat(name)
	if err != nil || !os.SameFile(sourceInfo, pathInfo) {
		_ = input.Close()
		if err == nil {
			err = fmt.Errorf("legacy session was replaced during migration")
		}
		return err
	}
	if m.afterSourceVerified != nil {
		m.afterSourceVerified()
	}
	if err := m.roots.sessions.Rename(name, quarantineName); err != nil {
		_ = input.Close()
		return err
	}
	quarantinedInfo, err := m.roots.sessions.Lstat(quarantineName)
	if err != nil || !os.SameFile(sourceInfo, quarantinedInfo) {
		restoreErr := restoreRelocationSourceName(m.roots.sessions, name, quarantineName)
		_ = input.Close()
		if err == nil {
			err = fmt.Errorf("legacy source name changed before quarantine")
		}
		return errors.Join(err, restoreErr)
	}
	if err := input.Close(); err != nil {
		return err
	}
	if err := m.roots.sessions.Link(quarantineName, restoreName); err != nil {
		return errors.Join(err, restoreRelocationSourceName(m.roots.sessions, name, quarantineName))
	}
	if m.afterSourceQuarantine != nil {
		m.afterSourceQuarantine()
	}
	if err := m.revalidate(); err != nil {
		return err
	}
	proofInfo, err := m.roots.sessions.Lstat(journal.Session.Proof)
	if err != nil {
		return err
	}
	if err := m.roots.sessions.Link(journal.Session.Proof, name); err != nil {
		return fmt.Errorf("publishing migrated legacy session without replacement: %w", err)
	}
	sessionPublished = true
	if err := syncRootDirectory(m.roots.sessions, "."); err != nil {
		return err
	}
	publishedInfo, err := m.roots.sessions.Lstat(name)
	if err != nil || !os.SameFile(proofInfo, publishedInfo) {
		if err == nil {
			err = fmt.Errorf("published legacy name does not identify prepared migration")
		}
		return err
	}
	if m.afterSessionPublished != nil {
		m.afterSessionPublished()
	}
	if err := verifyPublishedLegacySession(m.roots.sessions, name, id, journal.Session); err != nil {
		return err
	}
	if err := m.revalidate(); err != nil {
		return err
	}
	if err := m.verifyLegacyBlobPlans(owner, journal.Blobs); err != nil {
		return fmt.Errorf("verifying published legacy staged blobs: %w", err)
	}
	removeQuarantine := m.roots.sessions.Remove
	if m.removeQuarantine != nil {
		removeQuarantine = func(name string) error {
			return m.removeQuarantine(m.roots.sessions, name)
		}
	}
	if err := removeQuarantine(quarantineName); err != nil {
		return err
	}
	if m.afterQuarantineRemove != nil {
		m.afterQuarantineRemove()
	}
	if err := m.revalidate(); err != nil {
		return err
	}
	if err := m.verifyLegacyBlobPlans(owner, journal.Blobs); err != nil {
		return fmt.Errorf("verifying published legacy staged blobs: %w", err)
	}
	if err := verifyPublishedLegacySession(m.roots.sessions, name, id, journal.Session); err != nil {
		return fmt.Errorf("verifying published legacy session: %w", err)
	}
	if err := m.roots.sessions.Remove(restoreName); err != nil {
		return err
	}
	// With the restore control removed, recovery must retain the live aggregate.
	aggregateCommitted = true
	journal.State = legacyMigrationCommitted
	if err := writeJSONAtomicRoot(m.roots.sessions, journalName, journal); err != nil {
		return fmt.Errorf("recording committed legacy migration aggregate: %w", err)
	}
	if m.afterCommittedJournal != nil {
		m.afterCommittedJournal()
	}
	if err := m.commitLegacyBlobPlans(owner, journal.Blobs); err != nil {
		return fmt.Errorf("finalizing legacy staged-blob ownership proofs: %w", err)
	}
	if err := m.finalizeCommittedLegacyMigration(name, id, journal); err != nil {
		return fmt.Errorf("finalizing committed legacy session proof: %w", err)
	}
	return m.completeLegacyMigration(journalName, stagingRelative)
}

func (m *FilesystemLegacySessionMigrator) recoverLegacyMigration(
	id app.SessionID,
	name string,
) (bool, error) {
	journalName := filepath.Join(legacyMigrationControlDir, string(id)+".json")
	quarantineName := filepath.Join(legacyMigrationControlDir, string(id)+".original")
	restoreName := filepath.Join(legacyMigrationControlDir, string(id)+".restore")
	proofName := legacySessionProofName(id)
	proofCleanupName := legacySessionProofCleanupName(id)
	publishedRollbackName := legacyPublishedRollbackName(id)
	journal := legacyMigrationJournal{}
	journalPresent := false
	if file, err := openRootedRegular(m.roots.sessions, journalName); err == nil {
		raw, readErr := io.ReadAll(file)
		if err := errors.Join(readErr, file.Close()); err != nil {
			return false, err
		}
		if err := decodeStrictJSON(raw, &journal); err != nil {
			return false, fmt.Errorf("decoding legacy migration journal: %w", err)
		}
		if journal.Version != 4 {
			return false, fmt.Errorf("unsupported legacy migration journal version %d", journal.Version)
		}
		if journal.State != legacyMigrationPrepared &&
			journal.State != legacyMigrationRollback &&
			journal.State != legacyMigrationCommitted {
			return false, fmt.Errorf("unsupported legacy migration journal state %q", journal.State)
		}
		if err := validateLegacySessionPlan(id, journal.Session); err != nil {
			return false, err
		}
		for _, blob := range journal.Blobs {
			if err := validBlobID(blob.ID); err != nil {
				return false, fmt.Errorf("legacy migration journal has invalid blob ID: %w", err)
			}
			if blob.Filename == "" || filepath.Base(blob.Filename) != blob.Filename ||
				strings.ContainsAny(blob.Filename, `/\`) {
				return false, fmt.Errorf("legacy migration journal has invalid blob filename")
			}
			digest, decodeErr := hex.DecodeString(blob.Digest)
			if decodeErr != nil || len(digest) != sha256.Size || blob.Size < 0 ||
				blob.CreatedAt.IsZero() {
				return false, fmt.Errorf("legacy migration journal has invalid blob content identity")
			}
		}
		journalPresent = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	quarantinePresent, err := rootedRegularPresence(m.roots.sessions, quarantineName)
	if err != nil {
		return false, err
	}
	restorePresent, err := rootedRegularPresence(m.roots.sessions, restoreName)
	if err != nil {
		return false, err
	}
	proofPresent, err := rootedRegularPresence(m.roots.sessions, proofName)
	if err != nil {
		return false, err
	}
	proofCleanupPresent, err := rootedRegularPresence(m.roots.sessions, proofCleanupName)
	if err != nil {
		return false, err
	}
	publishedRollbackPresent, err := rootedRegularPresence(
		m.roots.sessions, publishedRollbackName,
	)
	if err != nil {
		return false, err
	}
	owner := app.BlobOwner{Subject: m.subject, Session: id}
	blobProofsPresent, err := m.legacyBlobProofsPresent(owner)
	if err != nil {
		return false, err
	}
	if !journalPresent {
		if quarantinePresent || restorePresent || proofCleanupPresent || publishedRollbackPresent {
			return false, fmt.Errorf("legacy migration controls exist without a journal")
		}
		if proofPresent || blobProofsPresent {
			format, present, err := rootedSessionFormat(m.roots.sessions, name)
			if err != nil {
				return false, err
			}
			if !present || format != sessionFormatLegacy {
				return false, fmt.Errorf(
					"pre-journal migration proofs lack their live legacy source",
				)
			}
		}
		return false, nil
	}
	livePresent, err := rootedRegularPresence(m.roots.sessions, name)
	if err != nil {
		return false, err
	}
	currentOwned := false
	if journal.State == legacyMigrationCommitted {
		if _, err := m.ensureCommittedLegacySession(name, id, journal.Session); err != nil {
			return false, fmt.Errorf("recovering committed legacy session: %w", err)
		}
		currentOwned = true
	} else {
		if proofCleanupPresent {
			if journal.State != legacyMigrationRollback {
				return false, fmt.Errorf("prepared-session cleanup control exists before rollback state")
			}
		} else if !proofPresent && journal.State != legacyMigrationRollback {
			return false, fmt.Errorf("legacy migration journal lacks its prepared-session proof")
		}
		if proofPresent {
			if err := verifyLegacySessionProof(m.roots.sessions, id, journal.Session); err != nil {
				return false, fmt.Errorf("verifying prepared-session proof: %w", err)
			}
		}
	}
	if journal.State != legacyMigrationCommitted && livePresent && proofPresent {
		if err := verifyPublishedLegacySession(
			m.roots.sessions, name, id, journal.Session,
		); err == nil {
			currentOwned = true
		} else if quarantinePresent || restorePresent {
			same, sameErr := rootedFileMatchesAny(
				m.roots.sessions, name, quarantineName, restoreName,
			)
			if sameErr != nil {
				return false, sameErr
			}
			if !same {
				return false, fmt.Errorf(
					"live session collides with journaled migration controls: %w", err,
				)
			}
		} else {
			format, _, classifyErr := rootedSessionFormat(m.roots.sessions, name)
			if classifyErr != nil {
				return false, classifyErr
			}
			if format == sessionFormatCurrent {
				return false, fmt.Errorf(
					"live current session lacks journaled ownership proof: %w", err,
				)
			}
		}
	}
	if currentOwned {
		if quarantinePresent || restorePresent {
			if err := m.verifyLegacyBlobPlans(owner, journal.Blobs); err != nil {
				return false, fmt.Errorf("verifying recovered legacy staged blobs: %w", err)
			}
		}
		if quarantinePresent {
			if err := m.roots.sessions.Remove(quarantineName); err != nil {
				return false, err
			}
		}
		if restorePresent {
			if err := m.roots.sessions.Remove(restoreName); err != nil {
				return false, err
			}
		}
		if journal.State != legacyMigrationCommitted {
			journal.State = legacyMigrationCommitted
			if err := writeJSONAtomicRoot(m.roots.sessions, journalName, journal); err != nil {
				return false, fmt.Errorf("recording recovered committed legacy aggregate: %w", err)
			}
		}
		if err := m.commitLegacyBlobPlans(owner, journal.Blobs); err != nil {
			return false, err
		}
		if err := m.finalizeCommittedLegacyMigration(name, id, journal); err != nil {
			return false, err
		}
		return true, nil
	}
	if journal.State == legacyMigrationCommitted {
		return false, fmt.Errorf("committed legacy migration lacks its owned live session")
	}
	if publishedRollbackPresent {
		if err := m.quarantinePublishedLegacySession(name, id, journal.Session); err != nil {
			return false, err
		}
	}
	if journal.State == legacyMigrationPrepared {
		if err := m.verifyLegacyBlobRollbackPlans(owner, journal.Blobs); err != nil {
			return false, err
		}
		if err := verifyLegacySessionProof(m.roots.sessions, id, journal.Session); err != nil {
			return false, err
		}
		journal.State = legacyMigrationRollback
		if err := writeJSONAtomicRoot(m.roots.sessions, journalName, journal); err != nil {
			return false, fmt.Errorf("recording recovered rollback state: %w", err)
		}
	}
	if livePresent && !quarantinePresent && !restorePresent {
		format, _, err := rootedSessionFormat(m.roots.sessions, name)
		if err != nil {
			return false, err
		}
		if format == sessionFormatCurrent {
			return false, fmt.Errorf("rolling-back migration collides with a current live session")
		}
	}
	if err := restoreLegacyOriginalFromControls(
		m.roots.sessions, name, quarantineName, restoreName,
	); err != nil {
		return false, err
	}
	for _, blob := range journal.Blobs {
		if err := m.rollbackLegacyBlobPlan(owner, blob, true); err != nil {
			return false, err
		}
	}
	if err := m.rollbackLegacySessionProof(name, id, journal.Session, true); err != nil {
		return false, err
	}
	if err := m.roots.sessions.Remove(journalName); err != nil {
		return false, err
	}
	if err := syncRootDirectory(m.roots.sessions, legacyMigrationControlDir); err != nil {
		return false, err
	}
	return false, nil
}

func rootedSessionFormat(
	root *os.Root,
	name string,
) (sessionFormat, bool, error) {
	file, err := openRootedRegular(root, name)
	if errors.Is(err, fs.ErrNotExist) {
		return sessionFormatLegacy, false, nil
	}
	if err != nil {
		return sessionFormatLegacy, false, err
	}
	format, classifyErr := classifySessionHandle(file)
	closeErr := file.Close()
	return format, true, errors.Join(classifyErr, closeErr)
}

func (m *FilesystemLegacySessionMigrator) prepareRootedBlob(
	owner app.BlobOwner,
	plan legacyBlobPlan,
	data []byte,
) (err error) {
	ownerDir, err := m.blobs.ownerDir(owner)
	if err != nil {
		return err
	}
	if plan.Filename == "" || filepath.Base(plan.Filename) != plan.Filename ||
		strings.ContainsAny(plan.Filename, `/\`) {
		return fmt.Errorf("sdd: staged filename must be a plain name")
	}
	if err := validBlobID(plan.ID); err != nil {
		return err
	}
	ownerRelative, err := filepath.Rel(m.blobs.dir, ownerDir)
	if err != nil || !filepath.IsLocal(ownerRelative) {
		return fmt.Errorf("sdd: staged blob owner escapes pinned store")
	}
	if err := m.roots.blobs.MkdirAll(ownerRelative, 0o755); err != nil {
		return err
	}
	metadata, err := legacyBlobMetadataBytes(owner, plan)
	if err != nil {
		return err
	}
	blobTemp := filepath.Join(ownerRelative, plan.ID+".blob.legacy-tmp")
	metadataTemp := filepath.Join(ownerRelative, plan.ID+".json.legacy-tmp")
	if err := reconcilePreparedRootedFile(
		m.roots.blobs, blobTemp, data, m.beforeBlobProofWrite, m.duringBlobStage,
	); err != nil {
		return err
	}
	if err := reconcilePreparedRootedFile(
		m.roots.blobs, metadataTemp, metadata, nil, nil,
	); err != nil {
		return err
	}
	if err := m.verifyLegacyBlobProofContent(ownerRelative, owner, plan); err != nil {
		return err
	}
	if m.afterBlobProofPrepared != nil {
		m.afterBlobProofPrepared()
	}
	return nil
}

func (m *FilesystemLegacySessionMigrator) publishRootedBlob(
	owner app.BlobOwner,
	plan legacyBlobPlan,
) error {
	ownerRelative, err := m.legacyBlobOwnerRelative(owner)
	if err != nil {
		return err
	}
	if err := m.verifyLegacyBlobProofContent(ownerRelative, owner, plan); err != nil {
		return err
	}
	blobRelative := filepath.Join(ownerRelative, plan.ID+".blob")
	metadataRelative := filepath.Join(ownerRelative, plan.ID+".json")
	blobTemp := filepath.Join(ownerRelative, plan.ID+".blob.legacy-tmp")
	metadataTemp := filepath.Join(ownerRelative, plan.ID+".json.legacy-tmp")
	if err := m.roots.blobs.Link(blobTemp, blobRelative); err != nil {
		return fmt.Errorf("publishing legacy staged-blob data without replacement: %w", err)
	}
	if m.afterBlobDataPublished != nil {
		m.afterBlobDataPublished()
	}
	if err := m.roots.blobs.Link(metadataTemp, metadataRelative); err != nil {
		return fmt.Errorf("publishing legacy staged-blob metadata without replacement: %w", err)
	}
	if m.afterBlobMetadataPublished != nil {
		m.afterBlobMetadataPublished()
	}
	if err := m.verifyLegacyBlobPlan(ownerRelative, owner, plan); err != nil {
		return err
	}
	if err := syncRootDirectory(m.roots.blobs, ownerRelative); err != nil {
		return err
	}
	return nil
}

func deterministicLegacyBlobID(owner app.BlobOwner, filename string, data []byte) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, "sdd-legacy-migration-blob-v1\x00")
	_, _ = io.WriteString(hash, owner.Subject)
	_, _ = io.WriteString(hash, "\x00")
	_, _ = io.WriteString(hash, string(owner.Session))
	_, _ = io.WriteString(hash, "\x00")
	_, _ = io.WriteString(hash, filename)
	_, _ = io.WriteString(hash, "\x00")
	_, _ = hash.Write(data)
	return fmt.Sprintf("%x", hash.Sum(nil)[:16])
}

func (m *FilesystemLegacySessionMigrator) ensureLegacyBlobTargetsAbsent(
	owner app.BlobOwner,
	plans []legacyBlobPlan,
) error {
	ownerDir, err := m.blobs.ownerDir(owner)
	if err != nil {
		return err
	}
	ownerRelative, err := filepath.Rel(m.blobs.dir, ownerDir)
	if err != nil || !filepath.IsLocal(ownerRelative) {
		return fmt.Errorf("sdd: staged blob owner escapes pinned store")
	}
	for _, plan := range plans {
		if err := validBlobID(plan.ID); err != nil {
			return err
		}
		for _, suffix := range []string{
			".blob", ".json", ".blob.legacy-rollback", ".json.legacy-rollback",
		} {
			path := filepath.Join(ownerRelative, plan.ID+suffix)
			if _, err := m.roots.blobs.Lstat(path); err == nil {
				return fmt.Errorf("planned legacy staged blob target already exists: %s", path)
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func (m *FilesystemLegacySessionMigrator) legacyBlobOwnerRelative(
	owner app.BlobOwner,
) (string, error) {
	ownerDir, err := m.blobs.ownerDir(owner)
	if err != nil {
		return "", err
	}
	ownerRelative, err := filepath.Rel(m.blobs.dir, ownerDir)
	if err != nil || !filepath.IsLocal(ownerRelative) {
		return "", fmt.Errorf("sdd: staged blob owner escapes pinned store")
	}
	return ownerRelative, nil
}

func (m *FilesystemLegacySessionMigrator) legacyBlobProofsPresent(
	owner app.BlobOwner,
) (bool, error) {
	ownerRelative, err := m.legacyBlobOwnerRelative(owner)
	if err != nil {
		return false, err
	}
	entries, err := fs.ReadDir(m.roots.blobs.FS(), ownerRelative)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".legacy-tmp") ||
			strings.HasSuffix(entry.Name(), ".legacy-rollback") {
			return true, nil
		}
	}
	return false, nil
}

type legacyBlobArtifact struct {
	final      string
	proof      string
	quarantine string
	metadata   bool
}

func legacyBlobArtifactNames(ownerRelative string, plan legacyBlobPlan) []legacyBlobArtifact {
	return []legacyBlobArtifact{
		{
			final:      filepath.Join(ownerRelative, plan.ID+".blob"),
			proof:      filepath.Join(ownerRelative, plan.ID+".blob.legacy-tmp"),
			quarantine: filepath.Join(ownerRelative, plan.ID+".blob.legacy-rollback"),
		},
		{
			final:      filepath.Join(ownerRelative, plan.ID+".json"),
			proof:      filepath.Join(ownerRelative, plan.ID+".json.legacy-tmp"),
			quarantine: filepath.Join(ownerRelative, plan.ID+".json.legacy-rollback"),
			metadata:   true,
		},
	}
}

func legacyBlobMetadataBytes(owner app.BlobOwner, plan legacyBlobPlan) ([]byte, error) {
	return json.Marshal(app.StagedBlob{
		ID:        plan.ID,
		Owner:     owner,
		Digest:    app.BlobDigest{Algorithm: "sha256", Value: plan.Digest},
		Size:      plan.Size,
		Filename:  plan.Filename,
		CreatedAt: plan.CreatedAt,
	})
}

func (m *FilesystemLegacySessionMigrator) verifyLegacyBlobProofContent(
	ownerRelative string,
	owner app.BlobOwner,
	plan legacyBlobPlan,
) error {
	digest, size, err := rootedFileDigest(
		m.roots.blobs, filepath.Join(ownerRelative, plan.ID+".blob.legacy-tmp"),
	)
	if err != nil {
		return err
	}
	if digest != plan.Digest || size != plan.Size {
		return fmt.Errorf("legacy staged-blob data proof differs from journal: %s", plan.ID)
	}
	metadataPath := filepath.Join(ownerRelative, plan.ID+".json.legacy-tmp")
	file, err := openRootedRegular(m.roots.blobs, metadataPath)
	if err != nil {
		return err
	}
	raw, readErr := io.ReadAll(file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return err
	}
	expected, err := legacyBlobMetadataBytes(owner, plan)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, expected) {
		return fmt.Errorf("legacy staged-blob metadata proof differs from journal: %s", plan.ID)
	}
	return nil
}

func (m *FilesystemLegacySessionMigrator) verifyLegacyBlobArtifactContent(
	owner app.BlobOwner,
	plan legacyBlobPlan,
	artifact legacyBlobArtifact,
	path string,
) error {
	if !artifact.metadata {
		digest, size, err := rootedFileDigest(m.roots.blobs, path)
		if err != nil {
			return err
		}
		if digest != plan.Digest || size != plan.Size {
			return fmt.Errorf("legacy staged-blob data proof differs from journal: %s", plan.ID)
		}
		return nil
	}
	file, err := openRootedRegular(m.roots.blobs, path)
	if err != nil {
		return err
	}
	raw, readErr := io.ReadAll(file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return err
	}
	expected, err := legacyBlobMetadataBytes(owner, plan)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, expected) {
		return fmt.Errorf("legacy staged-blob metadata proof differs from journal: %s", plan.ID)
	}
	return nil
}

func (m *FilesystemLegacySessionMigrator) verifyLegacyBlobPlan(
	ownerRelative string,
	owner app.BlobOwner,
	plan legacyBlobPlan,
) error {
	for _, artifact := range legacyBlobArtifactNames(ownerRelative, plan) {
		finalInfo, err := m.roots.blobs.Lstat(artifact.final)
		if err != nil {
			return err
		}
		tempInfo, err := m.roots.blobs.Lstat(artifact.proof)
		if err != nil {
			return err
		}
		if !finalInfo.Mode().IsRegular() || !tempInfo.Mode().IsRegular() ||
			!os.SameFile(finalInfo, tempInfo) {
			return fmt.Errorf("legacy staged-blob final lacks ownership proof: %s", artifact.final)
		}
	}
	return m.verifyLegacyBlobProofContent(ownerRelative, owner, plan)
}

func (m *FilesystemLegacySessionMigrator) verifyLegacyBlobPlans(
	owner app.BlobOwner,
	plans []legacyBlobPlan,
) error {
	if len(plans) == 0 {
		return nil
	}
	ownerRelative, err := m.legacyBlobOwnerRelative(owner)
	if err != nil {
		return err
	}
	for _, plan := range plans {
		if err := m.verifyLegacyBlobPlan(ownerRelative, owner, plan); err != nil {
			return err
		}
	}
	return nil
}

func (m *FilesystemLegacySessionMigrator) verifyLegacyBlobRollbackPlans(
	owner app.BlobOwner,
	plans []legacyBlobPlan,
) error {
	if len(plans) == 0 {
		return nil
	}
	ownerRelative, err := m.legacyBlobOwnerRelative(owner)
	if err != nil {
		return err
	}
	for _, plan := range plans {
		for _, artifact := range legacyBlobArtifactNames(ownerRelative, plan) {
			if present, err := rootedRegularPresence(
				m.roots.blobs, artifact.quarantine,
			); err != nil {
				return err
			} else if present {
				return fmt.Errorf("legacy staged-blob rollback control exists before rollback state")
			}
			proofInfo, err := m.roots.blobs.Lstat(artifact.proof)
			if err != nil || !proofInfo.Mode().IsRegular() {
				return fmt.Errorf("legacy staged-blob rollback proof is missing: %s", artifact.proof)
			}
			if err := m.verifyLegacyBlobArtifactContent(
				owner, plan, artifact, artifact.proof,
			); err != nil {
				return err
			}
			finalInfo, err := m.roots.blobs.Lstat(artifact.final)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil || !finalInfo.Mode().IsRegular() ||
				!os.SameFile(proofInfo, finalInfo) {
				return fmt.Errorf("legacy staged-blob final lacks rollback ownership: %s", artifact.final)
			}
		}
	}
	return nil
}

func (m *FilesystemLegacySessionMigrator) verifyCommittedLegacyBlob(
	ownerRelative string,
	owner app.BlobOwner,
	plan legacyBlobPlan,
) error {
	metadataPath := filepath.Join(ownerRelative, plan.ID+".json")
	file, err := openRootedRegular(m.roots.blobs, metadataPath)
	if err != nil {
		return err
	}
	raw, readErr := io.ReadAll(file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return err
	}
	expected, err := legacyBlobMetadataBytes(owner, plan)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, expected) {
		return fmt.Errorf("legacy staged-blob metadata differs from journal: %s", plan.ID)
	}
	digest, size, err := rootedFileDigest(
		m.roots.blobs, filepath.Join(ownerRelative, plan.ID+".blob"),
	)
	if err != nil {
		return err
	}
	if digest != plan.Digest || size != plan.Size {
		return fmt.Errorf("legacy staged-blob data differs from journal: %s", plan.ID)
	}
	return nil
}

func (m *FilesystemLegacySessionMigrator) commitLegacyBlobPlans(
	owner app.BlobOwner,
	plans []legacyBlobPlan,
) error {
	if len(plans) == 0 {
		return nil
	}
	ownerRelative, err := m.legacyBlobOwnerRelative(owner)
	if err != nil {
		return err
	}
	for _, plan := range plans {
		if err := m.verifyCommittedLegacyBlob(ownerRelative, owner, plan); err != nil {
			return err
		}
		for _, artifact := range legacyBlobArtifactNames(ownerRelative, plan) {
			if err := m.commitLegacyBlobProof(owner, plan, artifact); err != nil {
				return err
			}
			if m.afterBlobProofRemoved != nil {
				m.afterBlobProofRemoved()
			}
		}
	}
	return m.syncLegacyBlobOwnerIfPresent(ownerRelative)
}

func (m *FilesystemLegacySessionMigrator) restoreQuarantinedVisibleName(
	root *os.Root,
	quarantine string,
	visible string,
) error {
	quarantineInfo, err := root.Lstat(quarantine)
	if err != nil {
		return err
	}
	if !quarantineInfo.Mode().IsRegular() {
		return fmt.Errorf("quarantined name is not regular: %s", quarantine)
	}
	visibleInfo, err := root.Lstat(visible)
	if errors.Is(err, fs.ErrNotExist) {
		if err := root.Link(quarantine, visible); err != nil {
			return fmt.Errorf("restoring quarantined visible name without replacement: %w", err)
		}
		if m.afterQuarantineRestoreLink != nil {
			m.afterQuarantineRestoreLink()
		}
		visibleInfo, err = root.Lstat(visible)
	}
	if err != nil {
		return err
	}
	if !visibleInfo.Mode().IsRegular() || !os.SameFile(quarantineInfo, visibleInfo) {
		return fmt.Errorf("restored visible name does not match quarantined identity")
	}
	if err := syncRootDirectory(root, filepath.Dir(visible)); err != nil {
		return err
	}
	if err := root.Remove(quarantine); err != nil {
		return err
	}
	return syncRootDirectory(root, filepath.Dir(quarantine))
}

func (m *FilesystemLegacySessionMigrator) commitLegacyBlobProof(
	owner app.BlobOwner,
	plan legacyBlobPlan,
	artifact legacyBlobArtifact,
) error {
	proofPresent, err := rootedRegularPresence(m.roots.blobs, artifact.proof)
	if err != nil {
		return err
	}
	quarantinePresent, err := rootedRegularPresence(m.roots.blobs, artifact.quarantine)
	if err != nil {
		return err
	}
	if proofPresent && quarantinePresent {
		proofInfo, proofErr := m.roots.blobs.Lstat(artifact.proof)
		quarantineInfo, quarantineErr := m.roots.blobs.Lstat(artifact.quarantine)
		if err := errors.Join(proofErr, quarantineErr); err != nil {
			return err
		}
		if !os.SameFile(proofInfo, quarantineInfo) {
			return fmt.Errorf("legacy staged-blob proof and cleanup quarantine both exist: %s", artifact.proof)
		}
		if err := m.roots.blobs.Remove(artifact.quarantine); err != nil {
			return err
		}
		quarantinePresent = false
	}
	finalInfo, err := m.roots.blobs.Lstat(artifact.final)
	if err != nil || !finalInfo.Mode().IsRegular() {
		return fmt.Errorf("legacy staged-blob final changed before proof cleanup: %s", artifact.final)
	}
	if quarantinePresent {
		quarantineInfo, err := m.roots.blobs.Lstat(artifact.quarantine)
		if err != nil || !os.SameFile(finalInfo, quarantineInfo) {
			return fmt.Errorf("legacy staged-blob proof cleanup quarantine lacks final ownership: %s", artifact.final)
		}
		if err := m.verifyLegacyBlobArtifactContent(owner, plan, artifact, artifact.quarantine); err != nil {
			return err
		}
		return m.roots.blobs.Remove(artifact.quarantine)
	}
	if !proofPresent {
		return nil
	}
	proofInfo, err := m.roots.blobs.Lstat(artifact.proof)
	if err != nil || !os.SameFile(finalInfo, proofInfo) {
		return fmt.Errorf("legacy staged-blob ownership proof changed before commit: %s", artifact.final)
	}
	if err := m.verifyLegacyBlobArtifactContent(owner, plan, artifact, artifact.proof); err != nil {
		return err
	}
	if err := m.roots.blobs.Rename(artifact.proof, artifact.quarantine); err != nil {
		return err
	}
	movedInfo, err := m.roots.blobs.Lstat(artifact.quarantine)
	if err != nil || !os.SameFile(finalInfo, movedInfo) {
		restoreErr := m.restoreQuarantinedVisibleName(
			m.roots.blobs, artifact.quarantine, artifact.proof,
		)
		return errors.Join(
			fmt.Errorf("legacy staged-blob proof changed during cleanup: %s", artifact.proof),
			restoreErr,
		)
	}
	if err := m.verifyLegacyBlobArtifactContent(owner, plan, artifact, artifact.quarantine); err != nil {
		restoreErr := m.restoreQuarantinedVisibleName(
			m.roots.blobs, artifact.quarantine, artifact.proof,
		)
		return errors.Join(err, restoreErr)
	}
	return m.roots.blobs.Remove(artifact.quarantine)
}

func (m *FilesystemLegacySessionMigrator) rollbackLegacyBlobPlan(
	owner app.BlobOwner,
	plan legacyBlobPlan,
	allowMissing bool,
) error {
	ownerRelative, err := m.legacyBlobOwnerRelative(owner)
	if err != nil {
		return err
	}
	for _, artifact := range legacyBlobArtifactNames(ownerRelative, plan) {
		if err := m.rollbackLegacyBlobArtifact(owner, plan, artifact, allowMissing); err != nil {
			return err
		}
		if m.afterBlobRollbackPair != nil {
			m.afterBlobRollbackPair()
		}
	}
	return m.syncLegacyBlobOwnerIfPresent(ownerRelative)
}

func (m *FilesystemLegacySessionMigrator) rollbackLegacyBlobArtifact(
	owner app.BlobOwner,
	plan legacyBlobPlan,
	artifact legacyBlobArtifact,
	allowMissing bool,
) error {
	proofPresent, err := rootedRegularPresence(m.roots.blobs, artifact.proof)
	if err != nil {
		return err
	}
	quarantinePresent, err := rootedRegularPresence(m.roots.blobs, artifact.quarantine)
	if err != nil {
		return err
	}
	finalPresent, err := rootedRegularPresence(m.roots.blobs, artifact.final)
	if err != nil {
		return err
	}
	if quarantinePresent {
		if finalPresent {
			finalInfo, finalErr := m.roots.blobs.Lstat(artifact.final)
			quarantineInfo, quarantineErr := m.roots.blobs.Lstat(artifact.quarantine)
			if err := errors.Join(finalErr, quarantineErr); err != nil {
				return err
			}
			if !os.SameFile(finalInfo, quarantineInfo) {
				return fmt.Errorf("legacy staged-blob rollback quarantine collides with visible final: %s", artifact.final)
			}
			if err := m.roots.blobs.Remove(artifact.quarantine); err != nil {
				return err
			}
			quarantinePresent = false
		}
	}
	if quarantinePresent {
		if err := m.verifyLegacyBlobArtifactContent(
			owner, plan, artifact, artifact.quarantine,
		); err != nil {
			if proofPresent {
				if restoreErr := m.restoreQuarantinedVisibleName(
					m.roots.blobs, artifact.quarantine, artifact.final,
				); restoreErr != nil {
					return errors.Join(err, restoreErr)
				}
				return fmt.Errorf("restored nonmatching staged-blob rollback quarantine: %w", err)
			}
			return fmt.Errorf("refusing to clean nonmatching staged-blob rollback control: %w", err)
		}
		if proofPresent {
			proofInfo, err := m.roots.blobs.Lstat(artifact.proof)
			if err != nil {
				return err
			}
			quarantineInfo, err := m.roots.blobs.Lstat(artifact.quarantine)
			if err != nil {
				return err
			}
			if !os.SameFile(proofInfo, quarantineInfo) {
				if restoreErr := m.restoreQuarantinedVisibleName(
					m.roots.blobs, artifact.quarantine, artifact.final,
				); restoreErr != nil {
					return restoreErr
				}
				return fmt.Errorf("restored foreign staged-blob rollback quarantine")
			}
		} else if !allowMissing {
			return fmt.Errorf("legacy staged-blob rollback quarantine lacks its proof: %s", artifact.quarantine)
		}
		if err := m.roots.blobs.Remove(artifact.quarantine); err != nil {
			return err
		}
	}
	if !proofPresent {
		if finalPresent || !allowMissing {
			return fmt.Errorf("legacy staged-blob rollback lacks its ownership proof: %s", artifact.proof)
		}
		return nil
	}
	if err := m.verifyLegacyBlobArtifactContent(owner, plan, artifact, artifact.proof); err != nil {
		return fmt.Errorf("refusing to clean unverified legacy staged-blob proof: %w", err)
	}
	if finalPresent {
		finalInfo, err := m.roots.blobs.Lstat(artifact.final)
		if err != nil {
			return err
		}
		proofInfo, err := m.roots.blobs.Lstat(artifact.proof)
		if err != nil || !os.SameFile(finalInfo, proofInfo) {
			return fmt.Errorf("legacy staged-blob final lacks ownership proof: %s", artifact.final)
		}
		if m.beforeBlobFinalQuarantine != nil {
			m.beforeBlobFinalQuarantine()
		}
		if err := m.roots.blobs.Rename(artifact.final, artifact.quarantine); err != nil {
			return err
		}
		movedInfo, err := m.roots.blobs.Lstat(artifact.quarantine)
		if err != nil || !os.SameFile(proofInfo, movedInfo) {
			restoreErr := m.restoreQuarantinedVisibleName(
				m.roots.blobs, artifact.quarantine, artifact.final,
			)
			return errors.Join(
				fmt.Errorf("staged-blob final changed while entering rollback quarantine"),
				restoreErr,
			)
		}
		if err := m.verifyLegacyBlobArtifactContent(
			owner, plan, artifact, artifact.quarantine,
		); err != nil {
			restoreErr := m.restoreQuarantinedVisibleName(
				m.roots.blobs, artifact.quarantine, artifact.final,
			)
			return errors.Join(err, restoreErr)
		}
		if err := m.roots.blobs.Remove(artifact.quarantine); err != nil {
			return err
		}
	}
	if err := m.roots.blobs.Rename(artifact.proof, artifact.quarantine); err != nil {
		return err
	}
	if err := m.verifyLegacyBlobArtifactContent(
		owner, plan, artifact, artifact.quarantine,
	); err != nil {
		restoreErr := m.restoreQuarantinedVisibleName(
			m.roots.blobs, artifact.quarantine, artifact.proof,
		)
		return errors.Join(err, restoreErr)
	}
	return m.roots.blobs.Remove(artifact.quarantine)
}

func (m *FilesystemLegacySessionMigrator) syncLegacyBlobOwnerIfPresent(
	ownerRelative string,
) error {
	if _, err := m.roots.blobs.Lstat(ownerRelative); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return syncRootDirectory(m.roots.blobs, ownerRelative)
}

func (m *FilesystemLegacySessionMigrator) removeLegacyStagingDir(relative string) error {
	remove := m.roots.sessions.RemoveAll
	if m.removeLegacyStaging != nil {
		remove = func(relative string) error {
			return m.removeLegacyStaging(m.roots.sessions, relative)
		}
	}
	if err := remove(relative); err != nil {
		return err
	}
	return syncRootDirectory(m.roots.sessions, ".")
}

func (m *FilesystemLegacySessionMigrator) completeLegacyMigration(
	journalName string,
	stagingRelative string,
) error {
	if err := m.removeLegacyStagingDir(stagingRelative); err != nil {
		return err
	}
	if err := m.roots.sessions.Remove(journalName); err != nil &&
		!errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncRootDirectory(m.roots.sessions, legacyMigrationControlDir)
}

type legacyEventLog struct {
	events   []engine.Event
	payloads []json.RawMessage
}

func readLegacyEventsHandle(file *os.File, id app.SessionID) (legacyEventLog, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return legacyEventLog{}, err
	}
	var result legacyEventLog
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var event engine.Event
		if err := json.Unmarshal(raw, &event); err != nil {
			return legacyEventLog{}, fmt.Errorf("session log line %d: %w", line, err)
		}
		result.events = append(result.events, event)
		result.payloads = append(result.payloads, append(json.RawMessage(nil), raw...))
	}
	readErr := scanner.Err()
	if readErr != nil {
		return legacyEventLog{}, readErr
	}
	if len(result.events) == 0 {
		return legacyEventLog{}, fmt.Errorf("legacy session is empty")
	}
	for _, event := range result.events {
		if event.V != engine.LogVersion {
			return legacyEventLog{}, fmt.Errorf("legacy event seq %d has unsupported version %d", event.Seq, event.V)
		}
		if event.Session != string(id) {
			return legacyEventLog{}, fmt.Errorf("legacy event seq %d belongs to session %q", event.Seq, event.Session)
		}
		if event.Event == "" {
			return legacyEventLog{}, fmt.Errorf("legacy event seq %d has no event type", event.Seq)
		}
	}
	return result, nil
}

func legacyMetadata(id app.SessionID, subject string, project app.ProjectID, events []engine.Event) app.SessionMetadata {
	metadata := app.SessionMetadata{
		CodecVersion: app.SessionCodecVersion,
		ID:           id,
		Subject:      subject,
		Project:      project,
	}
	for _, event := range events {
		if event.TS.After(metadata.UpdatedAt) {
			metadata.UpdatedAt = event.TS
		}
		switch event.Event {
		case engine.EventSessionMeta:
			if participant, ok := event.Data["participant"].(string); ok && participant != "" {
				metadata.Participant = participant
			}
		case engine.EventLabeled:
			metadata.Label, _ = event.Data["label"].(string)
		}
	}
	return metadata
}
