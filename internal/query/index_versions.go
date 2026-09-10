package query

import "time"

// IndexVersionsQuery captures intent to report the stored versions of one
// search-index store grouped by derivation rule — the `sdd index gc` report
// (d-tac-c9c). The shell resolves the store and graph locations.
type IndexVersionsQuery struct {
	// Label names the store in output: the repo ID, or "local".
	Label    string
	GraphDir string
	IndexDir string
	// ExcludeEmbedded matches the store's indexing rule (connected-repo stores
	// skip binary-shipped entries), so current versions are judged the way the
	// store was written.
	ExcludeEmbedded bool
}

// IndexVersionGroup is one group in the report; Name is a group constant from
// internal/index.
type IndexVersionGroup struct {
	Name      string
	Versions  int
	Entries   int
	Oldest    time.Time
	Newest    time.Time
	Bytes     int64
	Droppable bool
}

// IndexVersionsResult is the report for one store.
type IndexVersionsResult struct {
	Label    string
	IndexDir string
	Entries  int
	Bytes    int64
	Groups   []IndexVersionGroup
}
