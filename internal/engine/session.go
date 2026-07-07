package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// LogVersion stamps every event line. A session generally does not survive
// an sdd upgrade mid-flight — replay rejects unknown versions rather than
// guessing (accepted per the surface spec). Exported for shells that append
// terminal events to a parked log without replaying it (teardown by handle).
const LogVersion = 1

// EventType is one of the closed set of session-log events.
type EventType string

const (
	// EventSessionMeta is the session-level header line (no instance):
	// participant identity, so a log is self-describing for list_sessions
	// descriptors without resolving any procedure spec.
	EventSessionMeta EventType = "session_meta"
	// EventLabeled carries a human-meaningful session label (no instance):
	// the dialogue's subject, agent-supplied and updatable as it sharpens.
	// Last one wins on fold and replay.
	EventLabeled       EventType = "labeled"
	EventStarted       EventType = "started"
	EventReport        EventType = "report"
	EventChooserAnswer EventType = "chooser_answer"
	EventOpResult      EventType = "op_result"
	EventServed        EventType = "served"
	EventTransition    EventType = "transition"
	EventCompleted     EventType = "completed"
	EventAbandoned     EventType = "abandoned"
	// EventRead records what a read surface served to the session: the tool,
	// the entry IDs, and the depth — metadata only, never payloads. Reads stay
	// free and ungated; tracking is logging, not gating (d-tac-dbk). No
	// instance — the read set is session-level, so evidence inspected by a
	// dispatching parent counts for its seeded children.
	EventRead EventType = "read"
	// EventParked records that a running move was deliberately parked back to
	// the session junction — state kept, resumable through next. Forensic
	// only: a resuming agent can tell a shelved move from one that drifted.
	EventParked EventType = "parked"
)

// ReadDepth classifies how deeply an entry was served: full means the body
// was served (a show primary, an injected chain's primary, or a same-session
// write); summary covers chain bullets, search headers, and snippets.
type ReadDepth string

const (
	ReadSummary ReadDepth = "summary"
	ReadFull    ReadDepth = "full"
)

// Event is one line of the append-only session log. Memory is the runtime
// source of truth; the log is the persistence, the session protocol, and
// the forensic record — transition reports are the trajectory evidence.
type Event struct {
	V        int            `json:"v"`
	TS       time.Time      `json:"ts"`
	Session  string         `json:"session"`
	Seq      int            `json:"seq"`
	Instance string         `json:"instance,omitempty"`
	Event    EventType      `json:"event"`
	Data     map[string]any `json:"data,omitempty"`
}

// EventSink receives events as they happen. The shell wires it to an
// append-only JSONL file; tests use an in-memory sink.
type EventSink interface {
	Append(Event) error
}

// WriterSink appends events as JSON lines to an io.Writer.
type WriterSink struct {
	w io.Writer
}

// NewWriterSink wraps a writer as an EventSink emitting one JSON line per
// event.
func NewWriterSink(w io.Writer) *WriterSink {
	return &WriterSink{w: w}
}

// Append writes the event as one JSON line.
func (s *WriterSink) Append(e Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = s.w.Write(b)
	return err
}

