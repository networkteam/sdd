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

func TestReferenceWrapsBodyWithCLIFraming(t *testing.T) {
	reference := Reference(Vocabulary{})
	for _, want := range []string{"Usage: sdd view", "Grammar:", "Examples:", "sdd view --layout='top(20)'"} {
		if !strings.Contains(reference, want) {
			t.Errorf("Reference output missing %q", want)
		}
	}
}
