package engine

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/networkteam/sdd/internal/query"
)

type compositeStoreValue struct {
	Items    []any
	Metadata map[string]any
	Findings []query.Finding
	Nested   *struct{ Labels []string }
}

type ownedStoreValue struct{ items []string }

type unsafeOwnedStoreValue struct{ items []string }

type badCloneStoreValue struct{}

func (v *ownedStoreValue) CloneStoreValue() any {
	return &ownedStoreValue{items: append([]string(nil), v.items...)}
}

func (*badCloneStoreValue) CloneStoreValue() any { return &ownedStoreValue{} }

func TestWriteStateClearsOnlyOptionalFields(t *testing.T) {
	env := newFixtureEnv(t)
	store := NewStore(env.spec)
	if _, err := store.WriteState(map[string]any{
		"index": map[string]any{"title": "View grammar", "topic": "cli/ux"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get("index"); !ok {
		t.Fatal("optional index was not stored")
	}
	written, err := store.WriteState(map[string]any{"index": nil})
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 || written[0] != "index" {
		t.Fatalf("cleared fields = %v", written)
	}
	if _, ok := store.Get("index"); ok {
		t.Fatal("optional index was not cleared")
	}
	if _, err := store.WriteState(map[string]any{"body": nil}); err == nil || !strings.Contains(err.Error(), "cannot be cleared") {
		t.Fatalf("required-field clear error = %v", err)
	}
}

func atomicStoreSpec() *Spec {
	text := VarType{Base: TypeText}
	boolean := VarType{Base: TypeBool}
	return &Spec{
		Params: map[string]VarDecl{
			"zParam": {Type: text},
		},
		State: map[string]VarDecl{
			"aText":     {Type: text},
			"bOptional": {Type: text, Optional: true},
			"zBool":     {Type: boolean},
			"zRequired": {Type: text},
		},
	}
}

func seededAtomicStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore(atomicStoreSpec())
	if err := store.SetStart(map[string]any{
		"zParam": "fixed", "aText": "original", "bOptional": "present",
		"zBool": true, "zRequired": "required",
	}); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestWriteStateRejectsEveryInvalidBatchWithoutMutation(t *testing.T) {
	tests := map[string]struct {
		fields  map[string]any
		message string
	}{
		"required clear": {
			fields:  map[string]any{"aText": "changed", "zRequired": nil},
			message: `field "zRequired" is required`,
		},
		"invalid type": {
			fields:  map[string]any{"bOptional": nil, "zBool": "not a bool"},
			message: `field "zBool"`,
		},
		"unknown field": {
			fields:  map[string]any{"aText": "changed", "zzUnknown": "value"},
			message: `field "zzUnknown" is not declared`,
		},
		"parameter": {
			fields:  map[string]any{"aText": "changed", "zParam": "changed"},
			message: `field "zParam" is a param`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store := seededAtomicStore(t)
			before := store.Export()
			beforeSnapshot := store.StateSnapshot()
			written, err := store.WriteState(test.fields)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("WriteState error = %v", err)
			}
			if written != nil {
				t.Fatalf("written fields = %v", written)
			}
			if !reflect.DeepEqual(store.Export(), before) || store.StateSnapshot() != beforeSnapshot {
				t.Fatalf("failed batch changed store: before=%#v after=%#v", before, store.Export())
			}
		})
	}
}

func TestStartInputsAndParamsRejectBatchesWithoutMutation(t *testing.T) {
	t.Run("params", func(t *testing.T) {
		store := seededAtomicStore(t)
		before := store.Export()
		err := store.SetParams(map[string]any{"zParam": true})
		if err == nil || !strings.Contains(err.Error(), `param "zParam"`) {
			t.Fatalf("SetParams error = %v", err)
		}
		if !reflect.DeepEqual(store.Export(), before) {
			t.Fatalf("failed params changed store: before=%#v after=%#v", before, store.Export())
		}
	})

	t.Run("missing required param", func(t *testing.T) {
		spec := atomicStoreSpec()
		spec.Params["aOptionalParam"] = VarDecl{Type: VarType{Base: TypeText}, Optional: true}
		store := NewStore(spec)
		err := store.SetParams(map[string]any{"aOptionalParam": "written first"})
		if err == nil || !strings.Contains(err.Error(), `missing required param "zParam"`) {
			t.Fatalf("SetParams error = %v", err)
		}
		if len(store.Export()) != 0 {
			t.Fatalf("failed params changed store: %#v", store.Export())
		}
	})

	t.Run("params and state", func(t *testing.T) {
		store := seededAtomicStore(t)
		before := store.Export()
		err := store.SetStart(map[string]any{
			"zParam": "changed", "aText": "changed", "zBool": "not a bool",
		})
		if err == nil || !strings.Contains(err.Error(), `field "zBool"`) {
			t.Fatalf("SetStart error = %v", err)
		}
		if !reflect.DeepEqual(store.Export(), before) {
			t.Fatalf("failed start inputs changed store: before=%#v after=%#v", before, store.Export())
		}
	})
}

func TestStoreOwnsCompositeValuesAcrossAPIBoundaries(t *testing.T) {
	store := NewStore(&Spec{})
	input := &compositeStoreValue{
		Items:    []any{"original", map[string]any{"key": "value"}},
		Metadata: map[string]any{"nested": []any{"kept"}},
		Findings: []query.Finding{{Observation: "unchanged"}},
		Nested:   &struct{ Labels []string }{Labels: []string{"stable"}},
	}
	store.beginJournal()
	store.WriteEngine("composite", input)
	input.Items[0] = "caller mutation"
	input.Metadata["nested"].([]any)[0] = "caller mutation"
	input.Findings[0].Observation = "caller mutation"
	input.Nested.Labels[0] = "caller mutation"
	journal := store.drainJournal()
	if journal["composite"].(*compositeStoreValue).Items[0] != "original" {
		t.Fatalf("caller mutation reached journal: %#v", journal)
	}
	journal["composite"].(*compositeStoreValue).Items[0] = "journal mutation"

	got, _ := store.Get("composite")
	composite, ok := got.(*compositeStoreValue)
	if !ok {
		t.Fatalf("stored type = %T", got)
	}
	if composite.Items[0] != "original" || composite.Metadata["nested"].([]any)[0] != "kept" ||
		composite.Findings[0].Observation != "unchanged" || composite.Nested.Labels[0] != "stable" {
		t.Fatalf("caller mutation reached store: %#v", composite)
	}

	composite.Items[1].(map[string]any)["key"] = "get mutation"
	context := store.TemplateContext()
	context["composite"].(*compositeStoreValue).Nested.Labels[0] = "context mutation"
	exported := store.Export()
	exported["composite"].Value.(*compositeStoreValue).Findings[0].Observation = "export mutation"
	again, _ := store.Get("composite")
	if !reflect.DeepEqual(again, &compositeStoreValue{
		Items:    []any{"original", map[string]any{"key": "value"}},
		Metadata: map[string]any{"nested": []any{"kept"}},
		Findings: []query.Finding{{Observation: "unchanged"}},
		Nested:   &struct{ Labels []string }{Labels: []string{"stable"}},
	}) {
		t.Fatalf("egress mutation reached store: %#v", again)
	}
}

func TestStoreUsesDomainValueClone(t *testing.T) {
	store := NewStore(&Spec{})
	input := &ownedStoreValue{items: []string{"original"}}
	store.WriteEngine("owned", []any{map[string]any{"value": input}})
	input.items[0] = "caller mutation"
	got, _ := store.Get("owned")
	owned, ok := got.([]any)[0].(map[string]any)["value"].(*ownedStoreValue)
	if !ok || !reflect.DeepEqual(owned.items, []string{"original"}) {
		t.Fatalf("domain clone = %#v (%T)", got, got)
	}
}

func TestStorePreservesTypedNilDomainValues(t *testing.T) {
	store := NewStore(&Spec{})
	var input *ownedStoreValue
	store.WriteEngine("owned", []any{input})
	got, _ := store.Get("owned")
	owned, ok := got.([]any)[0].(*ownedStoreValue)
	if !ok || owned != nil {
		t.Fatalf("typed nil = %#v (%T)", got, got)
	}
}

func TestStoreRejectsDomainCloneTypeChanges(t *testing.T) {
	store := NewStore(&Spec{})
	defer func() {
		got := recover()
		if got == nil || !strings.Contains(fmt.Sprint(got), "preserve its concrete type") {
			t.Fatalf("panic = %v", got)
		}
	}()
	store.WriteEngine("bad", []any{&badCloneStoreValue{}})
}

func TestStoreRejectsUnexportedMutableFieldsWithoutDomainClone(t *testing.T) {
	store := NewStore(&Spec{})
	defer func() {
		got := recover()
		if got == nil || !strings.Contains(fmt.Sprint(got), "unexported mutable field items; implement StoreValueCloner") {
			t.Fatalf("panic = %v", got)
		}
	}()
	store.WriteEngine("unsafe", unsafeOwnedStoreValue{items: []string{"aliased"}})
}

func TestStoreRejectsReferenceCycles(t *testing.T) {
	store := NewStore(&Spec{})
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	defer func() {
		got := recover()
		if got == nil || !strings.Contains(fmt.Sprint(got), "reference cycle") {
			t.Fatalf("panic = %v", got)
		}
	}()
	store.WriteEngine("cyclic", cyclic)
}

func TestStoreRejectsUnsupportedReferenceValues(t *testing.T) {
	values := map[string]any{
		"channel":        make(chan int),
		"function":       func() {},
		"unsafe pointer": unsafe.Pointer(new(int)),
	}
	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			store := NewStore(&Spec{})
			defer func() {
				got := recover()
				if got == nil || !strings.Contains(fmt.Sprint(got), "not JSON-persistable") {
					t.Fatalf("panic = %v", got)
				}
			}()
			store.WriteEngine("unsupported", value)
		})
	}
}

