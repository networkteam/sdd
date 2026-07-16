package local

import (
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
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/gofrs/flock"
	app "github.com/networkteam/sdd/application"
)

type FilesystemGraphStoreOptions struct {
	Project  app.ProjectID
	GraphDir string
}

// FilesystemGraphStore is the local canonical graph authority. It owns its
// revision cache and never requires callers to invalidate snapshots.
type FilesystemGraphStore struct {
	project app.ProjectID
	dir     string
	mu      sync.Mutex

	beforeApplyOperation    func(int) error
	beforeRollbackOperation func(int) error
}

type filesystemApplyRecord struct {
	Digest           string                 `json:"digest"`
	ExpectedRevision string                 `json:"expected_revision"`
	Result           app.ApplyResult        `json:"result"`
	Transaction      *filesystemTransaction `json:"transaction,omitempty"`
}

type filesystemTransaction struct {
	Operations []filesystemOperation `json:"operations"`
}

type filesystemOperation struct {
	LogicalPath string              `json:"logical_path"`
	Before      filesystemFileState `json:"before"`
	After       filesystemFileState `json:"after"`
	BackupPath  string              `json:"backup_path,omitempty"`
	StagedPath  string              `json:"staged_path,omitempty"`
}

type filesystemFileState struct {
	Exists bool   `json:"exists"`
	Digest string `json:"digest,omitempty"`
	Mode   uint32 `json:"mode,omitempty"`
}

