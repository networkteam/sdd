package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/networkteam/sdd/internal/model"
)

// DocumentWrite is one entry-less graph write under the session's recorded
// intent: a summary replacement, a WIP marker created or removed. The key is
// the intent's publication identity, recorded with the write (d-tac-n47).
type DocumentWrite struct {
	Target      MutationTarget
	Publication PublicationKey
	Mutation    DocumentMutation
}

// readDocument returns a document's current bytes on the target, or Absent.
func (a *Application) readDocument(ctx context.Context, identity RequestIdentity, target MutationTarget, logicalPath string) (_ DocumentPublication, err error) {
	_, runtime, err := a.resolve(ctx, identity, target.Project, AccessRead)
	if err != nil {
		return DocumentPublication{}, err
	}
	target, err = resolveMutationTarget(ctx, runtime, target)
	if err != nil {
		return DocumentPublication{}, err
	}
	acquired, err := runtime.acquire(ctx, target)
	if err != nil {
		return DocumentPublication{}, err
	}
	defer func() { err = errors.Join(err, acquired.Release()) }()
	publisher, err := publicationStoreOf(acquired, target.Project)
	if err != nil {
		return DocumentPublication{}, err
	}
	return publisher.ReadDocument(ctx, logicalPath)
}

// lookupDocumentPublication reports what a recorded intent's key published for
// a logical path, reading the target's store without changing it.
func (a *Application) lookupDocumentPublication(ctx context.Context, identity RequestIdentity, target MutationTarget, key PublicationKey, logicalPath string) (_ DocumentPublication, _ bool, err error) {
	_, runtime, err := a.resolve(ctx, identity, target.Project, AccessRead)
	if err != nil {
		return DocumentPublication{}, false, err
	}
	target, err = resolveMutationTarget(ctx, runtime, target)
	if err != nil {
		return DocumentPublication{}, false, err
	}
	acquired, err := runtime.acquire(ctx, target)
	if err != nil {
		return DocumentPublication{}, false, err
	}
	defer func() { err = errors.Join(err, acquired.Release()) }()
	publisher, err := publicationStoreOf(acquired, target.Project)
	if err != nil {
		return DocumentPublication{}, false, err
	}
	return publisher.LookupDocumentPublication(ctx, key, logicalPath)
}

// PublishDocument publishes one document write under its recorded key and runs
// the target's finalizers on what it changed. A write that changed nothing, a
// marker already present or already absent, completes without them.
func (a *Application) PublishDocument(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, write DocumentWrite) (_ DocumentPublication, err error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return DocumentPublication{}, err
	}
	if binding.Subject != principal.Subject {
		return DocumentPublication{}, &ApplicationError{Code: ErrorSessionOwnership, Message: "document publication ownership mismatch"}
	}
	stored, err := a.sessions.Load(ctx, binding.SessionID)
	if err != nil {
		return DocumentPublication{}, err
	}
	if err := verifyBinding(stored, binding); err != nil {
		return DocumentPublication{}, err
	}
	if err := write.Publication.Validate(); err != nil {
		return DocumentPublication{}, err
	}
	if write.Publication.Session != binding.SessionID {
		return DocumentPublication{}, fmt.Errorf("publication belongs to another session")
	}
	if write.Mutation.LogicalPath == "" || write.Mutation.LogicalPath != filepath.ToSlash(write.Mutation.LogicalPath) || strings.HasPrefix(write.Mutation.LogicalPath, "/") || strings.Contains(write.Mutation.LogicalPath, "..") {
		return DocumentPublication{}, fmt.Errorf("invalid document path %q", write.Mutation.LogicalPath)
	}
	target, err := resolveMutationTarget(ctx, runtime, write.Target)
	if err != nil {
		return DocumentPublication{}, err
	}
	if target.Project != runtime.options.Project.ID {
		return DocumentPublication{}, &ApplicationError{Code: ErrorWriteDenied, Message: "mutation target project must equal the session project"}
	}
	acquired, err := runtime.acquire(ctx, target)
	if err != nil {
		return DocumentPublication{}, err
	}
	defer func() { err = errors.Join(err, acquired.Release()) }()
	publisher, err := publicationStoreOf(acquired, project)
	if err != nil {
		return DocumentPublication{}, err
	}
	publication, err := publisher.PublishDocument(ctx, write.Publication, write.Mutation)
	if err != nil {
		return DocumentPublication{}, err
	}
	if publication.Revision == "" {
		return publication, nil
	}
	batch := MutationBatch{
		ID:      write.Publication.String(),
		Message: write.Mutation.Message,
		Changes: []DocumentChange{{LogicalPath: write.Mutation.LogicalPath, CanonicalBytes: write.Mutation.Content, Delete: write.Mutation.Content == nil}},
	}
	for _, finalizer := range acquired.Finalizers {
		if err := finalizer.Finalize(ctx, AppliedMutation{Project: project, BatchID: batch.ID, Revision: publication.Revision, Batch: batch}); err != nil {
			return DocumentPublication{}, fmt.Errorf("completing document publication (%s): %w", finalizer.Name(), err)
		}
	}
	return publication, nil
}

