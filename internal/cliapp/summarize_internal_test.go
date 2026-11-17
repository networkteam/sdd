package cliapp

import (
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/cliout"
	"github.com/networkteam/sdd/internal/command"
)

// The progress bar shows only on a terminal, so newFillProgress has no seam
// through the command; summarize workers call its callbacks concurrently.
func TestFillProgressEndsAtTheLastEntry(t *testing.T) {
	fill := &command.SummarizeCmd{}
	reporter := newFillProgress(fill)
	fill.OnPlanned(2)

	firstHeld, secondDone := make(chan struct{}), make(chan struct{})
	held := false
	reporter.Notify(func(p cliout.Progress) {
		if held || p.Note != "first" {
			return
		}
		held = true
		close(firstHeld)
		select {
		case <-secondDone:
		case <-time.After(200 * time.Millisecond):
		}
	})
	firstDone := make(chan struct{})
	go func() {
		fill.OnSummarized("first", "")
		close(firstDone)
	}()
	<-firstHeld
	go func() {
		fill.OnTitled("second", "")
		close(secondDone)
	}()
	<-firstDone
	<-secondDone
	reporter.Close()

	var last cliout.Progress
	for p, ok := reporter.Recv(); ok; p, ok = reporter.Recv() {
		last = p
	}
	if last.Done != 2 || last.Total != 2 {
		t.Errorf("the bar should end at the total once every entry reported, got %d of %d", last.Done, last.Total)
	}
}
