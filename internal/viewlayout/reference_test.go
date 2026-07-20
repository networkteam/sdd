package viewlayout

import (
	"strings"
	"testing"
)

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
