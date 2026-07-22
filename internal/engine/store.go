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

// Clone creates an isolated store for candidate mutation. Every stored value
// is a normalized JSON document, so a container-recursion copy fully isolates
// the candidate — a rejected batch leaves the source untouched.
func (s *Store) Clone() *Store {
	values := make(map[string]storeValue, len(s.values))
	for name, value := range s.values {
		value.Value = copyStoreValue(value.Value)
		values[name] = value
	}
	return &Store{spec: s.spec, values: values}
}

func (s *Store) transact(apply func(*Store) error) error {
	if s.engineJournal != nil {
		return fmt.Errorf("commands may mutate the store only through WriteEngine")
	}
	candidate := s.Clone()
	if err := apply(candidate); err != nil {
		return err
	}
	s.commit(candidate)
	return nil
}

func (s *Store) commit(candidate *Store) {
	// The candidate is already an isolated copy (Clone at transaction start,
	// fresh normalized values per write) and is discarded after adoption, so
	// no further copy is needed.
	s.values = candidate.values
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
	return s.transact(func(candidate *Store) error {
		if err := candidate.setParams(params); err != nil {
			return err
		}
		if len(seed) > 0 {
			_, err := candidate.writeState(seed, nil)
			return err
		}
		return nil
	})
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
	return s.transact(func(candidate *Store) error {
		return candidate.setParams(params)
	})
}

func (s *Store) setParams(params map[string]any) error {
	for name, raw := range params {
		decl, ok := s.spec.Params[name]
		if !ok {
			return fmt.Errorf("unknown param %q (declared: %s)", name, joinNames(s.spec.paramNames()))
		}
		v, err := decl.Type.ValidateValue(raw)
		if err != nil {
			return fmt.Errorf("param %q: %w", name, err)
		}
		nv, err := normalizeStoreValue(v)
		if err != nil {
			return fmt.Errorf("param %q: %w", name, err)
		}
		s.values[name] = storeValue{Value: nv, Provenance: ProvenanceParam}
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
	return s.writeState(fields, nil)
}

func (s *Store) writeState(fields map[string]any, validate func(*Store) error) ([]string, error) {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	err := s.transact(func(candidate *Store) error {
		for _, name := range names {
			raw := fields[name]
			decl, ok := candidate.spec.State[name]
			if !ok {
				if _, isParam := candidate.spec.Params[name]; isParam {
					return fmt.Errorf("field %q is a param — params are set at start and not report-writable", name)
				}
				return fmt.Errorf("field %q is not declared in state — reports can only write declared state fields", name)
			}
			if raw == nil {
				if !decl.Optional {
					return fmt.Errorf("field %q is required and cannot be cleared", name)
				}
				delete(candidate.values, name)
				continue
			}
			v, err := decl.Type.ValidateValue(raw)
			if err != nil {
				return fmt.Errorf("field %q: %w", name, err)
			}
			nv, err := normalizeStoreValue(v)
			if err != nil {
				return fmt.Errorf("field %q: %w", name, err)
			}
			candidate.values[name] = storeValue{Value: nv, Provenance: ProvenanceState}
		}
		if validate != nil {
			return validate(candidate)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

// WriteEngine writes an engine-produced value (an op result or chooser-call
// effect) per the producing function's Go contract. Never reachable from a
// report. The value is normalized to its JSON document form so pre-restart
// reads match the replayed form; a value that cannot marshal is a loud
// write-time error, leaving the store unchanged.
func (s *Store) WriteEngine(name string, v any) error {
	nv, err := normalizeStoreValue(v)
	if err != nil {
		return fmt.Errorf("engine write %q: %w", name, err)
	}
	s.values[name] = storeValue{Value: nv, Provenance: ProvenanceEngine}
	if s.engineJournal != nil {
		s.engineJournal[name] = copyStoreValue(nv)
	}
	return nil
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
	return copyStoreValue(sv.Value), true
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
		ctx[name] = copyStoreValue(sv.Value)
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

// Collected returns the param- and state-provenance values that are present
// and non-empty, excluding internal trust machinery. It is a resuming agent's
// view of what this instance has already gathered — the anchor, chosen scope,
// and reported judgments that persist across a handover and must not be
// re-derived. Engine-produced trust records are structurally excluded by the
// provenance filter; the explicit trust-machinery skip keeps that guarantee
// even if such a field were ever param- or state-declared.
func (s *Store) Collected() map[string]any {
	out := make(map[string]any)
	for name, sv := range s.values {
		if sv.Provenance != ProvenanceParam && sv.Provenance != ProvenanceState {
			continue
		}
		if isTrustMachineryField(name) {
			continue
		}
		if !s.Has(name) {
			continue
		}
		out[name] = copyStoreValue(sv.Value)
	}
	return out
}

// Export returns all values with provenance for session-log persistence.
func (s *Store) Export() map[string]ExportedValue {
	out := make(map[string]ExportedValue, len(s.values))
	for name, sv := range s.values {
		sv.Value = copyStoreValue(sv.Value)
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
		nv, err := normalizeStoreValue(v)
		if err != nil {
			return fmt.Errorf("replayed param %q: %w", name, err)
		}
		s.values[name] = storeValue{Value: nv, Provenance: ProvenanceParam}
	case ProvenanceState:
		decl, ok := s.spec.State[name]
		if !ok {
			return fmt.Errorf("replayed state field %q is not declared", name)
		}
		v, err := decl.Type.ValidateValue(ev.Value)
		if err != nil {
			return fmt.Errorf("replayed state field %q: %w", name, err)
		}
		nv, err := normalizeStoreValue(v)
		if err != nil {
			return fmt.Errorf("replayed state field %q: %w", name, err)
		}
		s.values[name] = storeValue{Value: nv, Provenance: ProvenanceState}
	case ProvenanceEngine:
		nv, err := normalizeStoreValue(ev.Value)
		if err != nil {
			return fmt.Errorf("replayed engine field %q: %w", name, err)
		}
		s.values[name] = storeValue{Value: nv, Provenance: ProvenanceEngine}
	default:
		return fmt.Errorf("replayed field %q has unknown provenance %q", name, ev.Provenance)
	}
	return nil
}

// normalizeStoreValue reduces a value to its JSON document form — the store's
// contract is that every value is a JSON document. Marshalling then decoding
// into an untyped `any` yields exactly the persisted shape (map[string]any,
// []any, string, float64, bool, nil), so a value read before a restart matches
// the same value replayed from the log, and typed inputs (engine.Ref,
// engine.FactIndex, time.Time) collapse to that shape at the write boundary. A
// value that cannot marshal (a channel, a func) is a loud write-time error, not
// a panic and not a silent skip.
func normalizeStoreValue(value any) (any, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("store value is not JSON-persistable: %w", err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("store value did not round-trip through JSON: %w", err)
	}
	return out, nil
}

// copyStoreValue returns an isolated copy of a normalized store value so a
// caller cannot mutate store internals through a returned map or slice. Values
// are guaranteed JSON shapes by normalizeStoreValue, so a container-recursion
// walk suffices — scalars (string, float64, bool, nil) are immutable and pass
// through, and no marshal step is needed on the read path (marshal failures are
// already surfaced at write time).
func copyStoreValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = copyStoreValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = copyStoreValue(item)
		}
		return out
	default:
		return value
	}
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
