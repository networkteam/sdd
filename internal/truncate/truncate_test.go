package truncate_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/truncate"
)

func TestBytesUnderCapIsClean(t *testing.T) {
	got := truncate.Bytes("short", 100, "pull-expr")
	if got.Text != "short" || !got.Cut.Clean() {
		t.Fatalf("under-cap text must pass clean, got %+v", got)
	}
	if got.Cut.Pull != "" {
		t.Errorf("a clean cut carries no pull, got %q", got.Cut.Pull)
	}
}

func TestBytesCutsOnLineBoundary(t *testing.T) {
	s := "line one\nline two\nline three"
	got := truncate.Bytes(s, len("line one\nline two\nlin"), "the-pull")
	if got.Text != "line one\nline two" {
		t.Fatalf("cut = %q, want the last whole line kept", got.Text)
	}
	c := got.Cut
	if c.Clean() || c.KeptBytes != len(got.Text) || c.TotalBytes != len(s) || c.Pull != "the-pull" {
		t.Fatalf("cut accounting = %+v", c)
	}
}

func TestBytesNeverSplitsARune(t *testing.T) {
	s := strings.Repeat("ä", 10) // 2 bytes each
	got := truncate.Bytes(s, 5, "")
	if got.Text != "ää" {
		t.Fatalf("cut = %q, want two whole runes under 5 bytes", got.Text)
	}
	long := "日本語テキスト" // 3 bytes each, no newline
	for max := 1; max < len(long); max++ {
		text := truncate.Bytes(long, max, "").Text
		if !strings.HasPrefix(long, text) || len(text)%3 != 0 {
			t.Fatalf("max %d: cut %q splits a rune", max, text)
		}
	}
}

func TestItemsKeepsWholeItemsWithinBytes(t *testing.T) {
	items := []string{"aaaa", "bbbb", "cccc", "dddd"}
	got := truncate.Items(items, func(s string) int { return len(s) }, 10, "rest")
	if len(got.Items) != 2 {
		t.Fatalf("kept %d items, want 2 whole items under 10 bytes", len(got.Items))
	}
	c := got.Cut
	if c.Dropped != 2 || c.Total != 4 || c.KeptBytes != 8 || c.TotalBytes != 16 || c.Pull != "rest" {
		t.Fatalf("cut accounting = %+v", c)
	}
}

func TestItemsUnderCapIsClean(t *testing.T) {
	got := truncate.Items([]int{1, 2}, func(int) int { return 4 }, 100, "rest")
	if len(got.Items) != 2 || !got.Cut.Clean() {
		t.Fatalf("under-cap items must pass clean, got %+v", got.Cut)
	}
}

func TestHeadCountsDrops(t *testing.T) {
	got := truncate.Head([]int{1, 2, 3, 4, 5}, 2, "rest")
	if len(got.Items) != 2 || got.Items[1] != 2 {
		t.Fatalf("kept = %v, want the first two", got.Items)
	}
	if got.Cut.Dropped != 3 || got.Cut.Total != 5 || got.Cut.Pull != "rest" {
		t.Fatalf("cut accounting = %+v", got.Cut)
	}
}

func TestZeroCapsAreUnbounded(t *testing.T) {
	if got := truncate.Bytes("anything", 0, ""); !got.Cut.Clean() || got.Text != "anything" {
		t.Fatalf("zero byte cap must pass through, got %+v", got)
	}
	if got := truncate.Head([]int{1, 2, 3}, 0, ""); len(got.Items) != 3 || !got.Cut.Clean() {
		t.Fatalf("zero item cap must pass through, got %+v", got.Cut)
	}
}

func TestCarriersSeparateDataFromMeta(t *testing.T) {
	list := truncate.Head([]string{"a", "b", "c"}, 1, "more")
	var c truncate.Carrier = list
	if items, ok := c.Payload().([]string); !ok || len(items) != 1 {
		t.Fatalf("payload = %#v, want the kept items alone", c.Payload())
	}
	if c.CutMeta().Dropped != 2 {
		t.Fatalf("meta = %+v", c.CutMeta())
	}
	text := truncate.Bytes("x\ny", 1, "")
	if _, ok := any(text).(truncate.Carrier); !ok {
		t.Fatal("Text must be a Carrier")
	}
}
