package model

import (
	"testing"
	"time"
)

func tsAt(day int) time.Time {
	return time.Date(2026, 6, day, 12, 0, 0, 0, time.UTC)
}

func sampleRecords() []StatsRecord {
	return []StatsRecord{
		// preflight, sonnet — 1000 in over 1s
		{Timestamp: tsAt(1), Op: "preflight", Provider: "anthropic", Model: "claude-sonnet-4-6", InputTokens: 1000, OutputTokens: 100, CacheReadTokens: 500, CacheCreateTokens: 200, DurationMS: 1000},
		// summarize, sonnet — 500 in over 0.5s
		{Timestamp: tsAt(3), Op: "summarize", Provider: "anthropic", Model: "claude-sonnet-4-6", InputTokens: 500, OutputTokens: 50, DurationMS: 500},
		// embed-documents, qwen — 10 items, 200 in over 2s
		{Timestamp: tsAt(3), Op: "embed-documents", Provider: "ollama", Model: "qwen3-embedding:8b", Items: 10, InputTokens: 200, DurationMS: 2000},
	}
}

func TestFilterStats(t *testing.T) {
	recs := sampleRecords()

	t.Run("nil since keeps all", func(t *testing.T) {
		if got := FilterStats(recs, nil, "", "", ""); len(got) != 3 {
			t.Fatalf("got %d, want 3", len(got))
		}
	})

	t.Run("since cutoff drops earlier", func(t *testing.T) {
		cut := tsAt(2)
		got := FilterStats(recs, &cut, "", "", "")
		if len(got) != 2 {
			t.Fatalf("got %d, want 2 (day-3 records only)", len(got))
		}
	})

	t.Run("op filter", func(t *testing.T) {
		got := FilterStats(recs, nil, "preflight", "", "")
		if len(got) != 1 || got[0].Op != "preflight" {
			t.Fatalf("got %+v, want one preflight", got)
		}
	})

	t.Run("provider filter", func(t *testing.T) {
		got := FilterStats(recs, nil, "", "ollama", "")
		if len(got) != 1 || got[0].Provider != "ollama" {
			t.Fatalf("got %+v, want one ollama", got)
		}
	})

	t.Run("model filter", func(t *testing.T) {
		got := FilterStats(recs, nil, "", "", "claude-sonnet-4-6")
		if len(got) != 2 {
			t.Fatalf("got %d, want 2 sonnet", len(got))
		}
	})

	t.Run("does not mutate input", func(t *testing.T) {
		_ = FilterStats(recs, nil, "preflight", "", "")
		if len(recs) != 3 {
			t.Fatalf("input mutated: len %d", len(recs))
		}
	})
}

func TestAggregateStats(t *testing.T) {
	report := AggregateStats(sampleRecords())

	t.Run("totals", func(t *testing.T) {
		tot := report.Totals
		if tot.Calls != 3 || tot.InputTokens != 1700 || tot.OutputTokens != 150 ||
			tot.CacheReadTokens != 500 || tot.CacheCreateTokens != 200 || tot.DurationMS != 3500 {
			t.Fatalf("totals wrong: %+v", tot)
		}
	})

	t.Run("by-model grouping and sort (calls desc)", func(t *testing.T) {
		if len(report.ByModel) != 2 {
			t.Fatalf("got %d model rows, want 2", len(report.ByModel))
		}
		first := report.ByModel[0]
		if first.Model != "claude-sonnet-4-6" || first.Calls != 2 {
			t.Fatalf("first model row should be sonnet with 2 calls, got %+v", first)
		}
		if first.InputTokens != 1500 || first.DurationMS != 1500 {
			t.Fatalf("sonnet sums wrong: %+v", first)
		}
		// sonnet: 1500 in over 1.5s = 1000 tok/s
		if first.TokensPerSec() != 1000 {
			t.Fatalf("sonnet throughput wrong: tok/s %.2f", first.TokensPerSec())
		}
		if first.HasItems() {
			t.Fatalf("chat model should report no items")
		}
	})

	t.Run("by-op grouping, sort (calls desc then name), throughput", func(t *testing.T) {
		if len(report.ByOp) != 3 {
			t.Fatalf("got %d op rows, want 3", len(report.ByOp))
		}
		// All ops have 1 call → tie broken alphabetically.
		wantOrder := []string{"embed-documents", "preflight", "summarize"}
		for i, w := range wantOrder {
			if report.ByOp[i].Op != w {
				t.Fatalf("op order[%d] = %q, want %q", i, report.ByOp[i].Op, w)
			}
		}
		var embed OpRollup
		for _, o := range report.ByOp {
			if o.Op == "embed-documents" {
				embed = o
			}
		}
		if !embed.HasItems() {
			t.Fatalf("embed op should report items")
		}
		// 10 items over 2s = 5 items/s; 200 in over 2s = 100 tok/s
		if embed.ItemsPerSec() != 5 || embed.TokensPerSec() != 100 {
			t.Fatalf("embed throughput wrong: items/s %.2f tok/s %.2f", embed.ItemsPerSec(), embed.TokensPerSec())
		}
	})
}

func TestThroughputZeroGuards(t *testing.T) {
	var m StatMetrics // all zero
	if m.TokensPerSec() != 0 || m.ItemsPerSec() != 0 {
		t.Fatalf("zero metrics must not divide by zero")
	}
	if got := m.Latency(); got != (Latency{}) {
		t.Fatalf("zero metrics must report an empty distribution, got %+v", got)
	}
}

func TestLatencyDistribution(t *testing.T) {
	var m StatMetrics
	// 1..100ms, added out of order so the accessor cannot rely on insertion order.
	for _, ms := range []int64{50, 100, 1, 25, 75} {
		m.add(StatsRecord{DurationMS: ms})
	}
	for i := int64(2); i <= 100; i++ {
		switch i {
		case 25, 50, 75, 100:
		default:
			m.add(StatsRecord{DurationMS: i})
		}
	}

	got := m.Latency()
	want := Latency{P50: 50, P90: 90, P99: 99, Max: 100}
	if got != want {
		t.Fatalf("Latency() = %+v, want %+v", got, want)
	}
}

func TestLatencySingleCall(t *testing.T) {
	var m StatMetrics
	m.add(StatsRecord{DurationMS: 42})
	want := Latency{P50: 42, P90: 42, P99: 42, Max: 42}
	if got := m.Latency(); got != want {
		t.Fatalf("Latency() = %+v, want %+v", got, want)
	}
}
