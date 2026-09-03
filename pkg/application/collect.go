package application

import (
	"context"
	"errors"
	"time"

	"github.com/networkteam/slogutils"
)

// CollectSessionsCmd asks for one reclamation pass over the composition's
// session store. Retention is how long an ended session is kept; zero means
// remove as soon as it has ended. Limit bounds the page one pass processes
// (zero: everything) and After is the cursor a previous pass returned as Next,
// so the sweep converges over repeated calls instead of loading every session.
type CollectSessionsCmd struct {
	Retention time.Duration
	Limit     int
	After     SessionID
}

// CollectSessionsResult reports what one pass did. Nothing here is actionable —
// the pass either removed something or deliberately left it, and the skips say
// which sessions it could not read so a caller can log them. Next is where the
// pass stopped: pass it back as After, and stop when it is empty.
type CollectSessionsResult struct {
	RemovedSessions []SessionID
	RemovedStaged   []SessionRef
	DrainedIntents  int
	Skipped         []SessionID
	Next            SessionID
}

// CollectSessions removes the sessions that are safe to remove and drains the
// pending declarations that can never converge. It is an operator act on the
// composition's stores, without identity or project (d-cpt-yjc); who may
// trigger it is the composition's to gate.
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
//
// One pass walks one page of each store from the cursor; a session the pass
// keeps does not starve later pages because the cursor moves past it. The two
// enumerations share the cursor, so Next is the earlier of where they stopped:
// the later one re-enumerates a few areas next pass, which is idempotent.
func (a *Application) CollectSessions(ctx context.Context, cmd CollectSessionsCmd) (CollectSessionsResult, error) {
	sessions := a.sessions
	blobs := a.blobs

	// List already skips a log this binary cannot read, which is why an
	// unreadable session is never a collection candidate.
	endedBefore := a.now().UTC().Add(-cmd.Retention)
	page, err := sessions.List(ctx, SessionFilter{EndedBefore: &endedBefore, After: cmd.After, Limit: cmd.Limit})
	if err != nil {
		return CollectSessionsResult{}, err
	}

	log := slogutils.FromContext(ctx)
	result := CollectSessionsResult{}

	for _, session := range page.Sessions {
		id := session.Metadata.ID
		drained, claimed, err := a.drainDeclarations(ctx, session)
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
		// The filter already selected on the ending; a store that ignores it
		// still never loses a live or in-window session here.
		if end := session.Metadata.Ended; end == nil || !end.EndedAt.Before(endedBefore) {
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

	orphans, next, err := collectOrphanStaging(ctx, sessions, blobs, cmd)
	if err != nil {
		return result, err
	}
	result.RemovedStaged = append(result.RemovedStaged, orphans...)
	result.Next = earlierCursor(page.Next, next.Session)
	return result, nil
}

// earlierCursor merges the two enumerations' stopping points: an exhausted one
// (empty) defers to the other, otherwise the earlier wins so nothing is skipped.
func earlierCursor(a, b SessionID) SessionID {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	case a < b:
		return a
	default:
		return b
	}
}

// collectOrphanStaging removes staging areas whose session is gone, one page
// from the cursor. Absence is distinguished from unreadability deliberately: a
// session this binary cannot decode still exists, so its staged blobs are not
// orphans.
func collectOrphanStaging(
	ctx context.Context,
	sessions SessionStore,
	blobs StagedBlobStore,
	cmd CollectSessionsCmd,
) ([]SessionRef, SessionRef, error) {
	page, err := blobs.StagedSessions(ctx, SessionRef{Session: cmd.After}, cmd.Limit)
	if err != nil {
		return nil, SessionRef{}, err
	}
	var removed []SessionRef
	for _, ref := range page.Sessions {
		_, err := sessions.Load(ctx, ref.Session)
		if err == nil || !errors.Is(err, ErrSessionNotFound) {
			continue
		}
		if err := blobs.DeleteStaged(ctx, ref); err != nil {
			return removed, SessionRef{}, err
		}
		removed = append(removed, ref)
	}
	return removed, page.Next, nil
}

// discardUnroutable retires one declaration from before the convergence rule.
// Such an intent names no routable target and no recovery verb can supply one,
// so no future run will ever complete it: recording the terminal discard is the
// only end state it can reach, and until it has one it keeps its session alive.
func (a *Application) discardUnroutable(
	ctx context.Context,
	session StoredSession,
	replay mutationRecoveryReplay,
	mutationID string,
	version uint64,
) (uint64, error) {
	prepared := replay.prepared
	if err := a.blobs.Release(ctx, prepared.Staged, mutationID); err != nil {
		return version, err
	}
	binding := SessionBinding{
		SessionID: session.Metadata.ID,
		Subject:   session.Metadata.Subject,
		Version:   version,
	}
	return appendRecoveryTerminal(ctx, a.sessions, binding, recoveryTerminalEvent{
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
func (a *Application) drainDeclarations(
	ctx context.Context,
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
		next, discardErr := a.discardUnroutable(ctx, session, replay, id, version)
		if discardErr != nil {
			return drained, true, discardErr
		}
		version = next
		drained++
	}
	return drained, claimed, nil
}
