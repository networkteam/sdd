package query

import (
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// StatsQuery captures intent to read and aggregate the local LLM/embedding
// stats sink. A nil Since means all-time; empty Op/Provider/Model strings mean
// no constraint on that field. Until is the reference "now" used for the
// range's upper bound in rendering.
type StatsQuery struct {
	StatsDir string
	Since    *time.Time
	Until    time.Time
	Op       string
	Provider string
	Model    string
}

// StatsResult is the aggregated output of a StatsQuery. SinkEmpty distinguishes
// "no sink on disk yet" from "sink had records but the filter excluded them
// all", so the renderer can phrase the empty case correctly.
type StatsResult struct {
	Report    model.StatsReport
	Source    string
	Since     *time.Time
	Until     time.Time
	SinkEmpty bool
}
