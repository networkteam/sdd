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

func TestReferenceRetainsTerminalShape(t *testing.T) {
	vocabulary := Vocabulary{
		Functions: []string{"active", "indexed", "as-list"}, Renders: []string{"as-list"},
		Algorithms: []string{"heat"}, Decays: []string{"exp-14d"}, Macros: []string{"top"},
	}
	reference := Reference(vocabulary)
	for _, want := range []string{"Usage: sdd view --layout=<spec>", "layout  := section", "Implemented pipeline vocabulary:", "active", "indexed", "top(N) — active entries ranked by heat", "Examples:"} {
		if !strings.Contains(reference, want) {
			t.Errorf("Reference output missing %q", want)
		}
	}
	for _, example := range ExampleSpecs() {
		want := "  sdd view --layout='" + example + "'"
		if !strings.Contains(reference, want) {
			t.Errorf("Reference output missing terminal example %q", want)
		}
	}
}

func TestActiveReferenceDescribesDerivedLifecycleSemantics(t *testing.T) {
	description := functionReference["active"].description
	for _, want := range []string{"derived active/open", "settled", "cascade-closed", "orphaned"} {
		if !strings.Contains(description, want) {
			t.Errorf("active description missing %q: %s", want, description)
		}
	}
}

func TestMarkdownUsesNativeStructureAndSharedExamples(t *testing.T) {
	vocabulary := Vocabulary{
		Functions: []string{"active", "indexed", "as-list"}, Renders: []string{"as-list"},
		Algorithms: []string{"heat"}, Decays: []string{"exp-14d"}, Macros: []string{"top"},
	}
	markdown := Markdown(vocabulary)
	for _, want := range []string{"# How to compose graph views", "## Grammar", "```text", "## Vocabulary", "| Category | Syntax | Meaning |", "| Filters | `indexed` |", "## Example layout specifications", "active:indexed:as-list"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("Markdown output missing %q", want)
		}
	}
	for _, unwanted := range []string{"sdd view", "--layout", "MCP", "debugging"} {
		if strings.Contains(markdown, unwanted) {
			t.Errorf("Markdown output contains host-specific framing %q", unwanted)
		}
	}
	if strings.Contains(markdown, "<br>") {
		t.Fatal("Markdown vocabulary uses HTML line-break packing")
	}
	wantRows := 2
	for _, section := range buildReference(vocabulary).sections {
		wantRows += len(section.items)
	}
	gotRows := 0
	for _, line := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(line, "|") {
			gotRows++
		}
	}
	if gotRows != wantRows {
		t.Fatalf("Markdown table rows = %d, want %d", gotRows, wantRows)
	}
	terminal := Reference(vocabulary)
	for _, example := range ExampleSpecs() {
		if !strings.Contains(markdown, example) || !strings.Contains(terminal, example) {
			t.Errorf("shared example %q is missing from a renderer", example)
		}
	}
}

func TestMarkdownCoversEveryLiveVocabularyItem(t *testing.T) {
	vocabulary := Vocabulary{
		Functions: []string{"source", "active", "indexed", "as-list"}, Renders: []string{"as-list"},
		Algorithms: []string{"heat"}, Decays: []string{"none"}, Macros: []string{"top"},
	}
	markdown := Markdown(vocabulary)
	for _, section := range buildReference(vocabulary).sections {
		for _, item := range section.items {
			if !strings.Contains(markdown, "`"+escapeTable(item.syntax)+"`") {
				t.Errorf("Markdown output missing live syntax %q", item.syntax)
			}
		}
	}
}
