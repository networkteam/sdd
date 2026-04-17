package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestSplitCSV_TrimsWhitespaceAndDropsEmpty is the regression test for the
// CSV whitespace-trim bug (s-prc-omw, d-tac-955) and the d-prc-8vh contract
// requiring regression tests for bug fixes. Before the fix,
// `--participants "Christopher, Claude"` stored " Claude" with leading space
// as a distinct participant identity across ~30 entries in the graph.
func TestSplitCSV_TrimsWhitespaceAndDropsEmpty(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"Christopher", []string{"Christopher"}},
		// The regression case: comma-space-separated input must not leak " Claude".
		{"Christopher, Claude", []string{"Christopher", "Claude"}},
		{"  Christopher  ,  Claude  ", []string{"Christopher", "Claude"}},
		{"Christopher,Claude", []string{"Christopher", "Claude"}},
		// Empty elements after trim are dropped so stray commas don't produce phantom participants.
		{"Christopher,,Claude", []string{"Christopher", "Claude"}},
		{",,", nil},
		{"   ", nil},
	}
	for _, tt := range tests {
		got := splitCSV(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitCSV(%q) = %#v, want %#v", tt.input, got, tt.want)
		}
	}
}

func TestParseAttachSpec(t *testing.T) {
	tests := []struct {
		spec       string
		wantSource string
		wantTarget string
	}{
		// Plain path — no colon, target empty (basename fallback handled by caller)
		{"file.md", "file.md", ""},
		{"/tmp/design.md", "/tmp/design.md", ""},

		// source:target mapping
		{"/tmp/design.md:plan.md", "/tmp/design.md", "plan.md"},
		{"notes.txt:renamed.txt", "notes.txt", "renamed.txt"},

		// Stdin alias
		{"-:plan-requirements.md", "-", "plan-requirements.md"},

		// Bare stdin (caller validates that target is required)
		{"-", "-", ""},

		// Colon in source path — splits on last colon
		{"/path/with:colon/file.md:target.md", "/path/with:colon/file.md", "target.md"},
	}

	for _, tt := range tests {
		src, tgt := parseAttachSpec(tt.spec)
		if src != tt.wantSource || tgt != tt.wantTarget {
			t.Errorf("parseAttachSpec(%q) = (%q, %q), want (%q, %q)",
				tt.spec, src, tgt, tt.wantSource, tt.wantTarget)
		}
	}
}

func TestParseAttachFlags_PlainPath(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "design.md")
	if err := os.WriteFile(f, []byte("# Design"), 0644); err != nil {
		t.Fatal(err)
	}

	atts, err := parseAttachFlags([]string{f}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 {
		t.Fatalf("got %d attachments, want 1", len(atts))
	}
	if atts[0].source != f {
		t.Errorf("source = %q, want %q", atts[0].source, f)
	}
	if atts[0].target != "design.md" {
		t.Errorf("target = %q, want %q", atts[0].target, "design.md")
	}
}

func TestParseAttachFlags_SourceTargetMapping(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "tmpXXX.md")
	if err := os.WriteFile(f, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	atts, err := parseAttachFlags([]string{f + ":plan.md"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 {
		t.Fatalf("got %d attachments, want 1", len(atts))
	}
	if atts[0].source != f {
		t.Errorf("source = %q, want %q", atts[0].source, f)
	}
	if atts[0].target != "plan.md" {
		t.Errorf("target = %q, want %q", atts[0].target, "plan.md")
	}
}

func TestParseAttachFlags_Stdin(t *testing.T) {
	reader := strings.NewReader("stdin content")

	atts, err := parseAttachFlags([]string{"-:output.md"}, reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 {
		t.Fatalf("got %d attachments, want 1", len(atts))
	}
	if atts[0].source != "-" {
		t.Errorf("source = %q, want %q", atts[0].source, "-")
	}
	if atts[0].target != "output.md" {
		t.Errorf("target = %q, want %q", atts[0].target, "output.md")
	}
	if string(atts[0].data) != "stdin content" {
		t.Errorf("data = %q, want %q", string(atts[0].data), "stdin content")
	}
}

func TestParseAttachFlags_BareStdinError(t *testing.T) {
	_, err := parseAttachFlags([]string{"-"}, strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for bare stdin, got nil")
	}
	if !strings.Contains(err.Error(), "requires a target name") {
		t.Errorf("error = %q, want it to mention target name requirement", err.Error())
	}
}

func TestParseAttachFlags_DuplicateStdinError(t *testing.T) {
	reader := strings.NewReader("content")

	_, err := parseAttachFlags([]string{"-:a.md", "-:b.md"}, reader)
	if err == nil {
		t.Fatal("expected error for duplicate stdin, got nil")
	}
	if !strings.Contains(err.Error(), "only be used once") {
		t.Errorf("error = %q, want it to mention single use", err.Error())
	}
}

func TestParseAttachFlags_MissingFileError(t *testing.T) {
	_, err := parseAttachFlags([]string{"/nonexistent/file.md"}, nil)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to mention not found", err.Error())
	}
}

func TestParseAttachFlags_MultipleAttachments(t *testing.T) {
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "a.md")
	f2 := filepath.Join(tmp, "b.md")
	if err := os.WriteFile(f1, []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}

	reader := strings.NewReader("from stdin")

	atts, err := parseAttachFlags([]string{f1, f2 + ":renamed.md", "-:stdin.md"}, reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 3 {
		t.Fatalf("got %d attachments, want 3", len(atts))
	}
	// First: plain path, basename fallback
	if atts[0].target != "a.md" {
		t.Errorf("atts[0].target = %q, want %q", atts[0].target, "a.md")
	}
	// Second: source:target mapping
	if atts[1].target != "renamed.md" {
		t.Errorf("atts[1].target = %q, want %q", atts[1].target, "renamed.md")
	}
	// Third: stdin
	if atts[2].target != "stdin.md" {
		t.Errorf("atts[2].target = %q, want %q", atts[2].target, "stdin.md")
	}
	if string(atts[2].data) != "from stdin" {
		t.Errorf("atts[2].data = %q, want %q", string(atts[2].data), "from stdin")
	}
}
