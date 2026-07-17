package model

import (
	"testing"
	"time"
)

// TopicCounts must report matched entries independently of rows: untagged
// entries aggregate into no row, but they did match the pipeline — Count()
// answering zero here would make surfaces claim "0 entries matched" falsely.
func TestTopicCounts_UntaggedEntriesStillCount(t *testing.T) {
	topic, err := ParseTopicPath("cli/view")
	if err != nil {
		t.Fatal(err)
	}
	tagged := &Entry{ID: "20260601-100000-s-tac-aaa", Type: TypeSignal, Kind: KindGap, Topics: []TopicPath{topic}}
	untagged := &Entry{ID: "20260601-110000-s-tac-bbb", Type: TypeSignal, Kind: KindGap}
	g := NewGraph([]*Entry{tagged, untagged})
	now := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)

	all := g.TopicCounts([]*Entry{tagged, untagged}, now)
	if len(all.Rows) != 1 {
		t.Fatalf("rows = %d, want 1 (only the tagged entry contributes)", len(all.Rows))
	}
	if got := all.Count(); got != 2 {
		t.Errorf("Count() = %d, want 2 — both entries matched the pipeline", got)
	}

	onlyUntagged := g.TopicCounts([]*Entry{untagged}, now)
	if len(onlyUntagged.Rows) != 0 {
		t.Fatalf("rows = %d, want 0 for an untagged-only set", len(onlyUntagged.Rows))
	}
	if got := onlyUntagged.Count(); got != 1 {
		t.Errorf("Count() = %d, want 1 — matched-anything must not depend on topics", got)
	}

	none := g.TopicCounts(nil, now)
	if got := none.Count(); got != 0 {
		t.Errorf("Count() = %d, want 0 for an empty filtered set", got)
	}
}
