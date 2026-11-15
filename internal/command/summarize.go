package command

// SummarizeCmd captures intent to generate or regenerate entry summaries and
// titles, which one LLM call writes together.
type SummarizeCmd struct {
	// EntryIDs lists specific entries to summarize. Empty means --all.
	EntryIDs []string
	// Force, on an --all run, regenerates every entry rather than filling
	// only what is missing: an entry without a summary gets both, an entry
	// with a summary but no title gets a title and keeps its summary. Named
	// entries always regenerate both, so Force has no effect on them.
	Force bool
	// Model is the LLM model to use for summary generation.
	Model string
	// Concurrency bounds the worker pool for batch summarize. Zero falls
	// back to model.DefaultLLMConcurrency.
	Concurrency int
	// ExplicitText, when non-nil, is written as the entry's summary directly
	// without invoking the LLM. Single entry only — handler rejects when set
	// alongside multiple EntryIDs or empty EntryIDs (--all).
	ExplicitText *string
	// ExplicitTitle, when non-nil, is written as the entry's title directly,
	// under the same single-entry rule as ExplicitText and combinable with it.
	ExplicitTitle *string
	// OnSummarized is called for each entry that gets a new summary.
	OnSummarized func(id, summary string)
	// OnTitled is called for each entry that gets a new title while keeping
	// its summary.
	OnTitled func(id, title string)
	// OnSkipped is called for each --all entry skipped because it already has
	// a summary and a title.
	OnSkipped func(id string)
}
