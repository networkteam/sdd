// Package embed is the public embedding boundary, the twin of pkg/llm for
// vectors (20260902-114838-d-tac-cov): one two-method interface over pure
// request and result types, sharing Identity, Usage, and the call record with
// the chat side, plus the same three decorators. Provider adapters and the
// configuration that selects them stay at each host's composition site.
package embed

import (
	"context"

	"github.com/networkteam/sdd/pkg/llm"
)

// Purpose names which side of retrieval a batch serves. Instruction-tuned
// encoders want different prefixes on documents and queries, so an adapter
// selects its template by purpose; the value doubles as the observability
// dimension, the same as a chat Purpose.
type Purpose string

const (
	PurposeDocument Purpose = "embed-document"
	PurposeQuery    Purpose = "embed-query"
)

// Request is one batch of texts to embed for one purpose.
type Request struct {
	Purpose Purpose
	Texts   []string
}

// Result carries one vector per input text, in input order, and reports what
// served the call.
type Result struct {
	Vectors  [][]float32
	Identity llm.Identity
	Usage    llm.Usage
}

// Embedder is the single port. Embed returns exactly len(req.Texts) vectors in
// order and fills Result.Identity on success; a failed call may attribute
// itself through *llm.Error. Fingerprint identifies the vector space the
// embedder produces and must be stable across calls: two embedders whose
// document vectors are comparable share it, two whose vectors are not, do not.
type Embedder interface {
	Embed(ctx context.Context, req Request) (Result, error)
	Fingerprint() string
}

// EmbedderFunc adapts a function to Embedder for test doubles and stubs; Space
// is what Fingerprint returns.
type EmbedderFunc struct {
	Space string
	Run   func(context.Context, Request) (Result, error)
}

func (f EmbedderFunc) Embed(ctx context.Context, req Request) (Result, error) {
	return f.Run(ctx, req)
}

func (f EmbedderFunc) Fingerprint() string {
	return f.Space
}