// ReadEvents parses a JSONL session log.
func ReadEvents(r io.Reader) ([]Event, error) {
	var events []Event
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, fmt.Errorf("session log line %d: %w", line, err)
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// Graphs is the engine's read-through to the current graph. The read side owns
// loading and freshness (a finders.GraphSource); the engine only reads through
// this value — Current returns the current graph (memoized by the source),
// Invalidate drops that memo after a graph write so the next read reloads.
// Declared here as an interface so the engine takes no finders import.
type Graphs interface {
	Current() (*model.Graph, error)
	Invalidate()
}

// Engine executes procedure instances against a graph provider and a registry.
// It is pure Go over data — shells (MCP, webapp) sit on top; side-effectful
// commands come in through the registry with their own dependencies. The
// graph is not held as a mutable field: the engine reads it through Graphs, so
// ownership of "what the current graph is" stays on the read side.
type Engine struct {
	Registry *Registry
	Graphs   Graphs
}

// New creates an engine reading the current graph through graphs.
func New(registry *Registry, graphs Graphs) *Engine {
	return &Engine{Registry: registry, Graphs: graphs}
}

// StaticGraphs is a Graphs backed by a fixed in-memory graph — for tests and
// callers with no reload need. Invalidate is a no-op: there is nothing behind
// the value to re-read.
type StaticGraphs struct{ Graph *model.Graph }

// Current returns the fixed graph.
func (s StaticGraphs) Current() (*model.Graph, error) { return s.Graph, nil }

// Invalidate is a no-op for a static graph.
func (s StaticGraphs) Invalidate() {}

// Session is one dialogue session: N procedure instances interleaved
// serially, one append-only event log. Per-participant; the shell owns file
// placement and lifecycle.
type Session struct {
	ID          string
	Participant string
	// Label is the session's human-meaningful subject line — what a user
	// picks a parked dialogue by. Agent-supplied via SetLabel, updatable;
	// empty until supplied.
	Label string

	engine    *Engine
	sink      EventSink
	now       func() time.Time
	seq       int
	counter   int
	instances map[string]*Instance
	order     []string
	// reads folds the session's read events into the deepest depth each entry
	// was served at — what the refsInspected gate evaluates. Session-level by
	// design: a parent's inspections count for children dispatched in the same
	// session, and replay restores the set from the logged read events.
	reads map[string]ReadDepth
	// sinkErr carries a deferred log-append failure; surfaced by the next
	// advance call so a durability problem can't pass silently.
	sinkErr error
}

// SessionOption configures a session.
type SessionOption func(*Session)

// WithClock injects the timestamp source (tests use a fixed clock).
func WithClock(now func() time.Time) SessionOption {
	return func(s *Session) { s.now = now }
}

// NewSession creates an empty session appending to sink.
func (e *Engine) NewSession(id, participant string, sink EventSink, opts ...SessionOption) *Session {
	s := &Session{
		ID:          id,
		Participant: participant,
		engine:      e,
		sink:        sink,
		now:         time.Now,
		instances:   map[string]*Instance{},
		reads:       map[string]ReadDepth{},
	}
	for _, o := range opts {
		o(s)
	}
	if sink != nil {
		s.appendEvent("", EventSessionMeta, map[string]any{
			"participant": participant,
		})
	}
	return s
}

// SetLabel records the session's subject label. Idempotent per value — a
// resent identical label appends nothing, so agents may safely carry the
// label on every call.
func (s *Session) SetLabel(label string) {
	if label == s.Label {
		return
	}
	s.Label = label
	s.appendEvent("", EventLabeled, map[string]any{"label": label})
}

// LogRead records what a read surface served to the session: entry IDs at
// full depth (body served) and summary depth (headers, chain bullets,
// snippets), attributed to the serving tool. Every call appends a read event
// — re-reads are part of the forensic record — and folds into the session's
// read set, where full depth never downgrades. Empty calls append nothing.
func (s *Session) LogRead(tool string, full, summary []string) {
	full = s.foldReads(full, ReadFull)
	summary = s.foldReads(summary, ReadSummary)
	if len(full) == 0 && len(summary) == 0 {
		return
	}
	data := map[string]any{"tool": tool}
	if len(full) > 0 {
		data["full"] = full
	}
	if len(summary) > 0 {
		data["summary"] = summary
	}
	s.appendEvent("", EventRead, data)
}

// foldReads folds IDs into the read set at depth (full wins over summary)
// and returns them deduplicated, empties dropped — the event payload.
func (s *Session) foldReads(ids []string, depth ReadDepth) []string {
	var kept []string
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		kept = append(kept, id)
		if s.reads[id] != ReadFull {
			s.reads[id] = depth
		}
	}
	return kept
}

