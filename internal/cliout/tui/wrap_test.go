package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

func TestWrapPromptTextKeepsEveryLineInsideTheWidth(t *testing.T) {
	// The relocation census is the text that actually got truncated: long
	// enough to leave the terminal, with the question at the far end.
	const census = "145 in-tree session or staged-blob payload file(s) need relocation to the machine-global store; " +
		"2 identity-less global session or staged-blob payload file(s) need rekeying to the repository-ID store. Relocate now?"

	for _, width := range []int{40, 60, 80, 120} {
		wrapped := wrapPromptText(census, width)
		for line := range strings.SplitSeq(wrapped, "\n") {
			if got := utf8.RuneCountInString(line); got > width {
				t.Errorf("width %d: line of %d runes exceeds the terminal: %q", width, got, line)
			}
		}
		if !strings.Contains(wrapped, "Relocate now?") {
			t.Errorf("width %d: wrapping dropped the question", width)
		}
	}
}

func TestWrapPromptTextPreservesIndentAndBlankLines(t *testing.T) {
	source := "Session store relocation is pending:\n" +
		"  - 145 in-tree session or staged-blob payload file(s) need relocation to the machine-global store\n" +
		"\n" +
		"Relocate now?"

	wrapped := wrapPromptText(source, 50)
	lines := strings.Split(wrapped, "\n")

	if lines[0] != "Session store relocation is pending:" {
		t.Errorf("first line = %q, want the heading untouched", lines[0])
	}
	var continuations int
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "  ") {
			continuations++
		}
	}
	if continuations < 2 {
		t.Errorf("expected the wrapped census item to stay indented across its continuations, got:\n%s", wrapped)
	}
	if !strings.Contains(wrapped, "\n\n") {
		t.Error("expected the blank separator line to survive wrapping")
	}
	if lines[len(lines)-1] != "Relocate now?" {
		t.Errorf("last line = %q, want the question on its own line", lines[len(lines)-1])
	}
}

func TestWrapPromptTextHandlesUnreportedAndNarrowWidths(t *testing.T) {
	long := strings.Repeat("word ", 40)
	for name, width := range map[string]int{"unreported": 0, "negative": -10, "absurdly narrow": 3} {
		t.Run(name, func(t *testing.T) {
			wrapped := wrapPromptText(long, width)
			if wrapped == "" {
				t.Fatal("wrapping produced no output")
			}
			limit := defaultPromptWidth
			if width > 0 {
				limit = minPromptWidth
			}
			for line := range strings.SplitSeq(wrapped, "\n") {
				if utf8.RuneCountInString(line) > limit {
					t.Errorf("line exceeds the %d-column fallback: %q", limit, line)
				}
			}
		})
	}
}

// The confirm view must keep its affordance reachable no matter how long the
// question runs — that failure is what made the prompt read as a status line.
func TestConfirmPromptViewKeepsAffordanceOnItsOwnLine(t *testing.T) {
	model := newConfirmPromptModel(ConfirmPrompt{Prompt: strings.Repeat("a very long question ", 20)})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	rendered := updated.(confirmPromptModel).View().Content

	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "[y/N]:") {
		t.Errorf("last line = %q, want the affordance on a line of its own", last)
	}
	for _, line := range lines {
		if utf8.RuneCountInString(line) > 60 {
			t.Errorf("line exceeds the terminal width: %q", line)
		}
	}
}
