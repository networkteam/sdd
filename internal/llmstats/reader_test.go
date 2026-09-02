package llmstats

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgllm "github.com/networkteam/sdd/pkg/llm"
)

func TestReaderAbsentSink(t *testing.T) {
	recs, err := NewReader(t.TempDir()).Read()
	if err != nil {
		t.Fatalf("absent sink should not error: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("absent sink should yield no records, got %d", len(recs))
	}
}

func TestReaderSkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	lines := "" +
		`{"ts":"2026-06-08T10:00:00Z","op":"preflight","provider":"anthropic","model":"m","input_tokens":100,"duration_ms":50}` + "\n" +
		"\n" + // blank line
		`not json at all` + "\n" +
		`{"ts":"not-a-time","op":"summarize","input_tokens":5}` + "\n" + // unparseable timestamp
		`{"ts":"2026-06-08T10:01:00Z","op":"embed-documents","provider":"ollama","model":"q","items":4,"input_tokens":40,"duration_ms":200}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "llm.jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	recs, err := NewReader(dir).Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2 valid (blank/garbage/bad-ts skipped)", len(recs))
	}
	if recs[0].Op != "preflight" || recs[1].Op != "embed-documents" || recs[1].Items != 4 {
		t.Fatalf("unexpected records: %+v", recs)
	}
}

// TestWriterReaderRoundTrip locks the single-path record shape: what FileSink
// writes is exactly what Reader reads back.
func TestWriterReaderRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sink, err := NewFileSink(dir)
	if err != nil {
		t.Fatal(err)
	}
	sink.RecordCall(context.Background(), pkgllm.CallStat{
		Purpose: "preflight", Identity: pkgllm.Identity{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		Usage: pkgllm.Usage{InputTokens: 6341, OutputTokens: 12, CacheReadTokens: 6163}, Duration: 5000 * time.Millisecond,
	})
	sink.RecordCall(context.Background(), pkgllm.CallStat{
		Purpose: "embed-documents", Identity: pkgllm.Identity{Provider: "ollama", Model: "qwen3-embedding:8b"},
		Items: 8, Usage: pkgllm.Usage{InputTokens: 1200}, Duration: 900 * time.Millisecond,
	})

	recs, err := NewReader(dir).Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	if recs[0].Op != "preflight" || recs[0].InputTokens != 6341 || recs[0].CacheReadTokens != 6163 {
		t.Fatalf("round-trip lost chat fields: %+v", recs[0])
	}
	if recs[1].Items != 8 || recs[1].Provider != "ollama" || recs[1].DurationMS != 900 {
		t.Fatalf("round-trip lost embed fields: %+v", recs[1])
	}
	if recs[0].Timestamp.IsZero() {
		t.Fatalf("timestamp not parsed back")
	}
}
