package application

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// attachmentTimeFormat renders an attachment's last activity or ending in a
// stable, unambiguous form for the interpreted-conflict and consent messages.
const attachmentTimeFormat = time.RFC3339

// RecordedStateOnlyNote is the single statement of the takeover fidelity limit,
// composed into the consent refusal and the successful-attach note so both
// runtime surfaces read identically.
const RecordedStateOnlyNote = "Only recorded session state resumes — step position, collected fields, staged files — not the other conversation's context."

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

// SessionRecencyWindow is the single threshold separating an active attachment
// from an idle one. Erring long is cheap, so it is generous.
const SessionRecencyWindow = 15 * time.Minute

// ChooserKind classifies who answers a pending chooser, mirrored from the
// engine for the served workflow response.
type ChooserKind string

// SessionBinding is a connection's write token for a durable session: identity
// plus the attachment it drives and the version it last observed. Append CAS on
// the version is the sole integrity mechanism.
type SessionBinding struct {
	SessionID    SessionID
	Subject      string
	Project      ProjectID
	MCPSessionID string
	Version      uint64
}

// ReleaseSession clears this connection's own live attachment stamp so the
// session stops reading held. Nothing is recorded: stepping away is transport,
// not an act on the dialogue (d-cpt-rw7). Releasing means "clear MY stamp", so
// when the current attachment is absent or belongs to another MCP session —
// already displaced, ended, or taken over — it is a no-op.
func (a *Application) ReleaseSession(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding) error {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return err
	}
	stored, err := runtime.options.Sessions.Load(ctx, binding.SessionID)
	if err != nil {
		return err
	}
	if err := validateStoredSession(stored); err != nil {
		return err
	}
	current := stored.Metadata.Attachment
	if current == nil || current.MCPSessionID != binding.MCPSessionID {
		return nil
	}
	metadata := stored.Metadata
	metadata.Attachment = nil
	metadata.UpdatedAt = runtime.options.Now().UTC().Round(0)
	_, err = runtime.options.Sessions.Append(ctx, binding.SessionID, stored.Version, SessionAppend{Metadata: &metadata})
	return err
}

// attachmentActive reports whether an attachment counts as active: present and
// stamped within the recency window on either side of now. A stale stamp (a
// server killed without a leave event) reads idle, and so does a far-future
// stamp from clock skew — activity outside [-window, +window) is not "now".
func attachmentActive(att *Attachment, now time.Time) bool {
	if att == nil {
		return false
	}
	delta := now.Sub(att.LastActivity)
	return delta >= -SessionRecencyWindow && delta < SessionRecencyWindow
}

func sessionBindingFrom(stored StoredSession) SessionBinding {
	binding := SessionBinding{SessionID: stored.Metadata.ID, Subject: stored.Metadata.Subject, Project: stored.Metadata.Project, Version: stored.Version}
	if stored.Metadata.Attachment != nil {
		binding.MCPSessionID = stored.Metadata.Attachment.MCPSessionID
	}
	return binding
}

func validateStoredSession(stored StoredSession) error {
	if !SupportedSessionCodecVersion(stored.Metadata.CodecVersion) {
		return &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported session codec version", Version: stored.Metadata.CodecVersion}
	}
	for _, event := range stored.Events {
		if !SupportedSessionCodecVersion(event.CodecVersion) {
			return &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported session event codec version", Version: event.CodecVersion}
		}
	}
	return nil
}

// verifyBinding enforces subject/project immutability, the attachment-identity
// check (my MCP session is still the current attachment), and the version CAS.
// A displaced writer fails typed as ErrorSessionDisplaced (naming who advanced
// the session and how); a genuine identity/project violation is
// ErrorSessionOwnership; a benign same-writer version race is
// ErrorSessionConflict.
func verifyBinding(stored StoredSession, binding SessionBinding) error {
	if err := verifyAttachment(stored, binding); err != nil {
		return err
	}
	if stored.Version != binding.Version {
		return &ApplicationError{Code: ErrorSessionConflict, Message: "session binding is stale"}
	}
	return nil
}

// verifyAttachment checks immutable identity and that this binding's MCP session
// is still the current attachment, without the version CAS. It is the shared
// front half of verifyBinding and the resync retry.
func verifyAttachment(stored StoredSession, binding SessionBinding) error {
	if err := validateStoredSession(stored); err != nil {
		return err
	}
	m := stored.Metadata
	if m.ID != binding.SessionID || m.Subject != binding.Subject || m.Project != binding.Project {
		return &ApplicationError{Code: ErrorSessionOwnership, Message: "session identity and project are immutable"}
	}
	if att := m.Attachment; att == nil || att.MCPSessionID != binding.MCPSessionID {
		return displacedError(stored)
	}
	return nil
}

// displacedError interprets a lost attachment for the writer that lost it: how
// its world ended (abandoned, concluded) or who holds it now (a takeover). The
// terminal record wins even when a newer attachment exists, so a writer whose
// dialogue was destroyed hears that, not "taken over" by whoever reopened
// afterward.
func displacedError(stored StoredSession) error {
	m := stored.Metadata
	if end := m.Ended; end != nil {
		return &ApplicationError{Code: ErrorSessionDisplaced, Ended: end, Message: endedMessage(m, *end)}
	}
	if att := m.Attachment; att != nil {
		return &ApplicationError{
			Code: ErrorSessionDisplaced, Attachment: att,
			Message: fmt.Sprintf("taken over by %s at %s; your position may be stale — %s",
				ClientLabel(att.ClientName), att.LastActivity.Format(attachmentTimeFormat), reorientSuffix),
		}
	}
	return &ApplicationError{Code: ErrorSessionDisplaced,
		Message: fmt.Sprintf("this session's attachment was displaced — %s", reorientSuffix)}
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

// ClientLabel names a client for a conflict or consent message, falling back
// when the transport carried no client name (e.g. bare stdio).
func ClientLabel(name string) string {
	if strings.TrimSpace(name) == "" {
		return "another client"
	}
	return name
}

// actorLabel names who acted on the session for an abandon message. A log with
// no metadata line of its own may name no participant.
func actorLabel(m SessionMetadata) string {
	if p := strings.TrimSpace(m.Participant); p != "" {
		return p
	}
	return "another participant"
}
