package handlers_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
)

// fakePuller scripts the git surface the sync-pull handler consumes and
// records whether MergePull was reached.
type fakePuller struct {
	clean       bool
	cleanErr    error
	pullOut     string
	pullErr     error
	mergeCalled bool
}

func (f *fakePuller) IsClean(context.Context) (bool, error) {
	return f.clean, f.cleanErr
}

func (f *fakePuller) MergePull(context.Context) (string, error) {
	f.mergeCalled = true
	return f.pullOut, f.pullErr
}

func TestSyncPull_CleanTreeMergesAndReports(t *testing.T) {
	puller := &fakePuller{clean: true, pullOut: "Already up to date."}
	h := handlers.New(handlers.Options{Puller: puller})

	var got string
	cmd := &command.SyncPullCmd{OnPulled: func(output string) { got = output }}

	if err := h.SyncPull(context.Background(), cmd); err != nil {
		t.Fatalf("SyncPull: %v", err)
	}
	if !puller.mergeCalled {
		t.Error("MergePull should have been called on a clean tree")
	}
	if got != "Already up to date." {
		t.Errorf("OnPulled output = %q, want %q", got, "Already up to date.")
	}
}

func TestSyncPull_DirtyTreeRefusesWithoutPulling(t *testing.T) {
	puller := &fakePuller{clean: false}
	h := handlers.New(handlers.Options{Puller: puller})

	called := false
	cmd := &command.SyncPullCmd{OnPulled: func(string) { called = true }}

	err := h.SyncPull(context.Background(), cmd)
	if err == nil {
		t.Fatal("SyncPull should refuse on a dirty working tree")
	}
	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("error = %q, want it to mention uncommitted changes", err.Error())
	}
	if puller.mergeCalled {
		t.Error("MergePull must not run on a dirty tree")
	}
	if called {
		t.Error("OnPulled must not fire when the pull is refused")
	}
}

func TestSyncPull_NilPullerErrors(t *testing.T) {
	h := handlers.New(handlers.Options{})
	if err := h.SyncPull(context.Background(), &command.SyncPullCmd{}); err == nil {
		t.Fatal("SyncPull should error when no Puller is configured")
	}
}

func TestSyncPull_PropagatesPullFailure(t *testing.T) {
	puller := &fakePuller{clean: true, pullErr: errors.New("network down")}
	h := handlers.New(handlers.Options{Puller: puller})

	err := h.SyncPull(context.Background(), &command.SyncPullCmd{})
	if err == nil {
		t.Fatal("SyncPull should propagate a pull failure")
	}
	if !strings.Contains(err.Error(), "network down") {
		t.Errorf("error = %q, want it to wrap the pull failure", err.Error())
	}
}
