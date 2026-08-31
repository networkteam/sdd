package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
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

type FactIndex struct {
	Title string `json:"title"`
	Topic string `json:"topic"`
}

type EntryDraft struct {
	Target            MutationTarget
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
	Index             *FactIndex
	AttachmentHandles []string
	// Canonical and Aliases carry a kind: actor signal's identity; Actor carries
	// a kind: role decision's bound actor canonical; Class carries a
	// kind: procedure decision's execution role. Mirrors the CLI-side
	// NewEntryCmd fields — a value on the wrong kind is a blocking finding at
	// the construction boundary.
	Canonical string
	Aliases   []string
	Actor     string
	Class     string
	// ProcedureSpec carries a kind: procedure decision's workflow declaration
	// as one structured document — {params?, state?, steps, framing?} —
	// converted strictly at draft-to-entry assembly. Interpretation stays
	// with the engine.
	ProcedureSpec map[string]any
	// FocusActors, FocusWhen, and Involvement carry a kind: focus decision's
	// advances list and its focus-level defaults. Mirrors the CLI-side
	// NewEntryCmd fields — ignored on other kinds, written onto the entry so
	// the model-layer validator sees the required involvement frontmatter.
	FocusActors   []string
	FocusWhen     *model.FocusWhen
	Involvement   []model.Involvement
	SkipPreflight bool
}

// ValidationError reports that model.ValidateEntry rejected a draft at the
// write gate. It carries the structural warnings so the workflow gate can
// re-serve them as actionable findings — naming the violated rule and the
// field — and route the instance back to a step that can fix it, rather than
// wedging behind an opaque hard error (closes half of s-prc-g0j).
type ValidationError struct {
	Warnings []model.Warning
}

func (e *ValidationError) Error() string {
	if len(e.Warnings) == 0 {
		return "validation failed"
	}
	parts := make([]string, 0, len(e.Warnings))
	for _, w := range e.Warnings {
		parts = append(parts, w.Message)
	}
	return "validation failed: " + strings.Join(parts, "; ")
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
func (a *Application) StageBlob(ctx context.Context, identity RequestIdentity, project ProjectID, ref SessionRef, filename string, content []byte) (StagedBlob, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return StagedBlob{}, err
	}
	if ref.Subject != principal.Subject {
		return StagedBlob{}, &ApplicationError{Code: ErrorSessionOwnership, Message: "staged blobs belong to another principal"}
	}
	return runtime.options.StagedBlobs.Stage(ctx, ref, filename, bytes.NewReader(content))
}

// OpenStagedBlob resolves read access and session ownership, then streams a
// staged blob's bytes — the read-side counterpart of StageBlob.
func (a *Application) OpenStagedBlob(ctx context.Context, identity RequestIdentity, project ProjectID, ref SessionRef, blobID string) (io.ReadCloser, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return nil, err
	}
	if ref.Subject != principal.Subject {
		return nil, &ApplicationError{Code: ErrorSessionOwnership, Message: "staged blobs belong to another principal"}
	}
	return runtime.options.StagedBlobs.Open(ctx, ref, blobID)
}