func NewFilesystemGraphStore(options FilesystemGraphStoreOptions) (*FilesystemGraphStore, error) {
	if options.Project == "" {
		return nil, fmt.Errorf("sdd: filesystem graph project is required")
	}
	if options.GraphDir == "" {
		return nil, fmt.Errorf("sdd: filesystem graph directory is required")
	}
	if err := os.MkdirAll(options.GraphDir, 0o755); err != nil {
		return nil, fmt.Errorf("sdd: creating filesystem graph directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(options.GraphDir, ".sdd-runtime", "applied"), 0o755); err != nil {
		return nil, fmt.Errorf("sdd: creating graph runtime directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(options.GraphDir, ".sdd-runtime", "transactions"), 0o755); err != nil {
		return nil, fmt.Errorf("sdd: creating graph transaction directory: %w", err)
	}
	return &FilesystemGraphStore{
		project: options.Project,
		dir:     options.GraphDir,
	}, nil
}

func (s *FilesystemGraphStore) Current(ctx context.Context) (*app.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlockFile(lock)
	if err := s.recoverPendingTransactionsLocked(); err != nil {
		return nil, err
	}
	return s.currentLocked(ctx)
}

func (s *FilesystemGraphStore) currentLocked(ctx context.Context) (*app.Snapshot, error) {
	revision, err := graphDirectoryRevision(s.dir)
	if err != nil {
		return nil, err
	}
	return app.LoadSnapshotFS(ctx, s.project, revision, os.DirFS(s.dir), ".")
}

func (s *FilesystemGraphStore) Apply(ctx context.Context, expectedRevision string, batch app.MutationBatch, blobs app.StagedBlobReader) (app.ApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lock()
	if err != nil {
		return app.ApplyResult{State: app.MutationNotApplied}, err
	}
	defer unlockFile(lock)
	if err := s.recoverPendingTransactionsLocked(); err != nil {
		return app.ApplyResult{State: app.MutationUnknown}, err
	}
	if prior, ok, err := s.loadApplyRecord(batch.ID); err != nil {
		return app.ApplyResult{State: app.MutationUnknown}, err
	} else if ok {
		if prior.Digest != batch.Digest {
			return app.ApplyResult{State: app.MutationNotApplied}, &app.ApplicationError{Code: app.ErrorRecoveryRequired, Message: "mutation ID reused with a different digest"}
		}
		if prior.Result.State == app.MutationUnknown {
			return s.reconcileRecord(batch.ID, prior)
		}
		return prior.Result, nil
	}
	currentRevision, err := graphDirectoryRevision(s.dir)
	if err != nil {
		return app.ApplyResult{State: app.MutationNotApplied}, err
	}
	if currentRevision != expectedRevision {
		return app.ApplyResult{State: app.MutationNotApplied, Revision: currentRevision}, &app.ApplicationError{Code: app.ErrorGraphConflict, Message: "graph revision changed", Revision: currentRevision}
	}
	wantDigest, err := app.MutationBatchDigest(batch)
	if err != nil {
		return app.ApplyResult{State: app.MutationNotApplied, Revision: currentRevision}, err
	}
	if batch.ID == "" || batch.Digest == "" || batch.Digest != wantDigest {
		return app.ApplyResult{State: app.MutationNotApplied, Revision: currentRevision}, &app.ApplicationError{Code: app.ErrorRecoveryRequired, Message: "mutation batch digest mismatch"}
	}
	transaction, err := s.prepareTransaction(ctx, batch, blobs)
	if err != nil {
		return app.ApplyResult{State: app.MutationNotApplied, Revision: currentRevision}, err
	}
	record := filesystemApplyRecord{
		Digest: batch.Digest, ExpectedRevision: expectedRevision,
		Result: app.ApplyResult{State: app.MutationUnknown, Revision: currentRevision}, Transaction: transaction,
	}
	if err := s.persistApplyRecord(batch.ID, record); err != nil {
		cleanupErr := s.removeTransaction(batch.ID)
		return app.ApplyResult{State: app.MutationNotApplied, Revision: currentRevision}, errors.Join(err, cleanupErr)
	}
	if err := s.applyTransaction(transaction); err != nil {
		if rollbackErr := s.rollbackTransaction(transaction); rollbackErr != nil {
			return app.ApplyResult{State: app.MutationUnknown, Revision: currentRevision}, errors.Join(err, fmt.Errorf("rolling back mutation %s: %w", batch.ID, rollbackErr))
		}
		rolledBackRevision, revisionErr := graphDirectoryRevision(s.dir)
		if revisionErr != nil {
			return app.ApplyResult{State: app.MutationUnknown}, errors.Join(err, revisionErr)
		}
		result := app.ApplyResult{State: app.MutationNotApplied, Revision: rolledBackRevision}
		record.Result = result
		if persistErr := s.persistApplyRecord(batch.ID, record); persistErr != nil {
			return app.ApplyResult{State: app.MutationUnknown, Revision: rolledBackRevision}, errors.Join(err, persistErr)
		}
		cleanupErr := s.removeTransaction(batch.ID)
		return result, errors.Join(err, cleanupErr)
	}
	newRevision, err := graphDirectoryRevision(s.dir)
	if err != nil {
		return app.ApplyResult{State: app.MutationUnknown}, err
	}
	result := app.ApplyResult{State: app.MutationApplied, Revision: newRevision}
	record.Result = result
	if err := s.persistApplyRecord(batch.ID, record); err != nil {
		return app.ApplyResult{State: app.MutationUnknown, Revision: newRevision}, err
	}
	return result, s.removeTransaction(batch.ID)
}

func (s *FilesystemGraphStore) prepareTransaction(ctx context.Context, batch app.MutationBatch, blobs app.StagedBlobReader) (_ *filesystemTransaction, err error) {
	type plannedOperation struct {
		logicalPath string
		after       []byte
		delete      bool
	}
	var planned []plannedOperation
	seen := map[string]bool{}
	add := func(logicalPath string, data []byte, deleteFile bool) error {
		if _, err := canonicalGraphPath(s.dir, logicalPath); err != nil {
			return err
		}
		if seen[logicalPath] {
			return fmt.Errorf("sdd: mutation contains duplicate graph path %q", logicalPath)
		}
		seen[logicalPath] = true
		if !deleteFile && len(data) == 0 {
			return fmt.Errorf("sdd: empty canonical bytes for %s", logicalPath)
		}
		planned = append(planned, plannedOperation{logicalPath: logicalPath, after: append([]byte(nil), data...), delete: deleteFile})
		return nil
	}
	for _, change := range batch.Changes {
		if err := add(change.LogicalPath, change.CanonicalBytes, change.Delete); err != nil {
			return nil, err
		}
	}
	for _, attachment := range batch.Attachments {
		if blobs == nil {
			return nil, fmt.Errorf("sdd: staged blob reader is required")
		}
		reader, err := blobs.Open(ctx, attachment.BlobID)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if int64(len(data)) != attachment.Size || !digestMatches(data, attachment.Digest) {
			return nil, fmt.Errorf("sdd: staged blob %s does not match prepared facts", attachment.BlobID)
		}
		if err := add(attachment.LogicalPath, data, false); err != nil {
			return nil, err
		}
	}

	if err := s.removeTransaction(batch.ID); err != nil {
		return nil, err
	}
	transactionDir, err := s.transactionDir(batch.ID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(transactionDir, 0o755); err != nil {
		return nil, err
	}
	if err := syncDirectory(filepath.Dir(transactionDir)); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, s.removeTransaction(batch.ID))
		}
	}()
	transaction := &filesystemTransaction{Operations: make([]filesystemOperation, 0, len(planned))}
	for index, item := range planned {
		target, err := canonicalGraphPath(s.dir, item.logicalPath)
		if err != nil {
			return nil, err
		}
		before, beforeBytes, err := readFilesystemState(target)
		if err != nil {
			return nil, fmt.Errorf("sdd: reading graph path %s before mutation: %w", item.logicalPath, err)
		}
		operation := filesystemOperation{LogicalPath: item.logicalPath, Before: before}
		if before.Exists {
			operation.BackupPath = transactionFilePath(batch.ID, "backup", index)
			backup, err := journalPath(s.dir, operation.BackupPath)
			if err != nil {
				return nil, err
			}
			if err := writeFileDurable(backup, beforeBytes, fs.FileMode(before.Mode)); err != nil {
				return nil, err
			}
		}
		if !item.delete {
			operation.After = filesystemFileState{Exists: true, Digest: filesystemDigest(item.after), Mode: uint32(0o644)}
			operation.StagedPath = transactionFilePath(batch.ID, "staged", index)
			staged, err := journalPath(s.dir, operation.StagedPath)
			if err != nil {
				return nil, err
			}
			if err := writeFileDurable(staged, item.after, 0o644); err != nil {
				return nil, err
			}
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		transaction.Operations = append(transaction.Operations, operation)
	}
	if err := syncDirectory(transactionDir); err != nil {
		return nil, err
	}
	return transaction, nil
}

func (s *FilesystemGraphStore) applyTransaction(transaction *filesystemTransaction) error {
	if err := s.validateTransactionStates(transaction); err != nil {
		return err
	}
	for index, operation := range transaction.Operations {
		if s.beforeApplyOperation != nil {
			if err := s.beforeApplyOperation(index); err != nil {
				return err
			}
		}
		target, err := canonicalGraphPath(s.dir, operation.LogicalPath)
		if err != nil {
			return err
		}
		if operation.After.Exists {
			staged, err := journalPath(s.dir, operation.StagedPath)
			if err != nil {
				return err
			}
			if err := os.Rename(staged, target); err != nil {
				return err
			}
		} else if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := syncDirectory(filepath.Dir(target)); err != nil {
			return err
		}
	}
	return nil
}

func (s *FilesystemGraphStore) rollbackTransaction(transaction *filesystemTransaction) error {
	if err := s.validateTransactionStates(transaction); err != nil {
		return err
	}
	for index := len(transaction.Operations) - 1; index >= 0; index-- {
		operation := transaction.Operations[index]
		if s.beforeRollbackOperation != nil {
			if err := s.beforeRollbackOperation(index); err != nil {
				return err
			}
		}
		target, err := canonicalGraphPath(s.dir, operation.LogicalPath)
		if err != nil {
			return err
		}
		current, _, err := readFilesystemState(target)
		if err != nil {
			return err
		}
		if sameFilesystemState(current, operation.Before) {
			continue
		}
		if operation.Before.Exists {
			backup, err := journalPath(s.dir, operation.BackupPath)
			if err != nil {
				return err
			}
			if err := restoreFile(backup, target, fs.FileMode(operation.Before.Mode)); err != nil {
				return err
			}
		} else if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := syncDirectory(filepath.Dir(target)); err != nil {
			return err
		}
	}
	return nil
}

func (s *FilesystemGraphStore) validateTransactionStates(transaction *filesystemTransaction) error {
	for _, operation := range transaction.Operations {
		target, err := canonicalGraphPath(s.dir, operation.LogicalPath)
		if err != nil {
			return err
		}
		current, _, err := readFilesystemState(target)
		if err != nil {
			return err
		}
		if !sameFilesystemState(current, operation.Before) && !sameFilesystemState(current, operation.After) {
			return fmt.Errorf("sdd: graph path %s changed outside pending mutation", operation.LogicalPath)
		}
	}
	return nil
}

func (s *FilesystemGraphStore) Reconcile(_ context.Context, mutationID, batchDigest string) (app.ApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lock()
	if err != nil {
		return app.ApplyResult{State: app.MutationUnknown}, err
	}
	defer unlockFile(lock)
	record, ok, err := s.loadApplyRecord(mutationID)
	if err != nil {
		return app.ApplyResult{State: app.MutationUnknown}, err
	}
	if !ok {
		// The applied-record directory is the filesystem adapter's canonical
		// batch ledger. Absence while holding its lock is definitive evidence
		// that this batch was not applied, not an unknown outcome.
		return app.ApplyResult{State: app.MutationNotApplied}, nil
	}
	if record.Digest != batchDigest {
		return app.ApplyResult{State: app.MutationUnknown}, &app.ApplicationError{Code: app.ErrorRecoveryRequired, Message: "mutation digest does not match recorded apply"}
	}
	return s.reconcileRecord(mutationID, record)
}

func (s *FilesystemGraphStore) ReadAttachmentPage(_ context.Context, entryID, filename string, offset int64, maxBytes int) (app.AttachmentPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.lock()
	if err != nil {
		return app.AttachmentPage{}, err
	}
	defer unlockFile(lock)
	if err := s.recoverPendingTransactionsLocked(); err != nil {
		return app.AttachmentPage{}, err
	}
	if offset < 0 || maxBytes <= 0 {
		return app.AttachmentPage{}, fmt.Errorf("sdd: invalid attachment page range")
	}
	attachmentDir, err := app.AttachmentDirRelPath(entryID)
	if err != nil {
		return app.AttachmentPage{}, err
	}
	if filename == "" {
		entries, readErr := os.ReadDir(filepath.Join(s.dir, attachmentDir))
		if readErr != nil {
			return app.AttachmentPage{}, readErr
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				if filename != "" {
					return app.AttachmentPage{}, fmt.Errorf("sdd: attachment name is required when entry %s has more than one attachment", entryID)
				}
				filename = entry.Name()
			}
		}
		if filename == "" {
			return app.AttachmentPage{}, fmt.Errorf("sdd: entry %s has no attachments", entryID)
		}
	}
	if filepath.Base(filename) != filename || strings.ContainsAny(filename, `/\\`) {
		return app.AttachmentPage{}, fmt.Errorf("sdd: invalid attachment filename %q", filename)
	}
	logicalPath := filepath.ToSlash(filepath.Join(attachmentDir, filename))
	target, err := safeGraphPath(s.dir, logicalPath)
	if err != nil {
		return app.AttachmentPage{}, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return app.AttachmentPage{}, err
	}
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	end := offset + int64(maxBytes)
	if end > int64(len(data)) {
		end = int64(len(data))
	}
	return app.AttachmentPage{
		Filename: filename, Content: append([]byte(nil), data[offset:end]...), Offset: offset,
		NextOffset: end, TotalSize: int64(len(data)), More: end < int64(len(data)), Digest: sha256Digest(data),
	}, nil
}

