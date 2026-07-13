// Package mcpapp adapts the protocol-neutral SDD application to MCP.
//
// New registers the shared tool surface over an sdd.Application. Handler
// exposes the raw Streamable HTTP application so an embedding composition can
// place its own authentication middleware in front. Authentication middleware
// must populate the MCP SDK's TokenInfo on every request; mcpapp translates
// that current request identity into sdd.RequestIdentity and never treats the
// session's initialization identity as authorization proof.
//
// RunStdio and RunHTTP are local convenience hosts. External applications
// normally use Handler and own transport policy, project routing, storage,
// LLM, embeddings, search index, and mutation finalizers through public SDD
// ports.
package mcpapp
