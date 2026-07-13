package mcpapp

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
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
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

	// graphs is the session's read-side graph seam — the finder-owned source
	// the engine reads through and the shell serves framing from. It memoizes
	// across one advance and is invalidated after a write (in the engine, keyed
	// on the write command's declaration) and at each advance entry, replacing
	// the mutable engine field the shell used to reassign by hand.
	graphs *finders.GraphSource

	// shellInstance is the session's base (shell-class) procedure instance —
	// the resident junction free dialogue pends on, where every move lands
	// when it ends. Set by the session door and re-derived on resume.
	shellInstance string

	// framingGraph/framingText cache the rendered session framing per graph
	// value — the source memoizes within one advance, so the cache mainly spares
	// re-renders within one response (resume rehydrating several serves).
	// Served-once memory lives on the server, keyed to the connection.
	framingGraph *model.Graph
	framingText  string

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
	// watched marks connections that already have a disconnect watcher —
	// one goroutine per connection, spawned on first bind.
	watched map[*mcp.ServerSession]bool
}

func newSessionStore(dir string) *sessionStore {
	return &sessionStore{
		dir:     dir,
		byMCP:   make(map[*mcp.ServerSession]*shellSession),
		byID:    make(map[string]*shellSession),
		watched: make(map[*mcp.ServerSession]bool),
	}
}

// markWatched records a disconnect watcher for the connection, returning
// false when one is already running.
func (st *sessionStore) markWatched(ms *mcp.ServerSession) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.watched[ms] {
		return false
	}
	st.watched[ms] = true
	return true
}

// bound returns the SDD session bound to the MCP session, or nil.
func (st *sessionStore) bound(ms *mcp.ServerSession) *shellSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.byMCP[ms]
}

// bind associates the MCP session with an SDD session, replacing any prior
// binding. Returns the previously bound session (nil when none) so the
// caller can apply the leave rule — end a quiescent session, park one with
// open moves.
func (st *sessionStore) bind(ms *mcp.ServerSession, ss *shellSession) *shellSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	prev := st.byMCP[ms]
	st.byMCP[ms] = ss
	st.byID[ss.id] = ss
	if prev == ss {
		return nil
	}
	return prev
}

// unbind detaches the MCP session, returning the session it was bound to
// (nil when none). Idempotent — the disconnect watcher and the stdio
// post-run sweep may both fire.
func (st *sessionStore) unbind(ms *mcp.ServerSession) *shellSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	ss := st.byMCP[ms]
	delete(st.byMCP, ms)
	delete(st.watched, ms)
	return ss
}

// drop removes a session from the in-memory index (after it ended).
func (st *sessionStore) drop(id string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.byID, id)
}

// lookupID returns an in-memory session by ID, or nil.
func (st *sessionStore) lookupID(id string) *shellSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.byID[id]
}

// liveIDs returns the session IDs currently bound to a connection. A
// session bound to a live connection is live by definition — it is not
// parked, so listings and open-threads blocks exclude it.
func (st *sessionStore) liveIDs() map[string]bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	live := make(map[string]bool, len(st.byMCP))
	for _, ss := range st.byMCP {
		live[ss.id] = true
	}
	return live
}

// connections returns the currently bound MCP sessions — the stdio
// post-run sweep drains them synchronously before the process exits.
func (st *sessionStore) connections() []*mcp.ServerSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]*mcp.ServerSession, 0, len(st.byMCP))
	for ms := range st.byMCP {
		out = append(out, ms)
	}
	return out
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
	Open         []instanceDescriptor `json:"open_instances" jsonschema:"running move instances with their current step (the session shell is not listed)"`
	LastActivity string               `json:"last_activity,omitempty"`
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
// liveness from completed/abandoned. Shell-class instances (the session
// base) are excluded from the open list — a session whose only running
// instance is its shell has no work to resume, so it never lists as open.
// Shell-ness comes from the started event's class field, keeping the
// descriptor self-derived (old logs without the field read as moves).
func deriveDescriptor(id string, events []engine.Event) sessionDescriptor {
	desc := sessionDescriptor{Session: id}
	type instState struct {
		procedure string
		step      string
		running   bool
		shell     bool
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
			if class, ok := ev.Data["class"].(string); ok {
				is.shell = class == string(model.ProcedureClassShell)
			}
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
		if !is.running || is.shell {
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
// every parked session — at least one running move instance (the shell
// alone doesn't count), and not currently bound to a live connection —
// newest activity first.
func (st *sessionStore) listOpenSessions() ([]sessionDescriptor, error) {
	entries, err := os.ReadDir(st.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading sessions dir: %w", err)
	}
	live := st.liveIDs()

	var out []sessionDescriptor
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		if live[id] {
			continue
		}
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

// teardownFold walks a session log for what teardown-by-handle needs beyond
// the descriptor: every still-running instance (the shell included, so the
// log closes fully terminal) and the WIP markers those instances currently
// hold (op_result writes to wipMarker; a later null write clears).
func teardownFold(events []engine.Event) (running []string, markers []string) {
	type instState struct {
		running bool
		marker  string
	}
	instances := map[string]*instState{}
	var order []string
	for _, ev := range events {
		switch ev.Event {
		case engine.EventStarted:
			if _, ok := instances[ev.Instance]; !ok {
				order = append(order, ev.Instance)
			}
			instances[ev.Instance] = &instState{running: true}
		case engine.EventCompleted, engine.EventAbandoned:
			if is, ok := instances[ev.Instance]; ok {
				is.running = false
			}
		case engine.EventOpResult:
			is, ok := instances[ev.Instance]
			if !ok {
				continue
			}
			if writes, ok := ev.Data["writes"].(map[string]any); ok {
				if v, present := writes["wipMarker"]; present {
					is.marker, _ = v.(string) // a null write clears
				}
			}
		}
	}
	for _, id := range order {
		is := instances[id]
		if !is.running {
			continue
		}
		running = append(running, id)
		if is.marker != "" {
			markers = append(markers, is.marker)
		}
	}
	return running, markers
}

// appendAbandons closes a parked session's log in place: one abandoned event
// per still-running instance, appended without replaying the session and
// without serving any framing — the point of teardown by handle (d-tac-dbk).
// Works on version-skewed logs too, since nothing is interpreted beyond the
// fold — exactly the stale sessions teardown exists for.
func (st *sessionStore) appendAbandons(id string, events []engine.Event, running []string, reason string) error {
	if len(running) == 0 {
		return nil
	}
	f, err := st.openLog(id)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sink := engine.NewWriterSink(f)
	seq := 0
	for _, ev := range events {
		if ev.Seq > seq {
			seq = ev.Seq
		}
	}
	data := map[string]any{"reason": reason}
	for _, inst := range running {
		seq++
		if err := sink.Append(engine.Event{
			V:        engine.LogVersion,
			TS:       time.Now(),
			Session:  id,
			Seq:      seq,
			Instance: inst,
			Event:    engine.EventAbandoned,
			Data:     data,
		}); err != nil {
			return fmt.Errorf("closing session log: %w", err)
		}
	}
	return nil
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