// ReadDepthOf returns the deepest depth the entry was served to this session
// at (empty when never served).
func (s *Session) ReadDepthOf(id string) ReadDepth {
	return s.reads[id]
}

// Park records that a running move was deliberately parked back to the
// session junction: state kept, position kept, resumable through the normal
// advance path. The event is forensic — nothing about the instance changes —
// but it makes the shelving auditable and legible to a resuming agent.
func (s *Session) Park(instanceID, note string) error {
	if err := s.checkSink(); err != nil {
		return err
	}
	inst, ok := s.instances[instanceID]
	if !ok {
		return fmt.Errorf("instance %q not found in session", instanceID)
	}
	if inst.Status != StatusRunning {
		return fmt.Errorf("instance %s has ended (%s) — only running moves park", instanceID, inst.Outcome)
	}
	data := map[string]any{"step": inst.Step}
	if note != "" {
		data["note"] = note
	}
	s.appendEvent(inst.ID, EventParked, data)
	return nil
}

// appendEvent writes one event to the log. Sink errors fail loudly at the
// call sites that can surface them; here the event is built centrally.
func (s *Session) appendEvent(instance string, typ EventType, data map[string]any) {
	if s.sink == nil {
		return
	}
	s.seq++
	// The sink owning durability means an append error must not corrupt the
	// in-memory run; it is carried on the session and surfaced by the next
	// advance call.
	if err := s.sink.Append(Event{
		V:        LogVersion,
		TS:       s.now(),
		Session:  s.ID,
		Seq:      s.seq,
		Instance: instance,
		Event:    typ,
		Data:     data,
	}); err != nil && s.sinkErr == nil {
		s.sinkErr = err
	}
}

// Instances returns the session's instances in start order.
func (s *Session) Instances() []*Instance {
	out := make([]*Instance, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.instances[id])
	}
	return out
}

// Instance returns a session instance by ID.
func (s *Session) Instance(id string) (*Instance, bool) {
	inst, ok := s.instances[id]
	return inst, ok
}

// Start begins a procedure instance from a loaded spec with typed params,
// then cascades and serves. Parent links a sub-procedure to its spawning
// instance.
func (s *Session) Start(spec *Spec, params map[string]any, parent string) (*Serve, error) {
	if err := s.checkSink(); err != nil {
		return nil, err
	}
	if len(spec.Steps) == 0 {
		return nil, fmt.Errorf("procedure %s: no steps", spec.Canonical)
	}
	if parent != "" {
		if _, ok := s.instances[parent]; !ok {
			return nil, fmt.Errorf("parent instance %q not found in session", parent)
		}
	}

	s.counter++
	inst := &Instance{
		ID:     fmt.Sprintf("i_%d", s.counter),
		Spec:   spec,
		Store:  NewStore(spec),
		Step:   spec.Steps[0].ID,
		Status: StatusRunning,
		Parent: parent,
	}
	if err := inst.Store.SetStart(params); err != nil {
		return nil, fmt.Errorf("start %s: %w", spec.Canonical, err)
	}
	s.instances[inst.ID] = inst
	s.order = append(s.order, inst.ID)

	seed, err := s.seedFromParent(inst, parent)
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", spec.Canonical, err)
	}

	data := map[string]any{
		"procedure": spec.Canonical,
		"entry":     spec.EntryID,
		"step":      inst.Step,
	}
	// Shell-ness rides the log so descriptors stay self-derived (no spec
	// resolution when listing); absent means move, which covers old logs.
	if spec.Class == model.ProcedureClassShell {
		data["class"] = string(spec.Class)
	}
	if len(params) > 0 {
		data["params"] = params
	}
	if parent != "" {
		data["parent"] = parent
	}
	// Seeds ride the started event so replay restores the inherited grounding
	// — a parked-and-resumed child keeps the record its gate passed on.
	if len(seed) > 0 {
		data["seed"] = seed
	}
	s.appendEvent(inst.ID, EventStarted, data)

	if err := s.cascade(inst); err != nil {
		return nil, err
	}
	return s.serve(inst)
}

