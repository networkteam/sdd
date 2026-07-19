package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Provenance classifies who wrote a store value. The split carries the trust
// property from the surface spec: reports can only write declared state
// fields; params are fixed at start; everything trust-bearing (created IDs,
// findings, confirmation records) is engine-written via ops and chooser
// calls, structurally out of a report's reach.
type Provenance string

const (
	ProvenanceParam  Provenance = "param"
	ProvenanceState  Provenance = "state"
	ProvenanceEngine Provenance = "engine"
)

type storeValue struct {
	Value      any
	Provenance Provenance
}

// Store is the typed variable store of one procedure instance. Declared
// params and state come from the spec; engine-written values enter through
// command contracts (their Writes set) and are validated against declaration
// collisions at spec load time, not here.
type Store struct {
	spec   *Spec
	values map[string]storeValue
	// engineJournal collects WriteEngine calls between beginJournal and
	// drainJournal, so a command's writes land in its op_result log event —
	// the fold replay restores them without re-running the side effect.
	engineJournal map[string]any
}

// NewStore creates an empty store bound to the spec that declares its
// params and state fields.
func NewStore(spec *Spec) *Store {
	return &Store{
		spec:   spec,
		values: make(map[string]storeValue),
	}
}

// SetStart applies the start-time input map, the shared seeding primitive
// (d-tac-tlo): a key naming a declared param sets that param (fixed at
// start), a key naming a declared state field seeds it — a start-time state
// write, equivalent to an immediate report, so an entry gate reading it is
// satisfied on entry. Move dispatch and direct start share this: a caller
// passing an anchor a procedure declares as state seeds the resolver, and the
// parent handoff seeds the same fields from the parent's store. Unknown keys
// are rejected.
func (s *Store) SetStart(inputs map[string]any) error {
	params := make(map[string]any)
	seed := make(map[string]any)
	for name, v := range inputs {
		switch {
		case s.isParam(name):
			params[name] = v
		case s.isState(name):
			seed[name] = v
		default:
			return fmt.Errorf("unknown start input %q (params: %s; seedable state: %s)", name, joinNames(s.spec.paramNames()), joinNames(s.spec.stateNames()))
		}
	}
	if err := s.SetParams(params); err != nil {
		return err
	}
	if len(seed) > 0 {
		if _, err := s.WriteState(seed); err != nil {
			return err
		}
	}
	return s.applyStateDefaults()
}

// applyStateDefaults writes each declared state field's Default for fields the
// caller left unset. Deterministic from the spec, so live start and replay
// (both routed through SetStart) agree without the value ever being logged. A
// field with a default therefore won't receive a later parent seed — the
// default counts as already-set — which is exactly right for a constant a
// procedure carries and seeds outward rather than one it inherits.
//
// A default is only safe on a NEW procedure or a NEW field: adding one to an
// existing field retroactively would apply it on replay of sessions that ran
// before it existed, silently rewriting their recovered state.
func (s *Store) applyStateDefaults() error {
	defaults := make(map[string]any)
	for name, decl := range s.spec.State {
		if decl.Default == nil {
			continue
		}
		if _, ok := s.values[name]; ok {
			continue
		}
		defaults[name] = decl.Default
	}
	if len(defaults) == 0 {
		return nil
	}
	_, err := s.WriteState(defaults)
	return err
}

func (s *Store) isParam(name string) bool {
	_, ok := s.spec.Params[name]
	return ok
}

func (s *Store) isState(name string) bool {
	_, ok := s.spec.State[name]
	return ok
}

// SetParams validates and writes the start-time params. Required params must
// be present; unknown params are rejected.
func (s *Store) SetParams(params map[string]any) error {
	for name, raw := range params {
		decl, ok := s.spec.Params[name]
		if !ok {
			return fmt.Errorf("unknown param %q (declared: %s)", name, joinNames(s.spec.paramNames()))
		}
		v, err := decl.Type.ValidateValue(raw)
		if err != nil {
			return fmt.Errorf("param %q: %w", name, err)
		}
		s.values[name] = storeValue{Value: v, Provenance: ProvenanceParam}
	}
	for name, decl := range s.spec.Params {
		if decl.Optional {
			continue
		}
		if _, ok := s.values[name]; !ok {
			return fmt.Errorf("missing required param %q", name)
		}
	}
	return nil
}

