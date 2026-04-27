package command

import "time"

// SummarizeCmd captures intent to generate or regenerate entry summaries.
type SummarizeCmd struct {
	// EntryIDs lists specific entries to summarize. Empty means --all.
	EntryIDs []string
	// Force regenerates summaries even if the hash matches.
	Force bool
	// Model is the LLM model to use for summary generation.
	Model string
	// Timeout per entry for the LLM call.
	Timeout time.Duration
	// Concurrency bounds the worker pool for batch summarize. Zero falls
	// back to model.DefaultLLMConcurrency.
	Concurrency int
	// ExplicitText, when non-nil, is written as the entry's summary directly
	// without invoking the LLM. Single entry only — handler rejects when set
	// alongside multiple EntryIDs or empty EntryIDs (--all). The summary hash
	// is recomputed from the current prompt input so subsequent automatic
	// regenerations skip-by-hash unless Force is set.
	ExplicitText *string
	// OnSummarized is called for each entry that gets a new summary.
	OnSummarized func(id, summary string)
	// OnSkipped is called for each entry whose hash matched (skipped).
	OnSkipped func(id string)
}
