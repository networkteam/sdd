package main

import (
	"github.com/networkteam/sdd/internal/cliout"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
)

// embedProgress bridges the embedding and cache-freshening command callbacks
// onto one cliout.Reporter, shared by `sdd index` and `sdd search`.
type embedProgress struct {
	reporter *cliout.Reporter
	total    int
	curRepo  string
}

func newEmbedProgress() *embedProgress {
	r := cliout.NewReporter()
	r.SetUnit("chunks")
	return &embedProgress{reporter: r}
}

// onPlanned grows the running total (member work is only known after each cache
// is fresh) and declares the indexing phase once real embedding work is planned
// — a zero-chunk (warm) plan neither advances the bar nor arms a footer.
func (p *embedProgress) onPlanned(n int) {
	if n > 0 {
		p.reporter.SetPhase(model.PhaseIndexing)
	}
	p.total += n
	p.reporter.SetTotal(p.total)
}

func (p *embedProgress) onBatchStart(ids []string, chunks int) {
	note := embedNote(ids, chunks)
	if p.curRepo != "" {
		note = p.curRepo + " · " + note
	}
	p.reporter.SetNote(note)
}

func (p *embedProgress) onEntryIndexed(_ string, chunks int) { p.reporter.Add(chunks) }

func (p *embedProgress) onRepoStart(id string) { p.curRepo = id }

// onPhase maps a handler-reported freshening phase onto the footer and clears
// the stale embed note — a cache pull is not embedding any batch.
func (p *embedProgress) onPhase(ph model.Phase) {
	if ph == model.PhaseConnecting || ph == model.PhaseSyncing {
		p.reporter.SetNote("")
	}
	p.reporter.SetPhase(ph)
}

// localBuild wires the local index build; onComplete carries the command's own
// summary totals back to the caller.
func (p *embedProgress) localBuild(force bool, onComplete func(indexed, skipped int)) *command.BuildIndexCmd {
	return &command.BuildIndexCmd{
		Force:          force,
		OnPlanned:      p.onPlanned,
		OnBatchStart:   p.onBatchStart,
		OnEntryIndexed: p.onEntryIndexed,
		OnComplete:     onComplete,
	}
}

// lazyFill wires the local lazy-fill that precedes a vector search.
func (p *embedProgress) lazyFill() *command.LazyFillIndexCmd {
	return &command.LazyFillIndexCmd{
		OnPlanned:      p.onPlanned,
		OnBatchStart:   p.onBatchStart,
		OnEntryIndexed: p.onEntryIndexed,
	}
}

// connected wires the connected-repo fill, keeping the footer label phase-true
// across the syncing → indexing transitions.
func (p *embedProgress) connected(force bool) *command.BuildConnectedIndexesCmd {
	return &command.BuildConnectedIndexesCmd{
		Force:          force,
		OnRepoStart:    p.onRepoStart,
		OnPlanned:      p.onPlanned,
		OnBatchStart:   p.onBatchStart,
		OnEntryIndexed: p.onEntryIndexed,
		OnPhase:        p.onPhase,
	}
}
