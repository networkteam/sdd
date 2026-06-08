package finders

import (
	"github.com/networkteam/sdd/internal/llmstats"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Stats reads the local LLM/embedding stats sink, applies the query's range and
// field filters, and aggregates the result. Pure read — the only side effect is
// the underlying file read in the reader. SinkEmpty reflects whether the sink
// held any records at all, before filtering.
func (f *Finder) Stats(q query.StatsQuery) (*query.StatsResult, error) {
	reader := llmstats.NewReader(q.StatsDir)
	records, err := reader.Read()
	if err != nil {
		return nil, err
	}

	filtered := model.FilterStats(records, q.Since, q.Op, q.Provider, q.Model)
	report := model.AggregateStats(filtered)

	return &query.StatsResult{
		Report:    report,
		Source:    reader.Path(),
		Since:     q.Since,
		Until:     q.Until,
		SinkEmpty: len(records) == 0,
	}, nil
}
