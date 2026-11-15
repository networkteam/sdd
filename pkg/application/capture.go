package application

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/llmops"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/pkg/application/types"
)

// PreflightEntry checks a draft without publishing it or opening staged bytes.
// The procedure persists its findings and invalidates them when checked input changes.
func (a *Application) PreflightEntry(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, draft EntryDraft) (PreflightEntryResult, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return PreflightEntryResult{}, err
	}
	draft.Target, err = resolveMutationTarget(ctx, runtime, draft.Target)
	if err != nil {
		return PreflightEntryResult{}, err
	}
	result := PreflightEntryResult{Target: draft.Target}
	if draft.EntryID == "" {
		entryType, err := entryTypeForKind(model.Kind(draft.Kind))
		if err != nil {
			return result, err
		}
		draft.EntryID = model.GenerateIDAt(entryType, draftLayer(draft.Layer), "000", a.now())
	}
	target, effective, err := captureTargetSnapshot(ctx, runtime, draft.Target)
	if err != nil {
		return result, err
	}
	entry, snapshot, _, err := a.prepareCaptureEntry(ctx, identity, principal, effective, target, draft)
	if err != nil {
		return result, err
	}
	registry, err := ProcedureRegistry()
	if err != nil {
		return result, err
	}
	finder := finders.New(finders.Options{
		PreflightRunner: effective.options.LLM, ProcedureRegistry: registry,
		Config: &model.PerRepoConfig{Language: effective.options.Language, Dependencies: effective.options.Dependencies},
	})
	preflight, err := finder.Preflight(ctx, snapshot.graph, query.PreflightQuery{Entry: entry})
	if err != nil {
		return result, fmt.Errorf("pre-flight: %w", err)
	}
	for _, finding := range preflight.Findings {
		result.Findings = append(result.Findings, Finding{Severity: string(finding.Severity), Category: finding.Category, Observation: finding.Observation})
	}
	return result, nil
}

// entryPublicationExists reports whether a committed publication exists under
// the key, reading the target's store without changing it.
func (a *Application) entryPublicationExists(ctx context.Context, identity RequestIdentity, target MutationTarget, key PublicationKey, entryID string) (exists bool, err error) {
	_, runtime, err := a.resolve(ctx, identity, target.Project, AccessRead)
	if err != nil {
		return false, err
	}
	target, err = resolveMutationTarget(ctx, runtime, target)
	if err != nil {
		return false, err
	}
	acquired, err := runtime.acquire(ctx, target)
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, acquired.Release()) }()
	publisher, err := publicationStoreOf(acquired, target.Project)
	if err != nil {
		return false, err
	}
	_, exists, err = publisher.LookupEntryPublication(ctx, key, entryID)
	return exists, err
}

