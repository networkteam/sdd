package application

import (
	"context"
	"time"
)

const SessionCodecVersion uint32 = 1

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
func (a *Application) ReleaseSession(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, cause AttachmentCause) error {
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
	metadata.AttachmentHistory = append(metadata.AttachmentHistory, endAttachment(*current, now, cause))
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
	if stored.Metadata.CodecVersion != SessionCodecVersion {
		return &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported session codec version", Version: stored.Metadata.CodecVersion}
	}
	for _, event := range stored.Events {
		if event.CodecVersion != SessionCodecVersion {
			return &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported session event codec version", Version: event.CodecVersion}
		}
	}
	return nil
}

// verifyBinding enforces subject/project immutability, the attachment-identity
// check (my MCP session is still the current attachment), and the version CAS.
// A displaced writer fails the identity check; a benign version race fails the
// CAS check.
func verifyBinding(stored StoredSession, binding SessionBinding) error {
	if err := validateStoredSession(stored); err != nil {
		return err
	}
	att := stored.Metadata.Attachment
	if stored.Metadata.ID != binding.SessionID || stored.Metadata.Subject != binding.Subject || stored.Metadata.Project != binding.Project ||
		att == nil || att.MCPSessionID != binding.MCPSessionID {
		return &ApplicationError{Code: ErrorSessionOwnership, Message: "session attachment was displaced"}
	}
	if stored.Version != binding.Version {
		return &ApplicationError{Code: ErrorSessionConflict, Message: "session binding is stale"}
	}
	return nil
}
