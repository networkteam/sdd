package query

// SyncStatusQuery captures intent to check whether the local graph is in
// sync with the upstream branch. Processing involves a (potentially
// rate-limited) git fetch and several read-only git plumbing calls; the
// side effect of touching the last-fetch marker is considered internal
// bookkeeping rather than a domain mutation, matching the PreflightQuery
// precedent where an LLM call is embedded in a read-intent query.
type SyncStatusQuery struct {
	// SDDDir is the .sdd/ directory where the last-fetch marker lives.
	SDDDir string
	// RespectCooldown, when true, short-circuits the query with a Skipped
	// state if the last fetch is within the configured cooldown window.
	// When false the fetch runs regardless — useful for callers invoked
	// by an explicit user action rather than incidental to a command.
	RespectCooldown bool
}
