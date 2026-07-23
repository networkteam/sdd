package tui

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/networkteam/sdd/internal/cliout"
	sddmodel "github.com/networkteam/sdd/internal/model"
)

// recordingRun stands in for the real program runner and flags whether a
// program was ever started — instant (dormant/armed-fast) operations must not
// start one.
func recordingRun() (programRunner, *bool) {
	started := false
	return func(m tea.Model) (tea.Model, error) {
		started = true
		return m, nil
	}, &started
}

func TestInteractive_ReturnsResultForInstantWork(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}
	work := func(context.Context) (int, error) { return 42, nil }

	run, started := recordingRun()
	val, err := interactiveWith(context.Background(), policy, View{InitialPhase: sddmodel.PhaseIndexing}, work, run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("result = %d, want 42", val)
	}
	if *started {
		t.Error("instant work must not start a program")
	}
}

func TestInteractive_PropagatesWorkError(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo, KeepAtOrAbove: slog.LevelWarn}
	boom := errors.New("boom")
	work := func(context.Context) (int, error) { return 0, boom }

	run, _ := recordingRun()
	_, err := interactiveWith(context.Background(), policy, View{InitialPhase: sddmodel.PhaseIndexing}, work, run)
	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want %v", err, boom)
	}
}

func TestInteractive_TranslatesCancellationToSentinel(t *testing.T) {
	policy := cliout.Policy{Display: slog.LevelInfo}
	work := func(context.Context) (int, error) { return 0, context.Canceled }

	run, _ := recordingRun()
	_, err := interactiveWith(context.Background(), policy, View{InitialPhase: sddmodel.PhaseIndexing}, work, run)
	if !errors.Is(err, cliout.ErrUserCancelled) {
		t.Errorf("error = %v, want ErrUserCancelled", err)
	}
}