func (s *FilesystemGraphStore) lock() (*flock.Flock, error) {
	lock := flock.New(filepath.Join(s.dir, ".sdd-runtime", "graph.lock"))
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	return lock, nil
}

func (s *FilesystemGraphStore) applyRecordPath(id string) (string, error) {
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return "", fmt.Errorf("sdd: invalid mutation ID %q", id)
	}
	return filepath.Join(s.dir, ".sdd-runtime", "applied", id+".json"), nil
}

func (s *FilesystemGraphStore) loadApplyRecord(id string) (filesystemApplyRecord, bool, error) {
	filename, err := s.applyRecordPath(id)
	if err != nil {
		return filesystemApplyRecord{}, false, err
	}
	raw, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return filesystemApplyRecord{}, false, nil
	}
	if err != nil {
		return filesystemApplyRecord{}, false, err
	}
	var record filesystemApplyRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return filesystemApplyRecord{}, false, err
	}
	return record, true, nil
}

func (s *FilesystemGraphStore) persistApplyRecord(id string, record filesystemApplyRecord) error {
	filename, err := s.applyRecordPath(id)
	if err != nil {
		return err
	}
	return writeJSONAtomic(filename, record)
}

func (s *FilesystemGraphStore) reconcileRecord(id string, record filesystemApplyRecord) (app.ApplyResult, error) {
	if record.Result.State != app.MutationUnknown {
		return record.Result, nil
	}
	if record.Transaction != nil {
		if err := s.rollbackTransaction(record.Transaction); err != nil {
			return app.ApplyResult{State: app.MutationUnknown, Revision: record.Result.Revision}, err
		}
		revision, err := graphDirectoryRevision(s.dir)
		if err != nil {
			return app.ApplyResult{State: app.MutationUnknown}, err
		}
		result := app.ApplyResult{State: app.MutationNotApplied, Revision: revision}
		record.Result = result
		if err := s.persistApplyRecord(id, record); err != nil {
			return app.ApplyResult{State: app.MutationUnknown, Revision: revision}, err
		}
		return result, s.removeTransaction(id)
	}
	revision, err := graphDirectoryRevision(s.dir)
	if err != nil {
		return app.ApplyResult{State: app.MutationUnknown}, err
	}
	if revision == record.ExpectedRevision {
		return app.ApplyResult{State: app.MutationNotApplied, Revision: revision}, nil
	}
	return app.ApplyResult{State: app.MutationUnknown, Revision: revision}, nil
}

