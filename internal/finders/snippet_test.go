package finders

import (
	"strings"
	"testing"
)

func TestSnippetAround_VectorModeUsesLeadingWindow(t *testing.T) {
	t.Parallel()
	// pos=0 is the vector-mode call shape: there's no specific match
	// position, so the citation should be the leading 2*window slice
	// of the chunk's body — what an agent reads to judge relevance.
	body := strings.Repeat("alpha beta gamma delta epsilon zeta eta theta iota kappa ", 20)
	got := snippetAround(body, 0, 50)
	if len(got) == 0 {
		t.Fatal("expected non-empty snippet")
	}
	if !strings.HasPrefix(got, "alpha") {
		t.Errorf("snippet should start at the head of the body; got %q", got[:min(20, len(got))])
	}
	// Must be bounded — pos=0 with window=50 means ~50 chars (one
	// half-window forward only since the start is clamped to 0).
	if len(got) > 100 {
		t.Errorf("snippet too long: %d chars (expected ≤100)", len(got))
	}
}

func TestSnippetAround_CenteredOnMatch(t *testing.T) {
	t.Parallel()
	// Text-mode call shape: pos > 0, snippet is centered (±window).
	prefix := strings.Repeat("filler word ", 50)
	suffix := strings.Repeat("more filler ", 50)
	needle := "TARGET"
	body := prefix + needle + suffix
	pos := strings.Index(body, needle)

	got := snippetAround(body, pos, 80)
	if !strings.Contains(got, needle) {
		t.Errorf("centered snippet must contain the match; got %q", got)
	}
	// Verify it pulled context from BOTH sides of the match.
	idxInSnippet := strings.Index(got, needle)
	if idxInSnippet < 10 {
		t.Errorf("expected meaningful left-side context, got %d bytes before match", idxInSnippet)
	}
	if len(got)-idxInSnippet-len(needle) < 10 {
		t.Errorf("expected meaningful right-side context")
	}
}

func TestSnippetAround_WordBoundaryStart(t *testing.T) {
	t.Parallel()
	// Place the window cut squarely in the middle of a word; the
	// trim-to-word-boundary pass must advance start past that word so
	// the snippet doesn't begin mid-word.
	body := "wordone wordtwo wordthree wordfour wordfive wordsix wordseven"
	// Window=10, pos in middle. Trim window to 10. Without word
	// boundary trim we'd start ~mid-word.
	got := snippetAround(body, 35, 10)
	if got == "" {
		t.Fatal("expected non-empty snippet")
	}
	// Strip the truncation markers and any leading whitespace, then
	// verify the first word is a complete word from the source body.
	core := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(got, " [...]"), "[...] "))
	first := strings.SplitN(core, " ", 2)[0]
	if first == "" {
		t.Fatalf("could not extract first word from %q", got)
	}
	idx := strings.Index(body, first)
	if idx < 0 {
		t.Fatalf("snippet first word %q not found in source", first)
	}
	if idx > 0 && body[idx-1] != ' ' {
		t.Errorf("snippet starts mid-word: first %q lands inside a word in %q", first, body)
	}
}

func TestSnippetAround_WordBoundaryEnd(t *testing.T) {
	t.Parallel()
	body := "alpha bravo charlie delta echo foxtrot golf hotel india juliet"
	// Pick a window that lands the end mid-word. trimToWordBoundaryEnd
	// should retreat to the previous space.
	got := snippetAround(body, 0, 23)
	if got == "" {
		t.Fatal("expected non-empty snippet")
	}
	// Strip the trailing truncation marker (the body extends past the
	// window) before comparing against the source.
	core := strings.TrimSpace(strings.TrimSuffix(got, " [...]"))
	if !strings.HasPrefix(body, core) {
		t.Fatalf("snippet core %q is not a prefix of %q", core, body)
	}
	tailIdx := len(core)
	if tailIdx < len(body) {
		next := body[tailIdx]
		if next != ' ' && next != '\t' && next != '\n' {
			t.Errorf("snippet ends mid-word: %q (next char in body: %q)", core, string(next))
		}
	}
}

