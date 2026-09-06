package embed_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
)

func batchOptions() embed.BatchOptions {
	return embed.BatchOptions{MaxItems: 8, MaxBytes: 1024, BufferItems: 2, Window: 15 * time.Millisecond, Concurrency: 2}
}
func newBatcher(t *testing.T, run func(context.Context, embed.Request) (embed.Result, error), options embed.BatchOptions) *embed.Batcher {
	t.Helper()
	b, err := embed.NewBatcher(t.Context(), embed.EmbedderFunc{Space: "fixed", Run: run}, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := b.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return b
}
func numberedVectors(req embed.Request) embed.Result {
	r := embed.Result{Vectors: make([][]float32, len(req.Texts)), Usage: llm.Usage{InputTokens: len(req.Texts)}}
	for i, text := range req.Texts {
		n, _ := strconv.Atoi(text)
		r.Vectors[i] = []float32{float32(n + 1), 1}
	}
	return r
}

func TestBatcherCrossCallerRoutingAndOversizedAdmission(t *testing.T) {
	var mu sync.Mutex
	calls, active, peak := 0, 0, 0
	var attributed []embed.BatchAttribution
	b := newBatcher(t, func(ctx context.Context, req embed.Request) (embed.Result, error) {
		mu.Lock()
		calls++
		active++
		peak = max(peak, active)
		attributed = append(attributed, embed.Attribution(ctx))
		mu.Unlock()
		if len(req.Texts) > 8 {
			t.Error("oversized provider batch")
		}
		timer := time.NewTimer(2 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return embed.Result{}, ctx.Err()
		}
		mu.Lock()
		active--
		mu.Unlock()
		return numberedVectors(req), nil
	}, batchOptions())
	var wg sync.WaitGroup
	for caller := range 12 {
		wg.Go(func() {
			texts := make([]string, 25)
			for i := range texts {
				texts[i] = strconv.Itoa(caller*25 + i)
			}
			result, err := b.Embed(embed.WithCaller(t.Context(), fmt.Sprintf("job-%d", caller)), embed.Request{Purpose: embed.PurposeDocument, Texts: texts})
			if err != nil {
				t.Error(err)
				return
			}
			if result.Usage.InputTokens != 0 {
				t.Error("usage duplicated across callers")
			}
			for i, v := range result.Vectors {
				if v[0] != float32(caller*25+i+1) {
					t.Errorf("misrouted vector %d: %v", i, v)
				}
			}
		})
	}
	wg.Wait()
	if calls >= 300 || peak > 2 || peak < 2 {
		t.Fatalf("calls=%d peak=%d", calls, peak)
	}
	shared := false
	for _, a := range attributed {
		if a.ID == "" {
			t.Error("missing batch ID")
		}
		shared = shared || len(a.Callers) > 1
	}
	if !shared {
		t.Fatal("no cross-caller batch")
	}
}

func TestBatcherTailWindowAndPayloadFlushing(t *testing.T) {
	options := batchOptions()
	options.MaxBytes = 3
	options.Window = 20 * time.Millisecond
	var calls atomic.Int32
	b := newBatcher(t, func(_ context.Context, req embed.Request) (embed.Result, error) {
		calls.Add(1)
		size := 0
		for _, s := range req.Texts {
			size += len(s)
		}
		if size > 3 {
			t.Error("payload overflow")
		}
		return numberedVectors(req), nil
	}, options)
	started := time.Now()
	result, err := b.Embed(t.Context(), embed.Request{Purpose: embed.PurposeDocument, Texts: []string{"11", "22", "33"}})
	if err != nil || len(result.Vectors) != 3 {
		t.Fatalf("%v %v", result, err)
	}
	if calls.Load() != 3 || time.Since(started) > time.Second {
		t.Fatalf("tail not flushed: calls=%d elapsed=%s", calls.Load(), time.Since(started))
	}
	if _, err := b.Embed(t.Context(), embed.Request{Purpose: embed.PurposeDocument, Texts: []string{"too long"}}); err == nil {
		t.Fatal("oversized text accepted")
	}
}

func TestBatcherCancellationDoesNotCancelSharedProvider(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	options := batchOptions()
	options.MaxItems = 2
	options.Concurrency = 1
	options.Window = time.Second
	b := newBatcher(t, func(ctx context.Context, req embed.Request) (embed.Result, error) {
		close(started)
		select {
		case <-release:
			return numberedVectors(req), nil
		case <-ctx.Done():
			return embed.Result{}, ctx.Err()
		}
	}, options)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first, second := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := b.Embed(ctx, embed.Request{Purpose: embed.PurposeDocument, Texts: []string{"1"}})
		first <- err
	}()
	go func() {
		_, err := b.Embed(t.Context(), embed.Request{Purpose: embed.PurposeDocument, Texts: []string{"2"}})
		second <- err
	}()
	<-started
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("first=%v", err)
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatalf("second=%v", err)
	}
}

func TestBatcherShutdownReleasesBlockedAdmission(t *testing.T) {
	started := make(chan struct{})
	options := batchOptions()
	options.MaxItems = 1
	options.Concurrency = 1
	options.BufferItems = 1
	b := newBatcher(t, func(ctx context.Context, _ embed.Request) (embed.Result, error) {
		close(started)
		<-ctx.Done()
		return embed.Result{}, ctx.Err()
	}, options)
	result := make(chan error, 1)
	go func() {
		_, err := b.Embed(t.Context(), embed.Request{Purpose: embed.PurposeDocument, Texts: []string{"1", "2", "3", "4", "5"}})
		result <- err
	}()
	<-started
	if err := b.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, embed.ErrBatcherClosed) && !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown=%v", err)
	}
}