// CreateEntry requires the EntryID, Publication and attachment references recorded
// for this invocation. It returns a prior publication before preparing again;
// summary generation may repeat until publication succeeds. Preflight acceptance
// belongs to the calling procedure. Required finalizers run on every attempt.
func (a *Application) CreateEntry(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, draft EntryDraft) (result CreateEntryResult, err error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return result, err
	}
	result = CreateEntryResult{Project: runtime.options.Project, Binding: binding, EntryID: draft.EntryID}
	if binding.Subject != principal.Subject {
		return result, &ApplicationError{Code: ErrorSessionOwnership, Message: "entry publication ownership mismatch"}
	}
	stored, err := a.sessions.Load(ctx, binding.SessionID)
	if err != nil {
		return result, err
	}
	if err := verifyBinding(stored, binding); err != nil {
		return result, err
	}
	if err := draft.Publication.Validate(); err != nil {
		return result, err
	}
	if draft.Publication.Session != binding.SessionID {
		return result, fmt.Errorf("publication belongs to another session")
	}
	if _, err := model.IDToRelPath(draft.EntryID); err != nil {
		return result, fmt.Errorf("recorded entry ID: %w", err)
	}
	draft.Target, err = resolveMutationTarget(ctx, runtime, draft.Target)
	if err != nil {
		return result, err
	}
	acquired, err := runtime.acquire(ctx, draft.Target)
	if err != nil {
		return result, err
	}
	defer func() {
		if acquired != nil {
			err = errors.Join(err, acquired.Release())
		}
	}()
	publisher, err := publicationStoreOf(acquired, project)
	if err != nil {
		return result, err
	}
	publication, exists, err := publisher.LookupEntryPublication(ctx, draft.Publication, draft.EntryID)
	if err != nil {
		return result, err
	}
	if !exists {
		readRuntime := *runtime
		readRuntime.options.Graph = acquired.Graph
		target, effective, err := readMaterializedSnapshot(ctx, &readRuntime, "")
		if err != nil {
			return result, err
		}
		releaseErr := acquired.Release()
		acquired = nil
		if releaseErr != nil {
			return result, releaseErr
		}
		entry, snapshot, attachments, err := a.prepareCaptureEntry(ctx, identity, principal, effective, target, draft)
		if err != nil {
			return result, err
		}
		if draft.SkipPreflight {
			entry.Preflight = "skipped"
		}
		titles, err := effective.similarTitles(ctx, snapshot, runtime.options.Graph, entry)
		if err != nil {
			return result, fmt.Errorf("generating summary: %w", err)
		}
		described, err := llmops.Describe(ctx, effective.options.LLM, entry, snapshot.graph, effective.options.Language, false, titles)
		if err != nil {
			return result, fmt.Errorf("generating summary: %w", err)
		}
		entry.Summary, entry.Title = described.Summary, described.Title
		logicalPath, err := model.IDToRelPath(draft.EntryID)
		if err != nil {
			return result, err
		}
		canonical := []byte(model.FormatFrontmatter(entry) + "\n" + entry.Content + "\n")
		document, err := ParseEntryDocument(logicalPath, canonical)
		if err != nil {
			return result, err
		}
		document.Attachments = append([]string(nil), entry.Attachments...)
		batch := MutationBatch{
			ID:          draft.Publication.String(),
			Changes:     []DocumentChange{{LogicalPath: logicalPath, Document: &document, CanonicalBytes: canonical}},
			Attachments: attachments,
			Message:     fmt.Sprintf("sdd: %s %s %s", entry.TypeLabel(), entry.LayerLabel(), entry.ShortContent(72)),
		}
		acquired, err = runtime.acquire(ctx, draft.Target)
		if err != nil {
			return result, err
		}
		publisher, err = publicationStoreOf(acquired, project)
		if err != nil {
			return result, err
		}
		publication, err = publisher.PublishEntry(ctx, draft.Publication, batch, ownedBlobReader{store: a.blobs, ref: SessionRef{Subject: principal.Subject, Session: binding.SessionID}})
		if err != nil {
			return result, err
		}
	}
	id, err := model.RelPathToID(publication.Document.LogicalPath)
	if err != nil {
		return result, err
	}
	if id != draft.EntryID {
		return result, fmt.Errorf("publication returned entry %s instead of %s", id, draft.EntryID)
	}
	batch, err := entryPublicationBatch(draft.Publication, publication)
	if err != nil {
		return result, err
	}
	for _, finalizer := range acquired.Finalizers {
		if err := finalizer.Finalize(ctx, AppliedMutation{Project: project, BatchID: batch.ID, Revision: publication.Revision, Batch: batch}); err != nil {
			return result, fmt.Errorf("completing entry publication (%s): %w", finalizer.Name(), err)
		}
	}
	result.EntryID = id
	result.Summary, _ = publication.Document.Frontmatter["summary"].(string)
	return result, nil
}