// WriteState validates and writes report-supplied state fields. Only fields
// declared in the spec's state block are writable — this is the report trust
// boundary. Returns the names actually written, sorted.
func (s *Store) WriteState(fields map[string]any) ([]string, error) {
	written := make([]string, 0, len(fields))
	for name, raw := range fields {
		decl, ok := s.spec.State[name]
		if !ok {
			if _, isParam := s.spec.Params[name]; isParam {
				return nil, fmt.Errorf("field %q is a param — params are set at start and not report-writable", name)
			}
			return nil, fmt.Errorf("field %q is not declared in state — reports can only write declared state fields", name)
		}
		v, err := decl.Type.ValidateValue(raw)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", name, err)
		}
		s.values[name] = storeValue{Value: v, Provenance: ProvenanceState}
		written = append(written, name)
	}
	sort.Strings(written)
	return written, nil
}

// WriteEngine writes an engine-produced value (an op result or chooser-call
// effect) per the producing function's Go contract. Never reachable from a
// report.
func (s *Store) WriteEngine(name string, v any) {
	s.values[name] = storeValue{Value: v, Provenance: ProvenanceEngine}
	if s.engineJournal != nil {
		s.engineJournal[name] = v
	}
}

// beginJournal starts collecting engine writes for the next command run.
func (s *Store) beginJournal() {
	s.engineJournal = make(map[string]any)
}

// drainJournal returns the collected engine writes and stops journaling.
func (s *Store) drainJournal() map[string]any {
	j := s.engineJournal
	s.engineJournal = nil
	return j
}

// Get returns the value and whether it is present.
func (s *Store) Get(name string) (any, bool) {
	sv, ok := s.values[name]
	if !ok {
		return nil, false
	}
	return sv.Value, true
}

// Has reports whether the field is present and non-empty. Empty strings,
// empty lists, and nil don't count as present — a presence predicate asking
// hasBody wants substance, not a key.
func (s *Store) Has(name string) bool {
	sv, ok := s.values[name]
	if !ok {
		return false
	}
	switch v := sv.Value.(type) {
	case nil:
		return false
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	default:
		return true
	}
}

// TemplateContext exposes every store value by name for instruction-unit and
// inject-arg template rendering.
func (s *Store) TemplateContext() map[string]any {
	ctx := make(map[string]any, len(s.values))
	for name, sv := range s.values {
		ctx[name] = sv.Value
	}
	return ctx
}

// StateSnapshot returns a deterministic fingerprint over the report-writable
// state values. confirmPlayback binds a confirmation to this fingerprint;
// playbackConfirmed compares it — any state edit after confirmation changes
// the snapshot and reopens playback.
func (s *Store) StateSnapshot() string {
	names := make([]string, 0, len(s.values))
	for name, sv := range s.values {
		if sv.Provenance == ProvenanceState {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		b, err := json.Marshal(s.values[name].Value)
		if err != nil {
			// Store values are JSON-representable by construction (they
			// arrive via JSON reports or validated Go values); a marshal
			// failure still must not silently alias two different states.
			b = fmt.Appendf(nil, "!err:%v", err)
		}
		fmt.Fprintf(h, "%s=%s\n", name, b)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Export returns all values with provenance for session-log persistence.
func (s *Store) Export() map[string]ExportedValue {
	out := make(map[string]ExportedValue, len(s.values))
	for name, sv := range s.values {
		out[name] = ExportedValue(sv)
	}
	return out
}

// ExportedValue is the persisted form of one store value.
type ExportedValue struct {
	Value      any        `json:"value"`
	Provenance Provenance `json:"provenance"`
}

// importValue restores a persisted value during replay, re-validating
// declared fields so a tampered log cannot smuggle malformed values past the
// type layer. Engine-written fields are restored verbatim (their shape is
// owned by the producing function's contract).
func (s *Store) importValue(name string, ev ExportedValue) error {
	switch ev.Provenance {
	case ProvenanceParam:
		decl, ok := s.spec.Params[name]
		if !ok {
			return fmt.Errorf("replayed param %q is not declared", name)
		}
		v, err := decl.Type.ValidateValue(ev.Value)
		if err != nil {
			return fmt.Errorf("replayed param %q: %w", name, err)
		}
		s.values[name] = storeValue{Value: v, Provenance: ProvenanceParam}
	case ProvenanceState:
		decl, ok := s.spec.State[name]
		if !ok {
			return fmt.Errorf("replayed state field %q is not declared", name)
		}
		v, err := decl.Type.ValidateValue(ev.Value)
		if err != nil {
			return fmt.Errorf("replayed state field %q: %w", name, err)
		}
		s.values[name] = storeValue{Value: v, Provenance: ProvenanceState}
	case ProvenanceEngine:
		s.values[name] = storeValue{Value: ev.Value, Provenance: ProvenanceEngine}
	default:
		return fmt.Errorf("replayed field %q has unknown provenance %q", name, ev.Provenance)
	}
	return nil
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
