package sdd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

const PreparedTransitionVersion uint32 = 1

const (
	eventMutationIntent   = "mutation_intent"
	eventMutationOutcome  = "mutation_outcome"
	eventFinalizerOutcome = "finalizer_outcome"
)

// PreparedTransition is the storage-neutral write-gate output. It contains
// only pinned v1 facts; adapters never reconstruct application intent.
type PreparedTransition struct {
	Version               uint32
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
		_ = runtime.options.StagedBlobs.Release(ctx, prepared.BlobOwner, prepared.Batch.ID)
		return TransitionResult{}, err
	}
	binding.Version = version
	apply, applyErr := runtime.options.Graph.Apply(ctx, prepared.ExpectedGraphRevision, prepared.Batch, ownedBlobReader{store: runtime.options.StagedBlobs, owner: prepared.BlobOwner})
	return a.finishTransition(ctx, runtime, binding, prepared, apply, applyErr, nil)
}

// RecoverPrepared reconciles an intent that has no durable definitive
// outcome. It is safe after process restart and retries unfinished finalizers.
func (a *Application) RecoverPrepared(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, mutationID string) (TransitionResult, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return TransitionResult{}, err
	}
	stored, err := runtime.options.Sessions.Load(ctx, binding.SessionID)
	if err != nil {
		return TransitionResult{}, err
	}
	if err := verifyBinding(stored, binding); err != nil {
		return TransitionResult{}, err
	}
	prepared, prior, finalizers, err := replayMutation(stored.Events, mutationID)
	if err != nil {
		return TransitionResult{}, err
	}
	if err := validatePreparedTransition(prepared, principal, binding, runtime.options.Project.ID); err != nil {
		return TransitionResult{}, err
	}
	binding.Version = stored.Version
	if prior.State == MutationApplied || prior.State == MutationNotApplied {
		return a.finishTransition(ctx, runtime, binding, prepared, prior, nil, finalizers)
	}
	apply, reconcileErr := runtime.options.Graph.Reconcile(ctx, prepared.Batch.ID, prepared.Batch.Digest)
	return a.finishTransition(ctx, runtime, binding, prepared, apply, reconcileErr, finalizers)
}

func (a *Application) finishTransition(ctx context.Context, runtime *ProjectRuntime, binding SessionBinding, prepared PreparedTransition, apply ApplyResult, applyErr error, prior map[string]FinalizerOutcome) (TransitionResult, error) {
	if prior == nil {
		prior = map[string]FinalizerOutcome{}
	}
	result := TransitionResult{Project: runtime.options.Project, Binding: binding, Apply: apply}
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
		for _, finalizer := range runtime.options.Finalizers {
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
	if apply.State != MutationUnknown {
		if err := runtime.options.StagedBlobs.Release(ctx, prepared.BlobOwner, prepared.Batch.ID); err != nil {
			return result, &ApplicationError{Code: ErrorRecoveryRequired, Message: "staged blob retention could not be released", ApplyState: apply.State, Revision: apply.Revision, Cause: err}
		}
	}
	if apply.State == MutationUnknown {
		return result, &ApplicationError{Code: ErrorRecoveryRequired, Message: "canonical mutation outcome is unknown", ApplyState: apply.State, Revision: apply.Revision, Cause: applyErr}
	}
	if applyErr != nil {
		return result, applyErr
	}
	return result, nil
}

func validatePreparedTransition(prepared PreparedTransition, principal Principal, binding SessionBinding, project ProjectID) error {
	if prepared.Version != PreparedTransitionVersion {
		return &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported prepared transition version", Version: prepared.Version}
	}
	if binding.Subject != principal.Subject || binding.Project != project || prepared.BlobOwner.Subject != principal.Subject || prepared.BlobOwner.Session != binding.SessionID {
		return &ApplicationError{Code: ErrorSessionOwnership, Message: "prepared transition ownership mismatch"}
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

func replayMutation(events []StoredEvent, mutationID string) (PreparedTransition, ApplyResult, map[string]FinalizerOutcome, error) {
	var prepared PreparedTransition
	apply := ApplyResult{State: MutationUnknown}
	finalizers := map[string]FinalizerOutcome{}
	for _, event := range events {
		if event.CodecVersion != SessionCodecVersion {
			return PreparedTransition{}, ApplyResult{}, nil, &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported session event codec version", Version: event.CodecVersion}
		}
		switch event.Code {
		case eventMutationIntent:
			var value mutationIntentEvent
			if err := json.Unmarshal(event.Payload, &value); err != nil {
				return PreparedTransition{}, ApplyResult{}, nil, err
			}
			if value.Prepared.Batch.ID == mutationID {
				prepared = value.Prepared
			}
		case eventMutationOutcome:
			var value mutationOutcomeEvent
			if err := json.Unmarshal(event.Payload, &value); err != nil {
				return PreparedTransition{}, ApplyResult{}, nil, err
			}
			if value.MutationID == mutationID {
				apply = value.Apply
			}
		case eventFinalizerOutcome:
			var value finalizerOutcomeEvent
			if err := json.Unmarshal(event.Payload, &value); err != nil {
				return PreparedTransition{}, ApplyResult{}, nil, err
			}
			if value.MutationID == mutationID {
				finalizers[value.Outcome.Name] = value.Outcome
			}
		}
	}
	if prepared.Batch.ID == "" {
		return PreparedTransition{}, ApplyResult{}, nil, fmt.Errorf("sdd: mutation intent %q not found", mutationID)
	}
	return prepared, apply, finalizers, nil
}

type ownedBlobReader struct {
	store StagedBlobStore
	owner BlobOwner
}

func (r ownedBlobReader) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	return r.store.Open(ctx, r.owner, id)
}
