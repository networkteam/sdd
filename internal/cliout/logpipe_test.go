package cliout

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// attrValue finds an attr by key in a LogEntry, returning its string value.
func attrValue(e LogEntry, key string) (string, bool) {
	for _, a := range e.Attrs {
		if a.Key == key {
			return a.Value.String(), true
		}
	}
	return "", false
}

func TestLogPipe_WithAttrsAccumulated(t *testing.T) {
	h, c := NewLogPipe(slog.LevelDebug)
	// Mirrors the codebase pattern: …FromContext(ctx).With("command", …).
	logger := slog.New(h).With("command", "search")
	logger.Info("lazy-indexed", "entry", "d-tac-5g9", "chunks", 3)

	e, ok := c.Recv()
	if !ok {
		t.Fatal("expected an entry")
	}
	if e.Message != "lazy-indexed" {
		t.Errorf("message = %q, want lazy-indexed", e.Message)
	}
	if v, ok := attrValue(e, "command"); !ok || v != "search" {
		t.Errorf("accumulated WithAttrs dropped: command=%q ok=%v", v, ok)
	}
	if v, ok := attrValue(e, "entry"); !ok || v != "d-tac-5g9" {
		t.Errorf("record attr missing: entry=%q ok=%v", v, ok)
	}
	if v, ok := attrValue(e, "chunks"); !ok || v != "3" {
		t.Errorf("record attr missing: chunks=%q ok=%v", v, ok)
	}
}

func TestLogPipe_WithGroupPrefixesKeys(t *testing.T) {
	h, c := NewLogPipe(slog.LevelDebug)
	logger := slog.New(h).WithGroup("idx").With("phase", "embed")
	logger.Info("batch", "n", 5)

	e, ok := c.Recv()
	if !ok {
		t.Fatal("expected an entry")
	}
	if _, ok := attrValue(e, "idx.phase"); !ok {
		t.Errorf("WithGroup did not prefix WithAttrs key; attrs=%v", e.Attrs)
	}
	if _, ok := attrValue(e, "idx.n"); !ok {
		t.Errorf("WithGroup did not prefix record key; attrs=%v", e.Attrs)
	}
}

func TestLogPipe_LevelFloorFilters(t *testing.T) {
	h, c := NewLogPipe(slog.LevelInfo)
	logger := slog.New(h)
	logger.Debug("below floor") // dropped by Enabled
	logger.Info("at floor")     // passes
	logger.Warn("above floor")  // passes

	var got []string
	c.Close()
	for {
		e, ok := c.Recv()
		if !ok {
			break
		}
		got = append(got, e.Message)
	}
	if len(got) != 2 || got[0] != "at floor" || got[1] != "above floor" {
		t.Errorf("level floor not applied: got %v", got)
	}
}

func TestLogPipe_NonBlockingDropOnFull(t *testing.T) {
	h, c := NewLogPipe(slog.LevelDebug)
	logger := slog.New(h)

	// No concurrent drain: if the send blocked, this loop would deadlock and
	// the test would time out — completing it proves the producer never waits.
	const sent = logChanCap * 3
	for range sent {
		logger.Info("noise")
	}

	c.Close()
	received := 0
	for {
		if _, ok := c.Recv(); !ok {
			break
		}
		received++
	}
	if received != logChanCap {
		t.Errorf("received %d entries, want exactly the channel capacity %d (excess must drop)", received, logChanCap)
	}
}

func TestLogConsumer_DrainsTailThenDone(t *testing.T) {
	h, c := NewLogPipe(slog.LevelDebug)
	logger := slog.New(h)
	logger.Info("one")
	logger.Info("two")
	c.Close() // close before draining: buffered tail must still flush

	first, ok := c.Recv()
	if !ok || first.Message != "one" {
		t.Fatalf("first recv = %q ok=%v, want one", first.Message, ok)
	}
	second, ok := c.Recv()
	if !ok || second.Message != "two" {
		t.Fatalf("second recv = %q ok=%v, want two", second.Message, ok)
	}
	if _, ok := c.Recv(); ok {
		t.Error("expected ok=false after tail drained and Close")
	}
}

func TestLogConsumer_CloseIdempotent(t *testing.T) {
	_, c := NewLogPipe(slog.LevelDebug)
	c.Close()
	c.Close() // must not panic on double close
	if _, ok := c.Recv(); ok {
		t.Error("expected ok=false after close")
	}
}

func TestLogPipe_HandleDoesNotRetainRecord(t *testing.T) {
	// Enabled gate is honored by slog.Logger; here we drive Handle directly to
	// confirm attrs are copied out (the record is not aliased into the entry).
	h, c := NewLogPipe(slog.LevelDebug)
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.String("k", "v"))
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	e, ok := c.Recv()
	if !ok {
		t.Fatal("expected entry")
	}
	if v, ok := attrValue(e, "k"); !ok || v != "v" {
		t.Errorf("attr not snapshotted: k=%q ok=%v", v, ok)
	}
}
