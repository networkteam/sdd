package command

import "time"

// SummarizeCmd captures intent to generate or regenerate entry summaries.
type SummarizeCmd struct {
	// EntryIDs lists specific entries to summarize. Empty means --all.
	EntryIDs []string
	// Force, on an --all run, regenerates every entry rather than filling
	// only those without a summary. Named entries always regenerate, so Force
	// has no effect on them.
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
	// alongside multiple EntryIDs or empty EntryIDs (--all).
	ExplicitText *string
	// OnSummarized is called for each entry that gets a new summary.
	OnSummarized func(id, summary string)
	// OnSkipped is called for each --all entry skipped because it already has
	// a summary.
	OnSkipped func(id string)
}
