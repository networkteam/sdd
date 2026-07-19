package model

import (
	"strings"
	"testing"
)

func TestParseTopicPath(t *testing.T) {
	cases := []struct {
		in       string
		want     []string
		wantErr  bool
		errMatch string
	}{
		{in: "foo", want: []string{"foo"}},
		{in: "foo/bar", want: []string{"foo", "bar"}},
		{in: "Catch-Up", want: []string{"Catch-Up"}},
		{in: "Übersicht/Bootstrap", want: []string{"Übersicht", "Bootstrap"}},
		{in: "infrastructure/cli/status", want: []string{"infrastructure", "cli", "status"}},
		{in: "label-with-hyphens", want: []string{"label-with-hyphens"}},

		{in: "", wantErr: true, errMatch: "empty"},
		{in: "/foo", wantErr: true, errMatch: "empty component"},
		{in: "foo/", wantErr: true, errMatch: "empty component"},
		{in: "foo//bar", wantErr: true, errMatch: "empty component"},
		{in: "foo bar", wantErr: true, errMatch: "invalid character"},
		{in: "foo.bar", wantErr: true, errMatch: "invalid character"},
		{in: "foo_bar", wantErr: true, errMatch: "invalid character"},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParseTopicPath(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", c.errMatch)
				}
				if c.errMatch != "" && !strings.Contains(err.Error(), c.errMatch) {
					t.Fatalf("expected error containing %q, got %q", c.errMatch, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got.Components) != len(c.want) {
				t.Fatalf("components: want %v, got %v", c.want, got.Components)
			}
			for i := range got.Components {
				if got.Components[i] != c.want[i] {
					t.Fatalf("component[%d]: want %q, got %q", i, c.want[i], got.Components[i])
				}
			}
		})
	}
}

func TestTopicPath_String(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"foo", "foo"},
		{"foo/bar", "foo/bar"},
		{"Catch-Up", "Catch-Up"},
		{"Übersicht/Bootstrap", "Übersicht/Bootstrap"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			p, err := ParseTopicPath(c.in)
			if err != nil {
				t.Fatalf("ParseTopicPath: %v", err)
			}
			if got := p.String(); got != c.want {
				t.Fatalf("String: want %q, got %q", c.want, got)
			}
		})
	}
}

func TestTopicPath_Equal(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"foo", "foo", true},
		{"foo", "FOO", true},
		{"Foo/Bar", "foo/BAR", true},
		{"foo", "bar", false},
		{"foo", "foo/bar", false},
		{"foo/bar", "foo", false},
	}
	for _, c := range cases {
		t.Run(c.a+"_vs_"+c.b, func(t *testing.T) {
			pa, err := ParseTopicPath(c.a)
			if err != nil {
				t.Fatalf("a: %v", err)
			}
			pb, err := ParseTopicPath(c.b)
			if err != nil {
				t.Fatalf("b: %v", err)
			}
			if got := pa.Equal(pb); got != c.want {
				t.Fatalf("Equal: want %v, got %v", c.want, got)
			}
		})
	}
}

func TestTopicPathFoldKeyMatchesEqualFold(t *testing.T) {
	sigma, err := ParseTopicPath("Σ")
	if err != nil {
		t.Fatal(err)
	}
	finalSigma, err := ParseTopicPath("ς")
	if err != nil {
		t.Fatal(err)
	}
	if !sigma.Equal(finalSigma) || sigma.FoldKey() != finalSigma.FoldKey() {
		t.Fatalf("equal-fold paths have keys %q and %q", sigma.FoldKey(), finalSigma.FoldKey())
	}
}

func TestTopicPath_HasPrefix(t *testing.T) {
	cases := []struct {
		path, prefix string
		want         bool
	}{
		{"UX", "UX", true},
		{"UX/CLI", "UX", true},
		{"UX/CLI/Status", "UX", true},
		{"UX/CLI/Status", "UX/CLI", true},
		{"UX/CLI", "UX/CLI", true},
		{"UX/CLI", "ux/cli", true}, // case-insensitive

		{"UX", "UX/CLI", false},            // prefix longer
		{"UXTesting", "UX", false},         // raw-string prefix doesn't match component-wise
		{"UX/Testing", "UXTesting", false}, // different first component
		{"foo/bar", "baz", false},
	}
	for _, c := range cases {
		t.Run(c.path+"_hasprefix_"+c.prefix, func(t *testing.T) {
			path, err := ParseTopicPath(c.path)
			if err != nil {
				t.Fatalf("path: %v", err)
			}
			pref, err := ParseTopicPath(c.prefix)
			if err != nil {
				t.Fatalf("prefix: %v", err)
			}
			if got := path.HasPrefix(pref); got != c.want {
				t.Fatalf("HasPrefix: want %v, got %v", c.want, got)
			}
		})
	}
}

func TestCanonicalizeTopicPaths(t *testing.T) {
	parse := func(s string) TopicPath {
		t.Helper()
		p, err := ParseTopicPath(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return p
	}

	in := []TopicPath{
		parse("Foo"),
		parse("foo"), // case-fold dup of Foo — drops
		parse("Bar/Baz"),
		parse("bar/BAZ"), // case-fold dup of Bar/Baz — drops
		parse("qux"),
	}
	out := CanonicalizeTopicPaths(in)
	if len(out) != 3 {
		t.Fatalf("want 3 unique, got %d (%v)", len(out), out)
	}
	if out[0].String() != "Foo" {
		t.Fatalf("[0]: want first-seen casing 'Foo', got %q", out[0].String())
	}
	if out[1].String() != "Bar/Baz" {
		t.Fatalf("[1]: want first-seen casing 'Bar/Baz', got %q", out[1].String())
	}
	if out[2].String() != "qux" {
		t.Fatalf("[2]: want 'qux', got %q", out[2].String())
	}
}
