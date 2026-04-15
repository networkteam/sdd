// Package query holds domain query structs — pure read intent, processed by
// finders. Queries are plain data; no methods with side effects.
package query

import (
	"time"

	"github.com/networkteam/resonance/framework/sdd/model"
)

// PreflightQuery captures intent to validate an entry against the graph using
// the pre-flight LLM validator. Pure read — no side effects of its own; the
// runner dependency injected into the finder handles the LLM call.
type PreflightQuery struct {
	Entry   *model.Entry
	Graph   *model.Graph
	Model   string        // LLM model identifier (e.g. "claude-haiku-4-5-20251001")
	Timeout time.Duration // hard timeout for the validator call
}

// Severity classifies a pre-flight finding. The tooling layer decides what
// to block on (currently: only SeverityHigh blocks); templates describe
// severity in purely semantic terms and never name a threshold.
type Severity string

const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
)

// Finding is a single observation from pre-flight validation.
type Finding struct {
	Severity    Severity
	Category    string
	Observation string
}

// PreflightResult holds all findings from a pre-flight validator run.
// An empty Findings slice means the validator reported no findings.
type PreflightResult struct {
	Findings []Finding
}

// HasBlocking reports whether any finding blocks entry creation. Currently
// only SeverityHigh blocks; this is the single source of truth for the
// blocking threshold (handler_new_entry uses it; templates do not).
func (r *PreflightResult) HasBlocking() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityHigh {
			return true
		}
	}
	return false
}
