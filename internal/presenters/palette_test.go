package presenters

import (
	"testing"

	"github.com/networkteam/sdd/internal/styles"
)

// The presenter palette must draw every concept from the shared source so a
// concept that also appears on the coordinator surface renders identically.
// Comparing rendered output (not the struct) keeps the guard cheap and honest:
// re-inlining a color here would diverge the strings and fail the test.
func TestPalette_DrawsFromSharedStyles(t *testing.T) {
	const probe = "probe"
	cases := []struct {
		name   string
		local  string
		shared string
	}{
		{"heading", clrHeading.Render(probe), styles.Heading.Render(probe)},
		{"identity", clrIdentity.Render(probe), styles.Identity.Render(probe)},
		{"id", clrID.Render(probe), styles.ID.Render(probe)},
		{"key", clrKey.Render(probe), styles.Key.Render(probe)},
		{"refkind", clrRefKind.Render(probe), styles.RefKind.Render(probe)},
		{"body", clrBody.Render(probe), styles.Body.Render(probe)},
		{"qualifier", clrQual.Render(probe), styles.Qualifier.Render(probe)},
		{"faint", clrFaint.Render(probe), styles.Faint.Render(probe)},
		{"inactive", clrInactive.Render(probe), styles.Inactive.Render(probe)},
	}
	for _, c := range cases {
		if c.local != c.shared {
			t.Errorf("%s: local %q != shared %q", c.name, c.local, c.shared)
		}
	}
}
