package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"

	"github.com/networkteam/sdd/internal/finders"
	internalllm "github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

type Finding struct {
	Severity    string
	Category    string
	Observation string
}

type EntryRef struct {
	ID   string
	Kind string
	Desc string
}

type EntryDraft struct {
	Kind              string
	Layer             string
	Intent            string
	Body              string
	Refs              []EntryRef
	Closes            []string
	Supersedes        []string
	Participants      []string
	Confidence        string
	Topics            []string
	AttachmentHandles []string
	SkipPreflight     bool
}

type CreateEntryResult struct {
	Project  ProjectRef
	Binding  SessionBinding
	EntryID  string
	Summary  string
	Findings []Finding
}

type MutationResult struct {
	Project ProjectRef
	Binding SessionBinding
}

// CurrentSnapshot resolves current read access and returns the opaque
// canonical snapshot for protocol adapters that host SDD's engine.
func (a *Application) CurrentSnapshot(ctx context.Context, identity RequestIdentity, project ProjectID) (*Snapshot, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return nil, err
	}
	return runtime.options.Graph.Current(ctx)
}

// StageBlob resolves current read access before placing immutable bytes in
// session-scoped scratch. Canonical write access is checked later at the
// mutation gate, so read-only principals can still conduct dialogue.
func (a *Application) StageBlob(ctx context.Context, identity RequestIdentity, project ProjectID, owner BlobOwner, filename string, content []byte) (StagedBlob, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return StagedBlob{}, err
	}
	if owner.Subject != principal.Subject {
		return StagedBlob{}, &ApplicationError{Code: ErrorSessionOwnership, Message: "staged blob owner does not match current principal"}
	}
	return runtime.options.StagedBlobs.Stage(ctx, owner, filename, bytes.NewReader(content))
}

