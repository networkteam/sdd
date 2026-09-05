// Package application owns SDD's protocol-neutral runtime, public request and
// result types, and the infrastructure ports implemented by embedding hosts.
//
// SearchRequest.SyncMode is required: SearchSyncNone reads the existing vector
// index, SearchSyncLocal first reconciles the selected project branch snapshot,
// and SearchSyncAll also reconciles searched dependencies. Text-only searches
// require a mode but do not need index maintenance. Existing synchronous callers
// should pass SearchSyncAll.
//
// Hosts can call ProjectRuntime.ReconcileSearchIndex independently to warm its
// current graph index, with optional callbacks after persistence. The host owns
// authorization and scheduling. Reconciliation adds missing entry versions;
// it does not remove old ones or watch for subsequent graph changes.
package application
