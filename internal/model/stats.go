package model

import (
	"sort"
	"time"
)

// StatsRecord is one LLM/embedding call's metrics in domain form — the parsed
// shape the aggregation works over. It mirrors the on-disk sink record but
// carries a parsed timestamp and no JSON concerns (those live at the I/O
// boundary in internal/llmstats).
type StatsRecord struct {
	Timestamp         time.Time
	Op                string
	Provider          string
	Model             string
	Items             int
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	DurationMS        int64
}

// StatMetrics holds the summed counters for a group of calls plus the derived
// throughput math. Embedded into the per-model and per-op rollups and reused
// for the report totals so the accumulation and throughput logic live once.
type StatMetrics struct {
	Calls             int
	Items             int
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	DurationMS        int64
}

// add folds one record's counters into the metrics.
func (m *StatMetrics) add(r StatsRecord) {
	m.Calls++
	m.Items += r.Items
	m.InputTokens += r.InputTokens
	m.OutputTokens += r.OutputTokens
	m.CacheReadTokens += r.CacheReadTokens
	m.CacheCreateTokens += r.CacheCreateTokens
	m.DurationMS += r.DurationMS
}

// durationSeconds returns the summed duration in seconds, guarding the
// zero-duration case so throughput callers never divide by zero.
func (m StatMetrics) durationSeconds() float64 {
	return float64(m.DurationMS) / 1000
}

// HasItems reports whether this group carries item counts (the embedding ops).
// Chat ops (preflight, summarize) leave Items zero, so items/s is meaningless
// for them and renders blank.
func (m StatMetrics) HasItems() bool { return m.Items > 0 }

// TokensPerSec is summed input tokens over summed wall-clock seconds. Zero when
// no time was recorded.
func (m StatMetrics) TokensPerSec() float64 {
	s := m.durationSeconds()
	if s == 0 {
		return 0
	}
	return float64(m.InputTokens) / s
}

// MsPerCall is the mean wall-clock duration per call in milliseconds.
func (m StatMetrics) MsPerCall() float64 {
	if m.Calls == 0 {
		return 0
	}
	return float64(m.DurationMS) / float64(m.Calls)
}

// ItemsPerSec is summed items over summed wall-clock seconds (embedding ops).
// Zero when no items or no time were recorded; pair with HasItems to decide
// whether to render it.
func (m StatMetrics) ItemsPerSec() float64 {
	s := m.durationSeconds()
	if s == 0 {
		return 0
	}
	return float64(m.Items) / s
}

// ModelRollup aggregates calls for one (model, provider) pair.
type ModelRollup struct {
	Model    string
	Provider string
	StatMetrics
}

// OpRollup aggregates calls for one operation.
type OpRollup struct {
	Op string
	StatMetrics
}

// StatsReport is the aggregated view of a filtered record set: overall totals
// plus per-model and per-op rollups, each sorted call-count-descending then by
// name for deterministic output.
type StatsReport struct {
	Totals  StatMetrics
	ByModel []ModelRollup
	ByOp    []OpRollup
}

// FilterStats returns the records matching the given range and exact-match
// filters. A nil since means all-time; empty op/provider/model strings mean no
// constraint on that field. The input slice is not mutated.
func FilterStats(records []StatsRecord, since *time.Time, op, provider, model string) []StatsRecord {
	out := make([]StatsRecord, 0, len(records))
	for _, r := range records {
		if since != nil && r.Timestamp.Before(*since) {
			continue
		}
		if op != "" && r.Op != op {
			continue
		}
		if provider != "" && r.Provider != provider {
			continue
		}
		if model != "" && r.Model != model {
			continue
		}
		out = append(out, r)
	}
	return out
}

type modelKey struct {
	model    string
	provider string
}

// AggregateStats rolls records up into totals, by-model, and by-op groups.
func AggregateStats(records []StatsRecord) StatsReport {
	var report StatsReport
	byModel := map[modelKey]*ModelRollup{}
	byOp := map[string]*OpRollup{}

	for _, r := range records {
		report.Totals.add(r)

		mk := modelKey{model: r.Model, provider: r.Provider}
		mr, ok := byModel[mk]
		if !ok {
			mr = &ModelRollup{Model: r.Model, Provider: r.Provider}
			byModel[mk] = mr
		}
		mr.add(r)

		or, ok := byOp[r.Op]
		if !ok {
			or = &OpRollup{Op: r.Op}
			byOp[r.Op] = or
		}
		or.add(r)
	}

	report.ByModel = make([]ModelRollup, 0, len(byModel))
	for _, mr := range byModel {
		report.ByModel = append(report.ByModel, *mr)
	}
	sort.Slice(report.ByModel, func(i, j int) bool {
		a, b := report.ByModel[i], report.ByModel[j]
		if a.Calls != b.Calls {
			return a.Calls > b.Calls
		}
		if a.Model != b.Model {
			return a.Model < b.Model
		}
		return a.Provider < b.Provider
	})

	report.ByOp = make([]OpRollup, 0, len(byOp))
	for _, or := range byOp {
		report.ByOp = append(report.ByOp, *or)
	}
	sort.Slice(report.ByOp, func(i, j int) bool {
		a, b := report.ByOp[i], report.ByOp[j]
		if a.Calls != b.Calls {
			return a.Calls > b.Calls
		}
		return a.Op < b.Op
	})

	return report
}
