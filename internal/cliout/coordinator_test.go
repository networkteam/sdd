package cliout

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/model"
)

// fakeStarter stands in for the bubble tea program: it records the opening
// backlog it was handed and forwards each live line onto a channel, so the
// coordinator's armed→live handoff is observable without a terminal. unpainted
// is handed back to the coordinator, standing in for lines the gate never
// released.
type fakeStarter struct {
	started   chan struct{}
	backlog   []LogEntry
	lines     chan LogEntry
	unpainted []LogEntry
}

func newFakeStarter() *fakeStarter {
	return &fakeStarter{started: make(chan struct{}), lines: make(chan LogEntry, 16)}
}

func (s *fakeStarter) Start(backlog []LogEntry, live *LogConsumer) ([]LogEntry, error) {
	s.backlog = backlog
	close(s.started)
	for {
		e, ok := live.Recv()
		if !ok {
			break
		}
		s.lines <- e
	}
	return s.unpainted, nil
}

func (s *fakeStarter) didStart() bool {
	select {
	case <-s.started:
		return true
	default:
		return false
	}
}

func TestCoordinator_DormantRendersPlainNoProgram(t *testing.T) {
	var buf bytes.Buffer
	coord := NewCoordinator(CoordinatorConfig{
		Policy:     Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn},
		Stderr:     &buf,
		StreamLogs: true,
	})
	fs := newFakeStarter()
	coord.SetStarter(fs)

	work := func(ctx context.Context) error {
		slogutils.FromContext(ctx).Info("cloning connected repo", "repo", "x")
		return nil
	}
	if err := coord.Run(context.Background(), work); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("cloning connected repo")) {
		t.Errorf("dormant did not render the line plainly; stderr=%q", buf.String())
	}
	if fs.didStart() {
		t.Error("instant work must not start a program")
	}
}

func TestCoordinator_ArmedFastCompletionFlushesPlainNoProgram(t *testing.T) {
	var buf bytes.Buffer
	coord := NewCoordinator(CoordinatorConfig{
		Policy:     Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn},
		Stderr:     &buf,
		StreamLogs: true,
		Debounce:   200 * time.Millisecond, // long enough that fast work resolves first
	})
	fs := newFakeStarter()
	coord.SetStarter(fs)

	work := func(ctx context.Context) error {
		l := slogutils.FromContext(ctx)
		l.Info("first")  // dormant: printed plainly, arms
		l.Info("second") // armed: buffered
		return nil
	}
	if err := coord.Run(context.Background(), work); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("first")) || !bytes.Contains([]byte(out), []byte("second")) {
		t.Errorf("both lines must reach stderr plainly; stderr=%q", out)
	}
	if fs.didStart() {
		t.Error("work completing inside the debounce must not start a program")
	}
}

func TestCoordinator_ArmedToLiveHandsBacklog(t *testing.T) {
	var buf bytes.Buffer
	coord := NewCoordinator(CoordinatorConfig{
		Policy:     Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn},
		Stderr:     &buf,
		StreamLogs: true,
		Debounce:   30 * time.Millisecond,
	})
	fs := newFakeStarter()
	coord.SetStarter(fs)

	release := make(chan struct{})
	finish := make(chan struct{})
	work := func(ctx context.Context) error {
		l := slogutils.FromContext(ctx)
		l.Info("dormant-line") // printed plainly, arms
		l.Info("armed-line")   // buffered into the backlog
		<-release              // block past the debounce so the program starts
		l.Info("live-line")    // forwarded to the running program
		<-finish               // hold so the line is observed before end-of-work
		return ctx.Err()
	}

	done := make(chan error, 1)
	go func() { done <- coord.Run(context.Background(), work) }()

	<-fs.started
	close(release)
	got := <-fs.lines // the live line reached the program before work returned
	close(finish)
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}

	if msgs := messages(fs.backlog); !eq(msgs, []string{"armed-line"}) {
		t.Errorf("backlog = %v, want [armed-line] (dormant line printed, not buffered)", msgs)
	}
	if got.Message != "live-line" {
		t.Errorf("live-forwarded = %q, want live-line", got.Message)
	}
	if !bytes.Contains(buf.Bytes(), []byte("dormant-line")) {
		t.Errorf("dormant line missing from stderr; stderr=%q", buf.String())
	}
}