// SummaryReplacement is the recorded input of a summary correction: the entry,
// the bytes it was read as (the write's precondition) and the new summary,
// title or both — an empty one leaves its field as stored.
type SummaryReplacement struct {
	Target       MutationTarget
	Publication  PublicationKey
	EntryID      string
	ExpectedBlob string
	Summary      string
	Title        string
}

// ReplaceSummary writes the corrected summary onto the entry's current
// document, conditioned on the document the correction was read from. A
// document that moved meanwhile is a conflict carrying the current summary, so
// the next attempt is a deliberate one (d-tac-wgw).
func (a *Application) ReplaceSummary(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, replacement SummaryReplacement) (DocumentPublication, error) {
	logicalPath, err := model.IDToRelPath(replacement.EntryID)
	if err != nil {
		return DocumentPublication{}, err
	}
	logicalPath = filepath.ToSlash(logicalPath)
	target := replacement.Target
	if target.Project == "" {
		target.Project = project
	}
	if published, exists, err := a.lookupDocumentPublication(ctx, identity, target, replacement.Publication, logicalPath); err != nil {
		return DocumentPublication{}, err
	} else if exists {
		return published, nil
	}
	current, err := a.readDocument(ctx, identity, target, logicalPath)
	if err != nil {
		return DocumentPublication{}, err
	}
	if current.Absent {
		return DocumentPublication{}, &ApplicationError{Code: ErrorInvalidArgument, Message: "entry not found: " + replacement.EntryID}
	}
	entry, err := model.ParseEntry(replacement.EntryID+".md", string(current.Content))
	if err != nil {
		return DocumentPublication{}, err
	}
	canonical := current.Content
	var title string
	if replacement.Title != "" {
		title, err = model.ParseTitle(replacement.Title)
		if err != nil {
			return DocumentPublication{}, &ApplicationError{Code: ErrorInvalidArgument, Message: err.Error()}
		}
	}
	summaryReplaced := replacement.Summary == "" || entry.Summary == replacement.Summary
	titleReplaced := title == "" || entry.Title == title
	if replacement.ExpectedBlob == "" || GitBlobID(current.Content) == replacement.ExpectedBlob {
		if replacement.Summary != "" {
			entry.Summary = replacement.Summary
		}
		if title != "" {
			entry.Title = title
		}
		canonical = []byte(model.FormatFrontmatter(entry) + "\n" + entry.Content + "\n")
	} else if !summaryReplaced || !titleReplaced {
		return DocumentPublication{}, summaryConflict(replacement.EntryID, entry.Summary, entry.Title)
	}
	// Otherwise an earlier attempt wrote the replacement and its completion did
	// not finish: publishing the same bytes is recognized, not repeated.
	return a.PublishDocument(ctx, identity, project, binding, DocumentWrite{
		Target: target, Publication: replacement.Publication,
		Mutation: DocumentMutation{LogicalPath: logicalPath, Content: canonical, ExpectedBlob: GitBlobID(current.Content), Message: "sdd: summarize " + replacement.EntryID + " (manual)"},
	})
}

