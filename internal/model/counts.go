package model

import (
	"sort"
	"time"
)

// ShapeCounts marks results carrying per-topic aggregate rows — produced by
// a section ending in `as-counts`. Each row is one effective-topic label with
// the number of entries carrying it and their summed heat. Mutually exclusive
// with the other shapes: a section produces one, never several.
const ShapeCounts RenderShape = "counts"

// CountRow is one aggregated topic row in a Counts result: the topic Label
// (first-seen casing), how many entries in the filtered set carry it as an
// effective topic, and the summed heat contribution of those entries.
type CountRow struct {
	Label string
	Count int
	Heat  float64
}

// Counts is the result shape produced by `as-counts`. It answers "what topics
// exist and how many entries each carries" without enumerating members — the
// mechanical lookup the capture-time topic procedure leans on instead of
// per-run agent inference. Rows are ordered by the finder (count descending,
// then heat descending, then label) so the busiest clusters surface first.
//
// Entries with no effective topics contribute to no row; surface them with
// the `untagged` filter instead.
type Counts struct {
	Rows []CountRow
}

// Shape implements SectionData.
func (Counts) Shape() RenderShape { return ShapeCounts }

// TopicCounts buckets the given entries by their effective topic labels and
// returns one CountRow per distinct topic: the number of entries carrying it
// and their summed heat (default exp-14d decay, the same default top(N) ranks
// by). Buckets are keyed case-insensitively (FoldKey) with first-seen casing
// winning for display, matching topic-path comparison semantics elsewhere.
//
// Rows are ordered count descending, then heat descending, then label
// ascending — the busiest clusters first. Entries with no effective topics
// contribute to no row; the `untagged` filter surfaces those separately.
//
// Pure: reads the graph's reverse-ref index via EffectiveTopics and the heat
// scorer, no I/O. now is injected so callers can pin the clock for tests.
func (g *Graph) TopicCounts(entries []*Entry, now time.Time) Counts {
	decay, _ := DecayByName(DefaultDecayName)

	type bucket struct {
		count   int
		heat    float64
		display string // first-seen casing
	}
	buckets := map[string]*bucket{}
	for _, e := range entries {
		heat := HeatScore(g, e, decay, now)
		for _, t := range g.EffectiveTopics(e) {
			key := t.FoldKey()
			b := buckets[key]
			if b == nil {
				b = &bucket{display: t.String()}
				buckets[key] = b
			}
			b.count++
			b.heat += heat
		}
	}

	rows := make([]CountRow, 0, len(buckets))
	for _, b := range buckets {
		rows = append(rows, CountRow{Label: b.display, Count: b.count, Heat: b.heat})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		if rows[i].Heat != rows[j].Heat {
			return rows[i].Heat > rows[j].Heat
		}
		return rows[i].Label < rows[j].Label
	})
	return Counts{Rows: rows}
}
