package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveStdinAttachment(t *testing.T) {
	tmp := t.TempDir()
	data := []byte("plan content here")

	path, err := saveStdinAttachment(tmp, "plan.md", data)
	if err != nil {
		t.Fatalf("saveStdinAttachment: %v", err)
	}

	// Path lives directly under the provided tmpDir.
	if filepath.Dir(path) != tmp {
		t.Errorf("saved under %q, want under %q", filepath.Dir(path), tmp)
	}

	// Filename: <hash>-<target>, hash is 8 hex chars
	base := filepath.Base(path)
	if !strings.HasSuffix(base, "-plan.md") {
		t.Errorf("filename %q should end with -plan.md", base)
	}
	hash := strings.TrimSuffix(base, "-plan.md")
	if len(hash) != 8 {
		t.Errorf("hash prefix %q should be 8 chars, got %d", hash, len(hash))
	}

	// Content was written
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("saved content = %q, want %q", string(got), string(data))
	}

	// Same content + same target → same path (stable hash)
	path2, err := saveStdinAttachment(tmp, "plan.md", data)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if path2 != path {
		t.Errorf("second save produced %q, want stable path %q", path2, path)
	}

	// Different content → different filename
	path3, err := saveStdinAttachment(tmp, "plan.md", []byte("different"))
	if err != nil {
		t.Fatalf("third save: %v", err)
	}
	if path3 == path {
		t.Errorf("different content should produce different path; both gave %q", path)
	}
}
