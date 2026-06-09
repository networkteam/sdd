package cliout

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func entry(level slog.Level, msg string) LogEntry {
	return LogEntry{Time: time.Now(), Level: level, Message: msg}
}

func messages(entries []LogEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Message
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPolicy_CaptureFloor(t *testing.T) {
	cases := []struct {
		name string
		p    Policy
		want slog.Level
	}{
		{"display only", Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}, slog.LevelInfo},
		{"keep below display", Policy{Display: slog.LevelWarn, KeepAtOrAbove: slog.LevelInfo}, slog.LevelInfo},
		{"fingers-crossed pulls to debug", Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn, FingersCrossed: &FingersCrossed{Trigger: slog.LevelError, Tail: 5}}, slog.LevelDebug},
		{"nil display defaults info", Policy{KeepAtOrAbove: slog.LevelError}, slog.LevelInfo},
	}
	for _, c := range cases {
		if got := c.p.CaptureFloor(); got != c.want {
			t.Errorf("%s: CaptureFloor = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPolicy_ShowInDisplay(t *testing.T) {
	p := Policy{Display: slog.LevelInfo}
	if p.ShowInDisplay(slog.LevelDebug) {
		t.Error("Debug should not show at Info display floor")
	}
	if !p.ShowInDisplay(slog.LevelInfo) {
		t.Error("Info should show at Info display floor")
	}
}

func TestRecorder_KeepAtOrAbove(t *testing.T) {
	p := Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}
	r := NewRecorder(p)
	r.Observe(entry(slog.LevelInfo, "info"))   // below keep
	r.Observe(entry(slog.LevelWarn, "warn"))   // kept
	r.Observe(entry(slog.LevelError, "error")) // kept

	got := messages(r.Flush())
	if !eq(got, []string{"warn", "error"}) {
		t.Errorf("kept = %v, want [warn error]", got)
	}
}

func TestRecorder_NoKeepNoFlush(t *testing.T) {
	p := Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelError}
	r := NewRecorder(p)
	r.Observe(entry(slog.LevelInfo, "a"))
	r.Observe(entry(slog.LevelWarn, "b"))
	if got := r.Flush(); got != nil {
		t.Errorf("expected nil flush with nothing kept, got %v", messages(got))
	}
}

func TestRecorder_FingersCrossedFlushesTailOnTrigger(t *testing.T) {
	p := Policy{
		Display:        slog.LevelInfo,
		KeepAtOrAbove:  slog.LevelError, // nothing kept until the error
		FingersCrossed: &FingersCrossed{Trigger: slog.LevelError, Tail: 3},
	}
	r := NewRecorder(p)
	r.Observe(entry(slog.LevelDebug, "d")) // all levels enter the tail
	r.Observe(entry(slog.LevelInfo, "i"))
	r.Observe(entry(slog.LevelWarn, "w"))
	if r.Failed() {
		t.Fatal("should not be armed before the trigger")
	}
	r.Observe(entry(slog.LevelError, "err")) // arms the flush

	if !r.Failed() {
		t.Fatal("error at trigger level should arm the flush")
	}
	// Tail=3 keeps the last three observed (i, w, err); the Error is also kept,
	// deduped to a single occurrence, all in arrival order.
	got := messages(r.Flush())
	if !eq(got, []string{"i", "w", "err"}) {
		t.Errorf("flush = %v, want [i w err]", got)
	}
}

func TestRecorder_FingersCrossedNotTriggered(t *testing.T) {
	p := Policy{
		Display:        slog.LevelInfo,
		KeepAtOrAbove:  slog.LevelError,
		FingersCrossed: &FingersCrossed{Trigger: slog.LevelError, Tail: 5},
	}
	r := NewRecorder(p)
	r.Observe(entry(slog.LevelInfo, "a"))
	r.Observe(entry(slog.LevelWarn, "b"))
	if got := r.Flush(); got != nil {
		t.Errorf("no trigger and nothing kept → nil flush, got %v", messages(got))
	}
}

func TestRecorder_MarkFailedArmsFlush(t *testing.T) {
	p := Policy{
		Display:        slog.LevelInfo,
		KeepAtOrAbove:  slog.LevelError,
		FingersCrossed: &FingersCrossed{Trigger: slog.LevelError, Tail: 2},
	}
	r := NewRecorder(p)
	r.Observe(entry(slog.LevelInfo, "x"))
	r.Observe(entry(slog.LevelInfo, "y"))
	r.MarkFailed() // operation returned an error without logging at the trigger level

	got := messages(r.Flush())
	if !eq(got, []string{"x", "y"}) {
		t.Errorf("flush = %v, want [x y] (tail flushed on external failure)", got)
	}
}

func TestRecorder_TailRingWraps(t *testing.T) {
	p := Policy{
		Display:        slog.LevelInfo,
		KeepAtOrAbove:  slog.LevelError,
		FingersCrossed: &FingersCrossed{Trigger: slog.LevelError, Tail: 2},
	}
	r := NewRecorder(p)
	for _, m := range []string{"1", "2", "3", "4"} {
		r.Observe(entry(slog.LevelInfo, m))
	}
	r.MarkFailed()
	got := messages(r.Flush())
	if !eq(got, []string{"3", "4"}) {
		t.Errorf("flush = %v, want [3 4] (only last Tail kept)", got)
	}
}

func TestRecorder_DedupKeptAndTail(t *testing.T) {
	p := Policy{
		Display:        slog.LevelInfo,
		KeepAtOrAbove:  slog.LevelWarn, // warn+error kept
		FingersCrossed: &FingersCrossed{Trigger: slog.LevelError, Tail: 5},
	}
	r := NewRecorder(p)
	r.Observe(entry(slog.LevelInfo, "i"))
	r.Observe(entry(slog.LevelWarn, "w"))  // kept AND in tail
	r.Observe(entry(slog.LevelError, "e")) // kept AND in tail, arms flush

	got := messages(r.Flush())
	// Each entry once, in arrival order: i (tail only), w, e.
	if !eq(got, []string{"i", "w", "e"}) {
		t.Errorf("flush = %v, want [i w e] with no duplicates", got)
	}
}

// captureHandler records the records routed to a durable sink for assertion.
type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func TestWriteEntries_PreservesLevelMessageAttrs(t *testing.T) {
	capt := &captureHandler{}
	sink := slog.New(capt)
	entries := []LogEntry{
		{Level: slog.LevelWarn, Message: "drift", Attrs: []slog.Attr{slog.String("entry", "d-tac-5g9")}},
		{Level: slog.LevelError, Message: "boom"},
	}
	WriteEntries(context.Background(), sink, entries)

	if len(capt.records) != 2 {
		t.Fatalf("re-emitted %d records, want 2", len(capt.records))
	}
	if capt.records[0].Level != slog.LevelWarn || capt.records[0].Message != "drift" {
		t.Errorf("record 0 = %v %q, want WARN drift", capt.records[0].Level, capt.records[0].Message)
	}
	var foundAttr bool
	capt.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "entry" && a.Value.String() == "d-tac-5g9" {
			foundAttr = true
		}
		return true
	})
	if !foundAttr {
		t.Error("attr not preserved through WriteEntries")
	}
}

func TestWriteEntries_NilSinkIsNoop(t *testing.T) {
	// Must not panic.
	WriteEntries(context.Background(), nil, []LogEntry{{Message: "x"}})
}