// CreateEntry runs SDD-owned validation and pre-flight, prepares canonical
// document/blob facts, then enters the durable transition protocol.
func (a *Application) CreateEntry(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, draft EntryDraft) (CreateEntryResult, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return CreateEntryResult{}, err
	}
	target, err := resolveMutationTarget(runtime, draft.Target)
	if err != nil {
		return CreateEntryResult{}, err
	}
	targetSnapshot, err := snapshotMutationTarget(ctx, runtime, target)
	if err != nil {
		return CreateEntryResult{}, err
	}
	snapshot, err := a.snapshotWithDependenciesFrom(ctx, identity, runtime, targetSnapshot)
	if err != nil {
		return CreateEntryResult{}, err
	}
	kind := model.Kind(draft.Kind)
	entryType, err := entryTypeForKind(kind)
	if err != nil {
		return CreateEntryResult{}, err
	}
	layer := draftLayer(draft.Layer)
	suffix, err := model.RandomSuffix(3)
	if err != nil {
		return CreateEntryResult{}, err
	}
	id := model.GenerateIDAt(entryType, layer, suffix, runtime.options.Now())
	entry, assemblyFindings := entryFromDraft(draft, id, runtime.options.Now())
	if len(assemblyFindings) > 0 {
		warnings := make([]model.Warning, 0, len(assemblyFindings))
		for _, f := range assemblyFindings {
			warnings = append(warnings, f.Warning())
		}
		return CreateEntryResult{}, &ValidationError{Warnings: warnings}
	}
	if len(entry.Participants) == 0 {
		if principal.Participant != "" {
			entry.Participants = []string{principal.Participant}
		}
	}
	// Attachment truth is established before the gate: staged handles resolve
	// to filenames on the entry and {{attachments}} links resolve against the
	// minted ID, so ValidateForWrite's link check sees what the entry actually
	// carries (20260707-175502-s-prc-lgu).
	owner := SessionRef{Subject: principal.Subject, Session: binding.SessionID}
	var materializations []AttachmentMaterialization
	for _, handle := range draft.AttachmentHandles {
		blob, err := runtime.options.StagedBlobs.Stat(ctx, owner, handle)
		if err != nil {
			return CreateEntryResult{}, fmt.Errorf("attachment %q is not staged in this session: %w", handle, err)
		}
		attachDir, err := model.AttachDirRelPath(id)
		if err != nil {
			return CreateEntryResult{}, err
		}
		entry.Attachments = append(entry.Attachments, blob.Filename)
		materializations = append(materializations, AttachmentMaterialization{
			BlobID: blob.ID, Digest: blob.Digest, Size: blob.Size, SourceName: blob.Filename,
			LogicalPath: filepath.ToSlash(filepath.Join(attachDir, blob.Filename)),
		})
	}
	entry.Content = model.ResolveAttachmentLinks(entry.Content, id)
	if entry.Refs, err = snapshot.graph.ResolveRefIDs(entry.Refs); err != nil {
		return CreateEntryResult{}, fmt.Errorf("resolving refs: %w", err)
	}
	if entry.Closes, err = snapshot.graph.ResolveIDs(entry.Closes); err != nil {
		return CreateEntryResult{}, fmt.Errorf("resolving closes: %w", err)
	}
	if entry.Supersedes, err = snapshot.graph.ResolveIDs(entry.Supersedes); err != nil {
		return CreateEntryResult{}, fmt.Errorf("resolving supersedes: %w", err)
	}
	// The construction boundary is the write gate: stray per-kind fields
	// surface as projection findings, and ValidateForWrite runs the full rule
	// set including the capture-only rules the read path waives — so this
	// surface enforces exactly what the CLI write path enforces.
	construction, findings := model.ConstructFromEntry(entry)
	validated, writeFindings := construction.ValidateForWrite(snapshot.graph)
	findings = append(findings, writeFindings...)
	if len(findings) > 0 {
		warnings := make([]model.Warning, 0, len(findings))
		for _, f := range findings {
			warnings = append(warnings, f.Warning())
		}
		return CreateEntryResult{}, &ValidationError{Warnings: warnings}
	}
	entry = validated

	result := CreateEntryResult{Project: runtime.options.Project, Binding: binding, EntryID: id}
	if !draft.SkipPreflight {
		resolver, err := ProcedureQueryResolver()
		if err != nil {
			return result, fmt.Errorf("pre-flight: %w", err)
		}
		finder := finders.New(finders.Options{
			PreflightRunner:   runtimeLLMRunner{executor: runtime.options.LLM, purpose: "preflight"},
			ProcedureResolver: resolver,
		})
		preflightCtx, cancel := context.WithTimeout(ctx, runtime.options.LLMTimeout)
		defer cancel()
		preflight, err := finder.Preflight(preflightCtx, snapshot.graph, query.PreflightQuery{Entry: entry})
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
	summaryCtx, cancelSummary := context.WithTimeout(ctx, runtime.options.LLMTimeout)
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
	document, err := parseEntryDocument(filepath.ToSlash(logicalPath), canonical)
	if err != nil {
		return result, err
	}
	batch := MutationBatch{
		ID: "entry-" + id, Changes: []DocumentChange{{LogicalPath: filepath.ToSlash(logicalPath), Document: &document, CanonicalBytes: canonical}},
		Message: fmt.Sprintf("sdd: %s %s %s", entry.TypeLabel(), entry.LayerLabel(), entry.ShortContent(72)),
	}
	batch.Attachments = materializations
	batch.Digest, err = MutationBatchDigest(batch)
	if err != nil {
		return result, err
	}
	transition, err := a.ApplyPrepared(ctx, identity, runtime.options.Project.ID, binding, PreparedTransition{
		Version: PreparedTransitionVersion, Target: target, ExpectedGraphRevision: targetSnapshot.Revision(), Batch: batch,
		Staged: owner, BlobIDs: append([]string(nil), draft.AttachmentHandles...),
	})
	result.Binding = transition.Binding
	if err != nil {
		return result, err
	}
	result.Summary = entry.Summary
	return result, nil
}

