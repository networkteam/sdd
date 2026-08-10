package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/networkteam/sdd/internal/model"
)

const (
	eventRecoveryAttempt   = "recovery_attempt"
	eventRecoveryTerminal  = "recovery_terminal"
	eventLegacyTargetBound = "legacy_target_bound"
)

// recoveryCauseGraphContention marks a terminal the engine recorded itself when
// a write lost every bounded apply retry, distinguishing it structurally from
// an operator's recovery decision (which leaves Cause empty).
const recoveryCauseGraphContention = "graph-contention"

type RecoveryState string

// State answers one question — has delivery been reached — so it carries the two
// durable conditions of the delivery contract plus the one outcome that is a
// participant's decision rather than a delivery result.
const (
	// RecoveryDelivered means the write reached its desired state: the batch
	// applied and finalization is proven. Nothing is owed.
	RecoveryDelivered RecoveryState = "delivered"
	// RecoveryPending means delivery is not proven yet. Pending is exactly the
	// actionable condition, and Reason names what is owed.
	RecoveryPending RecoveryState = "pending"
	// RecoveryAbandoned means a participant decided to stop pursuing delivery.
	// Reason names the decision.
	RecoveryAbandoned RecoveryState = "abandoned"
)

// RecoveryReason qualifies a state that does not explain itself: what delivery
// is waiting on, or which decision ended it.
type RecoveryReason string

const (
	RecoveryReasonOutcomeUnknown   RecoveryReason = "outcome-unknown"
	RecoveryReasonNotApplied       RecoveryReason = "not-applied"
	RecoveryReasonFinalizationOwed RecoveryReason = "finalization-owed"
	RecoveryReasonDiscarded        RecoveryReason = "discarded"
	RecoveryReasonAbandonedUnknown RecoveryReason = "abandoned-unknown"
)

type RecoveryItem struct {
	Session         SessionID
	MutationID      string
	Digest          string
	Target          MutationTarget
	OriginalSubject string
	State           RecoveryState
	// Reason qualifies State: what delivery waits on while pending, or which
	// decision ended it while abandoned. Empty when delivered.
	Reason RecoveryReason
	// Recovered records that recovery machinery touched this mutation — a
	// reconciliation or a verb. It is provenance, not state: a recovered write is
	// delivered exactly like one that never needed help.
	Recovered        bool
	LegacyUnroutable bool
	EntryIDs         []string
	LastEvidence     string
	// Cause is the terminal's structured cause (e.g. graph-contention for an
	// engine-recorded discard), empty for participant decisions and open items.
	Cause string
}

// Actionable reports whether this item awaits a recovery decision. It is derived
// from State rather than stored beside it, so the two cannot disagree.
func (i RecoveryItem) Actionable() bool { return i.State == RecoveryPending }

type RecoveryList struct {
	Project ProjectRef
	Items   []RecoveryItem
}

type RecoveryRequest struct {
	Session    SessionID
	MutationID string
	Verb       RecoveryVerb
	Reason     string
	Target     MutationTarget
}

type RecoveryReconcileRequest struct {
	Session    SessionID
	MutationID string
}

type RecoveryResult struct {
	Project    ProjectRef
	Item       RecoveryItem
	Transition TransitionResult
}

type recoveryAttemptEvent struct {
	MutationID      string         `json:"mutation_id"`
	Digest          string         `json:"digest"`
	Target          MutationTarget `json:"target"`
	OriginalSubject string         `json:"original_subject"`
	OriginalSession SessionID      `json:"original_session"`
	Actor           string         `json:"actor"`
	Verb            RecoveryVerb   `json:"verb"`
	Reason          string         `json:"reason,omitempty"`
	Evidence        string         `json:"evidence"`
	Reconciled      ApplyResult    `json:"reconciled"`
}

