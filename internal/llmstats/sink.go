// Package llmstats is the local host's durable recording of per-call LLM
// metrics: a pkg/llm StatsSink appending one JSON record per call to
// .sdd/stats/llm.jsonl, so prompt-cache effectiveness and token usage become
// measurable over time without an external service (d-tac-zis), plus the
// debug-logging sink the CLI composes in front of it.
package llmstats

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/networkteam/sdd/pkg/llm"
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
func (s *FileSink) RecordCall(_ context.Context, stat llm.CallStat) {
	line, err := json.Marshal(Record{
		Timestamp:         s.now().UTC().Format(time.RFC3339),
		Op:                stat.Purpose,
		Provider:          stat.Identity.Provider,
		Model:             stat.Identity.Model,
		Variant:           stat.Identity.Variant,
		Items:             stat.Items,
		InputTokens:       stat.Usage.InputTokens,
		OutputTokens:      stat.Usage.OutputTokens,
		CacheReadTokens:   stat.Usage.CacheReadTokens,
		CacheCreateTokens: stat.Usage.CacheCreateTokens,
		DurationMS:        stat.Duration.Milliseconds(),
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
