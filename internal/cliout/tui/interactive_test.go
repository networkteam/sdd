package tui

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/networkteam/sdd/internal/cliout"
)

// fakeRun stands in for the real program runner. Instant (dormant/armed-fast)
// operations never start a program, so it is not invoked on these paths — it
// exists to satisfy the seam and fail loudly if a program starts unexpectedly.
func fakeRun(m tea.Model) (tea.Model, error) { return m, nil }

func TestInteractive_ReturnsResultForInstantWork(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}
	work := func(context.Context) (int, error) { return 42, nil }

	val, err := interactiveWith(context.Background(), policy, View{Label: "indexing"}, work, fakeRun)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("result = %d, want 42", val)
	}
}

func TestInteractive_PropagatesWorkError(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}
	boom := errors.New("boom")
	work := func(context.Context) (int, error) { return 0, boom }

	_, err := interactiveWith(context.Background(), policy, View{Label: "indexing"}, work, fakeRun)
	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want %v", err, boom)
	}
}

func TestInteractive_TranslatesCancellationToSentinel(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo}
	work := func(context.Context) (int, error) { return 0, context.Canceled }

	_, err := interactiveWith(context.Background(), policy, View{Label: "indexing"}, work, fakeRun)
	if !errors.Is(err, cliout.ErrUserCancelled) {
		t.Errorf("error = %v, want ErrUserCancelled", err)
	}
}
