package styles

import "testing"

// Pin the concrete SGR each concept renders. Style.Render emits the declared
// color independent of the environment (NO_COLOR only affects the colorprofile
// writer at output time), so a wrong color VALUE — e.g. Body 252 → 250 — fails
// here even though the aliasing guards on the presenter/coordinator surfaces
// pass. Basic ANSI colors (0–15) render as 3x/9x SGR; 256-palette indices as
// 38;5;N. Reset is ESC[m.
func TestPalette_PinnedColorValues(t *testing.T) {
	const r = "\x1b[m"
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Heading", Heading.Render("x"), "\x1b[1;97mx" + r},
		{"Identity", Identity.Render("x"), "\x1b[97mx" + r},
		{"Qualifier", Qualifier.Render("x"), "\x1b[1;97mx" + r},
		{"ID", ID.Render("x"), "\x1b[38;5;220mx" + r},
		{"Key", Key.Render("x"), "\x1b[36mx" + r},
		{"RefKind", RefKind.Render("x"), "\x1b[38;5;141mx" + r},
		{"Body", Body.Render("x"), "\x1b[38;5;252mx" + r},
		{"Faint", Faint.Render("x"), "\x1b[38;5;240mx" + r},
		{"Inactive", Inactive.Render("x"), "\x1b[38;5;245mx" + r},
		{"Info", Info.Render("x"), "\x1b[36mx" + r},
		{"Warn", Warn.Render("x"), "\x1b[38;5;220mx" + r},
		{"Error", Error.Render("x"), "\x1b[1;91mx" + r},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, c.got, c.want)
		}
	}
}
