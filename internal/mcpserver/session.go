package mcpserver

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// session is the per-dialogue state machine. Slice 1 tracks a single
// transition: opened → grounded. The grounding flag is the enforced half of
// the guide/gate split — capture refuses to run before at least one ground
// call happened in the session.
type session struct {
	token      string
	openedAt   time.Time
	grounded   bool
	groundedAt time.Time
}

// sessionStore holds live sessions keyed by token. Tokens are issued by
// open_session and passed back explicitly on ground/capture calls, so the
// state machine is independent of transport-level session identity (HTTP
// reconnects keep the dialogue state).
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]*session
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]*session)}
}

func (st *sessionStore) open(now time.Time) *session {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failure is a broken platform; a panic is more honest
		// than serving guessable session tokens.
		panic(fmt.Sprintf("mcpserver: reading random bytes: %v", err))
	}
	s := &session{
		token:    hex.EncodeToString(buf),
		openedAt: now,
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sessions[s.token] = s
	return s
}

// markGrounded records a grounding call on the session. Returns false when
// the token is unknown.
func (st *sessionStore) markGrounded(token string, now time.Time) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	s, ok := st.sessions[token]
	if !ok {
		return false
	}
	s.grounded = true
	s.groundedAt = now
	return true
}

// lookup returns the session for token, or nil.
func (st *sessionStore) lookup(token string) *session {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.sessions[token]
}
