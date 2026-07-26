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
	CodecVersion uint32
	ID           SessionID
	Subject      string
	Project      ProjectID
	Participant  string
	Label        string
	// Branch is the session's explicit branch binding. Empty means unbound;
	// compositions without a branch concept leave it empty.
	Branch            string `json:"branch,omitempty"`
	Attachment        *Attachment
	AttachmentHistory []AttachmentRecord
	UpdatedAt         time.Time
}

// Attachment is the informational stamp of the client currently driving the
// session: integrity comes from CAS on append, and status is derived from
// LastActivity recency. UserWords records the user's verbatim ask that
// authorized this attachment — the live stamp carries its own consent, and any
// history record embedding it preserves the words automatically.
type Attachment struct {
	Subject       string
	ClientName    string
	ClientVersion string
	MCPSessionID  string
	LastActivity  time.Time
	UserWords     string `json:",omitempty"`
}

// AttachmentRecord closes out a past attachment with the specific cause it
// ended. The embedded Attachment carries the words that authorized it; Reason
// records the abandon note, so a displaced writer's next call can be told why.
type AttachmentRecord struct {
	Attachment Attachment
	EndedAt    time.Time
	Cause      AttachmentCause
	Reason     string `json:",omitempty"`
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
