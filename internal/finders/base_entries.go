package finders

import (
	"fmt"

	"github.com/networkteam/sdd/internal/basefacts"
	"github.com/networkteam/sdd/internal/baseprocedures"
	"github.com/networkteam/sdd/internal/model"
)

// BaseEntries assembles every embedded base entry shipped in the binary: the
// always-loaded base procedures and the base facts rendered against the live
// layout vocabulary. Both graph load paths (LoadGraph here, BuildSnapshot in
// application) merge this set with disk-wins precedence via model.MergeEmbedded,
// so the "what ships embedded, and with which vocabulary" wiring lives in one
// place. The embedded set is compile-time-shaped, so an error is a broken build.
func BaseEntries() ([]*model.Entry, error) {
	procedures, err := baseprocedures.Entries()
	if err != nil {
		return nil, fmt.Errorf("loading base procedures: %w", err)
	}
	facts, err := basefacts.Entries(LiveViewVocabulary())
	if err != nil {
		return nil, fmt.Errorf("loading base facts: %w", err)
	}
	return append(procedures, facts...), nil
}
