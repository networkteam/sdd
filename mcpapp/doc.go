// Package mcpapp adapts the protocol-neutral SDD application to MCP.
//
// New registers the shared tool surface over an application.Application. Handler
// exposes the raw Streamable HTTP application so an embedding composition can
// place its own authentication middleware in front. Authentication middleware
// must populate the MCP SDK's TokenInfo on every request; mcpapp translates
// that current request identity into application.RequestIdentity and never treats the
// session's initialization identity as authorization proof.
//
// RunStdio is the local process transport. HTTP compositions use Handler,
// coordinate its lifecycle through Shutdown, and own the listener, transport
// policy, project routing, storage, LLM, embeddings, search index, and
// mutation finalizers through public SDD ports.
package mcpapp
