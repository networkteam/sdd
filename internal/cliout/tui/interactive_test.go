package tui

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/cliout"
)

// captureHandler is a durable sink stand-in: it records the messages re-emitted
// to it on teardown. Guarded by a mutex since slog handlers may be hit from any
// goroutine.
type captureHandler struct {
	mu   sync.Mutex
	msgs []string
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgs = append(h.msgs, r.Message)
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }
func (h *captureHandler) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.msgs))
	copy(out, h.msgs)
	return out
}

// fakeRun mimics the bubble tea event loop without a terminal: it drains the
// log pipe into the recorder (exactly what the model's logMsg branch does),
// then returns the final model. This exercises the coordinator's real
// machinery — logger swap/restore, the work goroutine and its sync, and the
// teardown re-emit — independent of any rendering.
func fakeRun(m tea.Model) (tea.Model, error) {
	mm := m.(model)
	for {
		e, ok := mm.logs.Recv()
		if !ok {
			break
		}
		mm.rec.Observe(e)
	}
	return mm, nil
}

func TestInteractive_ReturnsResultReEmitsKeptRestoresLogger(t *testing.T) {
	realOrig := slog.Default()
	defer slog.SetDefault(realOrig)

	durable := &captureHandler{}
	orig := slog.New(durable)
	slog.SetDefault(orig)

	policy := cliout.Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}
	work := func(ctx context.Context) (int, error) {
		l := slogutils.FromContext(ctx)
		l.Info("indexed", "entry", "a") // below keep → not re-emitted
		l.Warn("slow embed")            // kept → re-emitted on teardown
		return 42, nil
	}

	val, err := interactiveWith(context.Background(), policy, View{Label: "indexing"}, work, fakeRun)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("result = %d, want 42", val)
	}
	if slog.Default() != orig {
		t.Error("durable logger was not restored after the operation")
	}

	msgs := durable.messages()
	if len(msgs) != 1 || msgs[0] != "slow embed" {
		t.Errorf("durable sink = %v, want only the kept Warn [slow embed]", msgs)
	}
}

func TestInteractive_PropagatesErrorAndFingersCrossedFlush(t *testing.T) {
	realOrig := slog.Default()
	defer slog.SetDefault(realOrig)

	durable := &captureHandler{}
	slog.SetDefault(slog.New(durable))

	policy := cliout.Policy{
		Display:        slog.LevelInfo,
		KeepAtOrAbove:  slog.LevelError, // nothing kept by level alone
		FingersCrossed: &cliout.FingersCrossed{Trigger: slog.LevelError, Tail: 10},
	}
	boom := errors.New("embed failed")
	work := func(ctx context.Context) (int, error) {
		l := slogutils.FromContext(ctx)
		l.Info("a")
		l.Info("b")
		return 0, boom // no Error logged; the returned error arms the flush
	}

	_, err := interactiveWith(context.Background(), policy, View{Label: "indexing"}, work, fakeRun)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}

	msgs := durable.messages()
	if len(msgs) != 2 || msgs[0] != "a" || msgs[1] != "b" {
		t.Errorf("durable sink = %v, want fingers-crossed tail [a b]", msgs)
	}
}

func TestInteractive_NoErrorNoKeptIsSilent(t *testing.T) {
	realOrig := slog.Default()
	defer slog.SetDefault(realOrig)

	durable := &captureHandler{}
	slog.SetDefault(slog.New(durable))

	policy := cliout.Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}
	work := func(ctx context.Context) (int, error) {
		slogutils.FromContext(ctx).Info("routine noise")
		return 7, nil
	}

	val, err := interactiveWith(context.Background(), policy, View{Label: "indexing"}, work, fakeRun)
	if err != nil || val != 7 {
		t.Fatalf("got (%d, %v), want (7, nil)", val, err)
	}
	if msgs := durable.messages(); len(msgs) != 0 {
		t.Errorf("durable sink = %v, want empty (routine info disappears with the view)", msgs)
	}
}
