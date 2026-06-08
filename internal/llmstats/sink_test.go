package llmstats

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/networkteam/sdd/internal/llm"
)

func TestFileSinkRecordCall(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "stats")
	sink, err := NewFileSink(dir)
	if err != nil {
		t.Fatalf("NewFileSink: %v", err)
	}

	sink.RecordCall(llm.CallStat{
		Op:                "preflight",
		Provider:          "anthropic",
		Model:             "claude-sonnet-4-6",
		InputTokens:       6341,
		OutputTokens:      8,
		CacheReadTokens:   6163,
		CacheCreateTokens: 0,
		DurationMS:        1245,
	})
	sink.RecordCall(llm.CallStat{
		Op:           "summarize",
		Model:        "claude-sonnet-4-6",
		InputTokens:  1865,
		OutputTokens: 139,
		DurationMS:   3373,
	})

	f, err := os.Open(filepath.Join(dir, "llm.jsonl"))
	if err != nil {
		t.Fatalf("open jsonl: %v", err)
	}
	defer f.Close()

	var recs []Record
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("unmarshal line %q: %v", sc.Text(), err)
		}
		recs = append(recs, r)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}

	if recs[0].Op != "preflight" || recs[0].Provider != "anthropic" || recs[0].CacheReadTokens != 6163 || recs[0].InputTokens != 6341 {
		t.Errorf("record 0 mismatch: %+v", recs[0])
	}
	if recs[0].Timestamp == "" {
		t.Errorf("record 0 missing timestamp")
	}
	if recs[1].Op != "summarize" || recs[1].OutputTokens != 139 {
		t.Errorf("record 1 mismatch: %+v", recs[1])
	}
}
