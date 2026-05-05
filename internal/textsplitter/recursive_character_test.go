// Tests adapted from github.com/tmc/langchaingo/textsplitter (MIT). See
// LICENSE-langchaingo and NOTICE.md.
//
// SDD adaptations: dropped the tiktoken-go and langchaingo/schema variants
// from upstream — the SDD chunker uses utf-8 rune counts and does not pull
// in either dependency. The remaining cases exercise the same separator
// hierarchy and overlap logic as upstream.

package textsplitter

import (
	"reflect"
	"testing"
)

func TestRecursiveCharacterSplitter(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name          string
		text          string
		chunkOverlap  int
		chunkSize     int
		separators    []string
		expected      []string
		keepSeparator bool
	}

	cases := []testCase{
		{
			name:         "cjk-newlines",
			text:         "哈里森\n很高兴遇见你\n欢迎来中国",
			chunkOverlap: 0,
			chunkSize:    10,
			separators:   []string{"\n\n", "\n", " "},
			expected: []string{
				"哈里森\n很高兴遇见你",
				"欢迎来中国",
			},
		},
		{
			name:         "single-newline-overlap-one",
			text:         "Hi, Harrison. \nI am glad to meet you",
			chunkOverlap: 1,
			chunkSize:    20,
			separators:   []string{"\n", "$"},
			expected: []string{
				"Hi, Harrison.",
				"I am glad to meet you",
			},
		},
		{
			name:         "double-newline-section",
			text:         "Hi.\nI'm Harrison.\n\nHow?\na\nbHi.\nI'm Harrison.\n\nHow?\na\nb",
			chunkOverlap: 1,
			chunkSize:    40,
			separators:   []string{"\n\n", "\n", " ", ""},
			expected: []string{
				"Hi.\nI'm Harrison.",
				"How?\na\nbHi.\nI'm Harrison.\n\nHow?\na\nb",
			},
		},
		{
			name:         "yaml-pair-fits",
			text:         "name: Harrison\nage: 30",
			chunkOverlap: 1,
			chunkSize:    40,
			separators:   []string{"\n\n", "\n", " ", ""},
			expected:     []string{"name: Harrison\nage: 30"},
		},
		{
			name:         "yaml-pairs-paragraph-split",
			text:         "name: Harrison\nage: 30\n\nname: Joe\nage: 32",
			chunkOverlap: 1,
			chunkSize:    40,
			separators:   []string{"\n\n", "\n", " ", ""},
			expected: []string{
				"name: Harrison\nage: 30",
				"name: Joe\nage: 32",
			},
		},
		{
			name:          "keep-separator-true",
			text:          "Hi, Harrison. \nI am glad to meet you",
			chunkOverlap:  0,
			chunkSize:     10,
			separators:    []string{"\n", "$"},
			keepSeparator: true,
			expected: []string{
				// Trailing space preserved when KeepSeparator is true and
				// the splitter does not cross the next separator boundary.
				"Hi, Harrison. ",
				"\nI am glad to meet you",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := NewRecursiveCharacter()
			s.ChunkOverlap = tc.chunkOverlap
			s.ChunkSize = tc.chunkSize
			s.Separators = tc.separators
			s.KeepSeparator = tc.keepSeparator

			got, err := s.SplitText(tc.text)
			if err != nil {
				t.Fatalf("SplitText: %v", err)
			}
			if !reflect.DeepEqual(got, tc.expected) {
				t.Fatalf("got %q, want %q", got, tc.expected)
			}
		})
	}
}
