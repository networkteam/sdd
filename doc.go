// Package sdd documents the public packages of the SDD framework, which live
// under pkg/.
//
// Package pkg/application owns the protocol-neutral runtime, its request and
// result types, and the infrastructure ports an embedding host implements.
// Package pkg/llm is the public LLM boundary: the one-method Runner contract
// over shared Request, Result, Identity, and Usage types. Package pkg/local
// provides the filesystem and in-memory adapters used by the standalone CLI
// and available to local embeddings. Package pkg/mcpapp exposes the shared MCP
// handler, while package pkg/sddtest provides reusable adapter conformance
// suites.
package sdd
