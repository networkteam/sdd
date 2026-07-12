// Package sdd exposes the protocol-neutral SDD application contract.
//
// The package deliberately owns SDD semantics. Compositions provide
// authentication and infrastructure through the ports declared here; they do
// not replace graph reads, procedure execution, pre-flight, or write gates.
package sdd
