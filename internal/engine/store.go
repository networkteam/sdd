package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
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

// Clone creates an isolated store for candidate mutation.
func (s *Store) Clone() *Store {
	values := make(map[string]storeValue, len(s.values))
	for name, value := range s.values {
		value.Value = cloneStoreValue(value.Value)
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
	s.values = candidate.Clone().values
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
			candidate.values[name] = storeValue{Value: v, Provenance: ProvenanceState}
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
// report.
func (s *Store) WriteEngine(name string, v any) {
	s.values[name] = storeValue{Value: cloneStoreValue(v), Provenance: ProvenanceEngine}
	if s.engineJournal != nil {
		s.engineJournal[name] = cloneStoreValue(v)
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
	return cloneStoreValue(sv.Value), true
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
		ctx[name] = cloneStoreValue(sv.Value)
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
		out[name] = cloneStoreValue(sv.Value)
	}
	return out
}

// Export returns all values with provenance for session-log persistence.
func (s *Store) Export() map[string]ExportedValue {
	out := make(map[string]ExportedValue, len(s.values))
	for name, sv := range s.values {
		sv.Value = cloneStoreValue(sv.Value)
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
		s.values[name] = storeValue{Value: cloneStoreValue(ev.Value), Provenance: ProvenanceEngine}
	default:
		return fmt.Errorf("replayed field %q has unknown provenance %q", name, ev.Provenance)
	}
	return nil
}

// StoreValueCloner lets domain values preserve their type-specific ownership.
type StoreValueCloner interface {
	CloneStoreValue() any
}

// Store values cross API boundaries by value so transactions own their state.
func cloneStoreValue(value any) any {
	if value == nil {
		return nil
	}
	return cloneStoreReflect(reflect.ValueOf(value), map[cloneVisit]bool{}).Interface()
}

type cloneVisit struct {
	typeOf  reflect.Type
	pointer uintptr
}

func cloneStoreReflect(value reflect.Value, active map[cloneVisit]bool) reflect.Value {
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		panic(fmt.Sprintf("engine store value kind %s is not JSON-persistable", value.Kind()))
	}
	if value.CanInterface() {
		dynamic := reflect.ValueOf(value.Interface())
		if dynamic.IsValid() && !isNilValue(dynamic) {
			cloner, ok := dynamic.Interface().(StoreValueCloner)
			if ok {
				cloned := reflect.ValueOf(cloner.CloneStoreValue())
				if !cloned.IsValid() || cloned.Type() != dynamic.Type() || !cloned.Type().AssignableTo(value.Type()) {
					panic(fmt.Sprintf("StoreValueCloner for %s must preserve its concrete type", dynamic.Type()))
				}
				return cloned
			}
		}
	}
	if visit, ok := cloneReferenceVisit(value); ok {
		if active[visit] {
			panic("engine store value contains a reference cycle; values must be JSON-persistable")
		}
		active[visit] = true
		defer delete(active, visit)
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneStoreReflect(value.Elem(), active)
		out := reflect.New(value.Type()).Elem()
		out.Set(cloned)
		return out
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type().Elem())
		out.Elem().Set(cloneStoreReflect(value.Elem(), active))
		return out
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			out.SetMapIndex(cloneStoreReflect(iter.Key(), active), cloneStoreReflect(iter.Value(), active))
		}
		return out
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := range value.Len() {
			out.Index(i).Set(cloneStoreReflect(value.Index(i), active))
		}
		return out
	case reflect.Array:
		out := reflect.New(value.Type()).Elem()
		for i := range value.Len() {
			out.Index(i).Set(cloneStoreReflect(value.Index(i), active))
		}
		return out
	case reflect.Struct:
		out := reflect.New(value.Type()).Elem()
		out.Set(value)
		for i := range value.NumField() {
			field := value.Type().Field(i)
			if field.PkgPath != "" {
				if carriesMutableAlias(field.Type, map[reflect.Type]bool{}) {
					panic(fmt.Sprintf("engine store value %s has unexported mutable field %s; implement StoreValueCloner", value.Type(), field.Name))
				}
				continue
			}
			out.Field(i).Set(cloneStoreReflect(value.Field(i), active))
		}
		return out
	default:
		return value
	}
}

func isNilValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func cloneReferenceVisit(value reflect.Value) (cloneVisit, bool) {
	if isNilValue(value) {
		return cloneVisit{}, false
	}
	var pointer uintptr
	switch value.Kind() {
	case reflect.Map:
		pointer = uintptr(value.UnsafePointer())
	case reflect.Pointer:
		pointer = value.Pointer()
	case reflect.Slice:
		if value.Len() == 0 {
			return cloneVisit{}, false
		}
		pointer = value.Pointer()
	default:
		return cloneVisit{}, false
	}
	return cloneVisit{typeOf: value.Type(), pointer: pointer}, true
}

func carriesMutableAlias(valueType reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[valueType] {
		return false
	}
	seen[valueType] = true
	switch valueType.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Slice, reflect.Pointer, reflect.Interface, reflect.UnsafePointer:
		return true
	case reflect.Array:
		return carriesMutableAlias(valueType.Elem(), seen)
	case reflect.Struct:
		for i := range valueType.NumField() {
			if carriesMutableAlias(valueType.Field(i).Type, seen) {
				return true
			}
		}
	}
	return false
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
