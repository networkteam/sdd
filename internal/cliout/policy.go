package cliout

import (
	"context"
	"log/slog"
	"sort"
)

// Policy decouples the durable record from the ephemeral display. The live
// view can be chatty (it vanishes on teardown), while only deliberately kept
// entries survive to the durable stderr sink.
type Policy struct {
	// Display is the level floor for the live view — typically chattier than
	// the durable floor, since the view is transient.
	Display slog.Leveler

	// KeepAtOrAbove re-emits entries at or above this level to the durable
	// sink on teardown, independent of the live display level.
	KeepAtOrAbove slog.Level

	// FingersCrossed, when set, retains a tail of recent entries at all
	// captured levels and flushes them to the durable sink if an error
	// occurs — so the context around a failure survives even though it was
	// never shown.
	FingersCrossed *FingersCrossed
}

// FingersCrossed configures the failure-triggered tail flush.
type FingersCrossed struct {
	// Trigger is the level that arms the flush when observed.
	Trigger slog.Level
	// Tail is the number of most-recent entries (all captured levels) kept
	// for the flush.
	Tail int
}

// displayLevel returns the configured display floor, defaulting to Info.
func (p Policy) displayLevel() slog.Level {
	if p.Display == nil {
		return slog.LevelInfo
	}
	return p.Display.Level()
}

// CaptureFloor is the minimum level the log pipe must admit so the policy can
// do its job: the display floor, the keep threshold, and — when fingers-crossed
// is armed — everything down to Debug, since a flush must be able to show
// context that was never displayed.
func (p Policy) CaptureFloor() slog.Level {
	floor := min(p.displayLevel(), p.KeepAtOrAbove)
	if p.FingersCrossed != nil {
		floor = min(floor, slog.LevelDebug)
	}
	return floor
}

// ShowInDisplay reports whether an entry at the given level belongs in the
// live view.
func (p Policy) ShowInDisplay(level slog.Level) bool {
	return level >= p.displayLevel()
}

// Recorder accumulates the entries a Policy will re-emit to the durable sink
// on teardown. It is fed every entry that enters the pipe (Observe) and
// produces the re-emit list (Flush). Fully independent of the display loop and
// of bubble tea, so it is unit-testable on its own.
type Recorder struct {
	policy Policy
	seq    int

	kept   []seqEntry // entries >= KeepAtOrAbove, in arrival order
	tail   []seqEntry // ring of last FingersCrossed.Tail entries (all captured levels)
	tailAt int        // ring write cursor
	failed bool
}

type seqEntry struct {
	seq int
	e   LogEntry
}

// NewRecorder builds a recorder for the given policy.
func NewRecorder(p Policy) *Recorder {
	return &Recorder{policy: p}
}

// Observe records an entry that entered the pipe. Entries at or above the
// keep threshold are retained for re-emit; when fingers-crossed is armed the
// entry also enters the tail ring, and an entry at the trigger level arms the
// flush.
func (r *Recorder) Observe(e LogEntry) {
	r.seq++
	se := seqEntry{seq: r.seq, e: e}

	if e.Level >= r.policy.KeepAtOrAbove {
		r.kept = append(r.kept, se)
	}

	if fc := r.policy.FingersCrossed; fc != nil && fc.Tail > 0 {
		if len(r.tail) < fc.Tail {
			r.tail = append(r.tail, se)
		} else {
			r.tail[r.tailAt] = se
		}
		r.tailAt = (r.tailAt + 1) % fc.Tail
		if e.Level >= fc.Trigger {
			r.failed = true
		}
	}
}

// MarkFailed arms the fingers-crossed flush from outside the log stream — used
// when the operation returns an error without having logged at the trigger
// level.
func (r *Recorder) MarkFailed() { r.failed = true }

// Failed reports whether the fingers-crossed flush is armed.
func (r *Recorder) Failed() bool { return r.failed }

// Flush returns the entries to re-emit to the durable sink, in arrival order
// and deduplicated. Always includes the kept entries; when fingers-crossed is
// armed it additionally includes the retained tail (all captured levels).
func (r *Recorder) Flush() []LogEntry {
	bySeq := make(map[int]LogEntry, len(r.kept)+len(r.tail))
	for _, se := range r.kept {
		bySeq[se.seq] = se.e
	}
	if r.failed && r.policy.FingersCrossed != nil {
		for _, se := range r.tail {
			bySeq[se.seq] = se.e
		}
	}
	if len(bySeq) == 0 {
		return nil
	}

	seqs := make([]int, 0, len(bySeq))
	for s := range bySeq {
		seqs = append(seqs, s)
	}
	sort.Ints(seqs)

	out := make([]LogEntry, len(seqs))
	for i, s := range seqs {
		out[i] = bySeq[s]
	}
	return out
}

// WriteEntries re-logs entries through the durable sink (typically the
// pre-swap slog.Default), reconstructing each record with its original level,
// message, and attributes. The sink's own level filter and formatting apply.
func WriteEntries(ctx context.Context, sink *slog.Logger, entries []LogEntry) {
	if sink == nil {
		return
	}
	for _, e := range entries {
		sink.LogAttrs(ctx, e.Level, e.Message, e.Attrs...)
	}
}
