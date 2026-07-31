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
	Branch     string `json:"branch,omitempty"`
	Attachment *Attachment
	// Ended is the session's single terminal record. Its presence is what makes
	// a session ended; nothing else about a session ends it (d-cpt-rw7).
	Ended     *SessionEnd `json:",omitempty"`
	UpdatedAt time.Time
}

// Attachment is the ephemeral stamp of the client currently driving the
// session: integrity comes from CAS on append, and status is derived from
// LastActivity recency. UserWords records the user's verbatim ask that
// authorized this attachment.
type Attachment struct {
	Subject       string
	ClientName    string
	ClientVersion string
	MCPSessionID  string
	LastActivity  time.Time
	UserWords     string `json:",omitempty"`
}

// SessionEnd records the participant act that ended a session, written once and
// never revised. Reason records the abandon note, so a displaced writer's next
// call can be told why. Who ended it is the session's own participant; the
// ending client's stamp is transport and does not enter the durable record.
type SessionEnd struct {
	Act     SessionEndAct
	EndedAt time.Time
	Reason  string `json:",omitempty"`
}

// SessionEndAct is the closed set of participant acts that end a dialogue.
type SessionEndAct string

const (
	SessionConcluded SessionEndAct = "concluded"
	SessionAbandoned SessionEndAct = "abandoned"
)

// UnmarshalJSON decodes stored metadata, recovering the terminal record from the
// attachment history superseded shapes carried it in. Decoding stays lenient
// about every other field in both directions (d-cpt-i2x).
func (m *SessionMetadata) UnmarshalJSON(data []byte) error {
	type metadata SessionMetadata
	var decoded struct {
		metadata
		AttachmentHistory []legacyAttachmentRecord
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = SessionMetadata(decoded.metadata)
	if m.Ended == nil {
		m.Ended = endFromLegacyHistory(decoded.AttachmentHistory)
	}
	return nil
}

// legacyAttachmentRecord is one entry of the attachment history superseded
// shapes appended to, decoded only far enough to recover a terminal act.
type legacyAttachmentRecord struct {
	EndedAt time.Time
	Cause   string
	Reason  string
}

// endFromLegacyHistory reads a superseded history backwards for the act that
// ended the dialogue, skipping the connection events those logs also recorded —
// a dropped socket ends nothing. A takeover, or a cause this binary does not
// know, stops the scan: something came after the act, so the session is not
// ended.
func endFromLegacyHistory(history []legacyAttachmentRecord) *SessionEnd {
	for i := len(history) - 1; i >= 0; i-- {
		record := history[i]
		switch record.Cause {
		case "disconnect", "shutdown", "switch":
			continue
		case "conclude":
			return &SessionEnd{Act: SessionConcluded, EndedAt: record.EndedAt, Reason: record.Reason}
		case "abandon":
			return &SessionEnd{Act: SessionAbandoned, EndedAt: record.EndedAt, Reason: record.Reason}
		}
		return nil
	}
	return nil
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
//
// List is also the enumeration collection sweeps over, and Delete is what makes
// them possible against any implementation rather than only the local one.
// Delete must be idempotent: removing a session that is already gone is
// success, since two sweeps may derive the same target set.
type SessionStore interface {
	Create(context.Context, SessionMetadata) (StoredSession, error)
	Load(context.Context, SessionID) (StoredSession, error)
	List(context.Context, SessionFilter) ([]StoredSession, error)
	Append(context.Context, SessionID, uint64, SessionAppend) (uint64, error)
	Delete(context.Context, SessionID) error
}