// A footer-only (StreamLogs=false) view arms on a progress event and never
// forwards its log lines to the program.
func TestCoordinator_TransientArmsOnProgressHidesLogs(t *testing.T) {
	reporter := NewReporter()
	coord := NewCoordinator(CoordinatorConfig{
		Policy:     Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn},
		Stderr:     io.Discard,
		StreamLogs: false,
		Debounce:   20 * time.Millisecond,
		Progress:   reporter,
	})
	fs := newFakeStarter()
	coord.SetStarter(fs)

	release := make(chan struct{})
	work := func(ctx context.Context) error {
		reporter.SetTotal(10) // progress event arms the coordinator
		<-release
		slogutils.FromContext(ctx).Info("indexed entry") // must not reach the program
		reporter.Close()
		return ctx.Err()
	}
	done := make(chan error, 1)
	go func() { done <- coord.Run(context.Background(), work) }()

	<-fs.started
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case e := <-fs.lines:
		t.Errorf("transient view forwarded a log line to the program: %q", e.Message)
	default:
	}
}

// Interrupt during dormant work (no program) still yields the sentinel — the
// dormant/armed cancellation path that a hard SIGINT would otherwise kill.
func TestCoordinator_CancellationBecomesSentinel(t *testing.T) {
	coord := NewCoordinator(CoordinatorConfig{
		Policy: Policy{Display: slog.LevelInfo},
		Stderr: io.Discard,
	})
	fs := newFakeStarter()
	coord.SetStarter(fs)

	running := make(chan struct{})
	work := func(ctx context.Context) error {
		close(running)
		<-ctx.Done()
		return ctx.Err() // context.Canceled
	}
	done := make(chan error, 1)
	go func() { done <- coord.Run(context.Background(), work) }()

	<-running
	coord.Interrupt()
	if err := <-done; !errors.Is(err, ErrUserCancelled) {
		t.Errorf("Run error = %v, want ErrUserCancelled", err)
	}
	if fs.didStart() {
		t.Error("cancelling dormant work must not start a program")
	}
}

// A reported phase declares real work, so it arms the coordinator even with
// zero totals — the footer must appear for phase-only work like a cache sync.
func TestCoordinator_PhaseArms(t *testing.T) {
	reporter := NewReporter()
	coord := NewCoordinator(CoordinatorConfig{
		Policy:     Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn},
		Stderr:     io.Discard,
		StreamLogs: false,
		Debounce:   20 * time.Millisecond,
		Progress:   reporter,
	})
	fs := newFakeStarter()
	coord.SetStarter(fs)

	release := make(chan struct{})
	work := func(ctx context.Context) error {
		reporter.SetPhase(model.PhaseSyncing) // phase-only, zero totals → must arm
		<-release
		reporter.Close()
		return ctx.Err()
	}
	done := make(chan error, 1)
	go func() { done <- coord.Run(context.Background(), work) }()

	<-fs.started // arms and starts a program on the phase alone
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// A cancelled work context yields the sentinel even when the failed work
// returned something other than context.Canceled — an exec dependency SIGKILLed
// mid-run reports "signal: killed", not context.Canceled. The user must still
// see the calm cancelled message, never a raw error with exit 1.
func TestCoordinator_CancelledCtxBecomesSentinelDespiteWrappedError(t *testing.T) {
	coord := NewCoordinator(CoordinatorConfig{
		Policy: Policy{Display: slog.LevelInfo},
		Stderr: io.Discard,
	})
	coord.SetStarter(newFakeStarter())

	running := make(chan struct{})
	work := func(ctx context.Context) error {
		close(running)
		<-ctx.Done()
		return errors.New("git clone: signal: killed") // not context.Canceled
	}
	done := make(chan error, 1)
	go func() { done <- coord.Run(context.Background(), work) }()

	<-running
	coord.Interrupt()
	if err := <-done; !errors.Is(err, ErrUserCancelled) {
		t.Errorf("Run error = %v, want ErrUserCancelled", err)
	}
}

// A warm index publishes a zero-total snapshot; it must not arm, even when the
// (query-embedding) work that follows outlasts the debounce — no spurious footer.
func TestCoordinator_ZeroProgressDoesNotArm(t *testing.T) {
	reporter := NewReporter()
	coord := NewCoordinator(CoordinatorConfig{
		Policy:     Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn},
		Stderr:     io.Discard,
		StreamLogs: false,
		Debounce:   20 * time.Millisecond,
		Progress:   reporter,
	})
	fs := newFakeStarter()
	coord.SetStarter(fs)

	work := func(ctx context.Context) error {
		reporter.SetTotal(0)                // zero-total: must not arm
		reporter.SetNote("query embedding") // note only: must not arm
		time.Sleep(60 * time.Millisecond)   // outlast the debounce a real arm would fire on
		reporter.Close()
		return nil
	}
	if err := coord.Run(context.Background(), work); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fs.didStart() {
		t.Error("warm progress (zero total) must not start a program")
	}
}