// seedFromParent copies grounding down the dispatch edge from a spawning
// parent's store into the child's declared state, returning the seeded values
// for the started-event log. The mapping is whatever the parent's most recently
// answered dispatching junction declared (dispatchSeed) — there is no default,
// so a child dispatched on a path that answered no seed-bearing option inherits
// nothing. When the answered option named a procedure, the seed applies only to
// a child of that procedure. For each child field ← parent field pair, a seed
// lands only when the child declares the field as state, has not already set it
// (a caller override wins), and the parent holds a non-empty source value.
func (s *Session) seedFromParent(inst *Instance, parent string) (map[string]any, error) {
	if parent == "" {
		return nil, nil
	}
	p, ok := s.instances[parent]
	if !ok {
		return nil, nil // parent existence is validated by the caller (Start)
	}
	if len(p.dispatchSeed) == 0 {
		return nil, nil // no seed-bearing junction was answered on the dispatch path
	}
	if p.dispatchProcedure != "" && p.dispatchProcedure != inst.Spec.Canonical {
		return nil, nil // the pending seed was declared for a different child procedure
	}

	seed := make(map[string]any)
	for childField, parentField := range p.dispatchSeed {
		if _, declared := inst.Spec.State[childField]; !declared {
			continue // the child must declare it as report-writable state to receive it
		}
		if inst.Store.Has(childField) {
			continue // already set on this instance — never overwrite
		}
		if !p.Store.Has(parentField) {
			continue // the parent has no evidence to hand down under this name
		}
		v, _ := p.Store.Get(parentField)
		seed[childField] = v
	}

	if len(seed) == 0 {
		return nil, nil
	}
	if _, err := inst.Store.WriteState(seed); err != nil {
		return nil, fmt.Errorf("seeding from parent %s: %w", parent, err)
	}
	return seed, nil
}

// recordDispatchSeed sets the parent instance's dispatch context to exactly
// what the just-answered option declares: a dispatch option stashes its
// mapping (so the next child dispatched under this parent inherits it), and an
// option carrying no handoff clears any prior stash. The clear matters on a
// multi-junction parent — after answering a seed-bearing option, a later answer
// to a plain option must not leave a stale mapping a subsequent child would
// inherit though its own junction never granted it. Called for every answered
// option, live and on replay, so both agree from the logged chooser answers.
// Not consume-once: a junction that fans out several children — evaluate
// recording several findings — seeds each until another option changes it.
func recordDispatchSeed(inst *Instance, opt *Option) {
	if opt.Dispatch != nil && len(opt.Dispatch.Seed) > 0 {
		inst.dispatchSeed = opt.Dispatch.Seed
		inst.dispatchProcedure = opt.Dispatch.Procedure
		return
	}
	inst.dispatchSeed = nil
	inst.dispatchProcedure = ""
}

// Report applies state fields from a transition report, re-evaluates, and
// cascades. Reports can only write declared state fields; batched fields for
// later steps are accepted. A report does not answer a pending chooser.
func (s *Session) Report(instanceID string, fields map[string]any) (*Serve, error) {
	if err := s.checkSink(); err != nil {
		return nil, err
	}
	inst, ok := s.instances[instanceID]
	if !ok {
		return nil, fmt.Errorf("instance %q not found in session", instanceID)
	}
	if inst.Status != StatusRunning {
		return nil, fmt.Errorf("instance %s has ended (%s)", instanceID, inst.Outcome)
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty report — send state fields or a chooser answer")
	}

	written, err := inst.Store.WriteState(fields)
	if err != nil {
		return nil, err
	}
	logged := make(map[string]any, len(written))
	for _, name := range written {
		v, _ := inst.Store.Get(name)
		logged[name] = v
	}
	s.appendEvent(inst.ID, EventReport, map[string]any{
		"step":   inst.Step,
		"fields": logged,
	})

	if err := s.cascade(inst); err != nil {
		return nil, err
	}
	return s.serve(inst)
}

