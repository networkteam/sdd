package llmstats_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/llmstats"
	"github.com/networkteam/sdd/pkg/llm"
)

// The whole recording chain, end to end: a runner's reported Identity reaches
// the JSONL row on disk through the observing decorator. Every hop was
// individually wired and the row still came out without a variant once, so
// this asserts the bytes rather than any one hop.
type fakeRunner struct {
	id  llm.Identity
	err error
}

func (f fakeRunner) Run(context.Context, llm.Request) (llm.Result, error) {
	if f.err != nil {
		return llm.Result{}, &llm.Error{Identity: f.id, Err: f.err}
	}
	return llm.Result{Text: "ok", Identity: f.id, Usage: llm.Usage{InputTokens: 12, OutputTokens: 34}}, nil
}

func recordThrough(t *testing.T, runner llm.Runner, purpose llm.Purpose) []map[string]any {
	t.Helper()
	dir := t.TempDir()
	sink, err := llmstats.NewFileSink(dir)
	if err != nil {
		t.Fatal(err)
	}
	observed := llm.Observed(runner, sink)
	_, _ = observed.Run(context.Background(), llm.Request{Purpose: purpose, UserPrompt: "p"})

	raw, err := os.ReadFile(filepath.Join(dir, "llm.jsonl"))
	if err != nil {
		t.Fatalf("no sink file written: %v", err)
	}
	var rows []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("bad row %q: %v", line, err)
		}
		rows = append(rows, row)
	}
	return rows
}

func TestVariantReachesTheSinkFile(t *testing.T) {
	rows := recordThrough(t, fakeRunner{id: llm.Identity{
		Provider: "ollama", Model: "glm-5.3-flash:cloud", Variant: "think=high",
	}}, llm.PurposeWritingGuide)

	if len(rows) != 1 {
		t.Fatalf("wrote %d rows, want 1", len(rows))
	}
	row := rows[0]
	for key, want := range map[string]any{
		"op":       "writing-guide",
		"provider": "ollama",
		"model":    "glm-5.3-flash:cloud",
		"variant":  "think=high",
	} {
		if row[key] != want {
			t.Errorf("%s = %v, want %v (row: %v)", key, row[key], want, row)
		}
	}
}

func TestVariantOmittedWhenUnset(t *testing.T) {
	rows := recordThrough(t, fakeRunner{id: llm.Identity{Provider: "anthropic", Model: "m"}}, llm.PurposePreflight)
	if _, present := rows[0]["variant"]; present {
		t.Errorf("a model at its defaults must write no variant key: %v", rows[0])
	}
}

// A failed call still lands as an attributed row: the typed llm.Error carries
// the identity past the failure, and the error text makes the row countable.
func TestFailureRowKeepsAttribution(t *testing.T) {
	rows := recordThrough(t, fakeRunner{
		id:  llm.Identity{Provider: "ollama", Model: "glm-5.3-flash:cloud", Variant: "think=high"},
		err: errors.New("boom"),
	}, llm.PurposeSummarize)

	if len(rows) != 1 {
		t.Fatalf("wrote %d rows, want 1", len(rows))
	}
	row := rows[0]
	for key, want := range map[string]any{
		"op":       "summarize",
		"provider": "ollama",
		"model":    "glm-5.3-flash:cloud",
		"variant":  "think=high",
		"error":    "boom",
	} {
		if row[key] != want {
			t.Errorf("%s = %v, want %v (row: %v)", key, row[key], want, row)
		}
	}
}
