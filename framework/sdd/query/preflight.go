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

// PreflightResult holds the parsed validator response.
type PreflightResult struct {
	Pass bool
	Gaps []string
}
