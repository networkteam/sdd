// Package llmstats provides a durable sink for per-call LLM metrics. It
// implements llm.StatsSink by appending one JSON record per call to a local
// JSONL file (.sdd/stats/llm.jsonl), so prompt-cache effectiveness and token
// usage become measurable over time without an external service (d-tac-zis).
package llmstats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/networkteam/sdd/internal/llm"
)

// FileSink appends one JSON record per LLM call to a JSONL file. Safe for
// concurrent use — batch operations record from multiple goroutines.
type FileSink struct {
	mu   sync.Mutex
	path string
	now  func() time.Time
}

// NewFileSink returns a sink that appends to <dir>/llm.jsonl, creating dir if
// it does not exist.
func NewFileSink(dir string) (*FileSink, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating stats dir %q: %w", dir, err)
	}
	return &FileSink{path: filepath.Join(dir, "llm.jsonl"), now: time.Now}, nil
}

// RecordCall appends one JSON line for the call. Errors are swallowed: stats
// collection is best-effort and must never break a capture or summarize. The
// on-disk shape is Record, shared with the reader (reader.go).
func (s *FileSink) RecordCall(stat llm.CallStat) {
	line, err := json.Marshal(Record{
		Timestamp:         s.now().UTC().Format(time.RFC3339),
		Op:                stat.Op,
		Provider:          stat.Provider,
		Model:             stat.Model,
		Variant:           stat.Variant,
		Items:             stat.Items,
		InputTokens:       stat.InputTokens,
		OutputTokens:      stat.OutputTokens,
		CacheReadTokens:   stat.CacheReadTokens,
		CacheCreateTokens: stat.CacheCreateTokens,
		DurationMS:        stat.DurationMS,
		Error:             stat.Error,
	})
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}
