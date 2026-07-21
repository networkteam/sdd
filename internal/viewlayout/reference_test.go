package viewlayout

import (
	"strings"
	"testing"
)

func TestReferenceBodyIsHostNeutral(t *testing.T) {
	vocabulary := Vocabulary{
		Functions:  []string{"active", "as-list"},
		Renders:    []string{"as-list"},
		Algorithms: []string{"heat"},
		Decays:     []string{"exp-14d"},
		Macros:     []string{"top"},
	}

	body := ReferenceBody(vocabulary)
	for _, want := range []string{"Grammar:", "active", "as-list", "heat", "exp-14d", "top(N)"} {
		if !strings.Contains(body, want) {
			t.Errorf("ReferenceBody output missing %q", want)
		}
	}
	for _, unwanted := range []string{"Usage:", "Examples:", "sdd view"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("ReferenceBody output contains host-specific framing %q", unwanted)
		}
	}
}

func TestReferenceBodyRendersErrorForUnregisteredNames(t *testing.T) {
	// A live name absent from its metadata map must render a loud ERROR in the
	// description position, not a blank or soft fallback — the fail-loud second
	// line behind the MissingReferenceNames coverage test.
	vocabulary := Vocabulary{
		Functions:    []string{"phantom-fn"},
		Macros:       []string{"phantom-macro"},
		LayoutMacros: []string{"phantom-layout"},
	}

	body := ReferenceBody(vocabulary)
	for _, want := range []string{
		`ERROR: missing reference metadata for "phantom-fn" — register it in functionReference`,
		`ERROR: missing reference metadata for "phantom-macro" — register it in macroReference`,
		`ERROR: missing reference metadata for "phantom-layout" — register it in macroReference`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("ReferenceBody output missing loud error %q", want)
		}
	}
	// The unregistered function name must still be listed (under Other primitives).
	if !strings.Contains(body, "phantom-fn") {
		t.Error("unregistered function name should still be listed")
	}
}

func TestReferenceWrapsBodyWithCLIFraming(t *testing.T) {
	reference := Reference(Vocabulary{})
	for _, want := range []string{"Usage: sdd view", "Grammar:", "Examples:", "sdd view --layout='top(20)'"} {
		if !strings.Contains(reference, want) {
			t.Errorf("Reference output missing %q", want)
		}
	}
}
