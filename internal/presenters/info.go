package presenters

import (
	"fmt"
	"io"

	"github.com/networkteam/sdd/internal/query"
)

// RenderInfo writes the session-framing header — participant, language
// (when configured), and search capability. The same lines prefix
// `sdd status` (via writeInfoHeader), so the two surfaces stay in
// lockstep.
func RenderInfo(w io.Writer, result *query.InfoResult) {
	writeInfoHeader(w, result.LocalParticipant, result.Language, result.Search)
}

// writeInfoHeader emits the canonical info lines. Shared between
// RenderInfo and RenderStatus so the two surfaces never drift.
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
