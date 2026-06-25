package finders

import "github.com/networkteam/sdd/internal/query"

// Info gathers session framing for the `sdd info` command — the session
// header surface that skill session-start injections read. Pure config
// inspection — no graph access required.
func (f *Finder) Info(_ query.InfoQuery) (*query.InfoResult, error) {
	return &query.InfoResult{
		LocalParticipant: f.localParticipant(),
		Language:         f.language(),
		Search:           f.searchCapability(),
	}, nil
}

// searchCapability reports whether vector search is configured. Returns
// "vector,text" when an embedding provider is set, "text" otherwise.
// Pure config inspection — actual index health (drift, missing rows) is
// reported by `sdd lint`.
func (f *Finder) searchCapability() string {
	if f.cfg != nil && f.cfg.Embedding.Provider != "" {
		return "vector,text"
	}
	return "text"
}