type recoveryTerminalEvent struct {
	MutationID      string         `json:"mutation_id"`
	Digest          string         `json:"digest"`
	Target          MutationTarget `json:"target"`
	OriginalSubject string         `json:"original_subject"`
	OriginalSession SessionID      `json:"original_session"`
	Actor           string         `json:"actor"`
	Verb            RecoveryVerb   `json:"verb"`
	Reason          string         `json:"reason,omitempty"`
	Cause           string         `json:"cause,omitempty"`
}

type legacyTargetBoundEvent struct {
	MutationID      string         `json:"mutation_id"`
	Target          MutationTarget `json:"target"`
	OriginalSubject string         `json:"original_subject"`
	OriginalSession SessionID      `json:"original_session"`
	Actor           string         `json:"actor"`
	Reason          string         `json:"reason"`
}

type mutationRecoveryReplay struct {
	prepared   PreparedTransition
	apply      ApplyResult
	finalizers map[string]FinalizerOutcome
	attempt    *recoveryAttemptEvent
	terminal   *recoveryTerminalEvent
	bound      *legacyTargetBoundEvent
}

// ListRecoveries is a free read projection. Closed terminal history is
// included only when requested; actionable states never perform acquisition
// or replay.
func (a *Application) ListRecoveries(ctx context.Context, identity RequestIdentity, project ProjectID, includeClosed bool) (RecoveryList, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return RecoveryList{}, err
	}
	return listRecoveriesRuntime(ctx, runtime, includeClosed)
}

func listRecoveriesRuntime(ctx context.Context, runtime *ProjectRuntime, includeClosed bool) (RecoveryList, error) {
	sessions, err := runtime.options.Sessions.List(ctx, SessionFilter{Project: runtime.options.Project.ID})
	if err != nil {
		return RecoveryList{}, err
	}
	result := RecoveryList{Project: runtime.options.Project}
	for _, stored := range sessions {
		ids, err := mutationIDs(stored.Events)
		if err != nil {
			return RecoveryList{}, err
		}
		for _, id := range ids {
			replay, err := replayRecovery(stored.Events, id)
			if err != nil {
				return RecoveryList{}, err
			}
			item := recoveryItem(stored, replay)
			if !includeClosed && !item.Actionable() {
				continue
			}
			result.Items = append(result.Items, item)
		}
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].Session != result.Items[j].Session {
			return result.Items[i].Session < result.Items[j].Session
		}
		return result.Items[i].MutationID < result.Items[j].MutationID
	})
	return result, nil
}

func renderRecoveryNotices(items []RecoveryItem) string {
	if len(items) == 0 {
		return ""
	}
	var rendered strings.Builder
	rendered.WriteString("Recovery\n\n")
	for _, item := range items {
		target := item.Target.Branch
		if item.LegacyUnroutable {
			target = "target binding required"
		}
		fmt.Fprintf(&rendered, "  a pending write awaits explicit recovery: %s · %s · %s\n", item.MutationID, item.Reason, target)
	}
	return strings.TrimRight(rendered.String(), "\n")
}

