package presenters

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/colorprofile"
)

// RenderResultLine writes a completed-operation summary in the CLI palette: a
// prominent headline (bold white, the qualifier style) followed by faint
// detail. On a non-TTY destination or with NO_COLOR the colorprofile writer
// downsamples to plain text, so agents and pipes get a clean line. detail may
// be empty.
//
// This is the styled-feedback surface the terminal-experience directive
// (d-cpt-mvb) calls for, sharing the palette established by d-cpt-n0f so the
// summary reads as one piece with sdd show and sdd stats.
func RenderResultLine(dst io.Writer, headline, detail string) {
	w := colorprofile.NewWriter(dst, os.Environ())
	if detail == "" {
		fmt.Fprintln(w, clrQual.Render(headline))
		return
	}
	fmt.Fprintln(w, clrQual.Render(headline)+" "+clrFaint.Render(detail))
}
