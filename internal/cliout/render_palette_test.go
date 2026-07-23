package cliout

import (
	"log/slog"
	"testing"

	"github.com/networkteam/sdd/internal/styles"
)

// The coordinator's footer chrome and log-level styles must draw from the same
// shared palette the presenter surface uses, so a concept shared across both
// (labels, body text, keys, levels) renders consistently. Comparing rendered
// output keeps the guard cheap.
func TestRenderPalette_DrawsFromSharedStyles(t *testing.T) {
	const probe = "probe"
	cases := []struct {
		name          string
		local, shared string
	}{
		{"label", StyleLabel.Render(probe), styles.Heading.Render(probe)},
		{"body", StyleBody.Render(probe), styles.Body.Render(probe)},
		{"debug", levelStyles[slog.LevelDebug].Render(probe), styles.Faint.Render(probe)},
		{"info", levelStyles[slog.LevelInfo].Render(probe), styles.Info.Render(probe)},
		{"warn", levelStyles[slog.LevelWarn].Render(probe), styles.Warn.Render(probe)},
		{"error", levelStyles[slog.LevelError].Render(probe), styles.Error.Render(probe)},
	}
	for _, c := range cases {
		if c.local != c.shared {
			t.Errorf("%s: local %q != shared %q", c.name, c.local, c.shared)
		}
	}
}