// CreateEntry runs SDD-owned validation and pre-flight, prepares canonical
// document/blob facts, then enters the durable transition protocol.
func (a *Application) CreateEntry(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, draft EntryDraft) (CreateEntryResult, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return CreateEntryResult{}, err
	}
	snapshot, err := a.snapshotWithDependencies(ctx, identity, runtime)
	if err != nil {
		return CreateEntryResult{}, err
	}
	kind := model.Kind(draft.Kind)
	entryType, err := entryTypeForKind(kind)
	if err != nil {
		return CreateEntryResult{}, err
	}
	layer := model.Layer(draft.Layer)
	if expanded, ok := model.LayerFromAbbrev[draft.Layer]; ok {
		layer = expanded
	}
	suffix, err := model.RandomSuffix(3)
	if err != nil {
		return CreateEntryResult{}, err
	}
	id := model.GenerateIDAt(entryType, layer, suffix, runtime.options.Now())
	topics := make([]model.TopicPath, 0, len(draft.Topics))
	for _, label := range draft.Topics {
		topic, err := model.ParseTopicPath(label)
		if err != nil {
			return CreateEntryResult{}, fmt.Errorf("topic %q: %w", label, err)
		}
		topics = append(topics, topic)
	}
	entry := &model.Entry{
		ID: id, Type: entryType, Kind: kind, Layer: layer, Intent: model.Intent(draft.Intent),
		Content: draft.Body, Participants: append([]string(nil), draft.Participants...),
		Confidence: draft.Confidence, Topics: topics, Time: runtime.options.Now(),
	}
	if len(entry.Participants) == 0 {
		if principal.Participant != "" {
			entry.Participants = []string{principal.Participant}
		}
	}
	for _, ref := range draft.Refs {
		entry.Refs = append(entry.Refs, model.Ref{ID: ref.ID, Kind: model.RefKind(ref.Kind), Desc: ref.Desc})
	}
	entry.Closes = append([]string(nil), draft.Closes...)
	entry.Supersedes = append([]string(nil), draft.Supersedes...)
	if entry.Refs, err = snapshot.graph.ResolveRefIDs(entry.Refs); err != nil {
		return CreateEntryResult{}, fmt.Errorf("resolving refs: %w", err)
	}
	if entry.Closes, err = snapshot.graph.ResolveIDs(entry.Closes); err != nil {
		return CreateEntryResult{}, fmt.Errorf("resolving closes: %w", err)
	}
	if entry.Supersedes, err = snapshot.graph.ResolveIDs(entry.Supersedes); err != nil {
		return CreateEntryResult{}, fmt.Errorf("resolving supersedes: %w", err)
	}
	model.ValidateEntry(entry, snapshot.graph)
	if len(entry.Warnings) > 0 {
		return CreateEntryResult{}, fmt.Errorf("validation failed: %s", entry.Warnings[0].Message)
	}

	result := CreateEntryResult{Project: runtime.options.Project, Binding: binding, EntryID: id}
	if !draft.SkipPreflight {
		finder := finders.New(finders.Options{PreflightRunner: runtimeLLMRunner{executor: runtime.options.LLM, purpose: "preflight"}})
		preflightCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		preflight, err := finder.Preflight(preflightCtx, query.PreflightQuery{Entry: entry, Graph: snapshot.graph})
		if err != nil {
			return result, fmt.Errorf("pre-flight: %w", err)
		}
		for _, finding := range preflight.Findings {
			result.Findings = append(result.Findings, Finding{Severity: string(finding.Severity), Category: finding.Category, Observation: finding.Observation})
		}
		for _, finding := range preflight.Findings {
			if finding.Severity == query.SeverityHigh {
				return result, nil
			}
		}
	} else {
		entry.Preflight = "skipped"
	}
	summaryCtx, cancelSummary := context.WithTimeout(ctx, time.Minute)
	defer cancelSummary()
	summary, err := internalllm.Summarize(summaryCtx, runtimeLLMRunner{executor: runtime.options.LLM, purpose: "summary"}, entry, snapshot.graph)
	if err != nil {
		return result, fmt.Errorf("generating summary: %w", err)
	}
	entry.Summary = summary.Summary

	logicalPath, err := model.IDToRelPath(id)
	if err != nil {
		return result, err
	}
	canonical := []byte(model.FormatFrontmatter(entry) + "\n" + entry.Content + "\n")
	batch := MutationBatch{
		ID: "entry-" + id, Changes: []DocumentChange{{LogicalPath: filepath.ToSlash(logicalPath), CanonicalBytes: canonical}},
		Message: fmt.Sprintf("sdd: %s %s %s", entry.TypeLabel(), entry.LayerLabel(), entry.ShortContent(72)),
	}
	owner := BlobOwner{Subject: principal.Subject, Session: binding.SessionID}
	for _, handle := range draft.AttachmentHandles {
		blob, err := runtime.options.StagedBlobs.Stat(ctx, owner, handle)
		if err != nil {
			return result, fmt.Errorf("attachment %q is not staged in this session: %w", handle, err)
		}
		attachDir, err := model.AttachDirRelPath(id)
		if err != nil {
			return result, err
		}
		batch.Attachments = append(batch.Attachments, AttachmentMaterialization{
			BlobID: blob.ID, Digest: blob.Digest, Size: blob.Size, SourceName: blob.Filename,
			LogicalPath: filepath.ToSlash(filepath.Join(attachDir, blob.Filename)),
		})
	}
	batch.Digest, err = MutationBatchDigest(batch)
	if err != nil {
		return result, err
	}
	transition, err := a.ApplyPrepared(ctx, identity, runtime.options.Project.ID, binding, PreparedTransition{
		Version: PreparedTransitionVersion, ExpectedGraphRevision: snapshot.Revision(), Batch: batch,
		BlobOwner: owner, BlobIDs: append([]string(nil), draft.AttachmentHandles...),
	})
	result.Binding = transition.Binding
	if err != nil {
		return result, err
	}
	result.Summary = entry.Summary
	return result, nil
}

