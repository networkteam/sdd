package embed

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/networkteam/sdd/pkg/llm"
)

var ErrBatcherClosed = errors.New("embed: batcher closed")

// BatchOptions bounds admitted document work separately from provider calls.
// MaxBytes is a UTF-8 input bound, not a provider token or wire-payload bound.
// Measure and MaxUnits optionally impose a provider-specific aggregate limit;
// the adapter must still enforce its exact wire contract. A single text above
// either limit fails explicitly. Entries with many texts are admitted one at
// a time and may span arbitrarily many batches.
type BatchOptions struct {
	MaxItems    int
	MaxBytes    int
	BufferItems int
	Window      time.Duration
	Concurrency int
	MaxUnits    int
	Measure     func(string) (int, error)
}

// Batcher combines document requests over one fixed embedder configuration.
// Route queries separately and compose Bounded inside the batcher for
// provider deadlines. Shared calls use the batcher's lifetime, never a caller's
// context. Place Observed inside Batcher: document results carry zero Usage
// because usage belongs to provider batches, not to participating callers.
type Batcher struct {
	inner    Embedder
	options  BatchOptions
	ctx      context.Context
	cancel   context.CancelFunc
	items    chan batchItem
	batches  chan []batchItem
	done     chan struct{}
	wg       sync.WaitGroup
	sequence atomic.Uint64
	instance string
}

type batchItem struct {
	text   string
	index  int
	units  int
	queued time.Time
	caller context.Context
	reply  chan batchReply
}

type batchReply struct {
	index    int
	vector   []float32
	identity llm.Identity
	err      error
}

type callerKey struct{}
type attributionKey struct{}

type BatchAttribution struct {
	ID      string
	Callers []string
}

// WithCaller attaches an opaque correlation ID, such as a host's job attempt.
// It affects observation only; no queue or retry semantics enter the batcher.
func WithCaller(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, callerKey{}, id)
}

// Attribution returns the shared-call identity available to an inner StatsSink.
// Callers is a set; usage must be recorded once against ID, never per caller.
func Attribution(ctx context.Context) BatchAttribution {
	a, _ := ctx.Value(attributionKey{}).(BatchAttribution)
	a.Callers = append([]string(nil), a.Callers...)
	return a
}

func NewBatcher(ctx context.Context, inner Embedder, options BatchOptions) (*Batcher, error) {
	if inner == nil || options.MaxItems < 1 || options.MaxBytes < 1 || options.BufferItems < 1 || options.Concurrency < 1 || options.Window <= 0 || (options.Measure == nil) != (options.MaxUnits == 0) || options.MaxUnits < 0 {
		return nil, fmt.Errorf("embed: invalid batch options")
	}
	lifetime, cancel := context.WithCancel(ctx)
	b := &Batcher{instance: rand.Text(), inner: inner, options: options, ctx: lifetime, cancel: cancel, items: make(chan batchItem, options.BufferItems), batches: make(chan []batchItem), done: make(chan struct{})}
	b.wg.Add(1 + options.Concurrency)
	go b.collect()
	for range options.Concurrency {
		go b.work()
	}
	go func() { b.wg.Wait(); close(b.done) }()
	return b, nil
}

func (b *Batcher) Fingerprint() string { return b.inner.Fingerprint() }