func TestBatcherPropagatesProviderAndVectorFailures(t *testing.T) {
	providerFailure := errors.New("provider failure")
	for _, kind := range []string{"provider", "count", "empty", "dimensions", "zero"} {
		t.Run(kind, func(t *testing.T) {
			b := newBatcher(t, func(_ context.Context, req embed.Request) (embed.Result, error) {
				r := numberedVectors(req)
				switch kind {
				case "provider":
					return embed.Result{}, providerFailure
				case "count":
					r.Vectors = nil
				case "empty":
					r.Vectors[0] = nil
				case "dimensions":
					r.Vectors[0] = []float32{1}
				case "zero":
					r.Vectors[0] = []float32{0, 0}
				}
				return r, nil
			}, batchOptions())
			_, err := b.Embed(t.Context(), embed.Request{Purpose: embed.PurposeDocument, Texts: []string{"1", "2"}})
			if err == nil {
				t.Fatal("failure swallowed")
			}
			if kind == "provider" && !errors.Is(err, providerFailure) {
				t.Fatalf("provider cause lost: %v", err)
			}
		})
	}
}

func TestBatcherComposesDeadlineAndQueryRouting(t *testing.T) {
	provider := embed.EmbedderFunc{Space: "fixed", Run: func(ctx context.Context, req embed.Request) (embed.Result, error) {
		if req.Purpose == embed.PurposeQuery {
			return numberedVectors(req), nil
		}
		<-ctx.Done()
		return embed.Result{}, ctx.Err()
	}}
	documents, err := embed.NewBatcher(t.Context(), embed.Bounded(provider, 30*time.Millisecond), batchOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := documents.Close(t.Context()); err != nil {
			t.Error(err)
		}
	}()
	routed := embed.EmbedderFunc{Space: provider.Fingerprint(), Run: func(ctx context.Context, req embed.Request) (embed.Result, error) {
		if req.Purpose == embed.PurposeQuery {
			return provider.Embed(ctx, req)
		}
		return documents.Embed(ctx, req)
	}}
	docResult := make(chan error, 1)
	go func() {
		_, err := routed.Embed(t.Context(), embed.Request{Purpose: embed.PurposeDocument, Texts: []string{"1"}})
		docResult <- err
	}()
	if _, err := routed.Embed(t.Context(), embed.Request{Purpose: embed.PurposeQuery, Texts: []string{"2"}}); err != nil {
		t.Fatal(err)
	}
	if err := <-docResult; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline=%v", err)
	}
}

func TestBatcherProviderUnitLimit(t *testing.T) {
	options := batchOptions()
	options.MaxUnits = 3
	options.Measure = func(text string) (int, error) { return len(text), nil }
	b := newBatcher(t, func(_ context.Context, req embed.Request) (embed.Result, error) {
		units := 0
		for _, text := range req.Texts {
			units += len(text)
		}
		if units > 3 {
			t.Errorf("provider units=%d", units)
		}
		return numberedVectors(req), nil
	}, options)
	result, err := b.Embed(t.Context(), embed.Request{Purpose: embed.PurposeDocument, Texts: []string{"11", "22", "33"}})
	if err != nil || len(result.Vectors) != 3 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	if _, err := b.Embed(t.Context(), embed.Request{Purpose: embed.PurposeDocument, Texts: []string{"1234"}}); err == nil {
		t.Fatal("oversized units accepted")
	}
}

func TestBatcherWindowStartsAtOldestItem(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		options := batchOptions()
		options.Window = time.Second
		called := make(chan int, 2)
		b := newBatcher(t, func(_ context.Context, req embed.Request) (embed.Result, error) {
			called <- len(req.Texts)
			return numberedVectors(req), nil
		}, options)
		var wg sync.WaitGroup
		submit := func(text string) {
			wg.Go(func() {
				if _, err := b.Embed(t.Context(), embed.Request{Purpose: embed.PurposeDocument, Texts: []string{text}}); err != nil {
					t.Error(err)
				}
			})
		}
		submit("1")
		synctest.Wait()
		time.Sleep(750 * time.Millisecond)
		submit("2")
		synctest.Wait()
		time.Sleep(250 * time.Millisecond)
		synctest.Wait()
		select {
		case count := <-called:
			if count != 2 {
				t.Fatalf("items=%d", count)
			}
		default:
			t.Fatal("later arrival reset the flush window")
		}
		wg.Wait()
	})
}

func TestBatcherRejectsEntireRequestBeforeAdmission(t *testing.T) {
	for _, mode := range []string{"bytes", "units", "measurement"} {
		t.Run(mode, func(t *testing.T) {
			options := batchOptions()
			options.MaxItems = 1
			options.MaxBytes = 2
			if mode != "bytes" {
				options.MaxBytes = 100
				options.MaxUnits = 2
				options.Measure = func(text string) (int, error) {
					if mode == "measurement" && text == "bad" {
						return 0, errors.New("cannot measure")
					}
					return len(text), nil
				}
			}
			var calls atomic.Int32
			b := newBatcher(t, func(_ context.Context, req embed.Request) (embed.Result, error) {
				calls.Add(1)
				return numberedVectors(req), nil
			}, options)
			if _, err := b.Embed(t.Context(), embed.Request{Purpose: embed.PurposeDocument, Texts: []string{"1", "2", "bad"}}); err == nil {
				t.Fatal("invalid trailing text accepted")
			}
			if _, err := b.Embed(t.Context(), embed.Request{Purpose: embed.PurposeDocument, Texts: []string{"3"}}); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 {
				t.Fatalf("invalid request dispatched work: calls=%d", calls.Load())
			}
		})
	}
}
