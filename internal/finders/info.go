package finders

import "github.com/networkteam/sdd/internal/query"

// Info gathers session framing for the `sdd info` command and as a
// header for `sdd status`. Pure config inspection — no graph access
// required.
func (f *Finder) Info(_ query.InfoQuery) (*query.InfoResult, error) {
	return &query.InfoResult{
		LocalParticipant: f.localParticipant(),
		Language:         f.language(),
		Search:           f.searchCapability(),
	}, nil
}