// ReconcileMutation refreshes one actionable recovery projection without
// choosing a terminal or graph-affecting verb. It exists for interactive
// clients that must present actions from current target evidence instead of
// guessing from a durable projection that may predate reconciliation.
func (a *Application) ReconcileMutation(ctx context.Context, identity RequestIdentity, project ProjectID, request RecoveryReconcileRequest) (result RecoveryResult, err error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return RecoveryResult{}, err
	}
	if request.Session == "" || strings.TrimSpace(request.MutationID) == "" {
		return RecoveryResult{}, fmt.Errorf("sdd: recovery session and mutation ID are required")
	}
	stored, err := runtime.options.Sessions.Load(ctx, request.Session)
	if err != nil {
		return RecoveryResult{}, err
	}
	replay, err := replayRecovery(stored.Events, request.MutationID)
	if err != nil {
		return RecoveryResult{}, err
	}
	if replay.terminal != nil {
		return RecoveryResult{Project: runtime.options.Project, Item: recoveryItem(stored, replay)}, &ApplicationError{Code: ErrorRecoveryRequired, Message: "mutation recovery is already terminal"}
	}
	prepared := replay.prepared
	if prepared.Version == LegacyPreparedTransitionVersion && replay.bound == nil {
		return RecoveryResult{Project: runtime.options.Project, Item: recoveryItem(stored, replay)}, &ApplicationError{Code: ErrorMigrationRequired, Message: "legacy prepared intent needs an explicitly authorized target binding or recapture", Version: prepared.Version}
	}
	if replay.bound != nil {
		prepared.Target = replay.bound.Target
		prepared.Version = PreparedTransitionVersion
	}
	if err := validatePreparedForRecovery(prepared, stored.Metadata); err != nil {
		return RecoveryResult{}, err
	}
	if runtime.options.Recovery == nil {
		return RecoveryResult{}, &ApplicationError{Code: ErrorWriteDenied, Message: "project has no recovery authorizer"}
	}
	if err := runtime.options.Recovery.AuthorizeRecovery(ctx, RecoveryAccessRequest{
		Actor: principal, Target: prepared.Target, Verb: RecoveryReconcile,
		OriginalSubject: stored.Metadata.Subject, OriginalSession: stored.Metadata.ID,
	}); err != nil {
		return RecoveryResult{}, err
	}
	binding := SessionBinding{SessionID: stored.Metadata.ID, Subject: stored.Metadata.Subject, Project: stored.Metadata.Project, Version: stored.Version}
	acquired, acquireErr := runtime.acquire(ctx, prepared.Target)
	if acquireErr != nil {
		attempt := recoveryAttemptEvent{
			MutationID: prepared.Batch.ID, Digest: prepared.Batch.Digest, Target: prepared.Target,
			OriginalSubject: stored.Metadata.Subject, OriginalSession: stored.Metadata.ID, Actor: principal.Subject,
			Verb: RecoveryReconcile, Evidence: "target acquisition failed: " + acquireErr.Error(), Reconciled: ApplyResult{State: MutationUnknown},
		}
		binding, appendErr := appendRecoveryAttempt(ctx, runtime.options.Sessions, binding, attempt)
		if appendErr != nil {
			return RecoveryResult{}, errors.Join(acquireErr, appendErr)
		}
		replay.attempt = &attempt
		return RecoveryResult{
			Project: runtime.options.Project, Item: recoveryItem(stored, replay),
			Transition: TransitionResult{Project: runtime.options.Project, Binding: binding, Apply: attempt.Reconciled},
		}, &ApplicationError{Code: ErrorRecoveryRequired, Message: "recovery target acquisition failed; abandon-unknown may acknowledge the recorded evidence", Cause: acquireErr}
	}
	defer func() {
		if releaseErr := acquired.Release(); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("releasing mutation target %s: %w", prepared.Target.Branch, releaseErr))
		}
	}()
	reconciled, reconcileErr := acquired.Graph.Reconcile(ctx, prepared.Batch.ID, prepared.Batch.Digest)
	evidence := "batch ID and digest reconciled"
	if reconcileErr != nil {
		evidence = "reconciliation failed: " + reconcileErr.Error()
		reconciled.State = MutationUnknown
	}
	attempt := recoveryAttemptEvent{
		MutationID: prepared.Batch.ID, Digest: prepared.Batch.Digest, Target: prepared.Target,
		OriginalSubject: stored.Metadata.Subject, OriginalSession: stored.Metadata.ID, Actor: principal.Subject,
		Verb: RecoveryReconcile, Evidence: evidence, Reconciled: reconciled,
	}
	binding, err = appendRecoveryAttempt(ctx, runtime.options.Sessions, binding, attempt)
	if err != nil {
		return RecoveryResult{}, err
	}
	replay.attempt = &attempt
	result = RecoveryResult{
		Project: runtime.options.Project, Item: recoveryItem(stored, replay),
		Transition: TransitionResult{Project: runtime.options.Project, Binding: binding, Apply: reconciled},
	}
	if reconcileErr != nil {
		return result, &ApplicationError{Code: ErrorRecoveryRequired, Message: "recovery reconciliation was non-definitive; abandon-unknown may acknowledge the recorded evidence", Cause: reconcileErr}
	}
	return result, nil
}

