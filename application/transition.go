package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	LegacyPreparedTransitionVersion uint32 = 1
	PreparedTransitionVersion       uint32 = 2
)

const (
	eventMutationIntent   = "mutation_intent"
	eventMutationOutcome  = "mutation_outcome"
	eventFinalizerOutcome = "finalizer_outcome"
)

// maxApplyAttempts bounds the read-fresh → revalidate → apply loop: the graph
// adapter's lock is per-call, so a concurrent process can move the revision
// between our fresh read and the CAS apply.
const maxApplyAttempts = 3

// PreparedTransition is the storage-neutral write-gate output. It contains
// only pinned v1 facts; adapters never reconstruct application intent.
type PreparedTransition struct {
	Version uint32
	Target  MutationTarget
	// ExpectedGraphRevision is prepare-time provenance only. The apply CAS
	// operand is the freshly revalidated revision (see applyOnAcquired), so a
	// concurrent unrelated append merges cleanly instead of failing the pin.
	ExpectedGraphRevision string
	Batch                 MutationBatch
	BlobOwner             BlobOwner
	BlobIDs               []string
}

type FinalizerOutcome struct {
	Name      string
	Succeeded bool
	Message   string
}

type TransitionResult struct {
	Project    ProjectRef
	Binding    SessionBinding
	Apply      ApplyResult
	Finalizers []FinalizerOutcome
}

type mutationIntentEvent struct {
	Prepared PreparedTransition `json:"prepared"`
}

type mutationOutcomeEvent struct {
	MutationID string      `json:"mutation_id"`
	Digest     string      `json:"digest"`
	Apply      ApplyResult `json:"apply"`
}

type finalizerOutcomeEvent struct {
	MutationID string           `json:"mutation_id"`
	Outcome    FinalizerOutcome `json:"outcome"`
}

// ApplyPrepared durably records intent before canonical apply and outcome
// afterward. Unknown apply outcomes retain staged blobs for reconciliation;
// definitive outcomes release them after all applied finalizers succeed.
func (a *Application) ApplyPrepared(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, prepared PreparedTransition) (TransitionResult, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return TransitionResult{}, err
	}
	if err := validatePreparedTransition(prepared, principal, binding, runtime.options.Project.ID); err != nil {
		return TransitionResult{}, err
	}
	stored, err := runtime.options.Sessions.Load(ctx, binding.SessionID)
	if err != nil {
		return TransitionResult{}, err
	}
	if err := verifyBinding(stored, binding); err != nil {
		return TransitionResult{}, err
	}
	intent, err := storedEvent(eventMutationIntent, mutationIntentEvent{Prepared: prepared})
	if err != nil {
		return TransitionResult{}, err
	}
	if err := runtime.options.StagedBlobs.Retain(ctx, prepared.BlobOwner, prepared.Batch.ID, prepared.BlobIDs); err != nil {
		return TransitionResult{}, err
	}
	version, err := runtime.options.Sessions.Append(ctx, binding.SessionID, stored.Version, SessionAppend{Events: []StoredEvent{intent}})
	if err != nil {
		if releaseErr := runtime.options.StagedBlobs.Release(ctx, prepared.BlobOwner, prepared.Batch.ID); releaseErr != nil {
			return TransitionResult{}, errors.Join(err, fmt.Errorf("releasing staged blob retention after intent append failed: %w", releaseErr))
		}
		return TransitionResult{}, err
	}
	binding.Version = version
	result := TransitionResult{Project: runtime.options.Project, Binding: binding, Apply: ApplyResult{State: MutationUnknown}}
	acquired, err := runtime.acquire(ctx, prepared.Target)
	if err != nil {
		return result, &ApplicationError{Code: ErrorRecoveryRequired, Message: "mutation target could not be acquired after intent persistence", Cause: err}
	}
	return a.applyOnAcquired(ctx, runtime, acquired, binding, prepared, nil, principal.Subject, RecoveryApply)
}

