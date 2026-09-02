package embed_test

import (
	"context"
	"errors"
	"testing"

	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
)

// recordingEmbedder returns one vector per text carrying its global position,
// and records the size of each call it served.
type recordingEmbedder struct {
	calls []int
	seen  int
	fail  int // fail the call whose first text has this position; 0 disables
}

func (r *recordingEmbedder) Embed(_ context.Context, req embed.Request) (embed.Result, error) {
	r.calls = append(r.calls, len(req.Texts))
	if r.fail > 0 && r.seen == r.fail {
		return embed.Result{}, &llm.Error{Identity: llm.Identity{Provider: "p"}, Err: errors.New("boom")}
	}
	vectors := make([][]float32, len(req.Texts))
	for i := range req.Texts {
		vectors[i] = []float32{float32(r.seen + i)}
	}
	r.seen += len(req.Texts)
	return embed.Result{Vectors: vectors, Identity: llm.Identity{Provider: "p", Model: "m"}, Usage: llm.Usage{InputTokens: len(req.Texts)}}, nil
}

func (r *recordingEmbedder) Fingerprint() string { return "rec/1" }

func texts(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "t"
	}
	return out
}

func TestBatchedSplitsInOrderAndSumsUsage(t *testing.T) {
	inner := &recordingEmbedder{}
	res, err := embed.Batched(inner, 4).Embed(context.Background(), embed.Request{Purpose: embed.PurposeDocument, Texts: texts(10)})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := inner.calls, []int{4, 4, 2}; len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if len(res.Vectors) != 10 {
		t.Fatalf("got %d vectors, want 10", len(res.Vectors))
	}
	for i, v := range res.Vectors {
		if v[0] != float32(i) {
			t.Fatalf("vector %d out of order: %v", i, v)
		}
	}
	if res.Usage.InputTokens != 10 || res.Identity.Model != "m" {
		t.Fatalf("result = %+v", res)
	}
	if embed.Batched(inner, 4).Fingerprint() != "rec/1" {
		t.Fatal("fingerprint not passed through")
	}
}

func TestBatchedPassesSmallRequestsThrough(t *testing.T) {
	inner := &recordingEmbedder{}
	if _, err := embed.Batched(inner, 8).Embed(context.Background(), embed.Request{Texts: texts(3)}); err != nil {
		t.Fatal(err)
	}
	if len(inner.calls) != 1 || inner.calls[0] != 3 {
		t.Fatalf("calls = %v, want one call of 3", inner.calls)
	}
}

func TestBatchedStopsAtFirstFailure(t *testing.T) {
	inner := &recordingEmbedder{fail: 4}
	_, err := embed.Batched(inner, 4).Embed(context.Background(), embed.Request{Texts: texts(10)})
	if err == nil {
		t.Fatal("expected the second batch's failure to propagate")
	}
	if _, ok := errors.AsType[*llm.Error](err); !ok {
		t.Fatalf("attribution lost: %T %v", err, err)
	}
	if len(inner.calls) != 2 {
		t.Fatalf("calls after failure = %v, want 2", inner.calls)
	}
}

func TestBatchedObservedInsideRecordsPerRoundTrip(t *testing.T) {
	sink := &recordingSink{}
	inner := &recordingEmbedder{}
	e := embed.Batched(embed.Observed(inner, sink), 4)
	if _, err := e.Embed(context.Background(), embed.Request{Purpose: embed.PurposeDocument, Texts: texts(10)}); err != nil {
		t.Fatal(err)
	}
	if len(sink.stats) != 3 || sink.stats[0].Items != 4 || sink.stats[2].Items != 2 {
		t.Fatalf("stats = %+v", sink.stats)
	}
}