// Answer resolves the pending chooser at the instance's current step: the
// named chooser must be the current step (no early, late, or double
// answers), the choice must be one of its options, and any carried fields
// are limited to the option's collect list. User answers carry the user's
// words verbatim for the auditable-relay record.
func (s *Session) Answer(instanceID, chooser, choice string, fields map[string]any, userWords string) (*Serve, error) {
	if err := s.checkSink(); err != nil {
		return nil, err
	}
	inst, ok := s.instances[instanceID]
	if !ok {
		return nil, fmt.Errorf("instance %q not found in session", instanceID)
	}
	if inst.Status != StatusRunning {
		return nil, fmt.Errorf("instance %s has ended (%s)", instanceID, inst.Outcome)
	}
	step := inst.currentStep()
	if step == nil {
		return nil, fmt.Errorf("instance %s: step %q not found", instanceID, inst.Step)
	}
	if step.Chooser == ChooserGate {
		return nil, fmt.Errorf("step %s is a gate — no chooser is pending", step.ID)
	}
	if chooser != step.ID {
		return nil, fmt.Errorf("chooser %q is not pending — the pending chooser is %q (answers are validated: no early, late, or double answers)", chooser, step.ID)
	}

	var opt *Option
	for i := range step.Options {
		if step.Options[i].Choice == choice {
			opt = &step.Options[i]
			break
		}
	}
	if opt == nil {
		choices := make([]string, 0, len(step.Options))
		for _, o := range step.Options {
			choices = append(choices, o.Choice)
		}
		sort.Strings(choices)
		return nil, fmt.Errorf("choice %q is not an option at %s (options: %s)", choice, step.ID, joinNames(choices))
	}

	// Fields on an answer are limited to the option's collect list — the
	// same state-only trust boundary as reports, narrowed further.
	if len(fields) > 0 {
		allowed := make(map[string]bool, len(opt.Collect))
		for _, cf := range opt.Collect {
			allowed[cf.Name] = true
		}
		for name := range fields {
			if !allowed[name] {
				return nil, fmt.Errorf("field %q is not collected by option %q", name, choice)
			}
		}
		if _, err := inst.Store.WriteState(fields); err != nil {
			return nil, err
		}
	}
	for _, cf := range opt.Collect {
		if cf.Optional {
			continue
		}
		if !inst.Store.Has(cf.Name) {
			return nil, fmt.Errorf("option %q requires field %q", choice, cf.Name)
		}
	}

	data := map[string]any{
		"chooser": step.ID,
		"choice":  choice,
		"kind":    string(step.Chooser),
	}
	if userWords != "" {
		data["userWords"] = userWords
	}
	if len(fields) > 0 {
		data["fields"] = fields
	}
	s.appendEvent(inst.ID, EventChooserAnswer, data)

	// A dispatching junction's declared handoff rides the answered option:
	// stash it so the next child started under this instance inherits it.
	recordDispatchSeed(inst, opt)

	if opt.Call != "" {
		if err := s.runCommand(inst, opt.Call); err != nil {
			return nil, err
		}
	}
	if err := s.transitionTo(inst, opt.To, false); err != nil {
		return nil, err
	}
	if err := s.cascade(inst); err != nil {
		return nil, err
	}
	return s.serve(inst)
}

// Serve re-serves the instance's current position without advancing it —
// shells use it to rehydrate an agent after resume_session.
func (s *Session) Serve(instanceID string) (*Serve, error) {
	if err := s.checkSink(); err != nil {
		return nil, err
	}
	inst, ok := s.instances[instanceID]
	if !ok {
		return nil, fmt.Errorf("instance %q not found in session", instanceID)
	}
	return s.serve(inst)
}