// RecoverMutation performs exactly one explicitly authorized verb. It always
// reconciles a freshly acquired concrete target before any graph-affecting or
// terminal action and never runs from startup, resume, or read surfaces.
func (a *Application) RecoverMutation(ctx context.Context, identity RequestIdentity, project ProjectID, request RecoveryRequest) (result RecoveryResult, err error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return RecoveryResult{}, err
	}
	if request.Session == "" || strings.TrimSpace(request.MutationID) == "" {
		return RecoveryResult{}, fmt.Errorf("sdd: recovery session and mutation ID are required")
	}
	stored, err := runtime.options.Sessions.Load(ctx, request.Session)
	if err != nil {
		return RecoveryResult{}, err
	}
	replay, err := replayRecovery(stored.Events, request.MutationID)
	if err != nil {
		return RecoveryResult{}, err
	}
	if replay.terminal != nil {
		return RecoveryResult{Project: runtime.options.Project, Item: recoveryItem(stored, replay)}, &ApplicationError{Code: ErrorRecoveryRequired, Message: "mutation recovery is already terminal"}
	}
	prepared := replay.prepared
	if prepared.Version == LegacyPreparedTransitionVersion {
		if replay.bound != nil {
			prepared.Target = replay.bound.Target
			prepared.Version = PreparedTransitionVersion
		} else if request.Verb == RecoveryBindTarget {
			return a.bindLegacyTarget(ctx, runtime, principal, stored, replay, request)
		} else {
			return RecoveryResult{Project: runtime.options.Project, Item: recoveryItem(stored, replay)}, &ApplicationError{Code: ErrorMigrationRequired, Message: "legacy prepared intent needs an explicitly authorized target binding or recapture", Version: prepared.Version}
		}
	}
	if request.Verb == RecoveryBindTarget {
		return RecoveryResult{}, &ApplicationError{Code: ErrorRecoveryRequired, Message: "bind-target is only valid for legacy prepared intents"}
	}
	if err := validatePreparedForRecovery(prepared, stored.Metadata); err != nil {
		return RecoveryResult{}, err
	}
	if runtime.options.Recovery == nil {
		return RecoveryResult{}, &ApplicationError{Code: ErrorWriteDenied, Message: "project has no recovery authorizer"}
	}
	access := RecoveryAccessRequest{
		Actor: principal, Target: prepared.Target, Verb: request.Verb,
		OriginalSubject: stored.Metadata.Subject, OriginalSession: stored.Metadata.ID,
	}
	if err := runtime.options.Recovery.AuthorizeRecovery(ctx, access); err != nil {
		return RecoveryResult{}, err
	}
	binding := SessionBinding{SessionID: stored.Metadata.ID, Subject: stored.Metadata.Subject, Project: stored.Metadata.Project, Version: stored.Version}
	acquired, acquireErr := runtime.acquire(ctx, prepared.Target)
	if acquireErr != nil {
		binding, appendErr := appendRecoveryAttempt(ctx, runtime.options.Sessions, binding, recoveryAttemptEvent{
			MutationID: prepared.Batch.ID, Digest: prepared.Batch.Digest, Target: prepared.Target,
			OriginalSubject: stored.Metadata.Subject, OriginalSession: stored.Metadata.ID, Actor: principal.Subject,
			Verb: request.Verb, Reason: request.Reason, Evidence: "target acquisition failed: " + acquireErr.Error(), Reconciled: ApplyResult{State: MutationUnknown},
		})
		if appendErr != nil {
			return RecoveryResult{}, errors.Join(acquireErr, appendErr)
		}
		if request.Verb != RecoveryAbandonUnknown {
			return RecoveryResult{Project: runtime.options.Project, Transition: TransitionResult{Project: runtime.options.Project, Binding: binding, Apply: ApplyResult{State: MutationUnknown}}}, &ApplicationError{Code: ErrorRecoveryRequired, Message: "recovery target acquisition failed; only abandon-unknown may terminally acknowledge this evidence", Cause: acquireErr}
		}
		return a.terminalRecovery(ctx, runtime, stored, prepared, binding, principal.Subject, request, RecoveryReasonAbandonedUnknown)
	}
	release := true
	defer func() {
		if release {
			if releaseErr := acquired.Release(); releaseErr != nil {
				err = errors.Join(err, fmt.Errorf("releasing mutation target %s: %w", prepared.Target.Branch, releaseErr))
			}
		}
	}()
	reconciled, reconcileErr := acquired.Graph.Reconcile(ctx, prepared.Batch.ID, prepared.Batch.Digest)
	evidence := "batch ID and digest reconciled"
	if reconcileErr != nil {
		evidence = "reconciliation failed: " + reconcileErr.Error()
		reconciled.State = MutationUnknown
	}
	binding, err = appendRecoveryAttempt(ctx, runtime.options.Sessions, binding, recoveryAttemptEvent{
		MutationID: prepared.Batch.ID, Digest: prepared.Batch.Digest, Target: prepared.Target,
		OriginalSubject: stored.Metadata.Subject, OriginalSession: stored.Metadata.ID, Actor: principal.Subject,
		Verb: request.Verb, Reason: request.Reason, Evidence: evidence, Reconciled: reconciled,
	})
	if err != nil {
		return RecoveryResult{}, err
	}
	switch request.Verb {
	case RecoveryApply:
		if reconciled.State != MutationNotApplied || reconcileErr != nil {
			return RecoveryResult{}, recoveryStateError(request.Verb, reconciled.State, reconcileErr)
		}
		release = false
		transition, err := a.applyOnAcquired(ctx, runtime, acquired, binding, prepared, replay.finalizers, principal.Subject, RecoveryApply)
		return RecoveryResult{Project: runtime.options.Project, Transition: transition}, err
	case RecoveryDiscard:
		if reconciled.State != MutationNotApplied || reconcileErr != nil {
			return RecoveryResult{}, recoveryStateError(request.Verb, reconciled.State, reconcileErr)
		}
		return a.terminalRecovery(ctx, runtime, stored, prepared, binding, principal.Subject, request, RecoveryReasonDiscarded)
	case RecoveryFinalizeRetry:
		if reconciled.State != MutationApplied || reconcileErr != nil {
			return RecoveryResult{}, recoveryStateError(request.Verb, reconciled.State, reconcileErr)
		}
		transition, err := a.finishTransition(ctx, runtime, acquired, binding, prepared, reconciled, nil, replay.finalizers, principal.Subject, RecoveryFinalizeRetry)
		return RecoveryResult{Project: runtime.options.Project, Transition: transition}, err
	case RecoveryAbandonUnknown:
		if reconciled.State != MutationUnknown {
			return RecoveryResult{}, recoveryStateError(request.Verb, reconciled.State, reconcileErr)
		}
		return a.terminalRecovery(ctx, runtime, stored, prepared, binding, principal.Subject, request, RecoveryReasonAbandonedUnknown)
	default:
		return RecoveryResult{}, fmt.Errorf("sdd: unknown recovery verb %q", request.Verb)
	}
}

