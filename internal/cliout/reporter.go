package cliout

import "sync"

// Progress is an absolute snapshot of how far an operation has come. Because
// every snapshot carries the running totals (not a delta), a dropped update
// self-corrects: the next one overwrites it with the true state.
type Progress struct {
	Done  int
	Total int
	Unit  string // optional noun for rendering, e.g. "entries"
}

// Ratio returns Done/Total clamped to [0,1], or 0 when Total is unknown.
func (p Progress) Ratio() float64 {
	if p.Total <= 0 {
		return 0
	}
	r := float64(p.Done) / float64(p.Total)
	switch {
	case r < 0:
		return 0
	case r > 1:
		return 1
	default:
		return r
	}
}

// Reporter receives absolute progress updates from the work goroutine and
// hands the latest to the display loop. It is a single-producer mailbox: the
// callbacks that drive it (OnBatchStart, OnEntryIndexed, …) fire sequentially
// from one goroutine, while the view loop is the sole consumer.
type Reporter struct {
	mu    sync.Mutex
	done  int
	total int
	unit  string

	ch        chan Progress
	closeCh   chan struct{}
	closeOnce sync.Once
}

// NewReporter builds a reporter with an empty mailbox.
func NewReporter() *Reporter {
	return &Reporter{
		ch:      make(chan Progress, 1),
		closeCh: make(chan struct{}),
	}
}

// SetTotal records the operation's total work units and publishes a snapshot.
func (r *Reporter) SetTotal(n int) {
	r.mu.Lock()
	r.total = n
	r.mu.Unlock()
	r.publish()
}

// SetUnit sets the noun used when rendering the count (e.g. "entries").
func (r *Reporter) SetUnit(unit string) {
	r.mu.Lock()
	r.unit = unit
	r.mu.Unlock()
	r.publish()
}

// Add advances the completed count by n and publishes a snapshot.
func (r *Reporter) Add(n int) {
	r.mu.Lock()
	r.done += n
	r.mu.Unlock()
	r.publish()
}

// publish drops any stale pending snapshot and posts the current one, so the
// mailbox always holds the latest state (latest-wins) rather than a backlog.
func (r *Reporter) publish() {
	r.mu.Lock()
	snap := Progress{Done: r.done, Total: r.total, Unit: r.unit}
	r.mu.Unlock()

	select {
	case r.ch <- snap:
		return
	default:
	}
	// Mailbox full: discard the stale snapshot, then post the fresh one.
	select {
	case <-r.ch:
	default:
	}
	select {
	case r.ch <- snap:
	default:
	}
}

// Recv returns the next progress snapshot, or ok=false once Close has been
// called and no snapshot is pending. A pending snapshot is preferred over the
// close signal so the final state isn't dropped.
func (r *Reporter) Recv() (Progress, bool) {
	select {
	case p := <-r.ch:
		return p, true
	default:
	}
	select {
	case p := <-r.ch:
		return p, true
	case <-r.closeCh:
		return Progress{}, false
	}
}

// Close signals that no further progress will be reported. Idempotent.
func (r *Reporter) Close() {
	r.closeOnce.Do(func() { close(r.closeCh) })
}
