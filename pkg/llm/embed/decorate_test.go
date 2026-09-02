package embed_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
)

type recordingSink struct{ stats []llm.CallStat }

func (s *recordingSink) RecordCall(_ context.Context, stat llm.CallStat) {
	s.stats = append(s.stats, stat)
}

func fixed(id llm.Identity) embed.Embedder {
	return embed.EmbedderFunc{Space: "fixed/1", Run: func(_ context.Context, req embed.Request) (embed.Result, error) {
		vectors := make([][]float32, len(req.Texts))
		for i := range req.Texts {
			vectors[i] = []float32{float32(i), 1}
		}
		return embed.Result{Vectors: vectors, Identity: id, Usage: llm.Usage{InputTokens: 7}}, nil
	}}
}

func TestDecoratorsPassFingerprintThrough(t *testing.T) {
	e := embed.Observed(embed.RateLimited(embed.Bounded(fixed(llm.Identity{}), time.Second), 100), &recordingSink{})
	if e.Fingerprint() != "fixed/1" {
		t.Fatalf("fingerprint = %q", e.Fingerprint())
	}
}

func TestBoundedAppliesDeadline(t *testing.T) {
	var deadlineSet bool
	inner := embed.EmbedderFunc{Space: "s", Run: func(ctx context.Context, _ embed.Request) (embed.Result, error) {
		_, deadlineSet = ctx.Deadline()
		return embed.Result{}, nil
	}}
	if _, err := embed.Bounded(inner, time.Second).Embed(context.Background(), embed.Request{}); err != nil {
		t.Fatal(err)
	}
	if !deadlineSet {
		t.Fatal("inner embedder saw no deadline")
	}
}

func TestRateLimitedHonoursCancel(t *testing.T) {
	slow := embed.RateLimited(fixed(llm.Identity{}), 0.01)
	_, _ = slow.Embed(context.Background(), embed.Request{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := slow.Embed(ctx, embed.Request{})
	if err == nil || (!errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "rate limiter")) {
		t.Fatalf("expected rate limiter cancellation, got %v", err)
	}
}

func TestObservedRecordsBatch(t *testing.T) {
	sink := &recordingSink{}
	id := llm.Identity{Provider: "ollama", Model: "m"}
	res, err := embed.Observed(fixed(id), sink).Embed(context.Background(), embed.Request{Purpose: embed.PurposeDocument, Texts: []string{"a", "b", "c"}})
	if err != nil || len(res.Vectors) != 3 {
		t.Fatalf("embed: %+v %v", res, err)
	}
	if len(sink.stats) != 1 {
		t.Fatalf("recorded %d stats, want 1", len(sink.stats))
	}
	got := sink.stats[0]
	if got.Purpose != "embed-document" || got.Items != 3 || got.Identity != id || got.Usage.InputTokens != 7 || got.Error != "" {
		t.Fatalf("stat = %+v", got)
	}
}

func TestObservedRecordsAttributedFailure(t *testing.T) {
	sink := &recordingSink{}
	id := llm.Identity{Provider: "openai", Model: "m"}
	failing := embed.EmbedderFunc{Space: "s", Run: func(context.Context, embed.Request) (embed.Result, error) {
		return embed.Result{}, &llm.Error{Identity: id, Err: errors.New("boom")}
	}}
	_, err := embed.Observed(failing, sink).Embed(context.Background(), embed.Request{Purpose: embed.PurposeQuery, Texts: []string{"q"}})
	if err == nil {
		t.Fatal("expected the failure to propagate")
	}
	if got := sink.stats[0]; got.Identity != id || got.Error != "boom" || got.Items != 1 {
		t.Fatalf("stat = %+v", got)
	}
}

func TestObservedNilSinkReturnsEmbedderUnwrapped(t *testing.T) {
	if _, err := embed.Observed(fixed(llm.Identity{}), nil).Embed(context.Background(), embed.Request{Texts: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
}