func captureTargetSnapshot(ctx context.Context, runtime *ProjectRuntime, target MutationTarget) (_ *Snapshot, _ *ProjectRuntime, err error) {
	acquired, err := runtime.acquire(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	defer func() { err = errors.Join(err, acquired.Release()) }()
	selected := *runtime
	selected.options.Graph = acquired.Graph
	return readMaterializedSnapshot(ctx, &selected, "")
}

func entryPublicationBatch(key PublicationKey, publication EntryPublication) (MutationBatch, error) {
	id, err := model.RelPathToID(publication.Document.LogicalPath)
	if err != nil {
		return MutationBatch{}, err
	}
	attachmentDir, err := model.AttachDirRelPath(id)
	if err != nil {
		return MutationBatch{}, err
	}
	batch := MutationBatch{ID: key.String(), Message: "sdd: capture " + id, Changes: []DocumentChange{{LogicalPath: publication.Document.LogicalPath, Document: &publication.Document}}}
	for _, name := range publication.Document.Attachments {
		batch.Attachments = append(batch.Attachments, AttachmentMaterialization{SourceName: name, LogicalPath: path.Join(attachmentDir, name)})
	}
	return batch, nil
}

func (a *Application) prepareCaptureEntry(ctx context.Context, identity RequestIdentity, principal Principal, runtime *ProjectRuntime, target *Snapshot, draft EntryDraft) (*model.Entry, *Snapshot, []AttachmentMaterialization, error) {
	snapshot, err := a.snapshotWithDependencyPolicy(ctx, identity, runtime, target, false)
	if err != nil {
		return nil, nil, nil, err
	}
	entry, findings := entryFromDraft(draft, draft.EntryID, a.now())
	if len(findings) > 0 {
		return nil, nil, nil, captureValidationError(findings)
	}
	if len(entry.Participants) == 0 {
		participant, err := a.participantFor(ctx, principal, runtime)
		if err != nil {
			return nil, nil, nil, err
		}
		if participant != "" {
			entry.Participants = []string{participant}
		}
	}
	attachmentDir, err := model.AttachDirRelPath(draft.EntryID)
	if err != nil {
		return nil, nil, nil, err
	}
	attachments := make([]AttachmentMaterialization, 0, len(draft.Attachments))
	for _, attachment := range draft.Attachments {
		if attachment.Filename == "" || attachment.Filename == "." || path.Base(attachment.Filename) != attachment.Filename || strings.ContainsAny(attachment.Filename, "\\\x00") {
			return nil, nil, nil, fmt.Errorf("invalid staged attachment filename %q", attachment.Filename)
		}
		entry.Attachments = append(entry.Attachments, attachment.Filename)
		attachments = append(attachments, AttachmentMaterialization{BlobID: attachment.BlobID, SourceName: attachment.Filename, LogicalPath: path.Join(attachmentDir, attachment.Filename)})
	}
	entry.Content = model.ResolveAttachmentLinks(entry.Content, draft.EntryID)
	if entry.Refs, err = snapshot.graph.ResolveRefIDs(entry.Refs); err != nil {
		return nil, nil, nil, fmt.Errorf("resolving refs: %w", err)
	}
	if entry.Closes, err = snapshot.graph.ResolveIDs(entry.Closes); err != nil {
		return nil, nil, nil, fmt.Errorf("resolving closes: %w", err)
	}
	if entry.Supersedes, err = snapshot.graph.ResolveIDs(entry.Supersedes); err != nil {
		return nil, nil, nil, fmt.Errorf("resolving supersedes: %w", err)
	}
	construction, findings := model.ConstructFromEntry(entry)
	validated, writeFindings := construction.ValidateForWrite(snapshot.graph)
	findings = append(findings, writeFindings...)
	if len(findings) > 0 {
		return nil, nil, nil, captureValidationError(findings)
	}
	return validated, snapshot, attachments, nil
}

func captureValidationError(findings []model.Finding) error {
	warnings := make([]types.Warning, 0, len(findings))
	for _, finding := range findings {
		warnings = append(warnings, finding.Warning())
	}
	return &ValidationError{Warnings: warnings}
}
