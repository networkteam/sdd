package application

import (
	"context"
	"time"
)

const SessionCodecVersion uint32 = 1

// SessionRecencyWindow is the single threshold separating an active attachment
// from an idle one. It labels listings and (in a later slice) gates takeover;
// erring long is cheap, so it is generous.
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

// ReleaseSession records the current attachment ending with a specific cause
// and clears it. Status is derived from the store afterwards; nothing is
// marked "released".
func (a *Application) ReleaseSession(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, cause AttachmentCause) error {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return err
	}
	stored, err := runtime.options.Sessions.Load(ctx, binding.SessionID)
	if err != nil {
		return err
	}
	if err := verifyBinding(stored, binding); err != nil {
		return err
	}
	now := runtime.options.Now().UTC().Round(0)
	metadata := stored.Metadata
	if metadata.Attachment != nil {
		metadata.AttachmentHistory = append(metadata.AttachmentHistory, AttachmentRecord{Attachment: *metadata.Attachment, EndedAt: now, Cause: cause})
		metadata.Attachment = nil
	}
	metadata.UpdatedAt = now
	_, err = runtime.options.Sessions.Append(ctx, binding.SessionID, stored.Version, SessionAppend{Metadata: &metadata})
	return err
}

// attachmentActive reports whether an attachment counts as active given the
// injected clock — present and stamped within the recency window. A server
// killed without a leave event leaves a stale stamp, so it reads idle.
func attachmentActive(att *Attachment, now time.Time) bool {
	return att != nil && now.Sub(att.LastActivity) < SessionRecencyWindow
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