// summaryConflict is the answer to a replacement whose document moved: the
// current summary and title and what a deliberate next attempt needs.
func summaryConflict(entryID, currentSummary, currentTitle string) error {
	return &ApplicationError{Code: ErrorGraphConflict, Message: fmt.Sprintf("the summary of %s changed since it was read and was not replaced; its current title is %q and its current summary is %q. Read the entry again before correcting it.", entryID, currentTitle, strings.TrimSpace(currentSummary))}
}

// WIPMarkerWrite is the recorded input of a WIP marker creation.
type WIPMarkerWrite struct {
	Target      MutationTarget
	Publication PublicationKey
	MarkerID    string
	EntryID     string
	Description string
}

// StartWIP publishes an exclusive WIP marker for the entry on the target. The
// marker's identity was allocated before the intent and its path is unique to
// the run, so a retry that finds the marker present publishes nothing.
func (a *Application) StartWIP(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, write WIPMarkerWrite) (DocumentPublication, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return DocumentPublication{}, err
	}
	participant, err := a.participantFor(ctx, principal, runtime)
	if err != nil {
		return DocumentPublication{}, err
	}
	if participant == "" {
		return DocumentPublication{}, fmt.Errorf("sdd: resolved participant is required to start WIP")
	}
	marker := &model.WIPMarker{ID: write.MarkerID, Entry: write.EntryID, Participant: participant, Exclusive: true, Content: write.Description, Time: a.now()}
	return a.PublishDocument(ctx, identity, project, binding, DocumentWrite{
		Target: write.Target, Publication: write.Publication,
		Mutation: DocumentMutation{
			LogicalPath: filepath.ToSlash(model.WIPMarkerPath(write.MarkerID)), Content: []byte(model.FormatWIPMarker(marker)),
			Message: fmt.Sprintf("sdd: wip start %s (%s)", write.EntryID, participant),
		},
	})
}

// FinishWIP removes the named WIP marker from the target. Removing a marker
// that is already absent succeeds and never removes another one.
func (a *Application) FinishWIP(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, target MutationTarget, key PublicationKey, markerID string) (DocumentPublication, error) {
	if target.Project == "" {
		target.Project = project
	}
	return a.PublishDocument(ctx, identity, project, binding, DocumentWrite{
		Target: target, Publication: key,
		Mutation: DocumentMutation{LogicalPath: filepath.ToSlash(model.WIPMarkerPath(markerID)), Message: "sdd: wip done " + markerID},
	})
}

// WIPMarkerID allocates the identity of a marker the resolved principal starts.
func (a *Application) WIPMarkerID(ctx context.Context, identity RequestIdentity, project ProjectID) (string, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return "", err
	}
	participant, err := a.participantFor(ctx, principal, runtime)
	if err != nil {
		return "", err
	}
	if participant == "" {
		return "", fmt.Errorf("sdd: resolved participant is required to start WIP")
	}
	return model.GenerateWIPMarkerID(participant), nil
}

func publicationStoreOf(acquired *AcquiredTarget, project ProjectID) (PublicationStore, error) {
	publisher, ok := acquired.Graph.(PublicationStore)
	if !ok {
		return nil, fmt.Errorf("publication is not configured for project %s", project)
	}
	return publisher, nil
}

// ownedBlobReader limits a publication to the staged blobs of its own session.
type ownedBlobReader struct {
	store StagedBlobStore
	ref   SessionRef
}

func (r ownedBlobReader) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	return r.store.Open(ctx, r.ref, id)
}
