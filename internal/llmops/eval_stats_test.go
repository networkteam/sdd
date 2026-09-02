//go:build eval

// Per-call usage measurement for the eval suites (d-tac-ma6): every eval call
// records through the same Observed decorator + FileSink pair production uses,
// attributed per provider, model, and variant — never into the repo's
// .sdd/stats sink. Each call also logs one testing.T line next to the test
// that made it, and TestMain prints the run's aggregated usage table.
//
// SDD_EVAL_STATS_DIR selects the sink directory (rows append to <dir>/llm.jsonl
// across runs, so several candidate runs can accumulate into one file); unset,
// the run writes to a fresh temp dir and prints its path.

package llmops_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/llmstats"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
)

var (
	evalSinkOnce sync.Once
	evalSinkDir  string
	evalSink     *llmstats.FileSink
	evalSinkErr  error
	evalRunStart = time.Now()
)

// evalFileSink returns the process-wide run sink, creating it on first use.
func evalFileSink(t *testing.T) pkgllm.StatsSink {
	t.Helper()
	evalSinkOnce.Do(func() {
		dir := os.Getenv("SDD_EVAL_STATS_DIR")
		if dir == "" {
			dir, evalSinkErr = os.MkdirTemp("", "sdd-eval-stats-")
			if evalSinkErr != nil {
				return
			}
		}
		evalSink, evalSinkErr = llmstats.NewFileSink(dir)
		if evalSinkErr == nil {
			evalSinkDir = dir
			fmt.Printf("eval usage sink: %s/llm.jsonl\n", dir)
		}
	})
	if evalSinkErr != nil {
		t.Fatalf("creating eval stats sink: %v", evalSinkErr)
	}
	return evalSink
}

// tLogSink writes one line per call into the test log, so each call's usage
// sits next to the test that made it.
type tLogSink struct{ t *testing.T }

func (s tLogSink) RecordCall(ctx context.Context, stat pkgllm.CallStat) {
	line := fmt.Sprintf("llm call: op=%s provider=%s model=%s dur=%s in=%d out=%d cache_read=%d cache_create=%d",
		stat.Purpose, stat.Identity.Provider, stat.Identity.String(),
		stat.Duration.Round(time.Millisecond),
		stat.Usage.InputTokens, stat.Usage.OutputTokens,
		stat.Usage.CacheReadTokens, stat.Usage.CacheCreateTokens)
	if stat.Error != "" {
		line += " error=" + stat.Error
	}
	s.t.Log(line)
}

type multiSink []pkgllm.StatsSink

func (m multiSink) RecordCall(ctx context.Context, stat pkgllm.CallStat) {
	for _, s := range m {
		s.RecordCall(ctx, stat)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	printEvalUsageSummary()
	os.Exit(code)
}

// printEvalUsageSummary aggregates this run's recorded calls (the sink file
// may carry earlier runs when SDD_EVAL_STATS_DIR is reused, so rows are
// filtered to the run's start).
func printEvalUsageSummary() {
	if evalSinkDir == "" {
		return
	}
	reader := llmstats.NewReader(evalSinkDir)
	records, err := reader.Read()
	if err != nil {
		fmt.Printf("eval usage summary unavailable: %v\n", err)
		return
	}
	run := model.FilterStats(records, &evalRunStart, "", "", "")
	fmt.Printf("\n=== eval LLM usage — this run (full file: %s) ===\n", reader.Path())
	presenters.RenderStatsTable(os.Stdout, &query.StatsResult{
		Report:    model.AggregateStats(run),
		Source:    reader.Path(),
		Since:     &evalRunStart,
		Until:     time.Now(),
		SinkEmpty: len(records) == 0,
	})
}