func (b *Batcher) Embed(ctx context.Context, req Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if b.ctx.Err() != nil {
		return Result{}, ErrBatcherClosed
	}
	if req.Purpose != PurposeDocument {
		return Result{}, fmt.Errorf("embed: unsupported batch purpose %q", req.Purpose)
	}
	result := Result{Vectors: make([][]float32, len(req.Texts))}
	replies := make(chan batchReply, min(len(req.Texts), b.options.MaxItems))
	caller, cancel := context.WithCancel(ctx)
	defer cancel()
	submitted, received := 0, 0
	var next *batchItem
	for received < len(req.Texts) {
		var admission chan batchItem
		var item batchItem
		if submitted < len(req.Texts) {
			if next == nil {
				text := req.Texts[submitted]
				units := 0
				if b.options.Measure != nil {
					var err error
					units, err = b.options.Measure(text)
					if err != nil {
						return Result{}, err
					}
					if units < 0 || units > b.options.MaxUnits {
						return Result{}, fmt.Errorf("embed: text exceeds provider unit limit")
					}
				}
				if len(text) > b.options.MaxBytes {
					return Result{}, fmt.Errorf("embed: text exceeds batch byte limit")
				}
				next = &batchItem{text: text, index: submitted, units: units, caller: caller, reply: replies, queued: time.Now()}
			}
			admission, item = b.items, *next
		}
		select {
		case admission <- item:
			submitted++
			next = nil
		case reply := <-replies:
			if reply.err != nil {
				return Result{}, reply.err
			}
			result.Vectors[reply.index] = reply.vector
			result.Identity = reply.identity
			received++
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-b.ctx.Done():
			return Result{}, ErrBatcherClosed
		}
	}

	return result, nil
}

func (b *Batcher) collect() {
	defer b.wg.Done()
	var batch []batchItem
	var timer *time.Timer
	var tick <-chan time.Time
	bytes, units := 0, 0
	stop := func() {
		if timer != nil {
			timer.Stop()
		}
		timer = nil
		tick = nil
	}
	defer stop()
	flush := func() bool {
		stop()
		if len(batch) == 0 {
			return true
		}
		select {
		case b.batches <- batch:
			batch = nil
			bytes, units = 0, 0
			return true
		case <-b.ctx.Done():
			return false
		}
	}
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-tick:
			if !flush() {
				return
			}
		case item := <-b.items:
			if item.caller.Err() != nil {
				continue
			}
			if len(batch) > 0 && (bytes+len(item.text) > b.options.MaxBytes || (b.options.Measure != nil && units+item.units > b.options.MaxUnits)) {
				if !flush() {
					return
				}
			}
			if len(batch) == 0 {
				timer = time.NewTimer(max(0, time.Until(item.queued.Add(b.options.Window))))
				tick = timer.C
			}
			batch = append(batch, item)
			bytes += len(item.text)
			units += item.units
			if len(batch) == b.options.MaxItems {
				if !flush() {
					return
				}
			}
		}
	}
}

func (b *Batcher) work() {
	defer b.wg.Done()
	for {
		select {
		case <-b.ctx.Done():
			return
		case items := <-b.batches:
			b.dispatch(items)
		}
	}
}

func (b *Batcher) dispatch(items []batchItem) {
	active := items[:0]
	texts := make([]string, 0, len(items))
	callers := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		if item.caller.Err() != nil {
			continue
		}
		active = append(active, item)
		texts = append(texts, item.text)
		id, _ := item.caller.Value(callerKey{}).(string)
		if id != "" && !seen[id] {
			seen[id] = true
			callers = append(callers, id)
		}
	}
	if len(active) == 0 {
		return
	}
	id := fmt.Sprintf("%s-%d", b.instance, b.sequence.Add(1))
	call := context.WithValue(b.ctx, attributionKey{}, BatchAttribution{ID: id, Callers: callers})
	result, err := b.inner.Embed(call, Request{Purpose: PurposeDocument, Texts: texts})
	if err == nil {
		err = validateBatchVectors(result.Vectors, len(active))
	}
	for i, item := range active {
		reply := batchReply{index: item.index, identity: result.Identity, err: err}
		if err == nil {
			reply.vector = result.Vectors[i]
		}
		select {
		case item.reply <- reply:
		case <-item.caller.Done():
		case <-b.ctx.Done():
		}
	}
}

// Close cancels queued and in-flight work. Callers receive explicit failure;
// no durable retry is performed. ctx bounds waiting for providers to stop.
func (b *Batcher) Close(ctx context.Context) error {
	b.cancel()
	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
