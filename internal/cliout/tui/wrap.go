package tui

import (
	"strings"
	"unicode/utf8"
)

// defaultPromptWidth renders prompts before the terminal has reported its size,
// and whenever it reports nothing usable.
const defaultPromptWidth = 80

// minPromptWidth stops a very narrow terminal from degrading wrapping to one
// word per line.
const minPromptWidth = 24

// wrapPromptText hard-wraps prompt prose to the usable terminal width,
// preserving each line's leading indentation on its continuations.
//
// Bubble tea truncates an over-long view line at the terminal edge instead of
// wrapping it, so any prompt whose text can exceed the width has to arrive
// already wrapped — otherwise its tail leaves the screen, taking whatever
// follows with it. That is not cosmetic: it is how the relocation prompt lost
// its [y/N] affordance and read as a status line, silently answering itself.
func wrapPromptText(text string, width int) string {
	width = usablePromptWidth(width)
	var wrapped []string
	for line := range strings.SplitSeq(text, "\n") {
		wrapped = append(wrapped, wrapPromptLine(line, width)...)
	}
	return strings.Join(wrapped, "\n")
}

// usablePromptWidth keeps one column in hand so a wrapped line never fills the
// terminal exactly, which some terminals follow with an extra line break.
func usablePromptWidth(width int) int {
	if width <= 0 {
		width = defaultPromptWidth
	}
	if width < minPromptWidth {
		return minPromptWidth
	}
	return width - 1
}

func wrapPromptLine(line string, width int) []string {
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{""}
	}
	indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
	limit := max(width-utf8.RuneCountInString(indent), 1)

	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if utf8.RuneCountInString(current)+1+utf8.RuneCountInString(word) > limit {
			lines = append(lines, indent+current)
			current = word
			continue
		}
		current += " " + word
	}
	return append(lines, indent+current)
}
