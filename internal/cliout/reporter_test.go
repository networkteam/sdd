package cliout

import (
	"math"
	"testing"
)

func TestReporter_AbsoluteSnapshots(t *testing.T) {
	r := NewReporter()
	r.SetTotal(10)
	r.Add(1)
	r.Add(1)

	// Latest-wins mailbox: the most recent absolute snapshot is what's read.
	p, ok := r.Recv()
	if !ok {
		t.Fatal("expected a snapshot")
	}
	if p.Done != 2 || p.Total != 10 {
		t.Errorf("snapshot = %+v, want Done=2 Total=10", p)
	}
}

func TestReporter_LatestWinsOnBacklog(t *testing.T) {
	r := NewReporter()
	r.SetTotal(100)
	// Many updates without draining — a backlog must collapse to the latest.
	for i := 1; i <= 50; i++ {
		r.Add(1)
	}
	p, ok := r.Recv()
	if !ok {
		t.Fatal("expected a snapshot")
	}
	if p.Done != 50 || p.Total != 100 {
		t.Errorf("snapshot = %+v, want Done=50 Total=100 (latest wins)", p)
	}
}

func TestReporter_SetUnit(t *testing.T) {
	r := NewReporter()
	r.SetUnit("entries")
	p, _ := r.Recv()
	if p.Unit != "entries" {
		t.Errorf("unit = %q, want entries", p.Unit)
	}
}

func TestReporter_RecvAfterCloseReportsDone(t *testing.T) {
	r := NewReporter()
	r.SetTotal(3)
	r.Add(3)
	// Drain the pending snapshot first.
	if _, ok := r.Recv(); !ok {
		t.Fatal("expected the pending snapshot")
	}
	r.Close()
	if _, ok := r.Recv(); ok {
		t.Error("expected ok=false after Close with empty mailbox")
	}
}

func TestReporter_CloseIdempotent(t *testing.T) {
	r := NewReporter()
	r.Close()
	r.Close() // must not panic
}

func TestProgress_Ratio(t *testing.T) {
	cases := []struct {
		p    Progress
		want float64
	}{
		{Progress{Done: 0, Total: 0}, 0}, // unknown total
		{Progress{Done: 5, Total: 0}, 0}, // unknown total, nonzero done
		{Progress{Done: 1, Total: 4}, 0.25},
		{Progress{Done: 4, Total: 4}, 1},
		{Progress{Done: 9, Total: 4}, 1},  // clamped
		{Progress{Done: -1, Total: 4}, 0}, // clamped
	}
	for _, c := range cases {
		if got := c.p.Ratio(); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("Ratio(%+v) = %v, want %v", c.p, got, c.want)
		}
	}
}
