package mcpapp

import (
	"strings"
	"testing"
)

func TestEmptyViewMessage(t *testing.T) {
	if got := emptyViewMessage(nil); got != "0 entries matched the layout." {
		t.Errorf("no-participants message = %q", got)
	}
	got := emptyViewMessage([]string{"Christopher", "Jonathan Philipp"})
	if !strings.Contains(got, "0 entries matched") {
		t.Errorf("message missing empty statement: %q", got)
	}
	if !strings.Contains(got, "Known participants: Christopher, Jonathan Philipp") {
		t.Errorf("message missing participant names: %q", got)
	}
}

func TestGuardViewSize(t *testing.T) {
	small := "line one\nline two"
	if guardViewSize(small) != small {
		t.Error("under-cap output should pass through unchanged")
	}

	var b strings.Builder
	for b.Len() <= 2*maxViewResultBytes {
		b.WriteString("an entry line that takes up some width\n")
	}
	full := b.String()
	out := guardViewSize(full)

	if len(out) >= len(full) {
		t.Errorf("over-cap output was not truncated: %d >= %d", len(out), len(full))
	}
	if !strings.Contains(out, "truncated at") {
		t.Error("truncation notice missing")
	}
	if !strings.Contains(out, "n(K)") {
		t.Error("truncation notice must name n() paging as the recovery")
	}
}