// Abandon explicitly discards a running instance, logged as an abandonment
// transition. It never cleans up implicitly — anything the instance holds
// (a WIP marker, staged attachments) is left standing for resume or groom.
func (s *Session) Abandon(instanceID, reason string) error {
	if err := s.checkSink(); err != nil {
		return err
	}
	inst, ok := s.instances[instanceID]
	if !ok {
		return fmt.Errorf("instance %q not found in session", instanceID)
	}
	if inst.Status != StatusRunning {
		return fmt.Errorf("instance %s has already ended (%s)", instanceID, inst.Outcome)
	}
	inst.Status = StatusAbandoned
	inst.Outcome = "abandoned"
	data := map[string]any{}
	if reason != "" {
		data["reason"] = reason
	}
	s.appendEvent(inst.ID, EventAbandoned, data)
	return nil
}

// SpecResolver resolves a procedure canonical to its loaded spec — replay
// uses it to rebind logged instances to their procedure definitions.
type SpecResolver func(canonical string) (*Spec, error)

// ReplaySession reconstructs a session by folding its event log: state is
// applied directly from the logged values — reports, op results, and
// transitions — never by re-running commands, so replay is free of side
// effects. The returned session continues appending to sink.
func (e *Engine) ReplaySession(id, participant string, events []Event, resolve SpecResolver, sink EventSink, opts ...SessionOption) (*Session, error) {
	s := e.NewSession(id, participant, nil, opts...)
	for _, ev := range events {
		if ev.V != LogVersion {
			return nil, fmt.Errorf("session log event seq %d has version %d, this sdd speaks version %d — sessions do not survive an sdd upgrade mid-flight", ev.Seq, ev.V, LogVersion)
		}
		if ev.Seq > s.seq {
			s.seq = ev.Seq
		}
		if err := s.applyEvent(ev, resolve); err != nil {
			return nil, fmt.Errorf("replaying seq %d (%s): %w", ev.Seq, ev.Event, err)
		}
	}
	s.sink = sink
	return s, nil
}

