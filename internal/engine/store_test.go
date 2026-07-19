package engine

import (
	"strings"
	"testing"
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
