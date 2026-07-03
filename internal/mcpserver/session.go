package mcpserver

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/networkteam/sdd/internal/engine"
)

// shellSession is one SDD dialogue session as the shell sees it: the engine
// session with its append-only log file, the staging scratch for attachments,
// and the served-instruction memory for the currently bound agent consumer.
// Engine and registry are per-session because the write commands close over
// session-owned state (staging dir, graph refresh).
type shellSession struct {
	id          string
	participant string

	engine  *engine.Engine
	sess    *engine.Session
	logFile *os.File

	// served tracks instruction units already served full-text to the bound
	// agent, keyed instance+"/"+unit. Reset by rebinding (resume_session) —
	// a new agent consumer gets full text again.
	served map[string]bool
	// framed marks that the session framing block was delivered to the
	// bound agent consumer.
	framed bool

	lastActivity time.Time
}

// sessionStore owns the shell's session lifecycle: creation, the binding of
// MCP protocol sessions to SDD sessions, and list/resume over the JSONL logs
// in the sessions directory.
type sessionStore struct {
	dir string

	mu    sync.Mutex
	byMCP map[*mcp.ServerSession]*shellSession
	byID  map[string]*shellSession
}

func newSessionStore(dir string) *sessionStore {
	return &sessionStore{
		dir:   dir,
		byMCP: make(map[*mcp.ServerSession]*shellSession),
		byID:  make(map[string]*shellSession),
	}
}

// bound returns the SDD session bound to the MCP session, or nil.
func (st *sessionStore) bound(ms *mcp.ServerSession) *shellSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.byMCP[ms]
}

// bind associates the MCP session with an SDD session, replacing any prior
// binding (the previous session stays open on disk and listable).
func (st *sessionStore) bind(ms *mcp.ServerSession, ss *shellSession) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.byMCP[ms] = ss
	st.byID[ss.id] = ss
}

// lookupID returns an in-memory session by ID, or nil.
func (st *sessionStore) lookupID(id string) *shellSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.byID[id]
}

// newSessionID mints a session handle: timestamped for human-readable file
// sorting, random-suffixed for uniqueness.
func newSessionID(now time.Time) string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failure is a broken platform; a panic is more honest
		// than colliding session logs.
		panic(fmt.Sprintf("mcpserver: reading random bytes: %v", err))
	}
	return "s_" + now.Format("20060102-150405") + "-" + hex.EncodeToString(buf)
}

func (st *sessionStore) logPath(id string) string {
	return filepath.Join(st.dir, id+".jsonl")
}

// stagingDir returns (creating on demand) the session's attachment scratch
// directory. Staged files are session-local and never a graph write; the
// write gate materializes them into the entry's attachment dir.
func (st *sessionStore) stagingDir(id string) (string, error) {
	dir := filepath.Join(st.dir, id+"-staging")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating staging dir: %w", err)
	}
	return dir, nil
}

// touch records session activity for descriptor freshness.
func (ss *shellSession) touch(now time.Time) { ss.lastActivity = now }

// close releases the session's log file handle. The log itself stays — it is
// the persistence and the forensic record.
func (ss *shellSession) close() {
	if ss.logFile != nil {
		_ = ss.logFile.Close()
		ss.logFile = nil
	}
}

// sessionDescriptor is the list_sessions row: self-derived from the JSONL
// log alone, with no procedure-spec resolution, so listing stays robust even
// when a procedure changed underneath a stale session.
type sessionDescriptor struct {
	Session      string               `json:"session" jsonschema:"session handle; pass to resume_session"`
	Label        string               `json:"label,omitempty" jsonschema:"the session's subject — agent-supplied, falling back to the first line of the most recent drafted body; blank when nothing was drafted"`
	Participant  string               `json:"participant,omitempty"`
	Anchor       string               `json:"anchor,omitempty" jsonschema:"entry the session's work is anchored on, when a procedure param carried one"`
	Open         []instanceDescriptor `json:"open_instances" jsonschema:"running procedure instances with their current step"`
	LastActivity string               `json:"last_activity,omitempty"`
	Current      bool                 `json:"current,omitempty" jsonschema:"true when this is the session bound to the calling agent"`
}

