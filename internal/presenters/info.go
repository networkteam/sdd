package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/query"
)

// RenderInfo writes the session-framing header — participant, language
// (when configured), and search capability. These lines are what skill
// session-start injections read for framing.
func RenderInfo(w io.Writer, result *query.InfoResult) {
	writeInfoHeader(w, result.LocalParticipant, result.Language, result.Search)
}

// writeInfoHeader emits the canonical session-header lines.
func writeInfoHeader(w io.Writer, localParticipant, language, search string) {
	if localParticipant != "" {
		fmt.Fprintf(w, "Local participant: %s\n", localParticipant)
	} else {
		fmt.Fprintln(w, "Local participant: (not configured — run sdd init)")
	}
	if language != "" {
		fmt.Fprintf(w, "Language: %s\n", language)
	}
	if search != "" {
		fmt.Fprintf(w, "Search: %s\n", search)
	}
}
