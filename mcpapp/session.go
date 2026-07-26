package mcpapp

import (
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	sdd "github.com/networkteam/sdd/application"
)

// shellSession is the connection-local handle for a durable root workflow.
// Authorization proof is never stored here; rootIdentity is only the most
// recent request identity used when a disconnect has no request context.
type shellSession struct {
	id           string
	root         *sdd.WorkflowSession
	rootIdentity sdd.RequestIdentity
}

// sessionStore owns only MCP connection bindings. Durable session state,
// attachment stamps, labels, and workflow events live behind Application ports.
type sessionStore struct {
	mu      sync.Mutex
	byMCP   map[*mcp.ServerSession]*shellSession
	watched map[*mcp.ServerSession]bool
}

func newSessionStore() *sessionStore {
	return &sessionStore{
		byMCP:   make(map[*mcp.ServerSession]*shellSession),
		watched: make(map[*mcp.ServerSession]bool),
	}
}

func (st *sessionStore) markWatched(session *mcp.ServerSession) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.watched[session] {
		return false
	}
	st.watched[session] = true
	return true
}

func (st *sessionStore) bound(session *mcp.ServerSession) *shellSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.byMCP[session]
}

func (st *sessionStore) bind(session *mcp.ServerSession, workflow *shellSession) *shellSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	previous := st.byMCP[session]
	st.byMCP[session] = workflow
	if previous == workflow {
		return nil
	}
	return previous
}

func (st *sessionStore) unbind(session *mcp.ServerSession) *shellSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	workflow := st.byMCP[session]
	delete(st.byMCP, session)
	delete(st.watched, session)
	return workflow
}

func (st *sessionStore) liveIDs() map[string]bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	result := make(map[string]bool, len(st.byMCP))
	for _, workflow := range st.byMCP {
		result[workflow.id] = true
	}
	return result
}

func (st *sessionStore) connections() []*mcp.ServerSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	result := make([]*mcp.ServerSession, 0, len(st.watched))
	for session := range st.watched {
		result = append(result, session)
	}
	return result
}

type sessionDescriptor struct {
	Session      string               `json:"session" jsonschema:"session handle; pass to resume_session"`
	Label        string               `json:"label,omitempty" jsonschema:"the session's subject — agent-supplied, falling back to the first line of the most recent drafted body; blank when nothing was drafted"`
	Participant  string               `json:"participant,omitempty"`
	Branch       string               `json:"branch,omitempty" jsonschema:"the session's declared branch binding"`
	Anchor       string               `json:"anchor,omitempty" jsonschema:"entry the session's work is anchored on, when a procedure param carried one"`
	Open         []instanceDescriptor `json:"open_instances" jsonschema:"running move instances with their current step (the session shell is not listed)"`
	ClientName   string               `json:"client_name,omitempty" jsonschema:"name of the client whose attachment last drove the session, when one has"`
	LastActivity string               `json:"last_activity,omitempty"`
	Activity     string               `json:"activity" jsonschema:"active (an attachment stamped within the recency window) or idle — a hint for whether attaching may interrupt someone"`
}

type instanceDescriptor struct {
	Instance  string `json:"instance"`
	Procedure string `json:"procedure"`
	Step      string `json:"step,omitempty"`
}