func (s *Session) applyEvent(ev Event, resolve SpecResolver) error {
	switch ev.Event {
	case EventSessionMeta:
		if p, ok := ev.Data["participant"].(string); ok && p != "" {
			s.Participant = p
		}

	case EventLabeled:
		if l, ok := ev.Data["label"].(string); ok {
			s.Label = l
		}

	case EventStarted:
		canonical, _ := ev.Data["procedure"].(string)
		entryID, _ := ev.Data["entry"].(string)
		step, _ := ev.Data["step"].(string)
		parent, _ := ev.Data["parent"].(string)
		spec, err := resolve(canonical)
		if err != nil {
			return err
		}
		if entryID != "" && spec.EntryID != entryID {
			return fmt.Errorf("procedure %s resolved to %s, but the session ran %s — the procedure changed underneath the session", canonical, spec.EntryID, entryID)
		}
		if spec.StepByID[step] == nil {
			return fmt.Errorf("procedure %s has no step %q", canonical, step)
		}
		inst := &Instance{
			ID:     ev.Instance,
			Spec:   spec,
			Store:  NewStore(spec),
			Step:   step,
			Status: StatusRunning,
			Parent: parent,
		}
		if params, ok := ev.Data["params"].(map[string]any); ok {
			if err := inst.Store.SetStart(params); err != nil {
				return err
			}
		} else if err := inst.Store.SetStart(nil); err != nil {
			return err
		}
		// Restore the grounding seeded from the parent at dispatch — logged on
		// the started event, re-applied here so a resumed child keeps the
		// record its gate passed on (no re-widen after a restart).
		if seed, ok := ev.Data["seed"].(map[string]any); ok {
			if _, err := inst.Store.WriteState(seed); err != nil {
				return err
			}
		}
		s.instances[inst.ID] = inst
		s.order = append(s.order, inst.ID)
		if n := instanceCounter(ev.Instance); n > s.counter {
			s.counter = n
		}

	case EventReport:
		inst, err := s.replayInstance(ev)
		if err != nil {
			return err
		}
		fields, _ := ev.Data["fields"].(map[string]any)
		if _, err := inst.Store.WriteState(fields); err != nil {
			return err
		}

	case EventChooserAnswer:
		inst, err := s.replayInstance(ev)
		if err != nil {
			return err
		}
		// The answer's own effects were logged separately (op_result,
		// transition); collected fields ride the answer event.
		if fields, ok := ev.Data["fields"].(map[string]any); ok {
			if _, err := inst.Store.WriteState(fields); err != nil {
				return err
			}
		}
		// Re-derive the dispatch handoff the answered option declares — it lives
		// in the spec, keyed by (step, choice) the answer already logs, so no
		// dedicated log field is needed. Only matters for a child dispatched
		// after resume; a child already dispatched restores its own seed from
		// its started event.
		if chooser, _ := ev.Data["chooser"].(string); chooser != "" {
			if step := inst.Spec.StepByID[chooser]; step != nil {
				choice, _ := ev.Data["choice"].(string)
				for i := range step.Options {
					if step.Options[i].Choice == choice {
						recordDispatchSeed(inst, &step.Options[i])
						break
					}
				}
			}
		}

	case EventOpResult:
		inst, err := s.replayInstance(ev)
		if err != nil {
			return err
		}
		writes, _ := ev.Data["writes"].(map[string]any)
		for name, v := range writes {
			if err := inst.Store.importValue(name, ExportedValue{Value: v, Provenance: ProvenanceEngine}); err != nil {
				return err
			}
		}

	case EventTransition:
		inst, err := s.replayInstance(ev)
		if err != nil {
			return err
		}
		to, _ := ev.Data["to"].(string)
		switch {
		case IsEndTarget(to):
			// The paired completed/abandoned event carries the terminal
			// status; nothing to move.
		case inst.Spec.StepByID[to] != nil:
			inst.Step = to
			inst.opDone = false
		default:
			return fmt.Errorf("transition target %q not in procedure %s", to, inst.Spec.Canonical)
		}

	case EventCompleted:
		inst, err := s.replayInstance(ev)
		if err != nil {
			return err
		}
		inst.Status = StatusCompleted
		inst.Outcome = "completed"

	case EventAbandoned:
		inst, err := s.replayInstance(ev)
		if err != nil {
			return err
		}
		inst.Status = StatusAbandoned
		inst.Outcome = "abandoned"

	case EventRead:
		// Restore the read set so a resumed session keeps the inspection
		// evidence its gates passed on (or will pass on).
		for _, key := range []struct {
			field string
			depth ReadDepth
		}{{"full", ReadFull}, {"summary", ReadSummary}} {
			if ids, ok := ev.Data[key.field].([]any); ok {
				for _, v := range ids {
					if id, ok := v.(string); ok && id != "" {
						if s.reads[id] != ReadFull {
							s.reads[id] = key.depth
						}
					}
				}
			}
		}

	case EventServed, EventParked:
		// Forensic only — no state effect.

	default:
		return fmt.Errorf("unknown event type %q", ev.Event)
	}
	return nil
}

func (s *Session) replayInstance(ev Event) (*Instance, error) {
	inst, ok := s.instances[ev.Instance]
	if !ok {
		return nil, fmt.Errorf("instance %q not started", ev.Instance)
	}
	return inst, nil
}

// instanceCounter extracts the numeric part of an i_N instance handle so a
// replayed session keeps allocating fresh handles.
func instanceCounter(id string) int {
	var n int
	if _, err := fmt.Sscanf(id, "i_%d", &n); err != nil {
		return 0
	}
	return n
}

// checkSink surfaces a deferred log-append failure before advancing.
func (s *Session) checkSink() error {
	if s.sinkErr != nil {
		return fmt.Errorf("session log append failed earlier — refusing to advance without durability: %w", s.sinkErr)
	}
	return nil
}
