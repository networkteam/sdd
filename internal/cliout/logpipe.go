// Package cliout is the bubble-tea-free core of the interactive terminal
// output experience for long-running sdd commands. It carries the log pipe
// (a slog.Handler whose records flow over a bounded channel to a display
// consumer), an absolute-count progress reporter, and the durable-record
// policy that decides which ephemeral entries survive teardown.
//
// Producers stay oblivious: handlers and finders log only through the
// context logger and never import this package. The CLI surface installs
// the pipe's handler when stderr is an interactive terminal and drives the
// transient view (in the sibling internal/cliout/tui package); otherwise it
// leaves the plain leveled stderr handler in place. This is the audience
// separation the terminal-experience architecture directive (d-cpt-mvb)
// draws — terminal-UI code out of the producer layers.
package cliout

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// logChanCap bounds the producer→display channel. Sends are non-blocking
// (drop-on-full), so this caps queue depth, not correctness — progress is
// reported as absolute counts and re-emitted kept entries come from the
// recorder, not the display channel.
const logChanCap = 64

// LogEntry is a snapshot of a slog.Record taken at Handle time. The record
// itself must not be retained past Handle, so the level, message, and the
// fully-accumulated attribute set (including any WithAttrs / WithGroup
// state) are copied out here. Rendered with structured styling at display
// time, not flattened to a string at capture.
type LogEntry struct {
	Time    time.Time
	Level   slog.Level
	Message string
	Attrs   []slog.Attr
}

// NewLogPipe builds a log pipe: a slog.Handler the caller installs for the
// operation, and a LogConsumer the display loop drains. capture is the floor
// level admitted into the pipe — the display may filter higher, and the
// policy recorder needs everything down to this floor (see Policy.CaptureFloor).
// Neither end references a bubble tea program, so there is no construction
// cycle between the pipe and the model that consumes it.
func NewLogPipe(capture slog.Leveler) (slog.Handler, *LogConsumer) {
	c := NewLogConsumer(logChanCap)
	return newPipeHandler(capture, c.Offer), c
}

// newPipeHandler builds the slog front that snapshots each admitted record into
// a LogEntry and hands it to sink. The coordinator uses this to route records
// into its lifecycle; NewLogPipe uses it to feed a LogConsumer directly.
func newPipeHandler(capture slog.Leveler, sink func(LogEntry)) *pipeHandler {
	return &pipeHandler{level: capture, sink: sink}
}

// pipeHandler is a slog.Handler that snapshots each record and hands it to a
// sink. WithAttrs/WithGroup state is accumulated so the codebase's
// `…FromContext(ctx).With("command", …)` pattern is honored rather than
// silently dropped.
type pipeHandler struct {
	level       slog.Leveler
	sink        func(LogEntry)
	groupPrefix string
	attrs       []slog.Attr // already group-prefixed at WithAttrs time
}

func (h *pipeHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *pipeHandler) Handle(_ context.Context, r slog.Record) error {
	// Snapshot: start from the accumulated WithAttrs set, then copy the
	// record's own attrs (group-prefixed). The slog.Record is not retained.
	attrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, h.prefix(a))
		return true
	})

	h.sink(LogEntry{Time: r.Time, Level: r.Level, Message: r.Message, Attrs: attrs})
	return nil
}

func (h *pipeHandler) WithAttrs(as []slog.Attr) slog.Handler {
	if len(as) == 0 {
		return h
	}
	nh := h.clone()
	for _, a := range as {
		nh.attrs = append(nh.attrs, h.prefix(a))
	}
	return nh
}

func (h *pipeHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	nh := h.clone()
	nh.groupPrefix = h.groupPrefix + name + "."
	return nh
}

func (h *pipeHandler) clone() *pipeHandler {
	attrs := make([]slog.Attr, len(h.attrs))
	copy(attrs, h.attrs)
	return &pipeHandler{
		level:       h.level,
		sink:        h.sink,
		groupPrefix: h.groupPrefix,
		attrs:       attrs,
	}
}

// prefix qualifies an attribute key with the current group path so flat
// rendering keeps group-nested keys distinct (mirrors slog's group
// semantics as the CLI handler renders them).
func (h *pipeHandler) prefix(a slog.Attr) slog.Attr {
	if h.groupPrefix == "" {
		return a
	}
	return slog.Attr{Key: h.groupPrefix + a.Key, Value: a.Value}
}

// LogConsumer is the display end of a log pipe. Recv hands the next entry to
// the view loop; Close signals end-of-work so a drained Recv reports done.
type LogConsumer struct {
	ch        chan LogEntry
	done      chan struct{}
	closeOnce sync.Once
}

// NewLogConsumer builds a consumer over a channel of the given capacity. The
// coordinator uses this to hand live lines to a running program; Offer feeds it.
func NewLogConsumer(capacity int) *LogConsumer {
	return &LogConsumer{ch: make(chan LogEntry, capacity), done: make(chan struct{})}
}

// Offer sends an entry non-blocking: a full channel drops it rather than
// stalling the producer, bounding time coupling between producer and display.
func (c *LogConsumer) Offer(e LogEntry) {
	select {
	case c.ch <- e:
	default:
	}
}

// Recv returns the next entry, or ok=false once work has finished and the
// channel is drained. Buffered entries are always preferred over the done
// signal so the tail isn't dropped at teardown.
func (c *LogConsumer) Recv() (LogEntry, bool) {
	// Drain a pending entry first — even after Close, the tail must flush.
	select {
	case e := <-c.ch:
		return e, true
	default:
	}
	select {
	case e := <-c.ch:
		return e, true
	case <-c.done:
		return LogEntry{}, false
	}
}

// Close signals that no further entries will be produced. Idempotent. After
// Close and once the channel is drained, Recv returns ok=false.
func (c *LogConsumer) Close() {
	c.closeOnce.Do(func() { close(c.done) })
}