func (a *Application) applyOnAcquired(ctx context.Context, runtime *ProjectRuntime, acquired *AcquiredTarget, binding SessionBinding, prepared PreparedTransition, prior map[string]FinalizerOutcome, actor string, terminalVerb RecoveryVerb) (result TransitionResult, err error) {
	defer func() {
		if releaseErr := acquired.Release(); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("releasing mutation target %s: %w", prepared.Target.Branch, releaseErr))
		}
	}()
	var apply ApplyResult
	var applyErr error
	for attempt := 1; ; attempt++ {
		snapshot, readErr := acquired.Graph.Current(ctx)
		if readErr != nil {
			return TransitionResult{Project: runtime.options.Project, Binding: binding, Apply: ApplyResult{State: MutationUnknown}}, &ApplicationError{Code: ErrorRecoveryRequired, Message: "reading mutation target before apply failed", Cause: readErr}
		}
		if revalidateErr := revalidatePreparedTransition(ctx, snapshot, prepared); revalidateErr != nil {
			return TransitionResult{Project: runtime.options.Project, Binding: binding, Apply: ApplyResult{State: MutationNotApplied, Revision: snapshot.Revision()}}, revalidateErr
		}
		apply, applyErr = acquired.Graph.Apply(ctx, snapshot.Revision(), prepared.Batch, ownedBlobReader{store: runtime.options.StagedBlobs, owner: prepared.BlobOwner})
		if !isGraphConflict(applyErr) || attempt >= maxApplyAttempts {
			break
		}
	}
	return a.finishTransition(ctx, runtime, acquired, binding, prepared, apply, applyErr, prior, actor, terminalVerb)
}

func (a *Application) finishTransition(ctx context.Context, runtime *ProjectRuntime, acquired *AcquiredTarget, binding SessionBinding, prepared PreparedTransition, apply ApplyResult, applyErr error, prior map[string]FinalizerOutcome, actor string, terminalVerb RecoveryVerb) (TransitionResult, error) {
	if prior == nil {
		prior = map[string]FinalizerOutcome{}
	}
	result := TransitionResult{Project: runtime.options.Project, Binding: binding, Apply: apply}
	if isGraphConflict(applyErr) {
		return a.discardContendedTransition(ctx, runtime, result, prepared, actor)
	}
	if apply.State != MutationUnknown {
		outcome, err := storedEvent(eventMutationOutcome, mutationOutcomeEvent{MutationID: prepared.Batch.ID, Digest: prepared.Batch.Digest, Apply: apply})
		if err != nil {
			return result, err
		}
		next, err := runtime.options.Sessions.Append(ctx, binding.SessionID, result.Binding.Version, SessionAppend{Events: []StoredEvent{outcome}})
		if err != nil {
			return result, &ApplicationError{Code: ErrorRecoveryRequired, Message: "canonical mutation outcome was not persisted", ApplyState: apply.State, Revision: apply.Revision, Cause: err}
		}
		result.Binding.Version = next
	}
	if apply.State == MutationApplied {
		for _, finalizer := range acquired.Finalizers {
			if previous, ok := prior[finalizer.Name()]; ok && previous.Succeeded {
				result.Finalizers = append(result.Finalizers, previous)
				continue
			}
			outcome := FinalizerOutcome{Name: finalizer.Name(), Succeeded: true}
			if err := finalizer.Finalize(ctx, AppliedMutation{Project: runtime.options.Project.ID, BatchID: prepared.Batch.ID, Revision: apply.Revision, Batch: prepared.Batch}); err != nil {
				outcome.Succeeded = false
				outcome.Message = err.Error()
			}
			event, err := storedEvent(eventFinalizerOutcome, finalizerOutcomeEvent{MutationID: prepared.Batch.ID, Outcome: outcome})
			if err != nil {
				return result, err
			}
			next, err := runtime.options.Sessions.Append(ctx, binding.SessionID, result.Binding.Version, SessionAppend{Events: []StoredEvent{event}})
			if err != nil {
				return result, &ApplicationError{Code: ErrorRecoveryRequired, Message: "finalizer outcome was not persisted", Cause: err}
			}
			result.Binding.Version = next
			result.Finalizers = append(result.Finalizers, outcome)
			if !outcome.Succeeded {
				return result, &ApplicationError{Code: ErrorRecoveryRequired, Message: "mutation finalizer failed", ApplyState: apply.State, Revision: apply.Revision}
			}
		}
	}
	if apply.State == MutationApplied {
		if err := runtime.options.StagedBlobs.Release(ctx, prepared.BlobOwner, prepared.Batch.ID); err != nil {
			return result, &ApplicationError{Code: ErrorRecoveryRequired, Message: "staged blob retention could not be released", ApplyState: apply.State, Revision: apply.Revision, Cause: err}
		}
		next, err := appendRecoveryTerminal(ctx, runtime.options.Sessions, result.Binding, recoveryTerminalEvent{
			MutationID: prepared.Batch.ID, Digest: prepared.Batch.Digest, Target: prepared.Target,
			OriginalSubject: prepared.BlobOwner.Subject, OriginalSession: prepared.BlobOwner.Session,
			Actor: actor, Verb: terminalVerb,
		})
		if err != nil {
			return result, &ApplicationError{Code: ErrorRecoveryRequired, Message: "recovered mutation outcome was not persisted", ApplyState: apply.State, Revision: apply.Revision, Cause: err}
		}
		result.Binding.Version = next
	}
	if apply.State == MutationUnknown {
		return result, &ApplicationError{Code: ErrorRecoveryRequired, Message: "canonical mutation outcome is unknown", ApplyState: apply.State, Revision: apply.Revision, Cause: applyErr}
	}
	if apply.State == MutationNotApplied && applyErr == nil {
		// A definitive not-applied with no error is a genuine awaiting-decision
		// outcome (deduplicated or reconciled), never a revision conflict —
		// conflicts short-circuit above and are re-tried, then closed.
		return result, &ApplicationError{Code: ErrorRecoveryRequired, Message: "canonical mutation was not applied and awaits an explicit recovery decision", ApplyState: apply.State, Revision: apply.Revision}
	}
	if applyErr != nil {
		return result, applyErr
	}
	return result, nil
}

