package main

import (
	"testing"

	"github.com/networkteam/sdd/internal/command"
)

// The shared wiring helper maps every command's callbacks onto one reporter: a
// zero-chunk (warm) plan reports no phase, real embedding work reports indexing,
// and a freshen phase (syncing) is not clobbered by a subsequent zero-chunk
// plan — so a text-only cross-repo read stays labeled syncing, never indexing.
func TestEmbedProgress_PhaseMapping(t *testing.T) {
	p := newEmbedProgress()

	p.onPlanned(0)
	if got := latestPhase(t, p); got != "" {
		t.Errorf("warm plan should report no phase; got %q", got)
	}

	p.connected(false).OnPhase(command.PhaseSyncing)
	if got := latestPhase(t, p); got != command.PhaseSyncing {
		t.Errorf("freshen should report syncing; got %q", got)
	}

	p.onPlanned(0) // a warm member fill must not overwrite syncing with indexing
	if got := latestPhase(t, p); got != command.PhaseSyncing {
		t.Errorf("zero-chunk plan must not clobber syncing; got %q", got)
	}

	p.onPlanned(5) // real embedding work
	if got := latestPhase(t, p); got != command.PhaseIndexing {
		t.Errorf("real embedding should report indexing; got %q", got)
	}
}

func latestPhase(t *testing.T, p *embedProgress) command.Phase {
	t.Helper()
	snap, ok := p.reporter.Recv()
	if !ok {
		t.Fatal("reporter closed unexpectedly")
	}
	return snap.Phase
}
