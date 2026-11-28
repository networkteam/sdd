package application_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
)

// Two captures interleave inside the same application: each publishes its own
// entry under its own recorded intent, and neither needs recovery.
func TestInterleavedCapturesBothLand(t *testing.T) {
	const captures = 2
	var preflightCalls int64
	entered := make(chan struct{}, captures)
	release := make(chan struct{})
	f := newWriteFixture(t, writeFixtureOptions{
		Runner: pkgllm.RunnerFunc(func(_ context.Context, request pkgllm.Request) (pkgllm.Result, error) {
			identity := pkgllm.Identity{Provider: "test", Model: "test"}
			if request.Purpose == pkgllm.PurposePreflight {
				atomic.AddInt64(&preflightCalls, 1)
				// Artificially slow stage: block until both captures have
				// entered preflight before either publication starts.
				entered <- struct{}{}
				<-release
				return pkgllm.Result{Text: `{"findings":[]}`, Identity: identity}, nil
			}
			return pkgllm.Result{Text: `{"title": "Test title", "summary": "Interleaved capture summary."}`, Identity: identity}, nil
		}),
	})
	bindings := make([]sdd.SessionBinding, captures)
	for i := range bindings {
		bindings[i] = openBinding(t, f.sessions, f.identity.Subject, sdd.SessionID(fmt.Sprintf("capture-%d", i)))
	}

	type outcome struct {
		id  string
		err error
	}
	results := make(chan outcome, captures)
	for i := range bindings {
		go func(n int) {
			draft := identifiedCaptureDraft(bindings[n], uint64(n+1), sdd.EntryDraft{
				Kind: "gap", Layer: "tactical", Body: fmt.Sprintf("Interleaved capture body %d.", n), Confidence: "high",
			})
			created, err := preflightAndCreateEntry(t, f.app, f.identity, bindings[n], draft)
			results <- outcome{id: created.EntryID, err: err}
		}(i)
	}
	for range captures {
		<-entered
	}
	close(release)

	ids := map[string]bool{}
	for range captures {
		out := <-results
		if out.err != nil || out.id == "" {
			t.Fatalf("interleaved capture = %q, %v", out.id, out.err)
		}
		ids[out.id] = true
	}
	if len(ids) != captures {
		t.Fatalf("interleaved captures produced %d distinct entries, want %d", len(ids), captures)
	}
	if atomic.LoadInt64(&preflightCalls) != captures {
		t.Fatalf("pre-flight ran %d times, want %d (once per capture)", preflightCalls, captures)
	}
}