func TestSnippetAround_EmptyText(t *testing.T) {
	t.Parallel()
	if got := snippetAround("", 0, 50); got != "" {
		t.Errorf("expected empty snippet for empty text; got %q", got)
	}
}

func TestSnippetAround_NegativePosFallsBackToHead(t *testing.T) {
	t.Parallel()
	body := "alpha beta gamma delta echo foxtrot golf hotel"
	got := snippetAround(body, -1, 20)
	if !strings.HasPrefix(got, "alpha") {
		t.Errorf("negative pos should fall back to leading window; got %q", got)
	}
}

func TestSnippetAround_MultibyteSafe(t *testing.T) {
	t.Parallel()
	// CJK characters are 3 bytes each in UTF-8. Snippet bounds operate
	// on bytes but must not split a rune. With our word-boundary trim
	// (which keys off ASCII whitespace), CJK passages typically lack
	// internal spaces — the snippet ends up returning the leading
	// portion verbatim. Test that the result is still a valid UTF-8
	// string.
	body := "前缀 哈里森很高兴遇见你 欢迎来中国 后缀"
	got := snippetAround(body, 0, 30)
	if got == "" {
		t.Fatal("expected non-empty snippet for CJK input")
	}
	if !validUTF8(got) {
		t.Errorf("snippet is not valid UTF-8: %q", got)
	}
}

func validUTF8(s string) bool {
	for _, r := range s {
		if r == 0xFFFD && len(s) > 0 && s[0] != 0xEF {
			return false
		}
	}
	return true
}

// TestSnippetAround_TruncationMarkers verifies the `[...]` markers
// signal whether the snippet covers the full source or a window into a
// longer one. Three cases:
//
//  1. Source fits entirely inside the window — no markers (the agent
//     is seeing the complete chunk).
//  2. Window starts at 0 but the source extends past the right edge —
//     trailing `[...]` only.
//  3. Window straddles a position deep inside a longer source —
//     leading AND trailing `[...]`.
func TestSnippetAround_TruncationMarkers(t *testing.T) {
	t.Parallel()

	t.Run("short text fits without markers", func(t *testing.T) {
		t.Parallel()
		got := snippetAround("alpha beta gamma", 0, 100)
		if strings.Contains(got, "[...]") {
			t.Errorf("complete-chunk snippet should not carry markers; got %q", got)
		}
	})

	t.Run("trailing marker when only the front fits", func(t *testing.T) {
		t.Parallel()
		body := strings.Repeat("filler word ", 100) // ~1200 chars
		got := snippetAround(body, 0, 50)
		if strings.HasPrefix(got, "[...]") {
			t.Errorf("vector-mode snippet starts at 0; should not have leading marker: %q", got)
		}
		if !strings.HasSuffix(got, "[...]") {
			t.Errorf("trailing marker missing; got %q", got)
		}
	})

	t.Run("leading and trailing markers when centered mid-text", func(t *testing.T) {
		t.Parallel()
		prefix := strings.Repeat("aa bb cc dd ee ff gg hh ii jj ", 20)
		suffix := strings.Repeat("kk ll mm nn oo pp qq rr ss tt ", 20)
		body := prefix + "TARGET " + suffix
		pos := strings.Index(body, "TARGET")

		got := snippetAround(body, pos, 60)
		if !strings.HasPrefix(got, "[...] ") {
			t.Errorf("centered snippet should have leading marker; got %q", got)
		}
		if !strings.HasSuffix(got, " [...]") {
			t.Errorf("centered snippet should have trailing marker; got %q", got)
		}
		if !strings.Contains(got, "TARGET") {
			t.Errorf("snippet must still contain the match; got %q", got)
		}
	})
}
