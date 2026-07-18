package application

import (
	"context"
	"encoding/json"
	"time"
)

type SessionID string

// SessionMetadata is structured routing and ownership data. Dialogue events
// remain opaque to the store.
type SessionMetadata struct {
	CodecVersion      uint32
	ID                SessionID
	Subject           string
	Project           ProjectID
	Participant       string
	Label             string
	Attachment        *Attachment
	AttachmentHistory []AttachmentRecord
	UpdatedAt         time.Time
}

// Attachment is the informational stamp of the client currently driving the
// session. It carries no lease: integrity comes from CAS on append, and status
// is derived from LastActivity recency, never from an expiry.
type Attachment struct {
	Subject       string
	ClientName    string
	ClientVersion string
	MCPSessionID  string
	LastActivity  time.Time
}

// AttachmentRecord closes out a past attachment with the specific cause it
// ended. UserWords records the consenting ask on a claim (populated in a later
// slice).
type AttachmentRecord struct {
	Attachment Attachment
	EndedAt    time.Time
	Cause      AttachmentCause
	UserWords  string `json:",omitempty"`
}

// AttachmentCause is the closed set of reasons an attachment ends.
type AttachmentCause string

const (
	CauseDisconnect AttachmentCause = "disconnect"
	CauseSwitch     AttachmentCause = "switch"
	CauseShutdown   AttachmentCause = "shutdown"
	CauseClaim      AttachmentCause = "claim"
	CauseConclude   AttachmentCause = "conclude"
	CauseAbandon    AttachmentCause = "abandon"
)

type StoredEvent struct {
	CodecVersion uint32
	Code         string
	Payload      json.RawMessage
}

type StoredSession struct {
	Metadata SessionMetadata
	Version  uint64
	Events   []StoredEvent
}

type SessionFilter struct {
	Subject string
	Project ProjectID
}

type SessionAppend struct {
	Metadata *SessionMetadata
	Events   []StoredEvent
}

// SessionStore persists structured metadata plus ordered opaque events. Append
// is the sole mutation primitive and must compare ExpectedVersion atomically.
type SessionStore interface {
	Create(context.Context, SessionMetadata) (StoredSession, error)
	Load(context.Context, SessionID) (StoredSession, error)
	List(context.Context, SessionFilter) ([]StoredSession, error)
	Append(context.Context, SessionID, uint64, SessionAppend) (uint64, error)
}