func TestStoreCloneIsolatesCandidateValues(t *testing.T) {
	store := NewStore(&Spec{})
	store.WriteEngine("items", []any{map[string]any{"value": "original"}})
	candidate := store.Clone()
	candidate.values["items"].Value.([]any)[0].(map[string]any)["value"] = "candidate mutation"
	got, _ := store.Get("items")
	if !reflect.DeepEqual(got, []any{map[string]any{"value": "original"}}) {
		t.Fatalf("candidate mutation reached source store: %#v", got)
	}
}

func TestStoreCollectedOwnsReturnedState(t *testing.T) {
	spec := &Spec{State: map[string]VarDecl{"items": {Type: VarType{Base: TypeText, List: true}}}}
	store := NewStore(spec)
	input := []any{"original"}
	if _, err := store.WriteState(map[string]any{"items": input}); err != nil {
		t.Fatal(err)
	}
	input[0] = "caller mutation"
	collected := store.Collected()
	collected["items"].([]any)[0] = "collected mutation"
	got, _ := store.Get("items")
	if !reflect.DeepEqual(got, []any{"original"}) {
		t.Fatalf("state alias reached store: %#v", got)
	}
}

func TestStoreTransactionsAreUnavailableDuringCommandJournal(t *testing.T) {
	store := seededAtomicStore(t)
	before := store.Export()
	store.beginJournal()
	if _, err := store.WriteState(map[string]any{"aText": "changed"}); err == nil || !strings.Contains(err.Error(), "only through WriteEngine") {
		t.Fatalf("WriteState error = %v", err)
	}
	if err := store.SetParams(map[string]any{"zParam": "changed"}); err == nil || !strings.Contains(err.Error(), "only through WriteEngine") {
		t.Fatalf("SetParams error = %v", err)
	}
	if err := store.SetStart(map[string]any{"zParam": "changed", "aText": "changed"}); err == nil || !strings.Contains(err.Error(), "only through WriteEngine") {
		t.Fatalf("SetStart error = %v", err)
	}
	store.drainJournal()
	if !reflect.DeepEqual(store.Export(), before) {
		t.Fatalf("command transaction attempt changed store: before=%#v after=%#v", before, store.Export())
	}
}