func (s *FilesystemGraphStore) recoverPendingTransactionsLocked() error {
	dir := filepath.Join(s.dir, ".sdd-runtime", "applied")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		record, ok, err := s.loadApplyRecord(id)
		if err != nil {
			return err
		}
		if !ok || record.Result.State != app.MutationUnknown || record.Transaction == nil {
			continue
		}
		if _, err := s.reconcileRecord(id, record); err != nil {
			return fmt.Errorf("sdd: recovering pending mutation %s: %w", id, err)
		}
	}
	return nil
}

func graphDirectoryRevision(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() && entry.Name() == ".sdd-runtime" {
			return filepath.SkipDir
		}
		if !entry.IsDir() {
			files = append(files, filename)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, filename := range files {
		rel, err := filepath.Rel(dir, filename)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(hash, filepath.ToSlash(rel))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *FilesystemGraphStore) removeTransaction(id string) error {
	dir, err := s.transactionDir(id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(dir))
}

func (s *FilesystemGraphStore) transactionDir(id string) (string, error) {
	if _, err := s.applyRecordPath(id); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, ".sdd-runtime", "transactions", id), nil
}

func transactionFilePath(id, class string, index int) string {
	return filepath.ToSlash(filepath.Join(".sdd-runtime", "transactions", id, class, fmt.Sprintf("%06d", index)))
}

func journalPath(root, logicalPath string) (string, error) {
	if !strings.HasPrefix(logicalPath, ".sdd-runtime/transactions/") {
		return "", fmt.Errorf("sdd: invalid transaction path %q", logicalPath)
	}
	return safeGraphPath(root, logicalPath)
}

func canonicalGraphPath(root, logicalPath string) (string, error) {
	if logicalPath == ".sdd-runtime" || strings.HasPrefix(logicalPath, ".sdd-runtime/") {
		return "", fmt.Errorf("sdd: canonical mutation cannot target runtime path %q", logicalPath)
	}
	return safeGraphPath(root, logicalPath)
}

func readFilesystemState(filename string) (filesystemFileState, []byte, error) {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return filesystemFileState{}, nil, nil
	}
	if err != nil {
		return filesystemFileState{}, nil, err
	}
	if !info.Mode().IsRegular() {
		return filesystemFileState{}, nil, fmt.Errorf("not a regular file")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return filesystemFileState{}, nil, err
	}
	return filesystemFileState{Exists: true, Digest: filesystemDigest(data), Mode: uint32(info.Mode().Perm())}, data, nil
}