func (a *Application) terminalRecovery(ctx context.Context, runtime *ProjectRuntime, stored StoredSession, prepared PreparedTransition, binding SessionBinding, actor string, request RecoveryRequest, reason RecoveryReason) (RecoveryResult, error) {
	next, err := releaseAndRecordTerminal(ctx, runtime, binding, prepared, recoveryTerminalEvent{
		MutationID: prepared.Batch.ID, Digest: prepared.Batch.Digest, Target: prepared.Target,
		OriginalSubject: stored.Metadata.Subject, OriginalSession: stored.Metadata.ID,
		Actor: actor, Verb: request.Verb, Reason: request.Reason,
	})
	if err != nil {
		return RecoveryResult{}, err
	}
	binding.Version = next
	item := RecoveryItem{
		Session: stored.Metadata.ID, MutationID: prepared.Batch.ID, Digest: prepared.Batch.Digest, Target: prepared.Target,
		OriginalSubject: stored.Metadata.Subject, State: RecoveryAbandoned, Reason: reason, Recovered: true, EntryIDs: mutationEntryIDs(prepared.Batch),
	}
	return RecoveryResult{Project: runtime.options.Project, Item: item, Transition: TransitionResult{Project: runtime.options.Project, Binding: binding}}, nil
}

func (a *Application) bindLegacyTarget(ctx context.Context, runtime *ProjectRuntime, principal Principal, stored StoredSession, replay mutationRecoveryReplay, request RecoveryRequest) (RecoveryResult, error) {
	prepared := replay.prepared
	if err := request.Target.Validate(runtime.options.Project.ID); err != nil {
		return RecoveryResult{}, err
	}
	for _, change := range prepared.Batch.Changes {
		if !change.Delete && !strings.HasPrefix(filepathSlash(change.LogicalPath), "wip/") && change.Document == nil {
			return RecoveryResult{}, &ApplicationError{Code: ErrorMigrationRequired, Message: "legacy intent lacks structured facts required for safe target binding; recapture explicitly", Version: prepared.Version}
		}
	}
	if runtime.options.Recovery == nil {
		return RecoveryResult{}, &ApplicationError{Code: ErrorWriteDenied, Message: "project has no recovery authorizer"}
	}
	if err := runtime.options.Recovery.AuthorizeRecovery(ctx, RecoveryAccessRequest{
		Actor: principal, Target: request.Target, Verb: RecoveryBindTarget,
		OriginalSubject: stored.Metadata.Subject, OriginalSession: stored.Metadata.ID,
	}); err != nil {
		return RecoveryResult{}, err
	}
	event, err := storedEvent(eventLegacyTargetBound, legacyTargetBoundEvent{
		MutationID: prepared.Batch.ID, Target: request.Target, OriginalSubject: stored.Metadata.Subject,
		OriginalSession: stored.Metadata.ID, Actor: principal.Subject, Reason: request.Reason,
	})
	if err != nil {
		return RecoveryResult{}, err
	}
	version, err := runtime.options.Sessions.Append(ctx, stored.Metadata.ID, stored.Version, SessionAppend{Events: []StoredEvent{event}})
	if err != nil {
		return RecoveryResult{}, err
	}
	// Binding supplies the target a pending legacy intent was missing; delivery is
	// still owed, so the item stays pending with its recorded reason intact.
	item := recoveryItem(stored, replay)
	item.Target = request.Target
	item.LegacyUnroutable = false
	item.Recovered = true
	return RecoveryResult{Project: runtime.options.Project, Item: item, Transition: TransitionResult{Project: runtime.options.Project, Binding: SessionBinding{SessionID: stored.Metadata.ID, Subject: stored.Metadata.Subject, Project: stored.Metadata.Project, Version: version}}}, nil
}

