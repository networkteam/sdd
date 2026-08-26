package textdiff_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/textdiff"
)

func TestUnifiedEqualIsEmpty(t *testing.T) {
	if got := textdiff.Unified("same\ntext", "same\ntext"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestUnifiedSingleHunkWithContext(t *testing.T) {
	old := "a\nb\nc\nd\ne"
	new := "a\nb\nC\nd\ne"
	got := textdiff.Unified(old, new)
	want := "@@\n a\n b\n-c\n+C\n d\n e"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestUnifiedDistantChangesSplitHunks(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = strings.Repeat("x", i+1)
	}
	old := strings.Join(lines, "\n")
	lines[0] = "first"
	lines[19] = "last"
	got := textdiff.Unified(old, strings.Join(lines, "\n"))
	if strings.Count(got, "@@") != 2 {
		t.Fatalf("distant changes should split into two hunks:\n%s", got)
	}
	if strings.Contains(got, "\n "+strings.Repeat("x", 10)+"\n") {
		t.Fatalf("middle lines outside context must not serve:\n%s", got)
	}
}

func TestUnifiedInsertionAndDeletion(t *testing.T) {
	got := textdiff.Unified("keep\ndrop", "keep\nadded\nmore")
	for _, want := range []string{"-drop", "+added", "+more", " keep"} {
		if !strings.Contains(got, want) {
			t.Fatalf("diff missing %q:\n%s", want, got)
		}
	}
}

func TestHeadTail(t *testing.T) {
	text := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10"
	got := textdiff.HeadTail(text, 3, 2)
	want := "1\n2\n3\n[… 5 lines elided …]\n9\n10"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if short := textdiff.HeadTail("a\nb", 3, 2); short != "a\nb" {
		t.Fatalf("short text must return whole, got %q", short)
	}
}