func sameFilesystemState(left, right filesystemFileState) bool {
	if left.Exists != right.Exists {
		return false
	}
	if !left.Exists {
		return true
	}
	return left.Digest == right.Digest && left.Mode == right.Mode
}

func filesystemDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func writeFileDurable(filename string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if err := file.Chmod(mode); err != nil {
		return errors.Join(err, file.Close())
	}
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(filename))
}

func restoreFile(backup, target string, mode fs.FileMode) error {
	data, err := os.ReadFile(backup)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".sdd-rollback-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := temp.Chmod(mode); err != nil {
		return errors.Join(err, temp.Close())
	}
	if _, err := temp.Write(data); err != nil {
		return errors.Join(err, temp.Close())
	}
	if err := temp.Sync(); err != nil {
		return errors.Join(err, temp.Close())
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, target)
}

func syncDirectory(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = handle.Close() }()
	return handle.Sync()
}

func safeGraphPath(root, logicalPath string) (string, error) {
	if logicalPath == "" || strings.Contains(logicalPath, "\\") || !fs.ValidPath(logicalPath) {
		return "", fmt.Errorf("sdd: invalid graph logical path %q", logicalPath)
	}
	target := filepath.Join(root, filepath.FromSlash(logicalPath))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("sdd: graph path escapes root: %q", logicalPath)
	}
	return target, nil
}

func sha256Digest(data []byte) app.BlobDigest {
	sum := sha256.Sum256(data)
	return app.BlobDigest{Algorithm: "sha256", Value: hex.EncodeToString(sum[:])}
}

func digestMatches(data []byte, digest app.BlobDigest) bool {
	return digest.Algorithm == "sha256" && sha256Digest(data).Value == digest.Value
}
