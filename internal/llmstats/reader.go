package llmstats

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// Record is the on-disk JSONL shape, shared by the writer (FileSink) and the
// Reader so the wire format has a single definition. Cost is intentionally
// absent: the active provider path reports tokens and the cache breakdown but
// no dollar cost, and we keep no pricing table (d-tac-zis).
type Record struct {
	Timestamp         string `json:"ts"`
	Op                string `json:"op"`
	Provider          string `json:"provider,omitempty"`
	Model             string `json:"model,omitempty"`
	Items             int    `json:"items,omitempty"`
	InputTokens       int    `json:"input_tokens"`
	OutputTokens      int    `json:"output_tokens"`
	CacheReadTokens   int    `json:"cache_read_tokens"`
	CacheCreateTokens int    `json:"cache_create_tokens"`
	DurationMS        int64  `json:"duration_ms"`
	Error             string `json:"error,omitempty"`
}

// toStatsRecord parses the timestamp and lifts the record into the pure domain
// shape the aggregation works over.
func (r Record) toStatsRecord() (model.StatsRecord, error) {
	ts, err := time.Parse(time.RFC3339, r.Timestamp)
	if err != nil {
		return model.StatsRecord{}, err
	}
	return model.StatsRecord{
		Timestamp:         ts,
		Op:                r.Op,
		Provider:          r.Provider,
		Model:             r.Model,
		Items:             r.Items,
		InputTokens:       r.InputTokens,
		OutputTokens:      r.OutputTokens,
		CacheReadTokens:   r.CacheReadTokens,
		CacheCreateTokens: r.CacheCreateTokens,
		DurationMS:        r.DurationMS,
		Error:             r.Error,
	}, nil
}

// Reader reads the per-call stats sink written by FileSink.
type Reader struct {
	path string
}

// NewReader returns a reader over <dir>/llm.jsonl — the same path FileSink
// writes, so the dir argument matches what NewFileSink was given.
func NewReader(dir string) *Reader {
	return &Reader{path: filepath.Join(dir, "llm.jsonl")}
}

// Path returns the sink file path, for display in the stats header.
func (r *Reader) Path() string { return r.path }

// Read returns every parseable record in the sink, in file order. An absent
// sink yields an empty slice and no error — that is the "no stats recorded
// yet" case, not a failure. Malformed lines are skipped rather than aborting
// the read: stats collection is best-effort, so one bad line must not blank the
// whole report.
func (r *Reader) Read() ([]model.StatsRecord, error) {
	f, err := os.Open(r.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening stats sink %q: %w", r.path, err)
	}
	defer f.Close()

	var out []model.StatsRecord
	sc := bufio.NewScanner(f)
	// Records are small, but a runaway line should not panic the scanner.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		sr, err := rec.toStatsRecord()
		if err != nil {
			continue
		}
		out = append(out, sr)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading stats sink %q: %w", r.path, err)
	}
	return out, nil
}
