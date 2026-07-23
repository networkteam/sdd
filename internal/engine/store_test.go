package engine

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

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

// TestStoreNormalizesTimeValuesWithoutPanic guards the store's JSON-document
// contract: a time.Time (bare or nested in containers) normalizes to its
// RFC3339 string on write, and the read/clone/commit paths never panic on it.
func TestStoreNormalizesTimeValuesWithoutPanic(t *testing.T) {
	spec := &Spec{State: map[string]VarDecl{"note": {Type: VarType{Base: TypeText}}}}
	store := NewStore(spec)
	stamp := time.Date(2026, 7, 22, 10, 30, 0, 0, time.UTC)
	if err := store.WriteEngine("stamp", stamp); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteEngine("wrapped", map[string]any{"at": stamp, "list": []any{stamp}}); err != nil {
		t.Fatal(err)
	}

	got, ok := store.Get("stamp")
	if !ok {
		t.Fatal("stamp missing")
	}
	if s, isStr := got.(string); !isStr || !strings.HasPrefix(s, "2026-07-22T10:30:00") {
		t.Fatalf("stamp normalized to %#v, want RFC3339 string", got)
	}
	wrapped, _ := store.Get("wrapped")
	if _, isStr := wrapped.(map[string]any)["at"].(string); !isStr {
		t.Fatalf("nested time normalized to %#v", wrapped)
	}

	// Export and a transactional write (which clones then commits with the
	// time-derived value present) must not panic.
	_ = store.Export()
	if _, err := store.WriteState(map[string]any{"note": "hello"}); err != nil {
		t.Fatal(err)
	}
	if again, _ := store.Get("stamp"); again != got {
		t.Fatalf("stamp changed across commit: %#v", again)
	}
}

// TestStoreRejectsNonMarshalableEngineValue confirms a value that cannot become
// a JSON document is a loud write-time error that leaves the store untouched —
// never a panic and never a silent skip.
func TestStoreRejectsNonMarshalableEngineValue(t *testing.T) {
	cases := map[string]any{
		"channel":     make(chan int),
		"func":        func() {},
		"nested func": map[string]any{"fn": func() {}},
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			store := NewStore(&Spec{})
			if err := store.WriteEngine("kept", "safe"); err != nil {
				t.Fatal(err)
			}
			before := store.Export()
			err := store.WriteEngine("bad", value)
			if err == nil || !strings.Contains(err.Error(), "JSON") {
				t.Fatalf("WriteEngine error = %v, want a JSON-persistable error", err)
			}
			if !reflect.DeepEqual(store.Export(), before) {
				t.Fatalf("rejected value changed store: %#v", store.Export())
			}
		})
	}
}

// TestStoreGetAndWriteStateIsolateContainers confirms the store never shares a
// mutable container with a caller: mutating the map passed to WriteState after
// the call, and mutating a map returned by Get, both leave the store unchanged.
func TestStoreGetAndWriteStateIsolateContainers(t *testing.T) {
	spec := &Spec{State: map[string]VarDecl{"index": {Type: VarType{Base: TypeFactIndex}}}}
	store := NewStore(spec)
	input := map[string]any{"title": "View grammar", "topic": "cli/ux"}
	if _, err := store.WriteState(map[string]any{"index": input}); err != nil {
		t.Fatal(err)
	}
	input["title"] = "caller mutation"

	first, ok := store.Get("index")
	if !ok {
		t.Fatal("index missing")
	}
	if first.(map[string]any)["title"] != "View grammar" {
		t.Fatalf("write input aliased store: %#v", first)
	}
	first.(map[string]any)["title"] = "get mutation"

	second, _ := store.Get("index")
	if second.(map[string]any)["title"] != "View grammar" {
		t.Fatalf("get result aliased store: %#v", second)
	}
}

func TestStoreCloneIsolatesCandidateValues(t *testing.T) {
	store := NewStore(&Spec{})
	if err := store.WriteEngine("items", []any{map[string]any{"value": "original"}}); err != nil {
		t.Fatal(err)
	}
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
