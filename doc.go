// Package sdd exposes the protocol-neutral SDD application contract.
//
// The package deliberately owns SDD semantics. Compositions provide
// authentication and infrastructure through the ports declared here; they do
// not replace graph reads, procedure execution, pre-flight, or write gates.
// Application resolves current access for every operation and owns durable
// workflow sessions, reads, search, prepared mutations, recovery, and
// finalizers. ProjectRuntime binds one project to its infrastructure ports;
// AccessResolver selects an authorized runtime per request. The sddtest
// package provides reusable conformance suites for port implementations, and
// package mcpapp exposes the shared MCP adapter.
package sdd
