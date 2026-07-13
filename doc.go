// Package sdd documents the public packages of the SDD framework.
//
// Package application owns the protocol-neutral runtime, its request and
// result types, and the infrastructure ports an embedding host implements.
// Package local provides the filesystem and in-memory adapters used by the
// standalone CLI and available to local embeddings. Package mcpapp exposes the
// shared MCP handler, while package sddtest provides reusable adapter
// conformance suites.
package sdd
