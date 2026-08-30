package llmstats_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/llmstats"
)

// The whole recording chain, end to end: a runner's Identity reaches the JSONL
// row on disk. Every hop was individually wired and the row still came out
// without a variant, so this asserts the bytes rather than any one hop.
type fakeRunner struct {
	id  llm.Identity
	err error
}

func (f fakeRunner) Identity() llm.Identity { return f.id }

func (f fakeRunner) Run(context.Context, llm.Request) (*llm.RunResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &llm.RunResult{Text: "ok", Meta: &llm.LLMMetadata{InputTokens: 12, OutputTokens: 34}}, nil
}

func recordThrough(t *testing.T, runner llm.Runner, op string) []map[string]any {
	t.Helper()
	dir := t.TempDir()
	sink, err := llmstats.NewFileSink(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := llm.WithStatsSink(context.Background(), sink)
	_, _ = llm.Run(ctx, runner, llm.Request{UserPrompt: "p"}, op)

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
	}}, "writing-guide")

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
	rows := recordThrough(t, fakeRunner{id: llm.Identity{Provider: "anthropic", Model: "m"}}, "preflight")
	if _, present := rows[0]["variant"]; present {
		t.Errorf("a model at its defaults must write no variant key: %v", rows[0])
	}
}
