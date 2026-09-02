package finders_test

import (
	"context"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/llmstats"
	"github.com/networkteam/sdd/internal/query"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
)

func TestStatsEmptySink(t *testing.T) {
	f := finders.New(finders.Options{})
	res, err := f.Stats(query.StatsQuery{StatsDir: t.TempDir()})
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if !res.SinkEmpty {
		t.Fatalf("expected SinkEmpty for an absent sink")
	}
	if res.Report.Totals.Calls != 0 {
		t.Fatalf("expected zero totals, got %+v", res.Report.Totals)
	}
}

func TestStatsReadsAndFilters(t *testing.T) {
	dir := t.TempDir()
	sink, err := llmstats.NewFileSink(dir)
	if err != nil {
		t.Fatal(err)
	}
	sink.RecordCall(context.Background(), pkgllm.CallStat{Purpose: "preflight", Identity: pkgllm.Identity{Provider: "anthropic", Model: "m"}, Usage: pkgllm.Usage{InputTokens: 100}, Duration: 100 * time.Millisecond})
	sink.RecordCall(context.Background(), pkgllm.CallStat{Purpose: "embed-documents", Identity: pkgllm.Identity{Provider: "ollama", Model: "q"}, Items: 4, Usage: pkgllm.Usage{InputTokens: 40}, Duration: 200 * time.Millisecond})

	f := finders.New(finders.Options{})

	all, err := f.Stats(query.StatsQuery{StatsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if all.SinkEmpty || all.Report.Totals.Calls != 2 {
		t.Fatalf("expected 2 calls, non-empty sink, got empty=%v calls=%d", all.SinkEmpty, all.Report.Totals.Calls)
	}

	ollama, err := f.Stats(query.StatsQuery{StatsDir: dir, Provider: "ollama"})
	if err != nil {
		t.Fatal(err)
	}
	if ollama.Report.Totals.Calls != 1 {
		t.Fatalf("provider filter: want 1 call, got %d", ollama.Report.Totals.Calls)
	}
	// Sink had records, so even a filter that excludes some is not SinkEmpty.
	if ollama.SinkEmpty {
		t.Fatalf("filtered result over a non-empty sink must not be SinkEmpty")
	}
}