type instanceDescriptor struct {
	Instance  string `json:"instance"`
	Procedure string `json:"procedure"`
	Step      string `json:"step,omitempty"`
}

// deriveDescriptor folds a session log into its descriptor without replaying
// through the engine: participant from the meta line, label from the last
// labeled line (falling back to the first line of the most recent drafted
// body), per-instance procedure and step from started/transition lines,
// liveness from completed/abandoned.
func deriveDescriptor(id string, events []engine.Event) sessionDescriptor {
	desc := sessionDescriptor{Session: id}
	type instState struct {
		procedure string
		step      string
		running   bool
	}
	instances := map[string]*instState{}
	var order []string
	var bodyLine string

	for _, ev := range events {
		if !ev.TS.IsZero() {
			desc.LastActivity = ev.TS.Format(time.RFC3339)
		}
		switch ev.Event {
		case engine.EventSessionMeta:
			if p, ok := ev.Data["participant"].(string); ok && p != "" {
				desc.Participant = p
			}
		case engine.EventLabeled:
			if l, ok := ev.Data["label"].(string); ok {
				desc.Label = l
			}
		case engine.EventReport:
			if fields, ok := ev.Data["fields"].(map[string]any); ok {
				if body, ok := fields["body"].(string); ok && strings.TrimSpace(body) != "" {
					bodyLine = firstLine(body)
				}
			}
		case engine.EventStarted:
			is := &instState{running: true}
			is.procedure, _ = ev.Data["procedure"].(string)
			is.step, _ = ev.Data["step"].(string)
			if params, ok := ev.Data["params"].(map[string]any); ok {
				if anchor, ok := params["anchor"].(string); ok && desc.Anchor == "" {
					desc.Anchor = anchor
				}
			}
			instances[ev.Instance] = is
			order = append(order, ev.Instance)
		case engine.EventTransition:
			if is, ok := instances[ev.Instance]; ok {
				if to, ok := ev.Data["to"].(string); ok && !engine.IsEndTarget(to) {
					is.step = to
				}
			}
		case engine.EventCompleted, engine.EventAbandoned:
			if is, ok := instances[ev.Instance]; ok {
				is.running = false
			}
		}
	}

	for _, instID := range order {
		is := instances[instID]
		if !is.running {
			continue
		}
		desc.Open = append(desc.Open, instanceDescriptor{
			Instance:  instID,
			Procedure: is.procedure,
			Step:      is.step,
		})
	}
	if desc.Label == "" {
		desc.Label = bodyLine
	}
	return desc
}

// firstLine reduces a drafted body to a label-sized single line.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	runes := []rune(s)
	if len(runes) > maxLabelLen {
		s = string(runes[:maxLabelLen-1]) + "…"
	}
	return s
}

// listOpenSessions scans the sessions directory and returns descriptors for
// every session with at least one running instance, newest activity first.
func (st *sessionStore) listOpenSessions() ([]sessionDescriptor, error) {
	entries, err := os.ReadDir(st.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading sessions dir: %w", err)
	}

	var out []sessionDescriptor
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		events, err := st.readLog(id)
		if err != nil {
			// A corrupt or version-skewed log must not hide the healthy
			// sessions; surface it as a row with no open instances.
			out = append(out, sessionDescriptor{Session: id, Participant: "(unreadable log: " + err.Error() + ")"})
			continue
		}
		desc := deriveDescriptor(id, events)
		if len(desc.Open) == 0 {
			continue
		}
		out = append(out, desc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastActivity > out[j].LastActivity })
	return out, nil
}

// readLog parses a session's JSONL log.
func (st *sessionStore) readLog(id string) ([]engine.Event, error) {
	f, err := os.Open(st.logPath(id))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return engine.ReadEvents(f)
}

// openLog opens (creating) a session's append-only log file.
func (st *sessionStore) openLog(id string) (*os.File, error) {
	if err := os.MkdirAll(st.dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating sessions dir: %w", err)
	}
	f, err := os.OpenFile(st.logPath(id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening session log: %w", err)
	}
	return f, nil
}
