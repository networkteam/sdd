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

// retiredSessionMetadataFields records, per codec version, the SessionMetadata
// fields that version wrote and the model no longer defines. Codec 1 carried a
// holder lease until attachment stamps replaced it, so logs written before that
// still hold the pair — a strict decode has to skip them rather than reject the
// very files it exists to read.
//
// Dropping a field from SessionMetadata means registering it here and bumping
// SessionCodecVersion. A removal that skips both steps is invisible until some
// strict reader meets an older log, which is exactly how the relocation outage
// this table answers came about.
var retiredSessionMetadataFields = map[uint32][]string{
	1: {"Holder", "HolderHistory"},
}

// RetiredSessionMetadataFields returns the metadata field names a persisted
// codec version wrote that the current model no longer defines. Decoders skip
// these; every other unknown field stays an error, so the strictness keeps
// working as a drift alarm.
func RetiredSessionMetadataFields(version uint32) []string {
	return retiredSessionMetadataFields[version]
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

// ReleaseSession ends this connection's own attachment, recording it in history
// with the given cause. Releasing means "end MY attachment", so when the current
// attachment is absent or belongs to another MCP session — already displaced,
// concluded, or taken over — it is a no-op: there is nothing of mine to end.
func (a *Application) ReleaseSession(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, cause AttachmentCause, reason string) error {
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
	now := runtime.options.Now().UTC().Round(0)
	metadata := stored.Metadata
	record := endAttachment(*current, now, cause)
	record.Reason = strings.TrimSpace(reason)
	metadata.AttachmentHistory = append(metadata.AttachmentHistory, record)
	metadata.Attachment = nil
	metadata.UpdatedAt = now
	_, err = runtime.options.Sessions.Append(ctx, binding.SessionID, stored.Version, SessionAppend{Metadata: &metadata})
	return err
}

// endAttachment closes out a past attachment with the cause it ended for the
// history log.
func endAttachment(att Attachment, endedAt time.Time, cause AttachmentCause) AttachmentRecord {
	return AttachmentRecord{Attachment: att, EndedAt: endedAt, Cause: cause}
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
// its world ended (abandoned, concluded) or who holds it now (a takeover). A
// terminal end as the most recent record wins even if a newer attachment
// exists, so a writer whose dialogue was destroyed hears that, not "taken over"
// by whoever reopened afterward. The message names who/when/why; the attachment
// and cause ride the typed error.
func displacedError(stored StoredSession) error {
	m := stored.Metadata
	if n := len(m.AttachmentHistory); n > 0 {
		rec := m.AttachmentHistory[n-1]
		when := rec.EndedAt.Format(attachmentTimeFormat)
		switch rec.Cause {
		case CauseAbandon:
			msg := fmt.Sprintf("abandoned by %s at %s", actorLabel(m, rec), when)
			if rec.Reason != "" {
				msg += ", reason: " + rec.Reason
			}
			msg += " — this session is torn down; start fresh"
			return &ApplicationError{Code: ErrorSessionDisplaced, Attachment: &rec.Attachment, AttachmentCause: rec.Cause, Message: msg}
		case CauseConclude:
			return &ApplicationError{Code: ErrorSessionDisplaced, Attachment: &rec.Attachment, AttachmentCause: rec.Cause,
				Message: fmt.Sprintf("this session was concluded at %s — start fresh", when)}
		}
	}
	if att := m.Attachment; att != nil {
		return &ApplicationError{
			Code: ErrorSessionDisplaced, Attachment: att, AttachmentCause: CauseClaim,
			Message: fmt.Sprintf("taken over by %s at %s (claim); your position may be stale — %s",
				ClientLabel(att.ClientName), att.LastActivity.Format(attachmentTimeFormat), reorientSuffix),
		}
	}
	if n := len(m.AttachmentHistory); n > 0 {
		rec := m.AttachmentHistory[n-1]
		return &ApplicationError{Code: ErrorSessionDisplaced, Attachment: &rec.Attachment, AttachmentCause: rec.Cause,
			Message: fmt.Sprintf("this session's attachment ended (%s) at %s — %s", rec.Cause, rec.EndedAt.Format(attachmentTimeFormat), reorientSuffix)}
	}
	return &ApplicationError{Code: ErrorSessionDisplaced,
		Message: fmt.Sprintf("this session's attachment was displaced — %s", reorientSuffix)}
}

// ClientLabel names a client for a conflict or consent message, falling back
// when the transport carried no client name (e.g. bare stdio).
func ClientLabel(name string) string {
	if strings.TrimSpace(name) == "" {
		return "another client"
	}
	return name
}

// actorLabel names who acted on the session for an abandon message: the
// session's participant, else the client that recorded the record.
func actorLabel(m SessionMetadata, rec AttachmentRecord) string {
	if p := strings.TrimSpace(m.Participant); p != "" {
		return p
	}
	return ClientLabel(rec.Attachment.ClientName)
}
