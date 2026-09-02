package application

import (
	"fmt"
	"strings"
	"time"
)

// attachmentTimeFormat renders an attachment's last activity or ending in a
// stable, unambiguous form for the interpreted-conflict and consent messages.
const attachmentTimeFormat = time.RFC3339

// reorientSuffix is the shared next-step guidance appended to displacement and
// surfaced-conflict messages.
const reorientSuffix = "reorient with resume_session or start fresh"

// NewSessionNote is the single statement of the way on from a dialogue that has
// ended: concluding is terminal, so continuing means a new session under a new
// handle rather than a revival of the spent one (d-tac-k4q). It is composed into
// the conclude serve and into every refusal an ended session answers with, so
// each surface names the same one path that works.
const NewSessionNote = "This session is finished — continuing means opening a new session with start_session, which returns a new handle to carry in place of this one. Its log stays readable for inspection until its retention window expires."

const SessionCodecVersion uint32 = 1

// FirstSessionCodecVersion is the oldest persisted session codec this binary
// still reads.
const FirstSessionCodecVersion uint32 = 1

// SupportedSessionCodecVersion reports whether a persisted codec version is one
// this binary can read. Read-compatibility with every shape sdd has written is
// permanent (d-cpt-i2x), so the whole range through the current version is
// accepted and superseded shapes are converted at decode; only a version this
// binary predates is a migration error.
func SupportedSessionCodecVersion(version uint32) bool {
	return version >= FirstSessionCodecVersion && version <= SessionCodecVersion
}

// SessionRecencyWindow is the single threshold separating an active session
// from an idle one in listings — a hint derived from the last-activity stamp,
// never a gate. Erring long is cheap, so it is generous.
const SessionRecencyWindow = 15 * time.Minute

// ChooserKind classifies who answers a pending chooser, mirrored from the
// engine for the served workflow response.
type ChooserKind string

// SessionBinding is a loaded session's write token: identity plus the version
// it last observed. Append CAS on the version is the sole integrity mechanism;
// possession of the handle is the whole authorization (d-cpt-aen).
type SessionBinding struct {
	SessionID SessionID
	Subject   string
	Project   ProjectID
	Version   uint64
}

// attachmentActive reports whether a session reads active: stamped within the
// recency window on either side of now. A stale stamp reads idle, and so does
// a far-future stamp from clock skew — activity outside [-window, +window) is
// not "now".
func attachmentActive(att *Attachment, now time.Time) bool {
	if att == nil {
		return false
	}
	delta := now.Sub(att.LastActivity)
	return delta >= -SessionRecencyWindow && delta < SessionRecencyWindow
}

func sessionBindingFrom(stored StoredSession) SessionBinding {
	return SessionBinding{SessionID: stored.Metadata.ID, Subject: stored.Metadata.Subject, Project: stored.Metadata.Project, Version: stored.Version}
}

func validateStoredSession(stored StoredSession) error {
	for _, event := range stored.Events {
		if !SupportedSessionCodecVersion(event.CodecVersion) {
			return &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported session event codec version", Version: event.CodecVersion}
		}
	}
	return nil
}

// verifyBinding enforces subject/project immutability, refuses a dialogue that
// has ended, and applies the version CAS. A benign same-writer version race is
// ErrorSessionConflict; a genuine identity/project violation is
// ErrorSessionOwnership.
func verifyBinding(stored StoredSession, binding SessionBinding) error {
	if err := verifyOwnership(stored, binding); err != nil {
		return err
	}
	if stored.Version != binding.Version {
		return &ApplicationError{Code: ErrorSessionConflict, Message: "session binding is stale"}
	}
	return nil
}

// verifyOwnership checks immutable identity and that the dialogue has not
// ended, without the version CAS. It is the shared front half of verifyBinding
// and the resync retry.
func verifyOwnership(stored StoredSession, binding SessionBinding) error {
	if err := validateStoredSession(stored); err != nil {
		return err
	}
	m := stored.Metadata
	if m.ID != binding.SessionID || m.Subject != binding.Subject || m.Project != binding.Project {
		return &ApplicationError{Code: ErrorSessionOwnership, Message: "session identity and project are immutable"}
	}
	if end := m.Ended; end != nil {
		return endedSessionError(m, *end)
	}
	return nil
}

// endedMessage tells a writer whose dialogue is over how it ended, naming
// who/when/why for a teardown.
func endedMessage(m SessionMetadata, end SessionEnd) string {
	when := end.EndedAt.Format(attachmentTimeFormat)
	if end.Act == SessionConcluded {
		return fmt.Sprintf("this session was concluded at %s. %s", when, NewSessionNote)
	}
	msg := fmt.Sprintf("abandoned by %s at %s", actorLabel(m), when)
	if end.Reason != "" {
		msg += ", reason: " + end.Reason
	}
	return fmt.Sprintf("%s — this session is torn down. %s", msg, NewSessionNote)
}

// endedSessionError refuses an operation that would carry on a dialogue already
// over, naming how it ended and the one path that works instead.
func endedSessionError(m SessionMetadata, end SessionEnd) error {
	return &ApplicationError{Code: ErrorSessionEnded, Ended: &end, Message: endedMessage(m, end)}
}

// actorLabel names who acted on the session for an abandon message. A log with
// no metadata line of its own may name no participant.
func actorLabel(m SessionMetadata) string {
	if p := strings.TrimSpace(m.Participant); p != "" {
		return p
	}
	return "another participant"
}
