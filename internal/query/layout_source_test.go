package query_test

import (
	"testing"

	"github.com/networkteam/sdd/internal/query"
)

func TestSectionSourceCapturedAndPreservedThroughMacros(t *testing.T) {
	layout, err := query.ParseLayout(`focus:brief,kind(done):n(8):name("Recent done"):as-list`)
	if err != nil {
		t.Fatal(err)
	}
	if got := layout.Sections[0].Source; got != "focus:brief" {
		t.Errorf("section 0 source = %q", got)
	}
	if got := layout.Sections[1].Source; got != `kind(done):n(8):name("Recent done"):as-list` {
		t.Errorf("section 1 source = %q", got)
	}
	expanded, err := query.ExpandMacros(layout)
	if err != nil {
		t.Fatal(err)
	}
	if got := expanded.Sections[0].Source; got != "focus:brief" {
		t.Errorf("macro expansion dropped the source: %q", got)
	}
	if got := expanded.Sections[0].Expr(); got != "focus:brief" {
		t.Errorf("Expr = %q, want the terse user source", got)
	}
}

func TestSectionExprFallsBackToFunctions(t *testing.T) {
	layout, err := query.ParseLayout(`kind(gap,question):active:n(15):name("Open"):brief:as-list`)
	if err != nil {
		t.Fatal(err)
	}
	section := layout.Sections[0]
	section.Source = ""
	if got := section.Expr(); got != `kind(gap,question):active:n(15):name("Open"):brief:as-list` {
		t.Errorf("Expr fallback = %q, want the functions rendered back to grammar", got)
	}
}