func (a *Application) ReplaceSummary(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, target MutationTarget, entryID, summary string) (MutationResult, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return MutationResult{}, err
	}
	target, err = resolveMutationTarget(runtime, target)
	if err != nil {
		return MutationResult{}, err
	}
	snapshot, err := snapshotMutationTarget(ctx, runtime, target)
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
	document, err := parseEntryDocument(filepath.ToSlash(path), canonical)
	if err != nil {
		return MutationResult{}, err
	}
	return a.applyDocumentMutation(ctx, identity, runtime, binding, target, mutationID, "sdd: summarize "+entryID+" (manual)", DocumentChange{LogicalPath: filepath.ToSlash(path), Document: &document, CanonicalBytes: canonical})
}

func (a *Application) StartWIP(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, target MutationTarget, entryID, description string) (string, MutationResult, error) {
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
	target, err = resolveMutationTarget(runtime, target)
	if err != nil {
		return "", MutationResult{}, err
	}
	result, err := a.applyDocumentMutation(ctx, identity, runtime, binding, target, "wip-start-"+marker.ID, fmt.Sprintf("sdd: wip start %s (%s)", entryID, principal.Participant), DocumentChange{
		LogicalPath: filepath.ToSlash(model.WIPMarkerPath(marker.ID)), CanonicalBytes: []byte(model.FormatWIPMarker(marker)),
	})
	return marker.ID, result, err
}

func (a *Application) FinishWIP(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, target MutationTarget, markerID string) (MutationResult, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return MutationResult{}, err
	}
	target, err = resolveMutationTarget(runtime, target)
	if err != nil {
		return MutationResult{}, err
	}
	return a.applyDocumentMutation(ctx, identity, runtime, binding, target, "wip-done-"+markerID, "sdd: wip done "+markerID, DocumentChange{LogicalPath: filepath.ToSlash(model.WIPMarkerPath(markerID)), Delete: true})
}

func (a *Application) applyDocumentMutation(ctx context.Context, identity RequestIdentity, runtime *ProjectRuntime, binding SessionBinding, target MutationTarget, id, message string, change DocumentChange) (MutationResult, error) {
	snapshot, err := snapshotMutationTarget(ctx, runtime, target)
	if err != nil {
		return MutationResult{}, err
	}
	batch := MutationBatch{ID: id, Message: message, Changes: []DocumentChange{change}}
	batch.Digest, err = MutationBatchDigest(batch)
	if err != nil {
		return MutationResult{}, err
	}
	transition, err := a.ApplyPrepared(ctx, identity, runtime.options.Project.ID, binding, PreparedTransition{
		Version: PreparedTransitionVersion, Target: target, ExpectedGraphRevision: snapshot.Revision(), Batch: batch,
		Staged: SessionRef{Subject: binding.Subject, Session: binding.SessionID},
	})
	return MutationResult{Project: runtime.options.Project, Binding: transition.Binding}, err
}

func resolveMutationTarget(runtime *ProjectRuntime, requested MutationTarget) (MutationTarget, error) {
	if requested.Project == "" && requested.Branch == "" {
		return runtime.defaultMutationTarget()
	}
	if requested.Project == "" {
		requested.Project = runtime.options.Project.ID
	}
	if err := requested.Validate(runtime.options.Project.ID); err != nil {
		return MutationTarget{}, err
	}
	return requested, nil
}

func snapshotMutationTarget(ctx context.Context, runtime *ProjectRuntime, target MutationTarget) (snapshot *Snapshot, err error) {
	acquired, err := runtime.acquire(ctx, target)
	if err != nil {
		return nil, err
	}
	defer func() {
		if releaseErr := acquired.Release(); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("releasing mutation target %s after snapshot: %w", target.Branch, releaseErr))
		}
	}()
	return acquired.Graph.Current(ctx)
}

func newMutationID(prefix string) (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("sdd: generating mutation ID: %w", err)
	}
	return prefix + "-" + hex.EncodeToString(random[:]), nil
}

// draftLayer expands an abbreviated layer to its canonical form.
func draftLayer(layer string) model.Layer {
	if expanded, ok := model.LayerFromAbbrev[layer]; ok {
		return expanded
	}
	return model.Layer(layer)
}