func validatePreparedForRecovery(prepared PreparedTransition, metadata SessionMetadata) error {
	if prepared.Version != PreparedTransitionVersion {
		return &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported prepared transition version", Version: prepared.Version}
	}
	if err := prepared.Target.Validate(metadata.Project); err != nil {
		return err
	}
	if prepared.Staged.Subject != metadata.Subject || prepared.Staged.Session != metadata.ID {
		return &ApplicationError{Code: ErrorSessionOwnership, Message: "prepared transition provenance mismatch"}
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

func appendRecoveryAttempt(ctx context.Context, sessions SessionStore, binding SessionBinding, value recoveryAttemptEvent) (SessionBinding, error) {
	event, err := storedEvent(eventRecoveryAttempt, value)
	if err != nil {
		return binding, err
	}
	version, err := sessions.Append(ctx, binding.SessionID, binding.Version, SessionAppend{Events: []StoredEvent{event}})
	if err != nil {
		return binding, err
	}
	binding.Version = version
	return binding, nil
}

func appendRecoveryTerminal(ctx context.Context, sessions SessionStore, binding SessionBinding, value recoveryTerminalEvent) (uint64, error) {
	event, err := storedEvent(eventRecoveryTerminal, value)
	if err != nil {
		return binding.Version, err
	}
	return sessions.Append(ctx, binding.SessionID, binding.Version, SessionAppend{Events: []StoredEvent{event}})
}

// releaseAndRecordTerminal is the shared teardown for a finished intent:
// release its retained blobs, then append the terminal record and return the
// advanced binding version. Used by operator recovery decisions and the
// engine's own contention discard alike.
func releaseAndRecordTerminal(ctx context.Context, runtime *ProjectRuntime, binding SessionBinding, prepared PreparedTransition, event recoveryTerminalEvent) (uint64, error) {
	if err := runtime.options.StagedBlobs.Release(ctx, prepared.Staged, prepared.Batch.ID); err != nil {
		return binding.Version, &ApplicationError{Code: ErrorRecoveryRequired, Message: "recovery could not release retained blobs", Cause: err}
	}
	return appendRecoveryTerminal(ctx, runtime.options.Sessions, binding, event)
}

func recoveryStateError(verb RecoveryVerb, state ApplyState, cause error) error {
	message := fmt.Sprintf("recovery verb %s is forbidden after reconciliation state %s", verb, state)
	return &ApplicationError{Code: ErrorRecoveryRequired, Message: message, ApplyState: state, Cause: cause}
}

func mutationIDs(events []StoredEvent) ([]string, error) {
	seen := map[string]bool{}
	var result []string
	for _, event := range events {
		if event.Code != eventMutationIntent {
			continue
		}
		var value mutationIntentEvent
		if err := json.Unmarshal(event.Payload, &value); err != nil {
			return nil, err
		}
		if value.Prepared.Batch.ID != "" && !seen[value.Prepared.Batch.ID] {
			seen[value.Prepared.Batch.ID] = true
			result = append(result, value.Prepared.Batch.ID)
		}
	}
	return result, nil
}

func replayRecovery(events []StoredEvent, mutationID string) (mutationRecoveryReplay, error) {
	replay := mutationRecoveryReplay{apply: ApplyResult{State: MutationUnknown}, finalizers: map[string]FinalizerOutcome{}}
	for _, event := range events {
		if !SupportedSessionCodecVersion(event.CodecVersion) {
			return mutationRecoveryReplay{}, &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported session event codec version", Version: event.CodecVersion}
		}
		switch event.Code {
		case eventMutationIntent:
			var value mutationIntentEvent
			if err := json.Unmarshal(event.Payload, &value); err != nil {
				return mutationRecoveryReplay{}, err
			}
			if value.Prepared.Batch.ID == mutationID {
				replay.prepared = value.Prepared
			}
		case eventMutationOutcome:
			var value mutationOutcomeEvent
			if err := json.Unmarshal(event.Payload, &value); err != nil {
				return mutationRecoveryReplay{}, err
			}
			if value.MutationID == mutationID {
				replay.apply = value.Apply
			}
		case eventFinalizerOutcome:
			var value finalizerOutcomeEvent
			if err := json.Unmarshal(event.Payload, &value); err != nil {
				return mutationRecoveryReplay{}, err
			}
			if value.MutationID == mutationID {
				replay.finalizers[value.Outcome.Name] = value.Outcome
			}
		case eventRecoveryAttempt:
			var value recoveryAttemptEvent
			if err := json.Unmarshal(event.Payload, &value); err != nil {
				return mutationRecoveryReplay{}, err
			}
			if value.MutationID == mutationID {
				replay.attempt = &value
			}
		case eventRecoveryTerminal:
			var value recoveryTerminalEvent
			if err := json.Unmarshal(event.Payload, &value); err != nil {
				return mutationRecoveryReplay{}, err
			}
			if value.MutationID == mutationID {
				replay.terminal = &value
			}
		case eventLegacyTargetBound:
			var value legacyTargetBoundEvent
			if err := json.Unmarshal(event.Payload, &value); err != nil {
				return mutationRecoveryReplay{}, err
			}
			if value.MutationID == mutationID {
				replay.bound = &value
			}
		}
	}
	if replay.prepared.Batch.ID == "" {
		return mutationRecoveryReplay{}, fmt.Errorf("sdd: mutation intent %q not found", mutationID)
	}
	return replay, nil
}

func recoveryItem(stored StoredSession, replay mutationRecoveryReplay) RecoveryItem {
	item := RecoveryItem{
		Session: stored.Metadata.ID, MutationID: replay.prepared.Batch.ID, Digest: replay.prepared.Batch.Digest,
		Target: replay.prepared.Target, OriginalSubject: stored.Metadata.Subject, EntryIDs: mutationEntryIDs(replay.prepared.Batch),
	}
	if replay.bound != nil {
		item.Target = replay.bound.Target
	}
	if replay.attempt != nil {
		item.LastEvidence = replay.attempt.Evidence
		item.Recovered = true
	}
	// A terminal is written by the ordinary write path too, so its presence says
	// nothing about provenance — only a recorded attempt does.
	if replay.terminal != nil {
		item.Cause = replay.terminal.Cause
		switch replay.terminal.Verb {
		case RecoveryDiscard:
			item.State, item.Reason = RecoveryAbandoned, RecoveryReasonDiscarded
		case RecoveryAbandonUnknown:
			item.State, item.Reason = RecoveryAbandoned, RecoveryReasonAbandonedUnknown
		default:
			item.State = RecoveryDelivered
		}
		return item
	}
	// Delivery derives from the recorded outcome, never from a terminal's
	// absence: the v1 writer recorded an applied, finalized mutation and never
	// wrote a terminal event, so terminal absence alone is not open work.
	switch replay.recordedApplyState() {
	case MutationApplied:
		if replay.finalizationOwed() {
			item.State, item.Reason = RecoveryPending, RecoveryReasonFinalizationOwed
		} else {
			item.State = RecoveryDelivered
		}
	case MutationNotApplied:
		item.State, item.Reason = RecoveryPending, RecoveryReasonNotApplied
	default:
		item.State, item.Reason = RecoveryPending, RecoveryReasonOutcomeUnknown
	}
	// An unbound legacy intent is unroutable only while a verb is still owed;
	// a write that already landed needs no target to act on.
	if item.Actionable() && replay.prepared.Version == LegacyPreparedTransitionVersion && replay.bound == nil {
		item.LegacyUnroutable = true
	}
	return item
}

// recordedApplyState is the mutation's outcome as the store records it: the
// canonical apply outcome when it is definitive, otherwise the latest recovery
// attempt's reconciliation. Only the two definitive states short-circuit, so an
// absent or unrecognized canonical state still consults the attempt.
func (r mutationRecoveryReplay) recordedApplyState() ApplyState {
	if r.apply.State == MutationApplied || r.apply.State == MutationNotApplied {
		return r.apply.State
	}
	if r.attempt != nil {
		return r.attempt.Reconciled.State
	}
	return MutationUnknown
}

// finalizationOwed reports whether finalization is still owed. Delivery needs
// positive proof — a recorded outcome that succeeded. No recorded outcome at all
// means no finalizer ever reported, so the write landed with its commit still
// owed, and a recorded failure is independently retryable work; both keep an
// applied mutation actionable. A target configuring no finalizers never reaches
// here, because an applied mutation records its terminal regardless of count.
func (r mutationRecoveryReplay) finalizationOwed() bool {
	if len(r.finalizers) == 0 {
		return true
	}
	for _, outcome := range r.finalizers {
		if !outcome.Succeeded {
			return true
		}
	}
	return false
}

func mutationEntryIDs(batch MutationBatch) []string {
	var result []string
	for _, change := range batch.Changes {
		if change.Document == nil {
			continue
		}
		if id, err := entryIDFromDocument(*change.Document); err == nil {
			result = append(result, id)
		}
	}
	return result
}

func entryIDFromDocument(document EntryDocument) (string, error) {
	return model.RelPathToID(document.LogicalPath)
}
