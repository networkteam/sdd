package presenters

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

func sampleReport() model.StatsReport {
	return model.AggregateStats([]model.StatsRecord{
		{Op: "preflight", Provider: "anthropic", Model: "claude-sonnet-4-6", InputTokens: 1000, OutputTokens: 100, CacheReadTokens: 500, CacheCreateTokens: 200, DurationMS: 1000},
		{Op: "embed-documents", Provider: "ollama", Model: "qwen3-embedding:8b", Items: 10, InputTokens: 200, DurationMS: 2000},
	})
}

func TestRenderStatsJSON(t *testing.T) {
	until := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	res := &query.StatsResult{Report: sampleReport(), Source: "/x/llm.jsonl", Until: until}

	var buf bytes.Buffer
	if err := RenderStatsJSON(&buf, res); err != nil {
		t.Fatal(err)
	}

	var out struct {
		Range struct {
			Since *string `json:"since"`
			Until string  `json:"until"`
		} `json:"range"`
		Totals  map[string]any `json:"totals"`
		ByModel []struct {
			Model string `json:"model"`
		} `json:"by_model"`
		ByOp []struct {
			Op          string   `json:"op"`
			ItemsPerSec *float64 `json:"items_per_s"`
			Latency     struct {
				P50MS int64 `json:"p50_ms"`
				P90MS int64 `json:"p90_ms"`
				P99MS int64 `json:"p99_ms"`
				MaxMS int64 `json:"max_ms"`
			} `json:"latency"`
		} `json:"by_op"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}

	if out.Range.Since != nil {
		t.Errorf("all-time range should have null since, got %v", *out.Range.Since)
	}
	if out.Range.Until != "2026-06-08" {
		t.Errorf("until = %q, want 2026-06-08", out.Range.Until)
	}
	if len(out.ByModel) != 2 || len(out.ByOp) != 2 {
		t.Fatalf("want 2 model + 2 op rows, got %d/%d", len(out.ByModel), len(out.ByOp))
	}
	for _, o := range out.ByOp {
		switch o.Op {
		case "preflight":
			if o.ItemsPerSec != nil {
				t.Errorf("chat op should have null items_per_s, got %v", *o.ItemsPerSec)
			}
		case "embed-documents":
			if o.ItemsPerSec == nil {
				t.Errorf("embed op should have a numeric items_per_s")
			}
			// One call at 2000ms: every percentile is that call.
			if o.Latency.P50MS != 2000 || o.Latency.P99MS != 2000 || o.Latency.MaxMS != 2000 {
				t.Errorf("embed latency = %+v, want 2000ms across the board", o.Latency)
			}
		}
	}
}

func TestRenderStatsJSONSinceSet(t *testing.T) {
	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	res := &query.StatsResult{Report: model.StatsReport{}, Source: "/x/llm.jsonl", Since: &since, Until: since}

	var buf bytes.Buffer
	if err := RenderStatsJSON(&buf, res); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"since": "2026-05-01"`) {
		t.Fatalf("expected since date in JSON, got:\n%s", buf.String())
	}
	// Empty report must still emit valid JSON with empty arrays, not null.
	if !strings.Contains(buf.String(), `"by_model": []`) || !strings.Contains(buf.String(), `"by_op": []`) {
		t.Fatalf("empty arrays expected, got:\n%s", buf.String())
	}
}

func TestRenderStatsTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	RenderStatsTable(&buf, &query.StatsResult{SinkEmpty: true, Source: "/x/llm.jsonl"})
	if !strings.Contains(buf.String(), "no stats recorded yet") {
		t.Fatalf("expected empty-sink message, got:\n%s", buf.String())
	}
}

func TestRenderStatsTableNoMatch(t *testing.T) {
	var buf bytes.Buffer
	// Sink had data (SinkEmpty false) but the filter left zero calls.
	RenderStatsTable(&buf, &query.StatsResult{Report: model.StatsReport{}, Source: "/x/llm.jsonl"})
	if !strings.Contains(buf.String(), "no calls in the selected range") {
		t.Fatalf("expected no-match message, got:\n%s", buf.String())
	}
}

func TestRenderStatsTable(t *testing.T) {
	res := &query.StatsResult{Report: sampleReport(), Source: ".sdd/stats/llm.jsonl"}
	var buf bytes.Buffer
	RenderStatsTable(&buf, res)
	out := buf.String()

	for _, want := range []string{"sdd stats", "Totals", "By model", "By operation", "claude-sonnet-4-6", "qwen3-embedding:8b", "preflight", "embed-documents"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q", want)
		}
	}
	t.Logf("rendered table:\n%s", out)
}

func TestHumanCount(t *testing.T) {
	cases := map[int]string{0: "0", 999: "999", 3100: "3.1k", 4000: "4k", 168000: "168k", 980000: "980k", 1840000: "1.84M"}
	for in, want := range cases {
		if got := humanCount(in); got != want {
			t.Errorf("humanCount(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanRate(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{{870, "870"}, {1390, "1.39k"}, {9.2, "9.2"}, {2100, "2.1k"}}
	for _, c := range cases {
		if got := humanRate(c.in); got != c.want {
			t.Errorf("humanRate(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	cases := map[int64]string{0: "0s", 500: "500ms", 18000: "18s", 2172000: "36m 12s", 8040000: "2h 14m"}
	for in, want := range cases {
		if got := humanDuration(in); got != want {
			t.Errorf("humanDuration(%d) = %q, want %q", in, got, want)
		}
	}
}