// Lines the program never painted (handed back as unpainted) print plainly
// exactly once, after teardown.
func TestCoordinator_PrintsUnpaintedLinesAfterTeardown(t *testing.T) {
	var buf bytes.Buffer
	coord := NewCoordinator(CoordinatorConfig{
		Policy:     Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn},
		Stderr:     &buf,
		StreamLogs: true,
		Debounce:   20 * time.Millisecond,
	})
	fs := newFakeStarter()
	fs.unpainted = []LogEntry{entry(slog.LevelInfo, "unpainted-line")}
	coord.SetStarter(fs)

	release := make(chan struct{})
	work := func(ctx context.Context) error {
		slogutils.FromContext(ctx).Info("dormant-line") // printed plainly, arms
		<-release                                       // block past the debounce so the program starts
		return ctx.Err()
	}
	done := make(chan error, 1)
	go func() { done <- coord.Run(context.Background(), work) }()

	<-fs.started
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n := strings.Count(buf.String(), "unpainted-line"); n != 1 {
		t.Errorf("unpainted line printed %d times, want exactly 1; stderr=%q", n, buf.String())
	}
}

func TestCoordinator_ReEmitsKeptToDurable(t *testing.T) {
	capt := &captureHandler{}
	ctx := slogutils.WithLogger(context.Background(), slog.New(capt))

	coord := NewCoordinator(CoordinatorConfig{
		Policy:     Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn},
		Stderr:     io.Discard,
		StreamLogs: false,
	})
	coord.SetStarter(newFakeStarter())

	work := func(ctx context.Context) error {
		l := slogutils.FromContext(ctx)
		l.Info("indexed") // below keep → not re-emitted
		l.Warn("slow embed")
		return nil
	}
	if err := coord.Run(ctx, work); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(capt.records) != 1 || capt.records[0].Message != "slow embed" {
		t.Errorf("durable re-emit = %d records, want only [slow embed]", len(capt.records))
	}
}

func TestCoordinator_PropagatesErrorWithFingersCrossedFlush(t *testing.T) {
	capt := &captureHandler{}
	ctx := slogutils.WithLogger(context.Background(), slog.New(capt))

	coord := NewCoordinator(CoordinatorConfig{
		Policy: Policy{
			Display:        slog.LevelInfo,
			KeepAtOrAbove:  slog.LevelError,
			FingersCrossed: &FingersCrossed{Trigger: slog.LevelError, Tail: 10},
		},
		Stderr:     io.Discard,
		StreamLogs: false,
	})
	coord.SetStarter(newFakeStarter())

	boom := errors.New("embed failed")
	work := func(ctx context.Context) error {
		l := slogutils.FromContext(ctx)
		l.Info("a")
		l.Info("b")
		return boom // returned error arms the tail flush
	}
	if err := coord.Run(ctx, work); !errors.Is(err, boom) {
		t.Fatalf("Run error = %v, want %v", err, boom)
	}
	if len(capt.records) != 2 || capt.records[0].Message != "a" || capt.records[1].Message != "b" {
		t.Errorf("fingers-crossed flush wrong; got %d records", len(capt.records))
	}
}

func TestCoordinator_StreamLogsSkipsReEmit(t *testing.T) {
	capt := &captureHandler{}
	ctx := slogutils.WithLogger(context.Background(), slog.New(capt))

	coord := NewCoordinator(CoordinatorConfig{
		Policy:     Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn},
		Stderr:     io.Discard,
		StreamLogs: true, // lines already surfaced live; must not re-emit
	})
	coord.SetStarter(newFakeStarter())

	work := func(ctx context.Context) error {
		l := slogutils.FromContext(ctx)
		l.Info("indexed")
		l.Warn("slow embed")
		return nil
	}
	if err := coord.Run(ctx, work); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(capt.records) != 0 {
		t.Errorf("streamed view must not re-emit; got %d records", len(capt.records))
	}
}

func TestCoordinator_SilentWhenNothingKept(t *testing.T) {
	capt := &captureHandler{}
	ctx := slogutils.WithLogger(context.Background(), slog.New(capt))

	coord := NewCoordinator(CoordinatorConfig{
		Policy:     Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn},
		Stderr:     io.Discard,
		StreamLogs: false,
	})
	coord.SetStarter(newFakeStarter())

	work := func(ctx context.Context) error {
		slogutils.FromContext(ctx).Info("routine noise")
		return nil
	}
	if err := coord.Run(ctx, work); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(capt.records) != 0 {
		t.Errorf("routine info must disappear with the view; got %d records", len(capt.records))
	}
}