func (a *Application) ReplaceSummary(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, entryID, summary string) (MutationResult, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return MutationResult{}, err
	}
	snapshot, err := runtime.options.Graph.Current(ctx)
	if err != nil {
		return MutationResult{}, err
	}
	current, ok := snapshot.graph.ByID[entryID]
	if !ok {
		return MutationResult{}, fmt.Errorf("entry not found: %s", entryID)
	}
	entry := *current
	entry.Summary = summary
	path, err := model.IDToRelPath(entryID)
	if err != nil {
		return MutationResult{}, err
	}
	canonical := []byte(model.FormatFrontmatter(&entry) + "\n" + entry.Content + "\n")
	mutationID, err := newMutationID("summary-" + entryID)
	if err != nil {
		return MutationResult{}, err
	}
	return a.applyDocumentMutation(ctx, identity, runtime, binding, mutationID, "sdd: summarize "+entryID+" (manual)", DocumentChange{LogicalPath: filepath.ToSlash(path), CanonicalBytes: canonical})
}

func (a *Application) StartWIP(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, entryID, description string) (string, MutationResult, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return "", MutationResult{}, err
	}
	if principal.Participant == "" {
		return "", MutationResult{}, fmt.Errorf("sdd: resolved principal participant is required to start WIP")
	}
	marker := &model.WIPMarker{
		ID: model.GenerateWIPMarkerID(principal.Participant), Entry: entryID, Participant: principal.Participant,
		Exclusive: true, Content: description, Time: runtime.options.Now(),
	}
	result, err := a.applyDocumentMutation(ctx, identity, runtime, binding, "wip-start-"+marker.ID, fmt.Sprintf("sdd: wip start %s (%s)", entryID, principal.Participant), DocumentChange{
		LogicalPath: filepath.ToSlash(model.WIPMarkerPath(marker.ID)), CanonicalBytes: []byte(model.FormatWIPMarker(marker)),
	})
	return marker.ID, result, err
}

func (a *Application) FinishWIP(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, markerID string) (MutationResult, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return MutationResult{}, err
	}
	return a.applyDocumentMutation(ctx, identity, runtime, binding, "wip-done-"+markerID, "sdd: wip done "+markerID, DocumentChange{LogicalPath: filepath.ToSlash(model.WIPMarkerPath(markerID)), Delete: true})
}

func (a *Application) applyDocumentMutation(ctx context.Context, identity RequestIdentity, runtime *ProjectRuntime, binding SessionBinding, id, message string, change DocumentChange) (MutationResult, error) {
	snapshot, err := runtime.options.Graph.Current(ctx)
	if err != nil {
		return MutationResult{}, err
	}
	batch := MutationBatch{ID: id, Message: message, Changes: []DocumentChange{change}}
	batch.Digest, err = MutationBatchDigest(batch)
	if err != nil {
		return MutationResult{}, err
	}
	transition, err := a.ApplyPrepared(ctx, identity, runtime.options.Project.ID, binding, PreparedTransition{
		Version: PreparedTransitionVersion, ExpectedGraphRevision: snapshot.Revision(), Batch: batch,
		BlobOwner: BlobOwner{Subject: binding.Subject, Session: binding.SessionID},
	})
	return MutationResult{Project: runtime.options.Project, Binding: transition.Binding}, err
}

func newMutationID(prefix string) (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("sdd: generating mutation ID: %w", err)
	}
	return prefix + "-" + hex.EncodeToString(random[:]), nil
}

func entryTypeForKind(kind model.Kind) (model.EntryType, error) {
	switch {
	case model.IsValidKindForType(model.TypeSignal, kind):
		return model.TypeSignal, nil
	case model.IsValidKindForType(model.TypeDecision, kind):
		return model.TypeDecision, nil
	default:
		return "", fmt.Errorf("unknown entry kind %q", kind)
	}
}

type runtimeLLMRunner struct {
	executor LLMExecutor
	purpose  string
}

func (r runtimeLLMRunner) Run(ctx context.Context, request internalllm.Request) (*internalllm.RunResult, error) {
	result, err := r.executor.Execute(ctx, LLMRequest{Purpose: r.purpose, SystemPrompt: request.SystemPrompt, Prompt: request.UserPrompt})
	if err != nil {
		return nil, err
	}
	return &internalllm.RunResult{Text: string(result.Output)}, nil
}
