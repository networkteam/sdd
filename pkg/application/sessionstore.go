package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
)

type SessionID string

// SessionMetadata is structured routing and ownership data. Dialogue events
// remain opaque to the store. The type itself is the metadata contract: its
// evolution is the Go type's own, and how a store survives that is the
// adapter's concern — schema migrations, or format discrimination in its
// persisted record (d-tac-8js).
type SessionMetadata struct {
	ID          SessionID
	Subject     string
	Project     ProjectID
	Participant string
	Label       string
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

// legacyEndStore is the single authority for the ending a log never recorded in
// its metadata, so collection, listings and resume never disagree about which
// sessions are over. Reads derive; writes pass through — a derived ending stays
// read-side and is never recorded back into the log.
type legacyEndStore struct{ SessionStore }

func (s legacyEndStore) Load(ctx context.Context, id SessionID) (StoredSession, error) {
	stored, err := s.SessionStore.Load(ctx, id)
	if err != nil {
		return stored, err
	}
	deriveLegacyEnd(&stored)
	return stored, nil
}

func (s legacyEndStore) List(ctx context.Context, filter SessionFilter) ([]StoredSession, error) {
	stored, err := s.SessionStore.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	for i := range stored {
		deriveLegacyEnd(&stored[i])
	}
	return stored, nil
}

// deriveLegacyEnd recovers the terminal record from what the log states about
// itself. A record in metadata — written there, or recovered at decode from a
// superseded shape — always wins.
func deriveLegacyEnd(stored *StoredSession) {
	if stored.Metadata.Ended != nil {
		return
	}
	stored.Metadata.Ended = endFromShellEvents(stored.Events)
}

// endFromShellEvents reads the shell instances for the act that ended the
// dialogue: logs written before the terminal record existed left the
// participant's conclude as nothing but the shell's own engine event. A shell no
// longer running is the dialogue over, since carrying it on would mean starting
// a fresh one — the revival an ended session refuses (d-tac-k4q). Both ways a
// shell leaves running map to the same act the write site records. Events this
// binary cannot decode derive nothing; the consumer reports the unreadable log.
func endFromShellEvents(events []StoredEvent) *SessionEnd {
	decoded, err := decodeWorkflowEvents(events)
	if err != nil {
		return nil
	}
	shells := map[string]bool{}
	var endedAt time.Time
	for _, event := range decoded {
		switch event.Event {
		case engine.EventStarted:
			if class, _ := event.Data["class"].(string); class == string(model.ProcedureClassShell) {
				shells[event.Instance] = false
			}
		case engine.EventCompleted, engine.EventAbandoned:
			if _, ok := shells[event.Instance]; !ok {
				continue
			}
			shells[event.Instance] = true
			if event.TS.After(endedAt) {
				endedAt = event.TS
			}
		}
	}
	if len(shells) == 0 {
		return nil
	}
	for _, ended := range shells {
		if !ended {
			return nil
		}
	}
	return &SessionEnd{Act: SessionConcluded, EndedAt: endedAt}
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
// Compositions must not run mixed engine versions against one session store:
// metadata carries no version guard (d-tac-8js), so an older engine reading
// metadata a newer one wrote is undetected there — only a session the newer
// engine actually advanced fails closed, through StoredEvent.CodecVersion.
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