// isGraphConflict reports whether err is the adapter's typed revision-conflict
// signal — the graph moved under a CAS apply.
func isGraphConflict(err error) bool {
	var appErr *ApplicationError
	return errors.As(err, &appErr) && appErr.Code == ErrorGraphConflict
}

// discardContendedTransition closes a durable intent whose bounded apply
// retries were all lost to concurrent writers. A revision conflict fails the
// CAS before any file write, so the intent carries no partial graph state:
// tear it down as a discard so it never surfaces as a pending recovery, and
// return the typed conflict inviting a plain re-try.
func (a *Application) discardContendedTransition(ctx context.Context, runtime *ProjectRuntime, result TransitionResult, prepared PreparedTransition, actor string) (TransitionResult, error) {
	next, err := releaseAndRecordTerminal(ctx, runtime, result.Binding, prepared, recoveryTerminalEvent{
		MutationID: prepared.Batch.ID, Digest: prepared.Batch.Digest, Target: prepared.Target,
		OriginalSubject: prepared.BlobOwner.Subject, OriginalSession: prepared.BlobOwner.Session,
		Actor: actor, Verb: RecoveryDiscard, Cause: recoveryCauseGraphContention,
		Reason: "graph contention: bounded apply retries exhausted",
	})
	if err != nil {
		return result, err
	}
	result.Binding.Version = next
	return result, &ApplicationError{
		Code:     ErrorGraphConflict,
		Message:  "the graph is being written concurrently and this change lost every apply retry; re-try the write",
		Revision: result.Apply.Revision,
	}
}

func validatePreparedTransition(prepared PreparedTransition, principal Principal, binding SessionBinding, project ProjectID) error {
	if prepared.Version != PreparedTransitionVersion {
		return &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported prepared transition version", Version: prepared.Version}
	}
	if binding.Subject != principal.Subject || binding.Project != project || prepared.BlobOwner.Subject != principal.Subject || prepared.BlobOwner.Session != binding.SessionID {
		return &ApplicationError{Code: ErrorSessionOwnership, Message: "prepared transition ownership mismatch"}
	}
	if err := prepared.Target.Validate(project); err != nil {
		return err
	}
	digest, err := MutationBatchDigest(prepared.Batch)
	if err != nil {
		return err
	}
	if prepared.Batch.ID == "" || prepared.Batch.Digest == "" || prepared.Batch.Digest != digest {
		return &ApplicationError{Code: ErrorRecoveryRequired, Message: "prepared mutation digest mismatch"}
	}
	return nil
}

func storedEvent(code string, value any) (StoredEvent, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return StoredEvent{}, err
	}
	return StoredEvent{CodecVersion: SessionCodecVersion, Code: code, Payload: payload}, nil
}

type ownedBlobReader struct {
	store StagedBlobStore
	owner BlobOwner
}

func (r ownedBlobReader) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	return r.store.Open(ctx, r.owner, id)
}
