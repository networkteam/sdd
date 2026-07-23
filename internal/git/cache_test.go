package git

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// A cancelled context SIGKILLs git, which reports "signal: killed" rather than
// context.Canceled; run must surface context.Canceled anyway so the coordinator
// maps cancellation to the calm sentinel instead of a raw error at exit 1.
func TestRun_CancelledContextSurfacesContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before the command runs

	err := run(ctx, "", "clone", "--quiet", "https://example.invalid/repo.git", filepath.Join(t.TempDir(), "clone"))
	if err == nil {
		t.Fatal("expected an error from a cancelled git run")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled run should surface context.Canceled; got %v", err)
	}
}
