package model

import (
	"strings"
	"testing"
)

// TestParseEntry_IgnoresLegacySummaryHash covers the no-migration path for the
// summary-hash removal (d-cpt-4qi / d-tac-y7l): the many existing entry files
// still carry a `summary_hash:` key, and they must load cleanly because
// frontmatter parses with lenient YAML (no KnownFields) — the now-unknown key
// is ignored. Re-serializing the loaded entry drops the key, so it disappears
// the next time that entry's summary is written.
func TestParseEntry_IgnoresLegacySummaryHash(t *testing.T) {
	content := "---\n" +
		"type: decision\n" +
		"layer: tactical\n" +
		"kind: plan\n" +
		"summary: A durable summary.\n" +
		"summary_hash: deadbeefdeadbeefdeadbeefdeadbeef\n" +
		"---\n\nBody of the entry.\n"

	e, err := ParseEntry("20260101-120000-d-tac-abc.md", content)
	if err != nil {
		t.Fatalf("ParseEntry with legacy summary_hash key: %v", err)
	}
	if e.Summary != "A durable summary." {
		t.Errorf("summary = %q, want %q", e.Summary, "A durable summary.")
	}

	// Re-serialized frontmatter carries the summary but no summary_hash.
	out := FormatFrontmatter(e)
	if strings.Contains(out, "summary_hash") {
		t.Errorf("FormatFrontmatter still emits summary_hash:\n%s", out)
	}
	if !strings.Contains(out, "summary: A durable summary.") {
		t.Errorf("FormatFrontmatter dropped the summary:\n%s", out)
	}
}
