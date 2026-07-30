package application

import (
	"context"
	"errors"
	"time"

	"github.com/networkteam/slogutils"
)

// CollectSessionsCmd asks for one reclamation pass over a project's session
// store. Retention is how long an ended session is kept; zero means remove as
// soon as it has ended.
type CollectSessionsCmd struct {
	Retention time.Duration
}

// CollectSessionsResult reports what one pass did. Nothing here is actionable —
// the pass either removed something or deliberately left it, and the skips say
// which sessions it could not read so a caller can log them.
type CollectSessionsResult struct {
	RemovedSessions []SessionID
	RemovedStaged   []SessionRef
	DrainedIntents  int
	Skipped         []SessionID
}

// CollectSessions removes the sessions that are safe to remove and drains the
// pending declarations that can never converge.
//
// The pass is lock-free and optimistic. An ended session is never written to
// again, so two processes starting at once derive the same target set and both
// simply delete; an already-deleted target is success. The target set is
// recomputed from scratch every run, so a partially removed session is finished
// by the next pass and there is nothing to reconcile after an interruption.
//
// What it protects, and the whole of it: it never removes a session that has
// not ended, one inside its retention window, one an in-flight declaration
// still claims, or one this binary cannot read — an unreadable log may belong
// to a newer version, so it is left alone rather than treated as garbage.
func (a *Application) CollectSessions(
	ctx context.Context,
	identity RequestIdentity,
	project ProjectID,
	cmd CollectSessionsCmd,
) (CollectSessionsResult, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessWrite)
	if err != nil {
		return CollectSessionsResult{}, err
	}
	return collectRuntime(ctx, runtime, cmd)
}

func collectRuntime(
	ctx context.Context,
	runtime *ProjectRuntime,
	cmd CollectSessionsCmd,
) (CollectSessionsResult, error) {
	sessions := runtime.options.Sessions
	blobs := runtime.options.StagedBlobs

	// List already skips a log this binary cannot read, which is why an
	// unreadable session is never a collection candidate.
	stored, err := sessions.List(ctx, SessionFilter{Project: runtime.options.Project.ID})
	if err != nil {
		return CollectSessionsResult{}, err
	}

	log := slogutils.FromContext(ctx)
	now := runtime.options.Now().UTC()
	result := CollectSessionsResult{}

	for _, session := range stored {
		id := session.Metadata.ID
		drained, claimed, err := drainDeclarations(ctx, runtime, session)
		result.DrainedIntents += drained
		if err != nil {
			// One unreadable payload stops that session, not the run.
			log.Warn("skipping session with unreadable declarations", "session", id, "err", err)
			result.Skipped = append(result.Skipped, id)
			continue
		}
		if claimed {
			continue
		}
		endedAt, ended := sessionEndedAt(session.Metadata)
		if !ended || now.Sub(endedAt) <= cmd.Retention {
			continue
		}
		if err := sessions.Delete(ctx, id); err != nil {
			return result, err
		}
		result.RemovedSessions = append(result.RemovedSessions, id)
		ref := SessionRef{Subject: session.Metadata.Subject, Session: id}
		if err := blobs.DeleteStaged(ctx, ref); err != nil {
			return result, err
		}
		result.RemovedStaged = append(result.RemovedStaged, ref)
	}

	orphans, err := collectOrphanStaging(ctx, sessions, blobs)
	if err != nil {
		return result, err
	}
	result.RemovedStaged = append(result.RemovedStaged, orphans...)
	return result, nil
}

// collectOrphanStaging removes staging areas whose session is gone. Absence is
// distinguished from unreadability deliberately: a session this binary cannot
// decode still exists, so its staged blobs are not orphans.
func collectOrphanStaging(
	ctx context.Context,
	sessions SessionStore,
	blobs StagedBlobStore,
) ([]SessionRef, error) {
	refs, err := blobs.StagedSessions(ctx)
	if err != nil {
		return nil, err
	}
	var removed []SessionRef
	for _, ref := range refs {
		_, err := sessions.Load(ctx, ref.Session)
		if err == nil || !errors.Is(err, ErrSessionNotFound) {
			continue
		}
		if err := blobs.DeleteStaged(ctx, ref); err != nil {
			return removed, err
		}
		removed = append(removed, ref)
	}
	return removed, nil
}

// sessionEndedAt reports when a session ended, by being concluded or explicitly
// abandoned. A terminal record as the most recent one is what ends a session,
// matching how a displaced writer is told its dialogue is over; a later
// attachment means something reopened it, so it is not ended.
func sessionEndedAt(metadata SessionMetadata) (time.Time, bool) {
	n := len(metadata.AttachmentHistory)
	if n == 0 {
		return time.Time{}, false
	}
	last := metadata.AttachmentHistory[n-1]
	if last.Cause != CauseConclude && last.Cause != CauseAbandon {
		return time.Time{}, false
	}
	if metadata.Attachment != nil && metadata.Attachment.LastActivity.After(last.EndedAt) {
		return time.Time{}, false
	}
	return last.EndedAt.UTC(), true
}

// discardUnroutable retires one declaration from before the convergence rule.
// Such an intent names no routable target and no recovery verb can supply one,
// so no future run will ever complete it: recording the terminal discard is the
// only end state it can reach, and until it has one it keeps its session alive.
func discardUnroutable(
	ctx context.Context,
	runtime *ProjectRuntime,
	session StoredSession,
	replay mutationRecoveryReplay,
	mutationID string,
	version uint64,
) (uint64, error) {
	prepared := replay.prepared
	if err := runtime.options.StagedBlobs.Release(ctx, prepared.Staged, mutationID); err != nil {
		return version, err
	}
	binding := SessionBinding{
		SessionID: session.Metadata.ID,
		Subject:   session.Metadata.Subject,
		Version:   version,
	}
	return appendRecoveryTerminal(ctx, runtime.options.Sessions, binding, recoveryTerminalEvent{
		MutationID: mutationID, Digest: prepared.Batch.Digest, Target: prepared.Target,
		OriginalSubject: session.Metadata.Subject, OriginalSession: session.Metadata.ID,
		Actor: session.Metadata.Subject, Verb: RecoveryDiscard,
		Reason: "collected: intent predates the convergence rule and can never be routed",
	})
}

// drainDeclarations sweeps one session's pending declarations, reporting how
// many were discarded and whether any still claims the session. A declaration
// that can still converge keeps its session alive; one from before the
// convergence rule that can never be routed is discarded, since nothing will
// ever complete it.
func drainDeclarations(
	ctx context.Context,
	runtime *ProjectRuntime,
	session StoredSession,
) (drained int, claimed bool, err error) {
	ids, err := mutationIDs(session.Events)
	if err != nil {
		return 0, true, err
	}
	// Each discard appends, so the version advances as the sweep goes.
	version := session.Version
	for _, id := range ids {
		replay, replayErr := replayRecovery(session.Events, id)
		if replayErr != nil {
			return drained, true, replayErr
		}
		item := recoveryItem(session, replay)
		if !item.Actionable() {
			continue
		}
		if !item.LegacyUnroutable {
			claimed = true
			continue
		}
		next, discardErr := discardUnroutable(ctx, runtime, session, replay, id, version)
		if discardErr != nil {
			return drained, true, discardErr
		}
		version = next
		drained++
	}
	return drained, claimed, nil
}
