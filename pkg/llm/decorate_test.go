package llm_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/networkteam/sdd/pkg/llm"
)

type recordingSink struct {
	mu    sync.Mutex
	stats []llm.CallStat
}

func (s *recordingSink) RecordCall(_ context.Context, stat llm.CallStat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats = append(s.stats, stat)
}

func okRunner(id llm.Identity) llm.Runner {
	return llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) {
		return llm.Result{Text: "ok", Identity: id, Usage: llm.Usage{InputTokens: 10, OutputTokens: 2}}, nil
	})
}

func TestBoundedAppliesDeadline(t *testing.T) {
	var deadlineSet bool
	inner := llm.RunnerFunc(func(ctx context.Context, _ llm.Request) (llm.Result, error) {
		_, deadlineSet = ctx.Deadline()
		return llm.Result{}, nil
	})
	if _, err := llm.Bounded(inner, time.Second).Run(context.Background(), llm.Request{}); err != nil {
		t.Fatal(err)
	}
	if !deadlineSet {
		t.Fatal("inner runner saw no deadline")
	}
}

func TestRateLimitedCallsInnerAndHonoursCancel(t *testing.T) {
	calls := 0
	inner := llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) {
		calls++
		return llm.Result{Text: "ok"}, nil
	})
	fast := llm.RateLimited(inner, 100)
	if out, err := fast.Run(context.Background(), llm.Request{}); err != nil || out.Text != "ok" || calls != 1 {
		t.Fatalf("fast run: out=%+v err=%v calls=%d", out, err, calls)
	}

	slow := llm.RateLimited(inner, 0.01)
	_, _ = slow.Run(context.Background(), llm.Request{}) // drain the single burst slot
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := slow.Run(ctx, llm.Request{})
	if err == nil || (!errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "rate limiter")) {
		t.Fatalf("expected rate limiter cancellation, got %v", err)
	}
}

func TestObservedRecordsSuccess(t *testing.T) {
	sink := &recordingSink{}
	id := llm.Identity{Provider: "p", Model: "m", Variant: "v=1"}
	_, err := llm.Observed(okRunner(id), sink).Run(context.Background(), llm.Request{Purpose: llm.PurposeSummarize})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.stats) != 1 {
		t.Fatalf("recorded %d stats, want 1", len(sink.stats))
	}
	got := sink.stats[0]
	if got.Purpose != "summarize" || got.Identity != id || got.Usage.InputTokens != 10 || got.Error != "" || got.Items != 0 {
		t.Fatalf("stat = %+v", got)
	}
}

func TestObservedRecordsAttributedFailure(t *testing.T) {
	sink := &recordingSink{}
	id := llm.Identity{Provider: "p", Model: "m"}
	failing := llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) {
		return llm.Result{}, &llm.Error{Identity: id, Err: errors.New("boom")}
	})
	_, err := llm.Observed(failing, sink).Run(context.Background(), llm.Request{Purpose: llm.PurposePreflight})
	if err == nil {
		t.Fatal("expected the failure to propagate")
	}
	got := sink.stats[0]
	if got.Identity != id || got.Error != "boom" || got.Usage != (llm.Usage{}) {
		t.Fatalf("stat = %+v", got)
	}
}

func TestObservedUnattributedFailureLeavesIdentityBlank(t *testing.T) {
	sink := &recordingSink{}
	failing := llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) {
		return llm.Result{}, errors.New("transport down")
	})
	_, _ = llm.Observed(failing, sink).Run(context.Background(), llm.Request{})
	if got := sink.stats[0]; got.Identity != (llm.Identity{}) || got.Error != "transport down" {
		t.Fatalf("stat = %+v", got)
	}
}

func TestObservedNilSinkReturnsRunnerUnwrapped(t *testing.T) {
	inner := okRunner(llm.Identity{})
	if _, err := llm.Observed(inner, nil).Run(context.Background(), llm.Request{}); err != nil {
		t.Fatal(err)
	}
}

func TestByPurposeRoutesAndFallsBack(t *testing.T) {
	pre := okRunner(llm.Identity{Model: "pre"})
	other := okRunner(llm.Identity{Model: "other"})
	routed := llm.ByPurpose(map[llm.Purpose]llm.Runner{llm.PurposePreflight: pre}, other)

	out, err := routed.Run(context.Background(), llm.Request{Purpose: llm.PurposePreflight})
	if err != nil || out.Identity.Model != "pre" {
		t.Fatalf("preflight route: %+v %v", out, err)
	}
	out, err = routed.Run(context.Background(), llm.Request{Purpose: llm.PurposeSummarize})
	if err != nil || out.Identity.Model != "other" {
		t.Fatalf("fallback route: %+v %v", out, err)
	}
}

func TestByPurposeWithoutFallbackFailsUnrouted(t *testing.T) {
	routed := llm.ByPurpose(map[llm.Purpose]llm.Runner{}, nil)
	_, err := routed.Run(context.Background(), llm.Request{Purpose: llm.PurposeWritingGuide})
	if err == nil || !strings.Contains(err.Error(), "writing-guide") {
		t.Fatalf("expected unrouted purpose error, got %v", err)
	}
}
