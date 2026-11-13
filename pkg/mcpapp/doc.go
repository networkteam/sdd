// Package mcpapp adapts the protocol-neutral SDD application to MCP.
//
// Register adds the shared tool surface over an application.Application to an
// MCP server the host constructs, beside tools of the host's own. New builds a
// server carrying SDD's tools alone; its Handler exposes stateless Streamable
// HTTP so an embedding composition can place its own authentication middleware
// in front. Authentication middleware
// must populate the MCP SDK's TokenInfo on every request; mcpapp translates
// that current request identity into application.RequestIdentity and never treats the
// session's initialization identity as authorization proof.
//
// HTTP uses no transport session headers or standalone GET stream. POST responses
// may stream SSE. Client name and version from initialization may be unavailable.
//
// Run accepts a caller-supplied transport; RunStdio uses process stdin/stdout.
// HTTP compositions use Handler,
// coordinate its lifecycle through Shutdown, and own the listener, transport
// policy, project routing, storage, LLM, embeddings, search index, and
// mutation finalizers through public SDD ports.
package mcpapp
