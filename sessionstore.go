package sdd

import (
	"context"
	"encoding/json"
	"time"
)

type SessionID string

// SessionMetadata is structured routing and ownership data. Dialogue events
// remain opaque to the store.
type SessionMetadata struct {
	CodecVersion  uint32
	ID            SessionID
	Subject       string
	Project       ProjectID
	Participant   string
	Label         string
	Holder        *SessionHolder
	HolderHistory []SessionHolderRecord
	UpdatedAt     time.Time
}

type SessionHolderRecord struct {
	Holder  SessionHolder
	EndedAt time.Time
	Reason  string
}

type SessionHolder struct {
	Subject       string
	MCPSessionID  string
	ClientName    string
	ClientVersion string
	Generation    uint64
	LastActivity  time.Time
	ExpiresAt     time.Time
}

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