// entryFromDraft materializes a draft's fields as a model.Entry — the one
// draft-to-entry assembly, shared by the write gate and the assemble-gate
// predicate (draftValidates). Shape problems — missing or unknown kind,
// malformed topic or index — come back as findings rather than errors, so
// both callers serve them as actionable diagnostics.
func entryFromDraft(draft EntryDraft, id string, now time.Time) (*model.Entry, []model.Finding) {
	kind := model.Kind(draft.Kind)
	entryType, err := entryTypeForKind(kind)
	if err != nil {
		return nil, []model.Finding{{Field: "kind", Value: draft.Kind, Message: err.Error()}}
	}
	var findings []model.Finding
	topics := make([]model.TopicPath, 0, len(draft.Topics))
	for _, label := range draft.Topics {
		topic, err := model.ParseTopicPath(label)
		if err != nil {
			findings = append(findings, model.Finding{Field: "topics", Value: label, Message: fmt.Sprintf("topic %q: %v", label, err)})
			continue
		}
		topics = append(topics, topic)
	}
	var index *model.FactIndex
	if draft.Index != nil {
		index, err = model.NewFactIndex(draft.Index.Title, draft.Index.Topic)
		if err != nil {
			findings = append(findings, model.Finding{Field: "index", Message: err.Error()})
		}
	}
	entry := &model.Entry{
		ID: id, Type: entryType, Kind: kind, Layer: draftLayer(draft.Layer), Intent: model.Intent(draft.Intent),
		Content: draft.Body, Participants: append([]string(nil), draft.Participants...),
		Confidence: draft.Confidence, Topics: topics, Index: index, Time: now,
		Canonical: draft.Canonical, Aliases: append([]string(nil), draft.Aliases...), Actor: draft.Actor,
		Class:       model.ProcedureClass(draft.Class),
		FocusActors: append([]string(nil), draft.FocusActors...), FocusWhen: draft.FocusWhen,
		Involvement: append([]model.Involvement(nil), draft.Involvement...),
	}
	if len(draft.ProcedureSpec) > 0 {
		if kind != model.KindProcedure {
			findings = append(findings, model.Finding{Field: "procedureSpec", Message: "a workflow declaration is only meaningful on a kind: procedure decision"})
		} else if spec, err := model.ProcedureSpecFromDocument(draft.ProcedureSpec); err != nil {
			findings = append(findings, model.Finding{Field: "procedureSpec", Message: err.Error()})
		} else {
			entry.ProcedureSpec = spec
		}
	}
	for _, ref := range draft.Refs {
		entry.Refs = append(entry.Refs, model.Ref{ID: ref.ID, Kind: model.RefKind(ref.Kind), Desc: ref.Desc})
	}
	entry.Closes = append([]string(nil), draft.Closes...)
	entry.Supersedes = append([]string(nil), draft.Supersedes...)
	return entry, findings
}

func entryTypeForKind(kind model.Kind) (model.EntryType, error) {
	// An empty kind is valid for both types at the model layer (defaults are
	// a construction concern), so it must be rejected here — deriving the
	// type from it would silently mint a kindless signal.
	if kind == "" {
		return "", fmt.Errorf("entry kind is required")
	}
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

// Identity passes the executor's own answer through unchanged. This adapter
// sits behind the port and cannot know what runs on the far side, so it
// reports rather than derives.
func (r runtimeLLMRunner) Identity() internalllm.Identity {
	id := r.executor.Identity()
	return internalllm.Identity{Provider: id.Provider, Model: id.Model, Variant: id.Variant}
}

// Run adapts the host's executor to the internal Runner, lifting the reported
// usage back into LLMMetadata so the shared call-recording path covers
// engine-mode calls exactly as it covers CLI ones; dropping it here is what
// left the whole engine flow absent from `sdd stats`.
func (r runtimeLLMRunner) Run(ctx context.Context, request internalllm.Request) (*internalllm.RunResult, error) {
	result, err := r.executor.Execute(ctx, LLMRequest{Purpose: r.purpose, SystemPrompt: request.SystemPrompt, Prompt: request.UserPrompt})
	if err != nil {
		return nil, err
	}
	return &internalllm.RunResult{
		Text: string(result.Output),
		Meta: &internalllm.LLMMetadata{
			InputTokens:       int(result.Usage.InputTokens),
			OutputTokens:      int(result.Usage.OutputTokens),
			CacheReadTokens:   int(result.Usage.CacheReadTokens),
			CacheCreateTokens: int(result.Usage.CacheCreateTokens),
		},
	}, nil
}
